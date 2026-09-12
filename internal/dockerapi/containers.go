package dockerapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Port mirrors one entry of a container's port mapping.
type Port struct {
	IP          string `json:"IP,omitempty"`
	PrivatePort uint16 `json:"PrivatePort"`
	PublicPort  uint16 `json:"PublicPort,omitempty"`
	Type        string `json:"Type"`
}

// Container is the summary view of one entry from GET /containers/json.
type Container struct {
	ID      string            `json:"Id"`
	Names   []string          `json:"Names"`
	Image   string            `json:"Image"`
	ImageID string            `json:"ImageID"`
	Command string            `json:"Command"`
	Created int64             `json:"Created"`
	State   string            `json:"State"`
	Status  string            `json:"Status"`
	Ports   []Port            `json:"Ports"`
	Labels  map[string]string `json:"Labels"`
	Mounts  []ContainerMount  `json:"Mounts"`
	Health  string            `json:"-"`
}

// parseHealthFromStatus extracts the health check state Docker embeds as a
// parenthetical suffix on the list endpoint's free-text Status (there is no
// structured health field on GET /containers/json, only on the per-container
// inspect endpoint).
func parseHealthFromStatus(status string) string {
	switch {
	case strings.Contains(status, "(healthy)"):
		return "healthy"
	case strings.Contains(status, "(unhealthy)"):
		return "unhealthy"
	case strings.Contains(status, "(health: starting)"):
		return "starting"
	default:
		return ""
	}
}

// ListContainersOptions controls GET /containers/json.
type ListContainersOptions struct {
	All     bool
	Filters map[string][]string
}

// ListContainers calls GET /containers/json.
func (c *Client) ListContainers(ctx context.Context, opts ListContainersOptions) ([]Container, error) {
	q := url.Values{}
	if opts.All {
		q.Set("all", "1")
	}
	if len(opts.Filters) > 0 {
		f, err := json.Marshal(opts.Filters)
		if err != nil {
			return nil, fmt.Errorf("dockerapi: encoding filters: %w", err)
		}
		q.Set("filters", string(f))
	}

	path := "/containers/json"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}

	resp, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var containers []Container
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		return nil, fmt.Errorf("dockerapi: decoding /containers/json response: %w", err)
	}
	for i := range containers {
		containers[i].Health = parseHealthFromStatus(containers[i].Status)
	}
	return containers, nil
}

// ContainerMount is one entry of a container's Mounts list.
type ContainerMount struct {
	Type        string `json:"Type"`
	Name        string `json:"Name,omitempty"`
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
	RW          bool   `json:"RW"`
}

// ContainerNetwork is one entry of a container's NetworkSettings.Networks map.
type ContainerNetwork struct {
	NetworkID string `json:"NetworkID"`
	IPAddress string `json:"IPAddress"`
}

// ContainerDetail is the view of GET /containers/{id}/json.
type ContainerDetail struct {
	ID            string
	Name          string
	Image         string
	Command       []string
	Created       string
	State         string
	Status        string
	ExitCode      int
	Health        string
	RestartPolicy string
	Mounts        []ContainerMount
	Networks      map[string]ContainerNetwork
	Env           []string
	Labels        map[string]string
	Raw           json.RawMessage
}

type containerInspectResponse struct {
	ID      string `json:"Id"`
	Name    string `json:"Name"`
	Created string `json:"Created"`
	State   struct {
		Status   string `json:"Status"`
		ExitCode int    `json:"ExitCode"`
		Health   *struct {
			Status string `json:"Status"`
		} `json:"Health,omitempty"`
	} `json:"State"`
	Config struct {
		Image  string            `json:"Image"`
		Cmd    []string          `json:"Cmd"`
		Env    []string          `json:"Env"`
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
	HostConfig struct {
		RestartPolicy struct {
			Name string `json:"Name"`
		} `json:"RestartPolicy"`
	} `json:"HostConfig"`
	Mounts          []ContainerMount `json:"Mounts"`
	NetworkSettings struct {
		Networks map[string]ContainerNetwork `json:"Networks"`
	} `json:"NetworkSettings"`
}

// InspectContainer calls GET /containers/{id}/json.
func (c *Client) InspectContainer(ctx context.Context, id string) (*ContainerDetail, error) {
	resp, err := c.do(ctx, http.MethodGet, "/containers/"+url.PathEscape(id)+"/json", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("dockerapi: reading container inspect response: %w", err)
	}

	var v containerInspectResponse
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("dockerapi: decoding container inspect response: %w", err)
	}

	health := ""
	if v.State.Health != nil {
		health = v.State.Health.Status
	}

	return &ContainerDetail{
		ID:            v.ID,
		Name:          v.Name,
		Image:         v.Config.Image,
		Command:       v.Config.Cmd,
		Created:       v.Created,
		State:         v.State.Status,
		Status:        v.State.Status,
		ExitCode:      v.State.ExitCode,
		Health:        health,
		RestartPolicy: v.HostConfig.RestartPolicy.Name,
		Mounts:        v.Mounts,
		Networks:      v.NetworkSettings.Networks,
		Env:           v.Config.Env,
		Labels:        v.Config.Labels,
		Raw:           raw,
	}, nil
}

// TopEntry is one process row from GET /containers/{id}/top.
type TopEntry struct {
	Titles    []string
	Processes [][]string
}

type topResponse struct {
	Titles    []string   `json:"Titles"`
	Processes [][]string `json:"Processes"`
}

// Top calls GET /containers/{id}/top.
func (c *Client) Top(ctx context.Context, id string, psArgs string) (*TopEntry, error) {
	path := "/containers/" + url.PathEscape(id) + "/top"
	if psArgs != "" {
		path += "?ps_args=" + url.QueryEscape(psArgs)
	}

	resp, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var v topResponse
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, fmt.Errorf("dockerapi: decoding top response: %w", err)
	}
	return &TopEntry{Titles: v.Titles, Processes: v.Processes}, nil
}
