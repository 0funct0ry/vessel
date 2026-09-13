package dockerapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// PortSpec describes one container port and its optional published host port.
type PortSpec struct{ Container, Host, Protocol string }

// MountSpec describes a named-volume or bind mount.
type MountSpec struct {
	Source, Target, Type string
	ReadOnly             bool
}

// Spec is Vessel's deliberately small container-provisioning surface.
type Spec struct {
	Name, Image            string
	Command, Entrypoint    []string
	Env                    []string
	Ports                  []PortSpec
	Mounts                 []MountSpec
	Network, RestartPolicy string
	MacAddress             string
	AdditionalNetworks     []string
	// NetworkAliases are extra DNS names for this container on every network
	// it joins (Network and each of AdditionalNetworks) — Docker's embedded
	// DNS otherwise only resolves a container by its own name, not by any
	// caller-meaningful name like a compose service's.
	NetworkAliases []string
	Labels         map[string]string
	Start          bool
}

type CreateResult struct {
	ID       string
	Warnings []string
}

// StartError means Docker created the container but could not start it.
// Callers can use the result to direct the user to the newly-created container.
type StartError struct {
	Result CreateResult
	Err    error
}

func (e *StartError) Error() string {
	return fmt.Sprintf("dockerapi: starting created container %s: %v", e.Result.ID, e.Err)
}
func (e *StartError) Unwrap() error { return e.Err }

func portKey(port PortSpec) string {
	protocol := strings.ToLower(strings.TrimSpace(port.Protocol))
	if protocol == "" {
		protocol = "tcp"
	}
	return strings.TrimSpace(port.Container) + "/" + protocol
}

// CreateContainer translates Spec to Docker's POST /containers/create body.
func (c *Client) CreateContainer(ctx context.Context, spec Spec) (CreateResult, error) {
	type binding struct {
		HostPort string `json:"HostPort"`
	}
	type mount struct {
		Type     string `json:"Type"`
		Source   string `json:"Source"`
		Target   string `json:"Target"`
		ReadOnly bool   `json:"ReadOnly,omitempty"`
	}
	type restart struct {
		Name string `json:"Name,omitempty"`
	}
	type endpoint struct {
		MacAddress string   `json:"MacAddress,omitempty"`
		Aliases    []string `json:"Aliases,omitempty"`
	}
	request := struct {
		Image            string              `json:"Image"`
		Cmd              []string            `json:"Cmd,omitempty"`
		Entrypoint       []string            `json:"Entrypoint,omitempty"`
		Env              []string            `json:"Env,omitempty"`
		ExposedPorts     map[string]struct{} `json:"ExposedPorts,omitempty"`
		Labels           map[string]string   `json:"Labels,omitempty"`
		NetworkingConfig *struct {
			EndpointsConfig map[string]endpoint `json:"EndpointsConfig"`
		} `json:"NetworkingConfig,omitempty"`
		HostConfig struct {
			PortBindings  map[string][]binding `json:"PortBindings,omitempty"`
			Binds         []string             `json:"Binds,omitempty"`
			Mounts        []mount              `json:"Mounts,omitempty"`
			NetworkMode   string               `json:"NetworkMode,omitempty"`
			RestartPolicy restart              `json:"RestartPolicy,omitempty"`
		} `json:"HostConfig"`
	}{Image: spec.Image, Cmd: spec.Command, Entrypoint: spec.Entrypoint, Env: spec.Env, Labels: spec.Labels}
	for _, p := range spec.Ports {
		key := portKey(p)
		if strings.TrimSpace(p.Container) == "" {
			continue
		}
		if request.ExposedPorts == nil {
			request.ExposedPorts = map[string]struct{}{}
		}
		request.ExposedPorts[key] = struct{}{}
		if strings.TrimSpace(p.Host) != "" {
			if request.HostConfig.PortBindings == nil {
				request.HostConfig.PortBindings = map[string][]binding{}
			}
			request.HostConfig.PortBindings[key] = append(request.HostConfig.PortBindings[key], binding{HostPort: p.Host})
		}
	}
	for _, item := range spec.Mounts {
		if item.Type == "bind" {
			bind := item.Source + ":" + item.Target
			if item.ReadOnly {
				bind += ":ro"
			}
			request.HostConfig.Binds = append(request.HostConfig.Binds, bind)
		} else {
			request.HostConfig.Mounts = append(request.HostConfig.Mounts, mount{Type: "volume", Source: item.Source, Target: item.Target, ReadOnly: item.ReadOnly})
		}
	}
	request.HostConfig.NetworkMode = spec.Network
	request.HostConfig.RestartPolicy = restart{Name: spec.RestartPolicy}
	if spec.Network != "" && (spec.MacAddress != "" || len(spec.NetworkAliases) > 0) {
		request.NetworkingConfig = &struct {
			EndpointsConfig map[string]endpoint `json:"EndpointsConfig"`
		}{EndpointsConfig: map[string]endpoint{spec.Network: {MacAddress: spec.MacAddress, Aliases: spec.NetworkAliases}}}
	}
	body, err := json.Marshal(request)
	if err != nil {
		return CreateResult{}, fmt.Errorf("dockerapi: encoding container create: %w", err)
	}
	path := "/containers/create"
	if spec.Name != "" {
		path += "?" + url.Values{"name": {spec.Name}}.Encode()
	}
	resp, err := c.do(ctx, http.MethodPost, path, body)
	if err != nil {
		return CreateResult{}, err
	}
	defer resp.Body.Close()
	var wire struct {
		ID       string   `json:"Id"`
		Warnings []string `json:"Warnings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wire); err != nil {
		return CreateResult{}, fmt.Errorf("dockerapi: decoding created container: %w", err)
	}
	warnings := wire.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	result := CreateResult{ID: wire.ID, Warnings: warnings}
	result.Warnings = append(result.Warnings, attachAdditionalNetworks(ctx, c, result.ID, spec.AdditionalNetworks, spec.NetworkAliases)...)
	if spec.Start {
		if err := c.Lifecycle(ctx, result.ID, "start", nil); err != nil {
			return result, &StartError{Result: result, Err: err}
		}
	}
	return result, nil
}

// attachAdditionalNetworks connects a freshly created container to every
// network beyond its primary one. Docker's create call only accepts a single
// network in NetworkingConfig, so extras are attached with one NetworkConnect
// call each; a failed attach is reported as a warning, not a fatal error,
// since the container itself was already created successfully.
func attachAdditionalNetworks(ctx context.Context, c *Client, containerID string, networks, aliases []string) []string {
	var warnings []string
	for _, network := range networks {
		if network == "" {
			continue
		}
		if err := c.NetworkConnectAliased(ctx, network, containerID, aliases); err != nil {
			warnings = append(warnings, fmt.Sprintf("could not attach to network %s: %v", network, err))
		}
	}
	return warnings
}
