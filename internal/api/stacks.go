package api

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/0funct0ry/vessel/internal/compose"
	"github.com/0funct0ry/vessel/internal/dockerapi"
	"github.com/0funct0ry/vessel/internal/store"
)

type stackInput struct {
	Name        string `json:"name"`
	ComposeYAML string `json:"compose_yaml"`
	EnvContent  string `json:"env_content"`
}

type stackServiceView struct {
	Name          string `json:"name"`
	Image         string `json:"image"`
	ContainerID   string `json:"container_id,omitempty"`
	ContainerName string `json:"container_name,omitempty"`
	State         string `json:"state"`
	Status        string `json:"status,omitempty"`
}

type stackView struct {
	ID             string             `json:"id"`
	Name           string             `json:"name"`
	Source         string             `json:"source"`
	ComposeYAML    string             `json:"compose_yaml"`
	EnvContent     string             `json:"env_content"`
	Status         string             `json:"status"`
	ServiceCount   int                `json:"service_count"`
	ContainerCount int                `json:"container_count"`
	Services       []stackServiceView `json:"services"`
	Warnings       []compose.Warning  `json:"warnings"`
	ParseError     string             `json:"parse_error,omitempty"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
}

// stackSource is always "internal" in M17.6: Git-backed stacks are deferred,
// but the column exists so adding a source later is not a table redesign.
const stackSource = "internal"

func stackErr(c *gin.Context, err error) {
	if errors.Is(err, store.ErrNotFound) {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "stack_not_found", "message": "stack not found"}})
		return
	}
	Fail(c, err)
}

func stackID() string { return "st_" + strconv.FormatInt(time.Now().UnixNano(), 36) }

// stackModel converts the stored row into the value the compose engine takes.
func stackModel(s store.Stack) compose.Stack {
	return compose.Stack{Name: s.Name, ComposeYAML: s.ComposeYAML, EnvContent: s.EnvContent}
}

// stackViewOf joins the stored definition with live Docker state. containers
// must already be filtered to this stack.
func stackViewOf(s store.Stack, containers []dockerapi.Container) stackView {
	view := stackView{ID: s.ID, Name: s.Name, Source: stackSource, ComposeYAML: s.ComposeYAML, EnvContent: s.EnvContent, Status: compose.StatusNotDeployed, ContainerCount: len(containers), Services: []stackServiceView{}, Warnings: []compose.Warning{}, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt}

	file, warnings, err := stackModel(s).Parse()
	if err != nil {
		view.ParseError = err.Error()
	}
	if warnings != nil {
		view.Warnings = warnings
	}

	byService := map[string]dockerapi.Container{}
	running := 0
	for _, container := range containers {
		byService[container.Labels[compose.LabelService]] = container
		if container.State == "running" {
			running++
		}
	}
	view.ServiceCount = len(file.Order)
	for _, name := range file.Order {
		service := stackServiceView{Name: name, Image: file.Services[name].Image, State: "not created"}
		if container, ok := byService[name]; ok {
			service.ContainerID, service.ContainerName, service.State, service.Status = container.ID, containerDisplayName(container), container.State, container.Status
		}
		view.Services = append(view.Services, service)
	}

	switch {
	case len(containers) == 0:
		view.Status = compose.StatusNotDeployed
	case running == 0:
		view.Status = compose.StatusStopped
	case running >= view.ServiceCount && len(containers) >= view.ServiceCount:
		view.Status = compose.StatusRunning
	default:
		view.Status = compose.StatusPartial
	}
	return view
}

func containerDisplayName(container dockerapi.Container) string {
	if len(container.Names) > 0 {
		return strings.TrimPrefix(container.Names[0], "/")
	}
	return container.ID
}

// stackContainers lists every compose-labeled container once, so the list view
// costs one Docker round trip rather than one per stack.
func (s *server) stackContainers(c *gin.Context) (map[string][]dockerapi.Container, error) {
	containers, err := s.docker.ListContainers(c.Request.Context(), dockerapi.ListContainersOptions{All: true, Filters: map[string][]string{"label": {compose.LabelProject}}})
	if err != nil {
		return nil, err
	}
	byProject := map[string][]dockerapi.Container{}
	for _, container := range containers {
		project := container.Labels[compose.LabelProject]
		byProject[project] = append(byProject[project], container)
	}
	return byProject, nil
}

func (s *server) handleStacks(c *gin.Context) {
	stacks, err := s.store.ListStacks(c)
	if err != nil {
		Fail(c, err)
		return
	}
	byProject, err := s.stackContainers(c)
	if err != nil {
		Fail(c, err)
		return
	}
	q := strings.ToLower(strings.TrimSpace(c.Query("q")))
	out := make([]stackView, 0, len(stacks))
	for _, item := range stacks {
		if q != "" && !strings.Contains(strings.ToLower(item.Name), q) {
			continue
		}
		out = append(out, stackViewOf(item, byProject[item.Name]))
	}
	c.JSON(http.StatusOK, out)
}

// lookupStack resolves the :name path parameter, writing the error response
// itself when the stack does not exist.
func (s *server) lookupStack(c *gin.Context) (store.Stack, bool) {
	item, err := s.store.GetStackByName(c, c.Param("name"))
	if err != nil {
		stackErr(c, err)
		return store.Stack{}, false
	}
	return item, true
}

func (s *server) handleStack(c *gin.Context) {
	item, ok := s.lookupStack(c)
	if !ok {
		return
	}
	containers, err := compose.Containers(c.Request.Context(), s.docker, item.Name)
	if err != nil {
		Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, stackViewOf(item, containers))
}

// validateStack checks the name and that the compose file parses. A file with
// warnings is accepted — warnings are shown next to the editor, not enforced.
func validateStack(in stackInput) error {
	if err := validResourceName(in.Name); err != nil {
		return err
	}
	if strings.TrimSpace(in.ComposeYAML) == "" {
		return invalidInput("compose_yaml is required")
	}
	if _, _, err := compose.ParseCompose([]byte(in.ComposeYAML), compose.ParseEnvFile(in.EnvContent)); err != nil {
		return invalidInput("%s", err.Error())
	}
	return nil
}

type graphNode struct {
	ID      string              `json:"id"`
	Kind    string              `json:"kind"`
	Name    string              `json:"name"`
	Service *compose.Service    `json:"service,omitempty"`
	Network *compose.NetworkDef `json:"network,omitempty"`
	Volume  *compose.VolumeDef  `json:"volume,omitempty"`
}

type graphEdge struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	From      string `json:"from"`
	To        string `json:"to"`
	MountPath string `json:"mount_path,omitempty"`
	ReadOnly  bool   `json:"read_only,omitempty"`
}

func graphNodesOf(nodes []compose.Node) []graphNode {
	out := make([]graphNode, len(nodes))
	for i, n := range nodes {
		out[i] = graphNode{ID: n.ID, Kind: n.Kind, Name: n.Name, Service: n.Service, Network: n.Network, Volume: n.Volume}
	}
	return out
}

func graphEdgesOf(edges []compose.Edge) []graphEdge {
	out := make([]graphEdge, len(edges))
	for i, e := range edges {
		out[i] = graphEdge{ID: e.ID, Kind: e.Kind, From: e.From, To: e.To, MountPath: e.MountPath, ReadOnly: e.ReadOnly}
	}
	return out
}

func composeNodesOf(nodes []graphNode) []compose.Node {
	out := make([]compose.Node, len(nodes))
	for i, n := range nodes {
		out[i] = compose.Node{ID: n.ID, Kind: n.Kind, Name: n.Name, Service: n.Service, Network: n.Network, Volume: n.Volume}
	}
	return out
}

func composeEdgesOf(edges []graphEdge) []compose.Edge {
	out := make([]compose.Edge, len(edges))
	for i, e := range edges {
		out[i] = compose.Edge{ID: e.ID, Kind: e.Kind, From: e.From, To: e.To, MountPath: e.MountPath, ReadOnly: e.ReadOnly}
	}
	return out
}

type stackGraphToInput struct {
	ComposeYAML string `json:"compose_yaml"`
}

type stackGraphToView struct {
	Nodes    []graphNode       `json:"nodes"`
	Edges    []graphEdge       `json:"edges"`
	Warnings []compose.Warning `json:"warnings"`
}

// handleStackGraphTo parses compose YAML structurally (no ${VAR} resolution,
// so it doesn't destroy variable references) and returns the Graph tab's
// node/edge representation.
func (s *server) handleStackGraphTo(c *gin.Context) {
	var in stackGraphToInput
	if err := decodeBody(c, &in); err != nil {
		Fail(c, err)
		return
	}
	if strings.TrimSpace(in.ComposeYAML) == "" {
		Fail(c, invalidInput("compose_yaml is required"))
		return
	}
	file, warnings, err := compose.ParseComposeStructure([]byte(in.ComposeYAML))
	if err != nil {
		Fail(c, invalidInput("%s", err.Error()))
		return
	}
	nodes, edges := compose.ToGraph(file)
	if warnings == nil {
		warnings = []compose.Warning{}
	}
	c.JSON(http.StatusOK, stackGraphToView{Nodes: graphNodesOf(nodes), Edges: graphEdgesOf(edges), Warnings: warnings})
}

type stackGraphFromInput struct {
	Nodes []graphNode `json:"nodes"`
	Edges []graphEdge `json:"edges"`
	// PreviousComposeYAML is the compose file the Graph tab was showing
	// before this edit (the frontend already holds it in state, so this is
	// free to supply). When present and still parseable, the edit is
	// patched onto that document in place (Compose.Patch) so untouched
	// keys keep their original order and formatting; a graph-edit-in-
	// progress reformat is limited to only what actually changed. Falls
	// back to a full Marshal when omitted (a brand-new stack has no prior
	// document to patch against) or when it fails to parse.
	PreviousComposeYAML string `json:"previous_compose_yaml,omitempty"`
}

type stackGraphFromView struct {
	ComposeYAML string `json:"compose_yaml"`
}

// handleStackGraphFrom rebuilds compose YAML from a Graph tab edit.
func (s *server) handleStackGraphFrom(c *gin.Context) {
	var in stackGraphFromInput
	if err := decodeBody(c, &in); err != nil {
		Fail(c, err)
		return
	}
	file, err := compose.FromGraph(composeNodesOf(in.Nodes), composeEdgesOf(in.Edges))
	if err != nil {
		Fail(c, invalidInput("%s", err.Error()))
		return
	}

	var out []byte
	if strings.TrimSpace(in.PreviousComposeYAML) != "" {
		if root, _, _, parseErr := compose.ParseComposeStructureNode([]byte(in.PreviousComposeYAML)); parseErr == nil {
			if patched, patchErr := file.Patch(root); patchErr == nil {
				out = patched
			}
		}
	}
	if out == nil {
		out, err = file.Marshal()
		if err != nil {
			Fail(c, err)
			return
		}
	}
	c.JSON(http.StatusOK, stackGraphFromView{ComposeYAML: string(out)})
}

func (s *server) handleStackCreate(c *gin.Context) {
	var in stackInput
	if err := decodeBody(c, &in); err != nil {
		Fail(c, err)
		return
	}
	if err := validateStack(in); err != nil {
		Fail(c, err)
		return
	}
	now := time.Now().UTC()
	item, err := s.store.CreateStack(c, store.Stack{ID: stackID(), Name: in.Name, ComposeYAML: in.ComposeYAML, EnvContent: in.EnvContent, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		stackErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, stackViewOf(item, nil))
}

func (s *server) handleStackUpdate(c *gin.Context) {
	item, ok := s.lookupStack(c)
	if !ok {
		return
	}
	var in stackInput
	if err := decodeBody(c, &in); err != nil {
		Fail(c, err)
		return
	}
	if in.ComposeYAML != "" {
		item.ComposeYAML = in.ComposeYAML
	}
	item.EnvContent = in.EnvContent
	if err := validateStack(stackInput{Name: item.Name, ComposeYAML: item.ComposeYAML, EnvContent: item.EnvContent}); err != nil {
		Fail(c, err)
		return
	}
	item.UpdatedAt = time.Now().UTC()
	item, err := s.store.UpdateStack(c, item)
	if err != nil {
		stackErr(c, err)
		return
	}
	containers, err := compose.Containers(c.Request.Context(), s.docker, item.Name)
	if err != nil {
		Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, stackViewOf(item, containers))
}

// streamLifecycle is the shared SSE body for up/down/redeploy. The engine is
// started before Stream takes over the response so a setup failure (an
// unparseable file, an unreachable daemon) is still a normal JSON error.
func (s *server) streamLifecycle(c *gin.Context, start func(item compose.Stack) (<-chan compose.Event, error)) {
	item, ok := s.lookupStack(c)
	if !ok {
		return
	}
	events, err := start(stackModel(item))
	if err != nil {
		if errors.Is(err, compose.ErrNoServices) {
			Fail(c, invalidInput("%s", err.Error()))
			return
		}
		Fail(c, err)
		return
	}
	Stream(c, func(send func(string, any) error) error {
		for event := range events {
			if err := send("step", event); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *server) handleStackUp(c *gin.Context) {
	s.streamLifecycle(c, func(item compose.Stack) (<-chan compose.Event, error) {
		return compose.Up(c.Request.Context(), s.docker, item)
	})
}

func (s *server) handleStackDown(c *gin.Context) {
	removeVolumes := c.Query("volumes") == "true"
	s.streamLifecycle(c, func(item compose.Stack) (<-chan compose.Event, error) {
		return compose.Down(c.Request.Context(), s.docker, item, removeVolumes)
	})
}

func (s *server) handleStackRedeploy(c *gin.Context) {
	s.streamLifecycle(c, func(item compose.Stack) (<-chan compose.Event, error) {
		return compose.Redeploy(c.Request.Context(), s.docker, item)
	})
}

func (s *server) handleStackDelete(c *gin.Context) {
	item, ok := s.lookupStack(c)
	if !ok {
		return
	}
	containers, err := compose.Containers(c.Request.Context(), s.docker, item.Name)
	if err != nil {
		Fail(c, err)
		return
	}
	// Deleting the definition of a stack that is still up would strand its
	// containers with no way back to the compose file that made them.
	for _, container := range containers {
		if container.State == "running" {
			c.AbortWithStatusJSON(http.StatusConflict, gin.H{"error": gin.H{"code": "stack_running", "message": "stop the stack (Down) before removing it"}})
			return
		}
	}
	if c.Query("volumes") == "true" {
		if events, err := compose.Down(c.Request.Context(), s.docker, stackModel(item), true); err == nil {
			for range events { //nolint:revive // drain the lifecycle before deleting the definition
			}
		}
	}
	if err := s.store.DeleteStack(c, item.ID); err != nil {
		stackErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

type stackLogLine struct {
	Service string `json:"service"`
	Stream  string `json:"stream"`
	TS      string `json:"ts,omitempty"`
	Line    string `json:"line"`
}

// handleStackLogs merges every labeled container's log stream into one SSE
// feed, prefixing each line with the service it came from.
func (s *server) handleStackLogs(c *gin.Context) {
	item, ok := s.lookupStack(c)
	if !ok {
		return
	}
	tail := c.DefaultQuery("tail", "200")
	containers, err := compose.Containers(c.Request.Context(), s.docker, item.Name)
	if err != nil {
		Fail(c, err)
		return
	}
	if len(containers) == 0 {
		Fail(c, invalidInput("stack %s has no containers", item.Name))
		return
	}
	Stream(c, func(send func(string, any) error) error {
		var wg sync.WaitGroup
		for _, container := range containers {
			service := container.Labels[compose.LabelService]
			if service == "" {
				service = containerDisplayName(container)
			}
			stream, err := s.docker.LogStream(c.Request.Context(), container.ID, dockerapi.LogsOptions{Follow: true, Tail: tail, Timestamps: true, Stdout: true, Stderr: true})
			if err != nil {
				_ = send("step", stackLogLine{Service: service, Stream: "vessel", Line: err.Error()})
				continue
			}
			wg.Add(1)
			go func(service string, stream dockerapi.LogStream) {
				defer wg.Done()
				defer func() { _ = stream.Close() }()
				for {
					line, err := stream.Next()
					if err != nil {
						if !errors.Is(err, io.EOF) {
							_ = send("step", stackLogLine{Service: service, Stream: "vessel", Line: err.Error()})
						}
						return
					}
					out := stackLogLine{Service: service, Stream: streamName(line.Stream), Line: line.Text}
					if !line.Time.IsZero() {
						out.TS = line.Time.UTC().Format(time.RFC3339Nano)
					}
					if err := send("step", out); err != nil {
						return
					}
				}
			}(service, stream)
		}
		wg.Wait()
		return nil
	})
}
