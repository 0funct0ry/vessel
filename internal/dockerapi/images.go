package dockerapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// Image is the summary view of one entry from GET /images/json.
type Image struct {
	ID          string            `json:"Id"`
	RepoTags    []string          `json:"RepoTags"`
	RepoDigests []string          `json:"RepoDigests"`
	Created     int64             `json:"Created"`
	Size        int64             `json:"Size"`
	Labels      map[string]string `json:"Labels"`
}

// ListImages calls GET /images/json.
func (c *Client) ListImages(ctx context.Context, all bool) ([]Image, error) {
	path := "/images/json"
	if all {
		path += "?all=1"
	}

	resp, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var images []Image
	if err := json.NewDecoder(resp.Body).Decode(&images); err != nil {
		return nil, fmt.Errorf("dockerapi: decoding /images/json response: %w", err)
	}
	return images, nil
}

// ImageDetail is the view of GET /images/{name}/json.
type ImageDetail struct {
	ID           string
	RepoTags     []string
	RepoDigests  []string
	Created      string
	Size         int64
	Architecture string
	Os           string
	Env          []string
	Entrypoint   []string
	Cmd          []string
	Labels       map[string]string
	Raw          json.RawMessage
}

type imageInspectResponse struct {
	ID           string   `json:"Id"`
	RepoTags     []string `json:"RepoTags"`
	RepoDigests  []string `json:"RepoDigests"`
	Created      string   `json:"Created"`
	Size         int64    `json:"Size"`
	Architecture string   `json:"Architecture"`
	Os           string   `json:"Os"`
	Config       struct {
		Env        []string          `json:"Env"`
		Entrypoint []string          `json:"Entrypoint"`
		Cmd        []string          `json:"Cmd"`
		Labels     map[string]string `json:"Labels"`
	} `json:"Config"`
}

// HistoryLayer is one entry from GET /images/{name}/history.
type HistoryLayer struct {
	ID        string   `json:"Id"`
	Created   int64    `json:"Created"`
	CreatedBy string   `json:"CreatedBy"`
	Size      int64    `json:"Size"`
	Comment   string   `json:"Comment"`
	Tags      []string `json:"Tags"`
}

// History calls GET /images/{name}/history, returning the image's layers
// ordered newest-first, as Docker returns them.
func (c *Client) History(ctx context.Context, name string) ([]HistoryLayer, error) {
	resp, err := c.do(ctx, http.MethodGet, "/images/"+url.PathEscape(name)+"/history", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var layers []HistoryLayer
	if err := json.NewDecoder(resp.Body).Decode(&layers); err != nil {
		return nil, fmt.Errorf("dockerapi: decoding /images/history response: %w", err)
	}
	return layers, nil
}

// InspectImage calls GET /images/{name}/json.
func (c *Client) InspectImage(ctx context.Context, name string) (*ImageDetail, error) {
	resp, err := c.do(ctx, http.MethodGet, "/images/"+url.PathEscape(name)+"/json", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("dockerapi: reading image inspect response: %w", err)
	}

	var v imageInspectResponse
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("dockerapi: decoding image inspect response: %w", err)
	}

	return &ImageDetail{
		ID:           v.ID,
		RepoTags:     v.RepoTags,
		RepoDigests:  v.RepoDigests,
		Created:      v.Created,
		Size:         v.Size,
		Architecture: v.Architecture,
		Os:           v.Os,
		Env:          v.Config.Env,
		Entrypoint:   v.Config.Entrypoint,
		Cmd:          v.Config.Cmd,
		Labels:       v.Config.Labels,
		Raw:          raw,
	}, nil
}
