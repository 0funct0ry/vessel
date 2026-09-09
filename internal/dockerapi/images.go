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

// ImportLine is one JSON-lines message emitted while Docker loads an image archive.
type ImportLine struct {
	Stream string `json:"stream,omitempty"`
	Error  string `json:"error,omitempty"`
}

// ImportStream reads Docker image-load progress one message at a time.
type ImportStream interface {
	Next() (ImportLine, error)
	Close() error
}

type importReader struct {
	body io.ReadCloser
	dec  *json.Decoder
}

func (r *importReader) Next() (ImportLine, error) {
	var line ImportLine
	if err := r.dec.Decode(&line); err != nil {
		return ImportLine{}, err
	}
	return line, nil
}

func (r *importReader) Close() error { return r.body.Close() }

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

// ExportImages streams Docker's tar archive for one or more image references.
func (c *Client) ExportImages(ctx context.Context, refs []string) (io.ReadCloser, error) {
	q := url.Values{}
	for _, ref := range refs {
		q.Add("names", ref)
	}
	resp, err := c.doStream(ctx, http.MethodGet, "/images/get?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// ImportImages streams an image tar archive directly to Docker and exposes its
// JSON-lines progress response.
func (c *Client) ImportImages(ctx context.Context, tar io.Reader) (ImportStream, error) {
	resp, err := c.doStreamReader(ctx, http.MethodPost, "/images/load?quiet=false", tar, "application/x-tar")
	if err != nil {
		return nil, err
	}
	return &importReader{body: resp.Body, dec: json.NewDecoder(bufio.NewReader(resp.Body))}, nil
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
