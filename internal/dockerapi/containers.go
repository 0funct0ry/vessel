package dockerapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
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
	ID              string            `json:"Id"`
	Names           []string          `json:"Names"`
	Image           string            `json:"Image"`
	ImageID         string            `json:"ImageID"`
	Command         string            `json:"Command"`
	Created         int64             `json:"Created"`
	State           string            `json:"State"`
	Status          string            `json:"Status"`
	Ports           []Port            `json:"Ports"`
	Labels          map[string]string `json:"Labels"`
	Mounts          []ContainerMount  `json:"Mounts"`
	Health          string            `json:"-"`
	NetworkSettings struct {
		Networks map[string]ContainerNetwork `json:"Networks"`
	} `json:"NetworkSettings"`
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
	NetworkID         string `json:"NetworkID"`
	IPAddress         string `json:"IPAddress"`
	GlobalIPv6Address string `json:"GlobalIPv6Address"`
}

// ContainerSecurity is the security-relevant subset of a container's
// HostConfig/Config, surfaced as its own struct rather than left buried in
// the raw inspect JSON.
type ContainerSecurity struct {
	Privileged      bool   `json:"privileged"`
	ReadonlyRootfs  bool   `json:"readonly_rootfs"`
	User            string `json:"user"`
	UsernsMode      string `json:"userns_mode"`
	AppArmorProfile string `json:"apparmor_profile"`
}

// ContainerResources is the resource-limit subset of a container's
// HostConfig.
type ContainerResources struct {
	CPUShares         int64   `json:"cpu_shares"`
	Cpus              float64 `json:"cpus"`
	Memory            int64   `json:"memory"`
	MemorySwap        int64   `json:"memory_swap"`
	MemoryReservation int64   `json:"memory_reservation"`
	PidsLimit         int64   `json:"pids_limit"`
	OomKillDisable    bool    `json:"oom_kill_disable"`
	CPUPeriod         int64   `json:"cpu_period"`
	CPUQuota          int64   `json:"cpu_quota"`
	CgroupParent      string  `json:"cgroup_parent"`
	CgroupnsMode      string  `json:"cgroupns_mode"`
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
	Ports         []Port
	Security      ContainerSecurity
	Resources     ContainerResources
	Raw           json.RawMessage
}

type containerInspectResponse struct {
	ID              string `json:"Id"`
	Name            string `json:"Name"`
	Created         string `json:"Created"`
	AppArmorProfile string `json:"AppArmorProfile"`
	State           struct {
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
		User   string            `json:"User"`
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
	HostConfig struct {
		RestartPolicy struct {
			Name string `json:"Name"`
		} `json:"RestartPolicy"`
		Privileged        bool   `json:"Privileged"`
		ReadonlyRootfs    bool   `json:"ReadonlyRootfs"`
		UsernsMode        string `json:"UsernsMode"`
		CPUShares         int64  `json:"CpuShares"`
		NanoCpus          int64  `json:"NanoCpus"`
		Memory            int64  `json:"Memory"`
		MemorySwap        int64  `json:"MemorySwap"`
		MemoryReservation int64  `json:"MemoryReservation"`
		PidsLimit         *int64 `json:"PidsLimit"`
		OomKillDisable    bool   `json:"OomKillDisable"`
		CPUPeriod         int64  `json:"CpuPeriod"`
		CPUQuota          int64  `json:"CpuQuota"`
		CgroupParent      string `json:"CgroupParent"`
		CgroupnsMode      string `json:"CgroupnsMode"`
	} `json:"HostConfig"`
	Mounts          []ContainerMount `json:"Mounts"`
	NetworkSettings struct {
		Networks map[string]ContainerNetwork `json:"Networks"`
		Ports    map[string][]struct {
			HostIP   string `json:"HostIp"`
			HostPort string `json:"HostPort"`
		} `json:"Ports"`
	} `json:"NetworkSettings"`
}

// portsFromInspect turns Docker's inspect NetworkSettings.Ports map (keyed
// "containerPort/proto", each value a possibly-empty list of host bindings)
// into the same flat Port list GET /containers/json already returns, so both
// endpoints share one shape.
func portsFromInspect(raw map[string][]struct {
	HostIP   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}) []Port {
	var ports []Port
	for key, bindings := range raw {
		privatePort, protocol, ok := strings.Cut(key, "/")
		if !ok {
			continue
		}
		private, err := strconv.ParseUint(privatePort, 10, 16)
		if err != nil {
			continue
		}
		if len(bindings) == 0 {
			ports = append(ports, Port{PrivatePort: uint16(private), Type: protocol})
			continue
		}
		for _, b := range bindings {
			public, _ := strconv.ParseUint(b.HostPort, 10, 16)
			ports = append(ports, Port{IP: b.HostIP, PrivatePort: uint16(private), PublicPort: uint16(public), Type: protocol})
		}
	}
	return ports
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

	var pidsLimit int64
	if v.HostConfig.PidsLimit != nil {
		pidsLimit = *v.HostConfig.PidsLimit
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
		Ports:         portsFromInspect(v.NetworkSettings.Ports),
		Security: ContainerSecurity{
			Privileged:      v.HostConfig.Privileged,
			ReadonlyRootfs:  v.HostConfig.ReadonlyRootfs,
			User:            v.Config.User,
			UsernsMode:      v.HostConfig.UsernsMode,
			AppArmorProfile: v.AppArmorProfile,
		},
		Resources: ContainerResources{
			CPUShares:         v.HostConfig.CPUShares,
			Cpus:              float64(v.HostConfig.NanoCpus) / 1e9,
			Memory:            v.HostConfig.Memory,
			MemorySwap:        v.HostConfig.MemorySwap,
			MemoryReservation: v.HostConfig.MemoryReservation,
			PidsLimit:         pidsLimit,
			OomKillDisable:    v.HostConfig.OomKillDisable,
			CPUPeriod:         v.HostConfig.CPUPeriod,
			CPUQuota:          v.HostConfig.CPUQuota,
			CgroupParent:      v.HostConfig.CgroupParent,
			CgroupnsMode:      v.HostConfig.CgroupnsMode,
		},
		Raw: raw,
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
