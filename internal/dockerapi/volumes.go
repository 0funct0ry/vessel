package dockerapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// Volume is the view of one entry from GET /volumes, and of GET
// /volumes/{name}.
type Volume struct {
	Name       string            `json:"Name"`
	Driver     string            `json:"Driver"`
	Mountpoint string            `json:"Mountpoint"`
	CreatedAt  string            `json:"CreatedAt"`
	Labels     map[string]string `json:"Labels"`
	Scope      string            `json:"Scope"`
	Raw        json.RawMessage   `json:"-"`
}

type listVolumesResponse struct {
	Volumes []Volume `json:"Volumes"`
}

// ListVolumes calls GET /volumes.
func (c *Client) ListVolumes(ctx context.Context) ([]Volume, error) {
	resp, err := c.do(ctx, http.MethodGet, "/volumes", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var v listVolumesResponse
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, fmt.Errorf("dockerapi: decoding /volumes response: %w", err)
	}
	return v.Volumes, nil
}

// InspectVolume calls GET /volumes/{name}.
func (c *Client) InspectVolume(ctx context.Context, name string) (*Volume, error) {
	resp, err := c.do(ctx, http.MethodGet, "/volumes/"+url.PathEscape(name), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("dockerapi: reading volume inspect response: %w", err)
	}

	var v Volume
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("dockerapi: decoding volume inspect response: %w", err)
	}
	v.Raw = raw
	return &v, nil
}
