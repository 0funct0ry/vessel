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
	LayersSize int64
	Images     []json.RawMessage
	Containers []json.RawMessage
	Volumes    []json.RawMessage
	BuildCache []json.RawMessage
	Raw        json.RawMessage
}

type diskUsageResponse struct {
	LayersSize int64             `json:"LayersSize"`
	Images     []json.RawMessage `json:"Images"`
	Containers []json.RawMessage `json:"Containers"`
	Volumes    []json.RawMessage `json:"Volumes"`
	BuildCache []json.RawMessage `json:"BuildCache"`
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

	return &DiskUsageInfo{
		LayersSize: v.LayersSize,
		Images:     v.Images,
		Containers: v.Containers,
		Volumes:    v.Volumes,
		BuildCache: v.BuildCache,
		Raw:        raw,
	}, nil
}
