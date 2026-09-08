package dockerapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
)

// ExecOptions describes the command Docker should attach to a container.
type ExecOptions struct {
	Cmd  []string
	User string
	TTY  bool
}

// ExecSession is the bidirectional, hijacked connection returned by Docker
// after starting an exec. Closing it closes stdin and the attached stream.
type ExecSession interface {
	io.Reader
	io.Writer
	io.Closer
}

type execSession struct {
	net.Conn
	reader  *bufio.Reader
	mux     bool
	pending []byte
}

func (s *execSession) Read(p []byte) (int, error) {
	if !s.mux {
		return s.reader.Read(p)
	}
	for len(s.pending) == 0 {
		var header [8]byte
		if _, err := io.ReadFull(s.reader, header[:]); err != nil {
			return 0, err
		}
		s.pending = make([]byte, binary.BigEndian.Uint32(header[4:8]))
		if _, err := io.ReadFull(s.reader, s.pending); err != nil {
			return 0, fmt.Errorf("dockerapi: reading exec frame payload: %w", err)
		}
	}
	n := copy(p, s.pending)
	s.pending = s.pending[n:]
	return n, nil
}

// CreateExec creates an interactive exec and returns Docker's exec ID.
func (c *Client) CreateExec(ctx context.Context, container string, opts ExecOptions) (string, error) {
	body, err := json.Marshal(struct {
		AttachStdin  bool     `json:"AttachStdin"`
		AttachStdout bool     `json:"AttachStdout"`
		AttachStderr bool     `json:"AttachStderr"`
		Cmd          []string `json:"Cmd"`
		User         string   `json:"User,omitempty"`
		Tty          bool     `json:"Tty"`
	}{true, true, true, opts.Cmd, opts.User, opts.TTY})
	if err != nil {
		return "", fmt.Errorf("dockerapi: encoding exec create: %w", err)
	}
	resp, err := c.do(ctx, http.MethodPost, "/containers/"+url.PathEscape(container)+"/exec", body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var result struct {
		ID string `json:"Id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("dockerapi: decoding exec create: %w", err)
	}
	if result.ID == "" {
		return "", fmt.Errorf("dockerapi: exec create response missing Id")
	}
	return result.ID, nil
}

// StartExec opens Docker's hijacked attach stream for an exec.
func (c *Client) StartExec(ctx context.Context, id string, tty bool) (ExecSession, error) {
	conn, err := c.dial(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	closeOnError := true
	defer func() {
		if closeOnError {
			_ = conn.Close()
		}
	}()
	body, err := json.Marshal(struct {
		Detach bool `json:"Detach"`
		Tty    bool `json:"Tty"`
	}{false, tty})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url("/exec/"+url.PathEscape(id)+"/start"), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("dockerapi: building exec start: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "tcp")
	if err := req.Write(conn); err != nil {
		return nil, fmt.Errorf("%w: writing exec start: %v", ErrUnreachable, err)
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, req)
	if err != nil {
		return nil, fmt.Errorf("%w: reading exec start: %v", ErrUnreachable, err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
		return nil, mapError(resp)
	}
	closeOnError = false
	return &execSession{Conn: conn, reader: br, mux: !tty}, nil
}

// ResizeExec changes the terminal size for a TTY exec.
func (c *Client) ResizeExec(ctx context.Context, id string, cols, rows int) error {
	q := url.Values{"w": {fmt.Sprint(cols)}, "h": {fmt.Sprint(rows)}}
	resp, err := c.do(ctx, http.MethodPost, "/exec/"+url.PathEscape(id)+"/resize?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}
