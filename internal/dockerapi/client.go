// Package dockerapi is a hand-written client for the Docker Engine API. It
// talks to the daemon directly over net/http (unix socket or TCP) — the
// docker/docker SDK is deliberately not used, per SPEC.md §4.
package dockerapi

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// defaultAPIVersion is the version prefix used until negotiation (triggered
// by a 400 "client version too new" response) lowers it.
const defaultAPIVersion = "v1.43"

// TLSConfig carries the --docker-tls-* flag values for a tcps:// host.
type TLSConfig struct {
	CACert     string
	Cert       string
	Key        string
	SkipVerify bool
}

// Client is a Docker Engine API client bound to one host.
type Client struct {
	httpClient *http.Client
	baseURL    string // e.g. "http://docker" or "https://1.2.3.4:2376"
	dial       func(context.Context) (net.Conn, error)

	mu         sync.RWMutex
	apiVersion string
}

// Option configures a Client.
type Option func(*clientOptions)

type clientOptions struct {
	tls     *TLSConfig
	timeout time.Duration
}

// WithTLS configures TLS for a tcps:// host.
func WithTLS(cfg TLSConfig) Option {
	return func(o *clientOptions) { o.tls = &cfg }
}

// WithTimeout overrides the default per-request timeout used for non-streaming calls.
func WithTimeout(d time.Duration) Option {
	return func(o *clientOptions) { o.timeout = d }
}

// New builds a Client for host, which must be a unix://, tcp:// or tcps://
// URL. For unix hosts, requests dial the socket directly and are addressed
// to "http://docker"; for tcp/tcps hosts, requests go straight to the
// daemon's network address.
func New(host string, opts ...Option) (*Client, error) {
	o := clientOptions{timeout: 30 * time.Second}
	for _, opt := range opts {
		opt(&o)
	}

	switch {
	case strings.HasPrefix(host, "unix://"):
		sockPath := strings.TrimPrefix(host, "unix://")
		transport := &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				d := net.Dialer{}
				return d.DialContext(ctx, "unix", sockPath)
			},
		}
		return &Client{
			httpClient: &http.Client{Transport: transport, Timeout: o.timeout},
			baseURL:    "http://docker",
			dial: func(ctx context.Context) (net.Conn, error) {
				d := net.Dialer{}
				return d.DialContext(ctx, "unix", sockPath)
			},
			apiVersion: defaultAPIVersion,
		}, nil

	case strings.HasPrefix(host, "tcp://"):
		addr := strings.TrimPrefix(host, "tcp://")
		dial := func(ctx context.Context) (net.Conn, error) {
			d := net.Dialer{}
			return d.DialContext(ctx, "tcp", addr)
		}
		return &Client{
			httpClient: &http.Client{Timeout: o.timeout},
			baseURL:    "http://" + addr,
			dial:       dial,
			apiVersion: defaultAPIVersion,
		}, nil

	case strings.HasPrefix(host, "tcps://"):
		addr := strings.TrimPrefix(host, "tcps://")
		tlsConf, err := buildTLSConfig(o.tls)
		if err != nil {
			return nil, err
		}
		transport := &http.Transport{TLSClientConfig: tlsConf}
		dial := func(ctx context.Context) (net.Conn, error) {
			d := tls.Dialer{Config: tlsConf}
			return d.DialContext(ctx, "tcp", addr)
		}
		return &Client{
			httpClient: &http.Client{Transport: transport, Timeout: o.timeout},
			baseURL:    "https://" + addr,
			dial:       dial,
			apiVersion: defaultAPIVersion,
		}, nil

	default:
		return nil, fmt.Errorf("dockerapi: unsupported host scheme %q: want unix://, tcp:// or tcps://", host)
	}
}

func buildTLSConfig(cfg *TLSConfig) (*tls.Config, error) {
	tlsConf := &tls.Config{MinVersion: tls.VersionTLS12}
	if cfg == nil {
		return tlsConf, nil
	}

	tlsConf.InsecureSkipVerify = cfg.SkipVerify

	if cfg.Cert != "" && cfg.Key != "" {
		cert, err := tls.LoadX509KeyPair(cfg.Cert, cfg.Key)
		if err != nil {
			return nil, fmt.Errorf("dockerapi: loading client cert/key: %w", err)
		}
		tlsConf.Certificates = []tls.Certificate{cert}
	}

	if cfg.CACert != "" {
		pem, err := os.ReadFile(cfg.CACert)
		if err != nil {
			return nil, fmt.Errorf("dockerapi: reading CA cert: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("dockerapi: no certificates found in %s", cfg.CACert)
		}
		tlsConf.RootCAs = pool
	}

	return tlsConf, nil
}

// APIVersion returns the API version prefix currently in use, e.g. "v1.43".
// It may change after negotiation triggered by a request.
func (c *Client) APIVersion() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.apiVersion
}

func (c *Client) setAPIVersion(v string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.apiVersion = v
}

func (c *Client) url(path string) string {
	c.mu.RLock()
	v := c.apiVersion
	c.mu.RUnlock()
	return c.baseURL + "/" + v + path
}
