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

// volumeMountHelperImage is the small, pinned utility image used to briefly
// mount a volume for browsing, exporting, or cloning — Docker has no API to
// inspect a volume's contents without attaching it to a container.
const volumeMountHelperImage = "busybox:1.36"

// WithVolumeMount creates a short-lived helper container with name's volume
// bind-mounted read-write at /vessel-volume, starts it, calls fn with its ID,
// and always removes the container afterward — even if fn returns an error.
// It is read-write so the volume file browser can upload, create folders,
// delete, rename, and edit files, not just list and download them.
func (c *Client) WithVolumeMount(ctx context.Context, name string, fn func(containerID string) error) error {
	result, err := c.CreateContainer(ctx, Spec{
		Image:   volumeMountHelperImage,
		Command: []string{"sleep", "3600"},
		Mounts:  []MountSpec{{Type: "volume", Source: name, Target: "/vessel-volume"}},
		Start:   true,
	})
	if err != nil {
		return fmt.Errorf("dockerapi: starting volume mount helper: %w", err)
	}
	defer func() {
		_ = c.RemoveContainer(context.WithoutCancel(ctx), result.ID, RemoveContainerOptions{Force: true})
	}()
	return fn(result.ID)
}

// CloneVolume copies one volume's contents into a newly created volume with
// the same driver and labels. If anything fails after the destination volume
// is created, the destination is removed so a failed clone doesn't leave a
// stray empty volume behind.
func (c *Client) CloneVolume(ctx context.Context, source, dest string) error {
	src, err := c.InspectVolume(ctx, source)
	if err != nil {
		return fmt.Errorf("dockerapi: inspecting source volume: %w", err)
	}
	if _, err := c.CreateVolume(ctx, CreateVolumeOptions{Name: dest, Driver: src.Driver, Labels: src.Labels}); err != nil {
		return fmt.Errorf("dockerapi: creating destination volume: %w", err)
	}
	cloneErr := func() error {
		result, err := c.CreateContainer(ctx, Spec{
			Image:   volumeMountHelperImage,
			Command: []string{"sleep", "3600"},
			Mounts: []MountSpec{
				{Type: "volume", Source: source, Target: "/from", ReadOnly: true},
				{Type: "volume", Source: dest, Target: "/to"},
			},
			Start: true,
		})
		if err != nil {
			return fmt.Errorf("dockerapi: starting volume clone helper: %w", err)
		}
		defer func() {
			_ = c.RemoveContainer(context.WithoutCancel(ctx), result.ID, RemoveContainerOptions{Force: true})
		}()
		if _, err := c.runExec(ctx, result.ID, []string{"cp", "-a", "/from/.", "/to/"}); err != nil {
			return fmt.Errorf("dockerapi: copying volume contents: %w", err)
		}
		return nil
	}()
	if cloneErr != nil {
		_ = c.RemoveVolume(context.WithoutCancel(ctx), dest, true)
		return cloneErr
	}
	return nil
}
