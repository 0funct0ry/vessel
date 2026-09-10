package dockerapi

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

// FileEntry is one entry returned by ListDirectory.
type FileEntry struct {
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	Type       string    `json:"type"`
	Size       int64     `json:"size"`
	Mode       string    `json:"mode"`
	ModifiedAt time.Time `json:"modified_at"`
}

// ListDirectory runs a short-lived non-interactive exec because Docker has no
// directory-list endpoint. It intentionally depends on exec; archive uploads
// and downloads below do not, so they remain available when exec is disabled.
func (c *Client) ListDirectory(ctx context.Context, container, dir string) ([]FileEntry, error) {
	// --full-time pins the timestamp format the parser expects and is
	// supported by both GNU coreutils and BusyBox's ls (verified against
	// alpine:3.20) — unlike --time-style=full-iso, which BusyBox rejects.
	// Numeric IDs (-n) keep the parser's columns stable across images with
	// different passwd files.
	output, err := c.runExec(ctx, container, []string{"ls", "-lan", "--full-time", "--", dir})
	if err != nil {
		return nil, err
	}
	return ParseDirectoryListing(dir, output)
}

// CreateDirectory makes a directory and missing parents through a one-shot exec.
func (c *Client) CreateDirectory(ctx context.Context, container, dir string) error {
	_, err := c.runExec(ctx, container, []string{"mkdir", "-p", "--", dir})
	return err
}

// RemovePath deletes a file or directory (recursively) through a one-shot exec;
// Docker's archive API has no native delete, so this depends on exec like
// ListDirectory and CreateDirectory above.
func (c *Client) RemovePath(ctx context.Context, container, path string) error {
	_, err := c.runExec(ctx, container, []string{"rm", "-rf", "--", path})
	return err
}

// RenamePath moves a file or directory to a new path through a one-shot exec;
// Docker's archive API has no native rename, so this depends on exec too.
func (c *Client) RenamePath(ctx context.Context, container, from, to string) error {
	_, err := c.runExec(ctx, container, []string{"mv", "--", from, to})
	return err
}

func (c *Client) runExec(ctx context.Context, container string, cmd []string) (string, error) {
	body, err := json.Marshal(struct {
		AttachStdout bool     `json:"AttachStdout"`
		AttachStderr bool     `json:"AttachStderr"`
		Cmd          []string `json:"Cmd"`
		Tty          bool     `json:"Tty"`
	}{true, true, cmd, false})
	if err != nil {
		return "", fmt.Errorf("dockerapi: encoding one-shot exec: %w", err)
	}
	resp, err := c.do(ctx, http.MethodPost, "/containers/"+url.PathEscape(container)+"/exec", body)
	if err != nil {
		return "", err
	}
	var created struct {
		ID string `json:"Id"`
	}
	err = json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	if err != nil {
		return "", fmt.Errorf("dockerapi: decoding one-shot exec: %w", err)
	}
	if created.ID == "" {
		return "", fmt.Errorf("dockerapi: exec create response missing Id")
	}
	start, _ := json.Marshal(struct {
		Detach bool `json:"Detach"`
		Tty    bool `json:"Tty"`
	}{false, false})
	resp, err = c.doStream(ctx, http.MethodPost, "/exec/"+url.PathEscape(created.ID)+"/start", start)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("dockerapi: reading one-shot exec: %w", err)
	}
	return demuxExecOutput(raw)
}

func demuxExecOutput(raw []byte) (string, error) {
	var out bytes.Buffer
	for len(raw) > 0 {
		if len(raw) < frameHeaderSize {
			return "", fmt.Errorf("dockerapi: truncated exec frame")
		}
		n := int(binary.BigEndian.Uint32(raw[4:8]))
		raw = raw[8:]
		if n > len(raw) {
			return "", fmt.Errorf("dockerapi: truncated exec frame payload")
		}
		out.Write(raw[:n])
		raw = raw[n:]
	}
	return out.String(), nil
}

// ParseDirectoryListing parses the stable output from ls -lan --full-time.
func ParseDirectoryListing(dir, output string) ([]FileEntry, error) {
	entries := []FileEntry{}
	for _, line := range strings.Split(strings.TrimSuffix(output, "\n"), "\n") {
		if line == "" || strings.HasPrefix(line, "total ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 8 {
			return nil, fmt.Errorf("dockerapi: parsing directory listing line %q", line)
		}
		size, err := strconv.ParseInt(fields[4], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("dockerapi: parsing file size: %w", err)
		}
		modified, err := time.Parse("2006-01-02 15:04:05 -0700", fields[5]+" "+fields[6]+" "+fields[7])
		if err != nil {
			return nil, fmt.Errorf("dockerapi: parsing file time: %w", err)
		}
		name := strings.Join(fields[8:], " ")
		if fields[0][0] == 'l' {
			name, _, _ = strings.Cut(name, " -> ")
		}
		if name == "." || name == ".." {
			continue
		}
		kind := "file"
		switch fields[0][0] {
		case 'd':
			kind = "dir"
		case 'l':
			kind = "symlink"
		}
		entries = append(entries, FileEntry{Name: name, Path: path.Join(dir, name), Type: kind, Size: size, Mode: fields[0], ModifiedAt: modified})
	}
	return entries, nil
}

// UploadFiles streams a tar archive to Docker's archive endpoint.
func (c *Client) UploadFiles(ctx context.Context, container, dir string, archive io.Reader) error {
	resp, err := c.doStreamReader(ctx, http.MethodPut, "/containers/"+url.PathEscape(container)+"/archive?path="+url.QueryEscape(dir), archive, "application/x-tar")
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// DownloadPath returns Docker's tar archive for a filesystem path.
func (c *Client) DownloadPath(ctx context.Context, container, filePath string) (io.ReadCloser, error) {
	resp, err := c.doStream(ctx, http.MethodGet, "/containers/"+url.PathEscape(container)+"/archive?path="+url.QueryEscape(filePath), nil)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// ReadFile downloads the single-file archive for filePath and extracts its
// content, for the file-browser's view/edit panel. Like DownloadPath, this
// uses the archive endpoint directly and does not depend on exec.
func (c *Client) ReadFile(ctx context.Context, container, filePath string) (name string, data []byte, err error) {
	archive, err := c.DownloadPath(ctx, container, filePath)
	if err != nil {
		return "", nil, err
	}
	defer archive.Close()
	tr := tar.NewReader(archive)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return "", nil, fmt.Errorf("dockerapi: no file found at %s", filePath)
		}
		if err != nil {
			return "", nil, fmt.Errorf("dockerapi: reading file archive: %w", err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		data, err = io.ReadAll(tr)
		if err != nil {
			return "", nil, fmt.Errorf("dockerapi: reading file archive: %w", err)
		}
		return path.Base(header.Name), data, nil
	}
}

// WriteFile overwrites (or creates) a single file at filePath by packing it
// into a one-entry tar and calling UploadFiles against its parent directory —
// the same archive endpoint upload uses, so this does not depend on exec.
func (c *Client) WriteFile(ctx context.Context, container, filePath string, data []byte) error {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{Name: path.Base(filePath), Mode: 0644, Size: int64(len(data))}); err != nil {
		return fmt.Errorf("dockerapi: building file archive: %w", err)
	}
	if _, err := tw.Write(data); err != nil {
		return fmt.Errorf("dockerapi: building file archive: %w", err)
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("dockerapi: building file archive: %w", err)
	}
	return c.UploadFiles(ctx, container, path.Dir(filePath), &buf)
}
