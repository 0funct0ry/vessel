package dockerapi

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// statsFrame mirrors the fields of one GET /containers/{id}/stats?stream=1
// JSON object that we need to compute CPU percent and memory usage.
type statsFrame struct {
	Read     string `json:"read"`
	CPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
		OnlineCPUs     uint32 `json:"online_cpus"`
	} `json:"cpu_stats"`
	PreCPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
	} `json:"precpu_stats"`
	MemoryStats struct {
		Usage uint64            `json:"usage"`
		Stats map[string]uint64 `json:"stats"`
	} `json:"memory_stats"`
}

// Stats is one computed sample from a container's stats stream.
type Stats struct {
	Read string
	// CPUPercent is nil for the first sample in a stream, since it is a
	// delta between two samples and there is no previous one yet.
	CPUPercent *float64
	MemUsage   uint64
	MemLimit   uint64
	Raw        json.RawMessage
}

// StatsReader streams computed Stats samples from a container. Call Next in
// a loop until it returns io.EOF (or another error); Close releases the
// underlying HTTP response.
type StatsReader struct {
	body io.ReadCloser
	dec  *json.Decoder

	havePrev  bool
	prevTotal uint64
	prevSys   uint64
}

// Close releases the underlying HTTP connection.
func (r *StatsReader) Close() error {
	return r.body.Close()
}

// Next decodes and returns the next stats sample.
func (r *StatsReader) Next() (Stats, error) {
	var raw json.RawMessage
	if err := r.dec.Decode(&raw); err != nil {
		return Stats{}, err
	}

	var f statsFrame
	if err := json.Unmarshal(raw, &f); err != nil {
		return Stats{}, fmt.Errorf("dockerapi: decoding stats frame: %w", err)
	}

	memUsage := f.MemoryStats.Usage
	if v, ok := f.MemoryStats.Stats["inactive_file"]; ok {
		// cgroup v2
		memUsage -= v
	} else if v, ok := f.MemoryStats.Stats["cache"]; ok {
		// cgroup v1
		memUsage -= v
	}

	var cpuPercent *float64
	if r.havePrev {
		cpuDelta := float64(f.CPUStats.CPUUsage.TotalUsage) - float64(r.prevTotal)
		sysDelta := float64(f.CPUStats.SystemCPUUsage) - float64(r.prevSys)
		if sysDelta > 0 && cpuDelta >= 0 {
			online := float64(f.CPUStats.OnlineCPUs)
			if online == 0 {
				online = 1
			}
			pct := (cpuDelta / sysDelta) * online * 100
			cpuPercent = &pct
		}
	}
	r.havePrev = true
	r.prevTotal = f.CPUStats.CPUUsage.TotalUsage
	r.prevSys = f.CPUStats.SystemCPUUsage

	return Stats{
		Read:       f.Read,
		CPUPercent: cpuPercent,
		MemUsage:   memUsage,
		MemLimit:   0,
		Raw:        raw,
	}, nil
}

// StatsStream calls GET /containers/{id}/stats?stream=1 and returns a
// StatsReader over the response body.
func (c *Client) StatsStream(ctx context.Context, id string) (*StatsReader, error) {
	path := "/containers/" + url.PathEscape(id) + "/stats?stream=1"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url(path), nil)
	if err != nil {
		return nil, fmt.Errorf("dockerapi: building stats request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, mapError(resp)
	}

	return &StatsReader{
		body: resp.Body,
		dec:  json.NewDecoder(bufio.NewReader(resp.Body)),
	}, nil
}
