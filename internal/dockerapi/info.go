package dockerapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// versionResponse mirrors the fields of GET /version that we care about.
type versionResponse struct {
	Version    string `json:"Version"`
	APIVersion string `json:"ApiVersion"`
	MinAPI     string `json:"MinAPIVersion"`
	Os         string `json:"Os"`
	Arch       string `json:"Arch"`
	KernelVer  string `json:"KernelVersion"`
}

// VersionInfo is the view of GET /version returned to callers.
type VersionInfo struct {
	Version    string
	APIVersion string
	MinAPI     string
	Os         string
	Arch       string
	KernelVer  string
}

// Ping calls the unversioned GET /_ping and returns nil if the engine
// responded successfully.
func (c *Client) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/_ping", nil)
	if err != nil {
		return fmt.Errorf("dockerapi: building ping request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return mapError(resp)
	}
	return nil
}

// Version calls GET /version and returns the engine's version info. This
// also drives API-version negotiation: it is safe (and cheap) to call before
// any other request.
func (c *Client) Version(ctx context.Context) (*VersionInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/version", nil)
	if err != nil {
		return nil, fmt.Errorf("dockerapi: building version request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, mapError(resp)
	}

	var v versionResponse
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, fmt.Errorf("dockerapi: decoding /version response: %w", err)
	}

	return &VersionInfo{
		Version:    v.Version,
		APIVersion: v.APIVersion,
		MinAPI:     v.MinAPI,
		Os:         v.Os,
		Arch:       v.Arch,
		KernelVer:  v.KernelVer,
	}, nil
}

// infoResponse mirrors the fields of GET /info that we care about.
type infoResponse struct {
	ID                string `json:"ID"`
	Containers        int    `json:"Containers"`
	ContainersRunning int    `json:"ContainersRunning"`
	ContainersPaused  int    `json:"ContainersPaused"`
	ContainersStopped int    `json:"ContainersStopped"`
	Images            int    `json:"Images"`
	OperatingSystem   string `json:"OperatingSystem"`
	OSType            string `json:"OSType"`
	Architecture      string `json:"Architecture"`
	NCPU              int    `json:"NCPU"`
	MemTotal          int64  `json:"MemTotal"`
	ServerVersion     string `json:"ServerVersion"`
}

// Info is the view of GET /info returned to callers.
type Info struct {
	ID                string
	Containers        int
	ContainersRunning int
	ContainersPaused  int
	ContainersStopped int
	Images            int
	OperatingSystem   string
	OSType            string
	Architecture      string
	NCPU              int
	MemTotal          int64
	ServerVersion     string
	Raw               json.RawMessage
}

// Info calls GET /info.
func (c *Client) Info(ctx context.Context) (*Info, error) {
	resp, err := c.do(ctx, http.MethodGet, "/info", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("dockerapi: reading /info response: %w", err)
	}

	var v infoResponse
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("dockerapi: decoding /info response: %w", err)
	}

	return &Info{
		ID:                v.ID,
		Containers:        v.Containers,
		ContainersRunning: v.ContainersRunning,
		ContainersPaused:  v.ContainersPaused,
		ContainersStopped: v.ContainersStopped,
		Images:            v.Images,
		OperatingSystem:   v.OperatingSystem,
		OSType:            v.OSType,
		Architecture:      v.Architecture,
		NCPU:              v.NCPU,
		MemTotal:          v.MemTotal,
		ServerVersion:     v.ServerVersion,
		Raw:               raw,
	}, nil
}

// DiskUsageInfo is the view of GET /system/df returned to callers.
type DiskUsageInfo struct {
	Images     []DiskImage
	Containers []DiskContainer
	Volumes    []DiskVolume
	BuildCache []DiskBuildCache
	Raw        json.RawMessage
}

type DiskImage struct {
	Size       int64
	Containers int
}
type DiskContainer struct {
	SizeRW int64
	State  string
}
type DiskVolume struct {
	Name       string
	UsageKnown bool
	UsageData  struct {
		Size     int64
		RefCount int
	}
}
type DiskBuildCache struct {
	Size       int64
	UsageCount int
}

type diskUsageResponse struct {
	Images []struct {
		Size       int64 `json:"Size"`
		Containers int   `json:"Containers"`
	} `json:"Images"`
	Containers []struct {
		SizeRW int64  `json:"SizeRw"`
		State  string `json:"State"`
	} `json:"Containers"`
	Volumes []struct {
		Name      string `json:"Name"`
		UsageData *struct {
			Size     int64 `json:"Size"`
			RefCount int   `json:"RefCount"`
		} `json:"UsageData"`
	} `json:"Volumes"`
	BuildCache []struct {
		Size       int64 `json:"Size"`
		UsageCount int   `json:"UsageCount"`
	} `json:"BuildCache"`
}

// DiskUsage calls GET /system/df.
func (c *Client) DiskUsage(ctx context.Context) (*DiskUsageInfo, error) {
	resp, err := c.do(ctx, http.MethodGet, "/system/df", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("dockerapi: reading /system/df response: %w", err)
	}

	var v diskUsageResponse
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("dockerapi: decoding /system/df response: %w", err)
	}

	info := &DiskUsageInfo{Raw: raw}
	for _, image := range v.Images {
		info.Images = append(info.Images, DiskImage{Size: image.Size, Containers: image.Containers})
	}
	for _, container := range v.Containers {
		info.Containers = append(info.Containers, DiskContainer{SizeRW: container.SizeRW, State: container.State})
	}
	for _, volume := range v.Volumes {
		out := DiskVolume{Name: volume.Name, UsageKnown: volume.UsageData != nil}
		if volume.UsageData != nil {
			out.UsageData.Size, out.UsageData.RefCount = volume.UsageData.Size, volume.UsageData.RefCount
		}
		info.Volumes = append(info.Volumes, out)
	}
	for _, cache := range v.BuildCache {
		info.BuildCache = append(info.BuildCache, DiskBuildCache{Size: cache.Size, UsageCount: cache.UsageCount})
	}
	return info, nil
}
