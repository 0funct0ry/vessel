package dockerapi

import (
	"archive/tar"
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
)

const MaxBuildContextBytes int64 = 200 << 20

var (
	ErrInvalidBuildPath     = errors.New("dockerapi: invalid build context path")
	ErrBuildContextTooLarge = errors.New("dockerapi: build context too large")
)

// BuildContextFile is one file to include in a Docker build context tarball.
type BuildContextFile struct {
	Path   string
	Reader io.Reader
	Size   int64
}

// WriteBuildContext writes a Dockerfile and additional files to a build-context
// tarball. Paths are always relative to the tar root and source bytes are
// bounded by limit.
func WriteBuildContext(w io.Writer, dockerfile BuildContextFile, files []BuildContextFile, limit int64) error {
	if limit <= 0 {
		limit = MaxBuildContextBytes
	}
	tw := tar.NewWriter(w)
	remaining := limit
	write := func(name string, r io.Reader, size int64) error {
		if err := validBuildContextPath(name); err != nil {
			return err
		}
		if r == nil || name == "" {
			return ErrInvalidBuildPath
		}
		if size < 0 || size > remaining {
			return ErrBuildContextTooLarge
		}
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: size}); err != nil {
			return fmt.Errorf("dockerapi: writing build context header: %w", err)
		}
		n, err := io.Copy(tw, io.LimitReader(r, size+1))
		if n != size {
			return fmt.Errorf("dockerapi: build context file %q changed while reading", name)
		}
		remaining -= n
		if n > size {
			return ErrBuildContextTooLarge
		}
		if err != nil {
			return fmt.Errorf("dockerapi: writing build context file: %w", err)
		}
		return nil
	}
	if err := write("Dockerfile", dockerfile.Reader, dockerfile.Size); err != nil {
		return err
	}
	for _, file := range files {
		if err := write(file.Path, file.Reader, file.Size); err != nil {
			return err
		}
	}
	return tw.Close()
}

func validBuildContextPath(name string) error {
	if name == "" || strings.HasPrefix(name, "/") || path.IsAbs(name) {
		return ErrInvalidBuildPath
	}
	for _, part := range strings.Split(strings.ReplaceAll(name, "\\", "/"), "/") {
		if part == ".." {
			return ErrInvalidBuildPath
		}
	}
	return nil
}

// ValidateBuildContextPath verifies that a user-supplied context filename is
// safely relative to the context tar root.
func ValidateBuildContextPath(name string) error { return validBuildContextPath(name) }

// BuildLine is one JSON object emitted by Docker while building an image.
type BuildLine struct {
	Stream string `json:"stream,omitempty"`
	Error  string `json:"error,omitempty"`
	Aux    struct {
		ID string `json:"ID,omitempty"`
	} `json:"aux,omitempty"`
}

type BuildStream interface {
	Next() (BuildLine, error)
	Close() error
}

type buildReader struct {
	body io.ReadCloser
	dec  *json.Decoder
}

func (r *buildReader) Next() (BuildLine, error) {
	var line BuildLine
	err := r.dec.Decode(&line)
	return line, err
}
func (r *buildReader) Close() error { return r.body.Close() }

// BuildImage sends a tar build context to Docker and returns its JSON progress stream.
func (c *Client) BuildImage(ctx context.Context, contextTar io.Reader, tags []string) (BuildStream, error) {
	q := url.Values{"dockerfile": {"Dockerfile"}}
	for _, tag := range tags {
		q.Add("t", tag)
	}
	resp, err := c.doStreamReader(ctx, http.MethodPost, "/build?"+q.Encode(), contextTar, "application/x-tar")
	if err != nil {
		return nil, err
	}
	return &buildReader{body: resp.Body, dec: json.NewDecoder(bufio.NewReader(resp.Body))}, nil
}
