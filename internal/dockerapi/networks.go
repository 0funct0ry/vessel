package dockerapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// NetworkContainer is one entry of a network's Containers map.
type NetworkContainer struct {
	Name        string `json:"Name"`
	IPv4Address string `json:"IPv4Address"`
	IPv6Address string `json:"IPv6Address"`
}

// IPAMConfig is one entry of a network's IPAM.Config list.
type IPAMConfig struct {
	Subnet     string            `json:"Subnet,omitempty"`
	Gateway    string            `json:"Gateway,omitempty"`
	IPRange    string            `json:"IPRange,omitempty"`
	AuxAddress map[string]string `json:"AuxiliaryAddresses,omitempty"`
}

// Network is the view of one entry from GET /networks, and of GET
// /networks/{id}.
type Network struct {
	ID         string `json:"Id"`
	Name       string `json:"Name"`
	Driver     string `json:"Driver"`
	Scope      string `json:"Scope"`
	Internal   bool   `json:"Internal"`
	Attachable bool   `json:"Attachable"`
	EnableIPv6 bool   `json:"EnableIPv6"`
	IPAM       struct {
		Driver  string            `json:"Driver"`
		Config  []IPAMConfig      `json:"Config"`
		Options map[string]string `json:"Options"`
	} `json:"IPAM"`
	Options    map[string]string           `json:"Options"`
	Containers map[string]NetworkContainer `json:"Containers"`
	Labels     map[string]string           `json:"Labels"`
	Raw        json.RawMessage             `json:"-"`
}

// ListNetworks calls GET /networks.
func (c *Client) ListNetworks(ctx context.Context) ([]Network, error) {
	resp, err := c.do(ctx, http.MethodGet, "/networks", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var networks []Network
	if err := json.NewDecoder(resp.Body).Decode(&networks); err != nil {
		return nil, fmt.Errorf("dockerapi: decoding /networks response: %w", err)
	}
	return networks, nil
}

// InspectNetwork calls GET /networks/{id}.
func (c *Client) InspectNetwork(ctx context.Context, id string) (*Network, error) {
	resp, err := c.do(ctx, http.MethodGet, "/networks/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("dockerapi: reading network inspect response: %w", err)
	}

	var v Network
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("dockerapi: decoding network inspect response: %w", err)
	}
	v.Raw = raw
	return &v, nil
}
