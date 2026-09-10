package dockerapi

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseDirectoryListing(t *testing.T) {
	entries, err := ParseDirectoryListing("/app", "total 12\ndrwxr-xr-x 2 root root 4096 2026-09-05 08:14:03.000000000 +0000 config\nlrwxrwxrwx 1 root root 7 2026-09-05 08:14:03.000000000 +0000 current link -> config\n-rw-r--r-- 1 root root 412 2026-09-07 09:41:00.000000000 +0000 api env\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 || entries[0].Type != "dir" || entries[1].Type != "symlink" || entries[1].Name != "current link" || entries[2].Path != "/app/api env" || entries[2].Size != 412 {
		t.Fatalf("entries = %#v", entries)
	}
	empty, err := ParseDirectoryListing("/", "total 0\n")
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty = %#v, %v", empty, err)
	}
}

func TestFileArchiveEndpointsAndOneShotExec(t *testing.T) {
	var putBody string
	var command []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1.43/containers/c1/exec":
			var request struct {
				Cmd []string `json:"Cmd"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			command = request.Cmd
			_, _ = io.WriteString(w, `{"Id":"e1"}`)
		case "/v1.43/exec/e1/start":
			frame := make([]byte, 8+8)
			binary.BigEndian.PutUint32(frame[4:8], 8)
			copy(frame[8:], "total 0\n")
			_, _ = w.Write(frame)
		case "/v1.43/containers/c1/archive":
			if r.Method == http.MethodPut {
				b, _ := io.ReadAll(r.Body)
				putBody = string(b)
				if r.URL.Query().Get("path") != "/tmp" {
					t.Errorf("path = %q", r.URL.Query().Get("path"))
				}
				w.WriteHeader(http.StatusOK)
				return
			}
			w.Header().Set("Content-Type", "application/x-tar")
			_, _ = io.WriteString(w, "tar")
		}
	}))
	defer srv.Close()
	c, err := New("tcp://" + strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	if entries, err := c.ListDirectory(context.Background(), "c1", "/tmp"); err != nil || len(entries) != 0 {
		t.Fatalf("list = %#v, %v", entries, err)
	}
	if got, want := strings.Join(command, " "), "ls -lan --full-time -- /tmp"; got != want {
		t.Fatalf("listing command = %q, want %q", got, want)
	}
	if err := c.UploadFiles(context.Background(), "c1", "/tmp", strings.NewReader("archive")); err != nil || putBody != "archive" {
		t.Fatalf("upload = %v, %q", err, putBody)
	}
	archive, err := c.DownloadPath(context.Background(), "c1", "/tmp/a")
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	got, _ := io.ReadAll(archive)
	if string(got) != "tar" {
		t.Fatalf("download = %q", got)
	}
	if err := c.RemovePath(context.Background(), "c1", "/tmp/a"); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(command, " "), "rm -rf -- /tmp/a"; got != want {
		t.Fatalf("remove command = %q, want %q", got, want)
	}
	if err := c.RenamePath(context.Background(), "c1", "/tmp/a", "/tmp/b"); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(command, " "), "mv -- /tmp/a /tmp/b"; got != want {
		t.Fatalf("rename command = %q, want %q", got, want)
	}
}

func TestReadFileExtractsSingleEntry(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{Name: "hello.txt", Mode: 0644, Size: 5}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-tar")
		_, _ = w.Write(buf.Bytes())
	}))
	defer srv.Close()
	c, err := New("tcp://" + strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	name, data, err := c.ReadFile(context.Background(), "c1", "/tmp/hello.txt")
	if err != nil || name != "hello.txt" || string(data) != "hello" {
		t.Fatalf("ReadFile = %q %q %v", name, data, err)
	}
}

func TestReadFileNoRegularEntry(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{Name: "empty", Typeflag: tar.TypeDir, Mode: 0755}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-tar")
		_, _ = w.Write(buf.Bytes())
	}))
	defer srv.Close()
	c, err := New("tcp://" + strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.ReadFile(context.Background(), "c1", "/tmp/empty"); err == nil {
		t.Fatal("expected an error for a directory-only archive")
	}
}

func TestWriteFileBuildsSingleEntryTar(t *testing.T) {
	var putPath string
	var putBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		putPath = r.URL.Query().Get("path")
		putBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c, err := New("tcp://" + strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.WriteFile(context.Background(), "c1", "/tmp/reports/hello.txt", []byte("hi there")); err != nil {
		t.Fatal(err)
	}
	if putPath != "/tmp/reports" {
		t.Fatalf("upload dir = %q", putPath)
	}
	tr := tar.NewReader(bytes.NewReader(putBody))
	header, err := tr.Next()
	if err != nil || header.Name != "hello.txt" || header.Size != 8 {
		t.Fatalf("tar header = %#v %v", header, err)
	}
	data, _ := io.ReadAll(tr)
	if string(data) != "hi there" {
		t.Fatalf("tar content = %q", data)
	}
}
