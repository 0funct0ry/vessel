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
		Limit uint64            `json:"limit"`
		Stats map[string]uint64 `json:"stats"`
	} `json:"memory_stats"`
	Networks map[string]struct {
		RXBytes uint64 `json:"rx_bytes"`
		TXBytes uint64 `json:"tx_bytes"`
	} `json:"networks"`
	BlkioStats struct {
		IOServiceBytesRecursive []struct {
			Op    string `json:"op"`
			Value uint64 `json:"value"`
		} `json:"io_service_bytes_recursive"`
	} `json:"blkio_stats"`
}

// Stats is one computed sample from a container's stats stream.
type Stats struct {
	Read string
	// CPUPercent is nil for the first sample in a stream, since it is a
	// delta between two samples and there is no previous one yet.
	CPUPercent *float64
	MemUsage   uint64
	MemLimit   uint64
	NetRX      uint64
	NetTX      uint64
	BlkRead    uint64
	BlkWrite   uint64
	Raw        json.RawMessage
}

// StatsStream is the API-facing iterator over container statistics.
type StatsStream interface {
	Next() (Stats, error)
	Close() error
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

	memUsage := adjustedMemoryUsage(f.MemoryStats.Usage, f.MemoryStats.Stats)
	var netRX, netTX uint64
	for _, network := range f.Networks {
		netRX += network.RXBytes
		netTX += network.TXBytes
	}
	var blkRead, blkWrite uint64
	for _, entry := range f.BlkioStats.IOServiceBytesRecursive {
		switch entry.Op {
		case "Read":
			blkRead += entry.Value
		case "Write":
			blkWrite += entry.Value
		}
	}

	var cpuPercent *float64
	if r.havePrev {
		if pct, ok := cpuPercentFor(f.CPUStats.CPUUsage.TotalUsage, r.prevTotal, f.CPUStats.SystemCPUUsage, r.prevSys, f.CPUStats.OnlineCPUs); ok {
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
		MemLimit:   f.MemoryStats.Limit,
		NetRX:      netRX,
		NetTX:      netTX,
		BlkRead:    blkRead,
		BlkWrite:   blkWrite,
		Raw:        raw,
	}, nil
}

func adjustedMemoryUsage(usage uint64, stats map[string]uint64) uint64 {
	adjustment, ok := stats["inactive_file"] // cgroup v2
	if !ok {
		adjustment = stats["cache"] // cgroup v1
	}
	if adjustment > usage {
		return 0
	}
	return usage - adjustment
}

func cpuPercentFor(total, previousTotal, system, previousSystem uint64, onlineCPUs uint32) (float64, bool) {
	if total < previousTotal || system <= previousSystem {
		return 0, false
	}
	online := float64(onlineCPUs)
	if online == 0 {
		online = 1
	}
	return (float64(total-previousTotal) / float64(system-previousSystem)) * online * 100, true
}

// StatsStream calls GET /containers/{id}/stats?stream=1 and returns a
// StatsReader over the response body.
func (c *Client) StatsStream(ctx context.Context, id string) (StatsStream, error) {
	return c.stats(ctx, id, true)
}

// Stats returns one immediate Docker stats sample. Unlike StatsStream, its CPU
// percentage uses Docker's embedded precpu_stats baseline.
func (c *Client) Stats(ctx context.Context, id string) (Stats, error) {
	stream, err := c.stats(ctx, id, false)
	if err != nil {
		return Stats{}, err
	}
	defer stream.Close()
	sample, err := stream.Next()
	if err != nil {
		return Stats{}, err
	}
	if sample.CPUPercent == nil {
		var frame statsFrame
		if err := json.Unmarshal(sample.Raw, &frame); err != nil {
			return Stats{}, fmt.Errorf("dockerapi: decoding stats frame: %w", err)
		}
		if pct, ok := cpuPercentFor(frame.CPUStats.CPUUsage.TotalUsage, frame.PreCPUStats.CPUUsage.TotalUsage, frame.CPUStats.SystemCPUUsage, frame.PreCPUStats.SystemCPUUsage, frame.CPUStats.OnlineCPUs); ok {
			sample.CPUPercent = &pct
		}
	}
	return sample, nil
}

func (c *Client) stats(ctx context.Context, id string, stream bool) (*StatsReader, error) {
	path := "/containers/" + url.PathEscape(id) + "/stats?stream=" + map[bool]string{true: "1", false: "0"}[stream]

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
