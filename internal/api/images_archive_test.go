package api

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/0funct0ry/vessel/internal/dockerapi"
)

type archiveImportStream struct {
	lines []dockerapi.ImportLine
	at    int
}

func (s *archiveImportStream) Next() (dockerapi.ImportLine, error) {
	if s.at >= len(s.lines) {
		return dockerapi.ImportLine{}, io.EOF
	}
	line := s.lines[s.at]
	s.at++
	return line, nil
}
func (*archiveImportStream) Close() error { return nil }

func TestImageExportHeadersAndBody(t *testing.T) {
	fake := newFakeDockerClient()
	fake.export = io.NopCloser(strings.NewReader("tar-data"))
	router := NewRouter(Config{Docker: fake})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/images/export?ref=acme%2Fapi%3A1", nil))
	if got := w.Code; got != http.StatusOK {
		t.Fatalf("status = %d: %s", got, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); got != "application/x-tar" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := w.Header().Get("Content-Disposition"); !strings.HasPrefix(got, "attachment; filename=\"vessel-images-") || !strings.HasSuffix(got, ".tar\"") {
		t.Fatalf("Content-Disposition = %q", got)
	}
	if got := w.Body.String(); got != "tar-data" {
		t.Fatalf("body = %q", got)
	}
}

func TestImageImportMultipartSSE(t *testing.T) {
	fake := newFakeDockerClient()
	fake.importStream = &archiveImportStream{lines: []dockerapi.ImportLine{{Stream: "Loading layer\n"}, {Stream: "Loaded image: acme/api:1\n"}}}
	router := NewRouter(Config{Docker: fake})
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("tar", "images.tar")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("tar-data"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/images/import", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("response = %d %q %s", w.Code, w.Header().Get("Content-Type"), w.Body.String())
	}
	if got := string(fake.importBody); got != "tar-data" {
		t.Fatalf("uploaded body = %q", got)
	}
	if got := w.Body.String(); !strings.Contains(got, "event: import") || !strings.Contains(got, `event: done`) || !strings.Contains(got, `"images":["acme/api:1"]`) {
		t.Fatalf("SSE = %s", got)
	}
}

func TestImageExportFailureUsesEnvelope(t *testing.T) {
	fake := newFakeDockerClient()
	fake.err = dockerapi.ErrUnreachable
	w := httptest.NewRecorder()
	NewRouter(Config{Docker: fake}).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/images/export?ref=acme%2Fapi%3A1", nil))
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "docker_unreachable") {
		t.Fatalf("response = %d %s", w.Code, w.Body.String())
	}
}

func TestImageImportErrorSSE(t *testing.T) {
	fake := newFakeDockerClient()
	fake.importStream = &archiveImportStream{lines: []dockerapi.ImportLine{{Error: "bad archive"}}}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("tar", "images.tar")
	_, _ = part.Write([]byte("tar-data"))
	_ = writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/images/import", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	NewRouter(Config{Docker: fake}).ServeHTTP(w, req)
	if got := w.Body.String(); !strings.Contains(got, "event: error") || !strings.Contains(got, "bad archive") {
		t.Fatalf("SSE = %s", got)
	}
}

var _ context.Context
