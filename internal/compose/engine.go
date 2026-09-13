package compose

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"

	"github.com/0funct0ry/vessel/internal/dockerapi"
)

// Docker Compose's own container labels. Vessel writes exactly these so a
// stack deployed here is still recognizable to the `docker compose` CLI, and
// so the containers list can show which stack a container belongs to.
const (
	LabelProject = "com.docker.compose.project"
	LabelService = "com.docker.compose.service"
)

// Stack statuses reported by Status.
const (
	StatusRunning     = "running"
	StatusStopped     = "stopped"
	StatusPartial     = "partial"
	StatusNotDeployed = "not_deployed"
)

// Docker is the slice of the Engine client the lifecycle needs. Keeping it an
// interface lets the API layer pass its own client and tests pass a fake.
type Docker interface {
	ListContainers(context.Context, dockerapi.ListContainersOptions) ([]dockerapi.Container, error)
	CreateContainer(context.Context, dockerapi.Spec) (dockerapi.CreateResult, error)
	CreateNetwork(context.Context, dockerapi.CreateNetworkOptions) (*dockerapi.Network, error)
	CreateVolume(context.Context, dockerapi.CreateVolumeOptions) (*dockerapi.Volume, error)
	RemoveContainer(context.Context, string, dockerapi.RemoveContainerOptions) error
	RemoveVolume(context.Context, string, bool) error
	RemoveNetwork(context.Context, string) error
	Lifecycle(context.Context, string, string, url.Values) error
	PullImage(context.Context, string) (dockerapi.PullStream, error)
}

// Stack is the stored definition the engine deploys. It mirrors the persisted
// store.Stack without importing it, so compose stays free of the store layer.
type Stack struct {
	Name        string
	ComposeYAML string
	EnvContent  string
}

// Event is one step of a deployment, streamed to the caller as it happens.
type Event struct {
	Kind   string `json:"kind"`   // network | volume | container | stack
	Name   string `json:"name"`   // resource name, or the stack name for summaries
	Phase  string `json:"phase"`  // creating | created | starting | started | removing | removed | exists | skipped | error | done
	Detail string `json:"detail"` // human-readable detail, or the raw daemon error
}

// Parse decodes a stack's stored compose file with its stored .env applied.
func (s Stack) Parse() (Compose, []Warning, error) {
	return ParseCompose([]byte(s.ComposeYAML), ParseEnvFile(s.EnvContent))
}

// projectFilter is the label filter every lifecycle operation uses to find the
// containers that belong to one stack.
func projectFilter(name string) dockerapi.ListContainersOptions {
	return dockerapi.ListContainersOptions{All: true, Filters: map[string][]string{"label": {LabelProject + "=" + name}}}
}

// Up creates the stack's networks, named volumes and containers, then starts
// each container. Events stream on the returned channel until it closes. A
// service that fails does not abort the run: the remaining services are still
// attempted and the closing summary event names what succeeded and what did
// not, because a half-deployed stack the user can see beats an opaque abort.
func Up(ctx context.Context, cl Docker, stack Stack) (<-chan Event, error) {
	file, _, err := stack.Parse()
	if err != nil {
		return nil, err
	}
	existing, err := cl.ListContainers(ctx, projectFilter(stack.Name))
	if err != nil {
		return nil, err
	}
	deployed := map[string]bool{}
	for _, container := range existing {
		deployed[container.Labels[LabelService]] = true
	}

	events := make(chan Event, 16)
	go func() {
		defer close(events)
		emit := func(e Event) bool {
			select {
			case events <- e:
				return true
			case <-ctx.Done():
				return false
			}
		}

		for _, name := range sortedKeys(file.Networks) {
			definition := file.Networks[name]
			actual := resourceName(stack.Name, name, definition.Name)
			if definition.External {
				emit(Event{"network", actual, "skipped", "external network, not created"})
				continue
			}
			emit(Event{"network", actual, "creating", ""})
			if _, err := cl.CreateNetwork(ctx, dockerapi.CreateNetworkOptions{Name: actual, Driver: definition.Driver, Labels: withProjectLabels(definition.Labels, stack.Name, "")}); err != nil {
				// A network left over from an earlier Up (a partial failure, or
				// a redeploy that hasn't gone through Down yet) is not a reason
				// to fail the run — it's still the network this stack wants.
				if isConflict(err) {
					emit(Event{"network", actual, "exists", "already exists"})
					continue
				}
				emit(Event{"network", actual, "error", err.Error()})
				continue
			}
			emit(Event{"network", actual, "created", ""})
		}

		for _, name := range sortedKeys(file.Volumes) {
			definition := file.Volumes[name]
			actual := resourceName(stack.Name, name, definition.Name)
			if definition.External {
				emit(Event{"volume", actual, "skipped", "external volume, not created"})
				continue
			}
			emit(Event{"volume", actual, "creating", ""})
			if _, err := cl.CreateVolume(ctx, dockerapi.CreateVolumeOptions{Name: actual, Driver: definition.Driver, Labels: withProjectLabels(definition.Labels, stack.Name, "")}); err != nil {
				// Docker's own volume create is idempotent for a matching name,
				// but treat any conflict the same way: a volume already there is
				// not a failure, it's this stack's volume from an earlier run.
				if isConflict(err) {
					emit(Event{"volume", actual, "exists", "already exists"})
					continue
				}
				emit(Event{"volume", actual, "error", err.Error()})
				continue
			}
			emit(Event{"volume", actual, "created", ""})
		}

		var started, failed []string
		for _, service := range file.Order {
			container := containerName(stack.Name, service)
			if deployed[service] {
				emit(Event{"container", container, "exists", "already deployed; run Redeploy to replace it"})
				started = append(started, service)
				continue
			}
			emit(Event{"container", container, "creating", ""})
			spec := serviceSpec(stack.Name, service, file)
			result, err := cl.CreateContainer(ctx, spec)
			if err != nil && isMissingImage(err) {
				if pullErr := pullImage(ctx, cl, spec.Image, emit); pullErr != nil {
					failed = append(failed, service)
					emit(Event{"container", container, "error", pullErr.Error()})
					continue
				}
				emit(Event{"container", container, "creating", ""})
				result, err = cl.CreateContainer(ctx, spec)
			}
			if err != nil {
				failed = append(failed, service)
				emit(Event{"container", container, "error", err.Error()})
				continue
			}
			emit(Event{"container", container, "created", result.ID})
			emit(Event{"container", container, "starting", ""})
			if err := cl.Lifecycle(ctx, result.ID, "start", nil); err != nil {
				failed = append(failed, service)
				emit(Event{"container", container, "error", err.Error()})
				continue
			}
			started = append(started, service)
			emit(Event{"container", container, "started", result.ID})
		}

		phase, detail := "done", fmt.Sprintf("%d of %d services running", len(started), len(file.Order))
		if len(failed) > 0 {
			phase = "error"
			detail = fmt.Sprintf("%s — failed: %s", detail, strings.Join(failed, ", "))
		}
		emit(Event{"stack", stack.Name, phase, detail})
	}()
	return events, nil
}

// Down stops and removes every container labeled with the stack's project,
// then the stack's own non-external networks — matching `docker compose
// down`'s default of tearing down containers and networks but keeping named
// volumes unless removeVolumes is set. Without this, a partial or repeated Up
// would keep hitting "network already exists" against a network Down left
// behind.
func Down(ctx context.Context, cl Docker, stack Stack, removeVolumes bool) (<-chan Event, error) {
	containers, err := cl.ListContainers(ctx, projectFilter(stack.Name))
	if err != nil {
		return nil, err
	}
	file, _, parseErr := stack.Parse()

	events := make(chan Event, 16)
	go func() {
		defer close(events)
		emit := func(e Event) bool {
			select {
			case events <- e:
				return true
			case <-ctx.Done():
				return false
			}
		}
		removed := 0
		for _, container := range containers {
			name := displayName(container)
			emit(Event{"container", name, "removing", ""})
			if err := cl.Lifecycle(ctx, container.ID, "stop", nil); err != nil {
				// A container that is already stopped is not a failure; the
				// remove below is the step that has to succeed.
				emit(Event{"container", name, "skipped", err.Error()})
			}
			if err := cl.RemoveContainer(ctx, container.ID, dockerapi.RemoveContainerOptions{Force: true}); err != nil {
				emit(Event{"container", name, "error", err.Error()})
				continue
			}
			removed++
			emit(Event{"container", name, "removed", ""})
		}
		if parseErr == nil {
			for _, name := range sortedKeys(file.Networks) {
				definition := file.Networks[name]
				if definition.External {
					continue
				}
				actual := resourceName(stack.Name, name, definition.Name)
				emit(Event{"network", actual, "removing", ""})
				if err := cl.RemoveNetwork(ctx, actual); err != nil {
					// Not fatal to the teardown: another stack's container
					// sharing this network by an explicit `name:`, or it never
					// having existed, both just mean there's nothing to do here.
					emit(Event{"network", actual, "skipped", err.Error()})
					continue
				}
				emit(Event{"network", actual, "removed", ""})
			}
		}
		if removeVolumes && parseErr == nil {
			for _, name := range sortedKeys(file.Volumes) {
				definition := file.Volumes[name]
				if definition.External {
					continue
				}
				actual := resourceName(stack.Name, name, definition.Name)
				emit(Event{"volume", actual, "removing", ""})
				if err := cl.RemoveVolume(ctx, actual, false); err != nil {
					emit(Event{"volume", actual, "error", err.Error()})
					continue
				}
				emit(Event{"volume", actual, "removed", ""})
			}
		}
		emit(Event{"stack", stack.Name, "done", fmt.Sprintf("removed %d container(s)", removed)})
	}()
	return events, nil
}

// Redeploy is Down (keeping volumes) followed by Up, over one event stream.
func Redeploy(ctx context.Context, cl Docker, stack Stack) (<-chan Event, error) {
	down, err := Down(ctx, cl, stack, false)
	if err != nil {
		return nil, err
	}
	events := make(chan Event, 16)
	go func() {
		defer close(events)
		for event := range down {
			if event.Kind == "stack" && event.Phase == "done" {
				continue // one summary per redeploy, emitted by Up
			}
			select {
			case events <- event:
			case <-ctx.Done():
				return
			}
		}
		up, err := Up(ctx, cl, stack)
		if err != nil {
			select {
			case events <- Event{"stack", stack.Name, "error", err.Error()}:
			case <-ctx.Done():
			}
			return
		}
		for event := range up {
			select {
			case events <- event:
			case <-ctx.Done():
				return
			}
		}
	}()
	return events, nil
}

// Status reports the stack's deployment state by counting its labeled
// containers against the number of services its compose file declares.
func Status(ctx context.Context, cl Docker, stackName string, serviceCount int) (string, error) {
	containers, err := cl.ListContainers(ctx, projectFilter(stackName))
	if err != nil {
		return "", err
	}
	if len(containers) == 0 {
		return StatusNotDeployed, nil
	}
	running := 0
	for _, container := range containers {
		if container.State == "running" {
			running++
		}
	}
	switch {
	case running == 0:
		return StatusStopped, nil
	case running >= serviceCount && len(containers) >= serviceCount:
		return StatusRunning, nil
	default:
		return StatusPartial, nil
	}
}

// Containers returns the stack's containers, newest name order, for the detail
// view and the merged log stream.
func Containers(ctx context.Context, cl Docker, stackName string) ([]dockerapi.Container, error) {
	containers, err := cl.ListContainers(ctx, projectFilter(stackName))
	if err != nil {
		return nil, err
	}
	sort.Slice(containers, func(i, j int) bool { return displayName(containers[i]) < displayName(containers[j]) })
	return containers, nil
}

// serviceSpec translates one compose service into Vessel's container spec.
func serviceSpec(project, service string, file Compose) dockerapi.Spec {
	definition := file.Services[service]
	spec := dockerapi.Spec{
		Name:          containerName(project, service),
		Image:         definition.Image,
		Command:       strings.Fields(definition.Command),
		Entrypoint:    strings.Fields(definition.Entrypoint),
		RestartPolicy: definition.Restart,
		Labels:        withProjectLabels(definition.Labels, project, service),
		// Docker's embedded DNS only resolves a container by its own name
		// otherwise — compose's actual behavior is every service reachable by
		// its short service name, which is what every service's environment
		// (DB_HOST=db, REDIS_HOST=cache, ...) assumes.
		NetworkAliases: []string{service},
	}
	for _, key := range sortedKeys(definition.Env) {
		spec.Env = append(spec.Env, key+"="+definition.Env[key])
	}
	for _, port := range definition.Ports {
		spec.Ports = append(spec.Ports, parsePort(port))
	}
	for _, mount := range definition.Volumes {
		spec.Mounts = append(spec.Mounts, parseMount(project, mount, file))
	}
	for i, network := range definition.Networks {
		definitionOf := file.Networks[network]
		actual := resourceName(project, network, definitionOf.Name)
		if i == 0 {
			spec.Network = actual
			continue
		}
		spec.AdditionalNetworks = append(spec.AdditionalNetworks, actual)
	}
	return spec
}

// parsePort accepts compose's short syntax: "80", "8080:80",
// "127.0.0.1:8080:80" and any of those with a "/udp" suffix.
func parsePort(entry string) dockerapi.PortSpec {
	protocol := "tcp"
	if at := strings.LastIndex(entry, "/"); at >= 0 {
		protocol, entry = entry[at+1:], entry[:at]
	}
	parts := strings.Split(entry, ":")
	switch len(parts) {
	case 1:
		return dockerapi.PortSpec{Container: parts[0], Protocol: protocol}
	case 2:
		return dockerapi.PortSpec{Container: parts[1], Host: parts[0], Protocol: protocol}
	default:
		return dockerapi.PortSpec{Container: parts[len(parts)-1], Host: parts[len(parts)-2], Protocol: protocol}
	}
}

// parseMount accepts compose's short volume syntax. A source that names a
// top-level volume becomes a project-scoped named volume; anything starting
// with "/" or "." is a bind.
func parseMount(project, entry string, file Compose) dockerapi.MountSpec {
	parts := strings.Split(entry, ":")
	if len(parts) == 1 {
		return dockerapi.MountSpec{Type: "volume", Target: parts[0]}
	}
	mount := dockerapi.MountSpec{Type: "volume", Source: parts[0], Target: parts[1], ReadOnly: len(parts) > 2 && parts[2] == "ro"}
	if definition, ok := file.Volumes[parts[0]]; ok {
		mount.Source = resourceName(project, parts[0], definition.Name)
		return mount
	}
	if strings.HasPrefix(parts[0], "/") || strings.HasPrefix(parts[0], ".") || strings.HasPrefix(parts[0], "~") {
		mount.Type = "bind"
	}
	return mount
}

// resourceName is Compose's naming rule for networks and volumes: an explicit
// `name:` wins, otherwise the key is prefixed with the project.
func resourceName(project, key, explicit string) string {
	if explicit != "" {
		return explicit
	}
	return project + "_" + key
}

// containerName matches Compose v2's `<project>-<service>-<index>` layout.
// Vessel deploys exactly one container per service, so the index is always 1.
func containerName(project, service string) string { return project + "-" + service + "-1" }

func withProjectLabels(labels map[string]string, project, service string) map[string]string {
	out := make(map[string]string, len(labels)+2)
	for key, value := range labels {
		out[key] = value
	}
	out[LabelProject] = project
	if service != "" {
		out[LabelService] = service
	}
	return out
}

// isMissingImage reports whether err is CreateContainer's 404 for an image
// that isn't present locally — the only case an image-scoped 404 arises in
// this codepath.
func isMissingImage(err error) bool {
	return errors.Is(err, dockerapi.ErrNotFound)
}

// isConflict reports whether err is Docker's 409 for a resource that already
// exists by that name — not a deploy failure, just this stack's own resource
// surviving from an earlier run.
func isConflict(err error) bool {
	return errors.Is(err, dockerapi.ErrConflict)
}

// pullImage pulls an image not yet present locally, emitting one "image"
// event for the attempt and one for its outcome, so a slow first deploy is
// visible in the console rather than looking hung on "creating".
func pullImage(ctx context.Context, cl Docker, image string, emit func(Event) bool) error {
	emit(Event{"image", image, "pulling", ""})
	stream, err := cl.PullImage(ctx, image)
	if err != nil {
		emit(Event{"image", image, "error", err.Error()})
		return err
	}
	defer func() { _ = stream.Close() }()
	for {
		progress, err := stream.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			emit(Event{"image", image, "error", err.Error()})
			return err
		}
		if progress.Error != "" {
			emit(Event{"image", image, "error", progress.Error})
			return fmt.Errorf("pull %s: %s", image, progress.Error)
		}
	}
	emit(Event{"image", image, "pulled", ""})
	return nil
}

func displayName(container dockerapi.Container) string {
	if len(container.Names) > 0 {
		return strings.TrimPrefix(container.Names[0], "/")
	}
	return container.ID
}

func sortedKeys[V any](in map[string]V) []string {
	out := make([]string, 0, len(in))
	for key := range in {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
