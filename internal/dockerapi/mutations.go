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

// Lifecycle performs a Docker container lifecycle action.
func (c *Client) Lifecycle(ctx context.Context, id, action string, query url.Values) error {
	path := "/containers/" + url.PathEscape(id) + "/" + action
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	resp, err := c.do(ctx, http.MethodPost, path, nil)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

func (c *Client) RenameContainer(ctx context.Context, id, name string) error {
	return c.Lifecycle(ctx, id, "rename", url.Values{"name": {name}})
}

type RemoveContainerOptions struct{ Force, Volumes bool }

func (c *Client) RemoveContainer(ctx context.Context, id string, opts RemoveContainerOptions) error {
	q := url.Values{}
	if opts.Force {
		q.Set("force", "1")
	}
	if opts.Volumes {
		q.Set("v", "1")
	}
	path := "/containers/" + url.PathEscape(id)
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}
	resp, err := c.do(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// PullProgress is one JSON object emitted by Docker's image pull endpoint.
type PullProgress struct {
	ID             string `json:"id,omitempty"`
	Status         string `json:"status,omitempty"`
	ProgressDetail struct {
		Current int64 `json:"current,omitempty"`
		Total   int64 `json:"total,omitempty"`
	} `json:"progressDetail,omitempty"`
	Error string `json:"error,omitempty"`
}
type PullReader struct {
	body io.ReadCloser
	dec  *json.Decoder
}

// PullStream is the pull progress stream exposed to API consumers.
type PullStream interface {
	Next() (PullProgress, error)
	Close() error
}

func (r *PullReader) Close() error { return r.body.Close() }
func (r *PullReader) Next() (PullProgress, error) {
	var p PullProgress
	if err := r.dec.Decode(&p); err != nil {
		return PullProgress{}, err
	}
	return p, nil
}
func (c *Client) PullImage(ctx context.Context, reference string) (PullStream, error) {
	q := url.Values{"fromImage": {reference}}
	resp, err := c.doStream(ctx, http.MethodPost, "/images/create?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	return &PullReader{body: resp.Body, dec: json.NewDecoder(bufio.NewReader(resp.Body))}, nil
}

func (c *Client) TagImage(ctx context.Context, id, repo, tag string) error {
	q := url.Values{"repo": {repo}}
	if tag != "" {
		q.Set("tag", tag)
	}
	resp, err := c.do(ctx, http.MethodPost, "/images/"+url.PathEscape(id)+"/tag?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

type RemoveImageOptions struct{ Force, NoPrune bool }

func (c *Client) RemoveImage(ctx context.Context, id string, opts RemoveImageOptions) error {
	q := url.Values{}
	if opts.Force {
		q.Set("force", "1")
	}
	if opts.NoPrune {
		q.Set("noprune", "1")
	}
	path := "/images/" + url.PathEscape(id)
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}
	resp, err := c.do(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

type CreateVolumeOptions struct {
	Name, Driver string
	DriverOpts   map[string]string
	Labels       map[string]string
}

func (c *Client) CreateVolume(ctx context.Context, opts CreateVolumeOptions) (*Volume, error) {
	body, err := json.Marshal(struct {
		Name       string            `json:"Name"`
		Driver     string            `json:"Driver,omitempty"`
		DriverOpts map[string]string `json:"DriverOpts,omitempty"`
		Labels     map[string]string `json:"Labels,omitempty"`
	}{opts.Name, opts.Driver, opts.DriverOpts, opts.Labels})
	if err != nil {
		return nil, fmt.Errorf("dockerapi: encoding volume: %w", err)
	}
	resp, err := c.do(ctx, http.MethodPost, "/volumes/create", body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var v Volume
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, fmt.Errorf("dockerapi: decoding created volume: %w", err)
	}
	return &v, nil
}
func (c *Client) RemoveVolume(ctx context.Context, name string, force bool) error {
	q := url.Values{}
	if force {
		q.Set("force", "1")
	}
	path := "/volumes/" + url.PathEscape(name)
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}
	resp, err := c.do(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

type CreateNetworkOptions struct {
	Name, Driver, Subnet, Gateway string
	Labels                        map[string]string
}

func (c *Client) CreateNetwork(ctx context.Context, opts CreateNetworkOptions) (*Network, error) {
	type ipam struct {
		Config []IPAMConfig `json:"Config,omitempty"`
	}
	request := struct {
		Name   string            `json:"Name"`
		Driver string            `json:"Driver,omitempty"`
		IPAM   ipam              `json:"IPAM,omitempty"`
		Labels map[string]string `json:"Labels,omitempty"`
	}{Name: opts.Name, Driver: opts.Driver, Labels: opts.Labels}
	if opts.Subnet != "" || opts.Gateway != "" {
		request.IPAM = ipam{Config: []IPAMConfig{{Subnet: opts.Subnet, Gateway: opts.Gateway}}}
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("dockerapi: encoding network: %w", err)
	}
	resp, err := c.do(ctx, http.MethodPost, "/networks/create", body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var created struct {
		ID      string `json:"Id"`
		Warning string `json:"Warning"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return nil, fmt.Errorf("dockerapi: decoding created network: %w", err)
	}
	return &Network{ID: created.ID, Name: opts.Name, Driver: opts.Driver, Labels: opts.Labels}, nil
}
func (c *Client) RemoveNetwork(ctx context.Context, id string) error {
	resp, err := c.do(ctx, http.MethodDelete, "/networks/"+url.PathEscape(id), nil)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}
func (c *Client) NetworkConnect(ctx context.Context, id, container string, disconnect bool) error {
	body, err := json.Marshal(struct {
		Container string `json:"Container"`
	}{container})
	if err != nil {
		return err
	}
	action := "connect"
	if disconnect {
		action = "disconnect"
	}
	resp, err := c.do(ctx, http.MethodPost, "/networks/"+url.PathEscape(id)+"/"+action, body)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// PruneReport retains Docker's returned deletion list and reclaimed byte count.
type PruneReport struct {
	Deleted        []string        `json:"deleted"`
	SpaceReclaimed int64           `json:"space_reclaimed"`
	Raw            json.RawMessage `json:"-"`
}

func (c *Client) Prune(ctx context.Context, kind string) (*PruneReport, error) {
	resp, err := c.do(ctx, http.MethodPost, "/"+url.PathEscape(kind)+"/prune", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var wire struct {
		Deleted  []string `json:"ContainersDeleted"`
		Images   []string `json:"ImagesDeleted"`
		Volumes  []string `json:"VolumesDeleted"`
		Networks []string `json:"NetworksDeleted"`
		Caches   []string `json:"CachesDeleted"`
		Space    int64    `json:"SpaceReclaimed"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nil, fmt.Errorf("dockerapi: decoding prune response: %w", err)
	}
	deleted := wire.Deleted
	for _, values := range [][]string{wire.Images, wire.Volumes, wire.Networks, wire.Caches} {
		deleted = append(deleted, values...)
	}
	return &PruneReport{Deleted: deleted, SpaceReclaimed: wire.Space, Raw: raw}, nil
}
