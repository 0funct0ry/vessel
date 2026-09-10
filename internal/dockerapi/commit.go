package dockerapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// CommitOptions controls Docker's snapshot of a container filesystem as an image.
type CommitOptions struct {
	Repo    string
	Tag     string
	Comment string
	Pause   bool
}

// CommitResult is Docker's identifier for the newly created image.
type CommitResult struct {
	ImageID string
}

// CommitContainer calls Docker's POST /commit endpoint. The container is a
// query parameter in the Engine API; Vessel's own public route remains
// POST /containers/{id}/commit.
func (c *Client) CommitContainer(ctx context.Context, id string, opts CommitOptions) (CommitResult, error) {
	q := url.Values{"container": {id}, "repo": {opts.Repo}, "pause": {fmt.Sprintf("%t", opts.Pause)}}
	if opts.Tag != "" {
		q.Set("tag", opts.Tag)
	}
	if opts.Comment != "" {
		q.Set("comment", opts.Comment)
	}
	resp, err := c.do(ctx, http.MethodPost, "/commit?"+q.Encode(), nil)
	if err != nil {
		return CommitResult{}, err
	}
	defer resp.Body.Close()
	var wire struct {
		ID string `json:"Id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wire); err != nil {
		return CommitResult{}, fmt.Errorf("dockerapi: decoding container commit response: %w", err)
	}
	return CommitResult{ImageID: wire.ID}, nil
}
