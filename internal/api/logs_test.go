package api

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/0funct0ry/vessel/internal/dockerapi"
)

type fakeLogStream struct {
	lines  []dockerapi.LogLine
	next   int
	closed bool
	wait   <-chan struct{}
}

func (s *fakeLogStream) Next() (dockerapi.LogLine, error) {
	if s.wait != nil {
		<-s.wait
		return dockerapi.LogLine{}, context.Canceled
	}
	if s.next == len(s.lines) {
		return dockerapi.LogLine{}, io.EOF
	}
	line := s.lines[s.next]
	s.next++
	return line, nil
}
func (s *fakeLogStream) Close() error { s.closed = true; return nil }

type blockingSSEWriter struct {
	header  http.Header
	body    bytes.Buffer
	started chan struct{}
	release <-chan struct{}
	once    sync.Once
}

func (w *blockingSSEWriter) Header() http.Header { return w.header }
func (w *blockingSSEWriter) WriteHeader(int)     {}
func (w *blockingSSEWriter) Flush()              {}
func (w *blockingSSEWriter) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.started) })
	<-w.release
	return w.body.Write(p)
}

func TestContainerLogsSSEOptionsAndPayload(t *testing.T) {
	fake := newFakeDockerClient()
	fake.logs = &fakeLogStream{lines: []dockerapi.LogLine{{Time: time.Date(2026, 9, 7, 1, 2, 3, 4, time.UTC), Stream: dockerapi.StreamStderr, Text: "panic"}}}
	response := performRequest(NewRouter(Config{Docker: fake}), http.MethodGet, "/api/v1/containers/c1/logs?follow=true&tail=all&since=2026-09-07T01:00:00%2B01:00&until=42&stream=stderr")
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !bytes.Contains(response.Body.Bytes(), []byte("event: log\ndata: {\"line\":\"panic\",\"stream\":\"stderr\",\"ts\":\"2026-09-07T01:02:03.000000004Z\"}")) {
		t.Fatalf("SSE body=%q", response.Body.String())
	}
	if len(fake.logCalls) != 1 {
		t.Fatalf("LogStream calls=%d", len(fake.logCalls))
	}
	opts := fake.logCalls[0]
	if !opts.Follow || opts.Tail != "all" || opts.Since != "2026-09-07T00:00:00Z" || opts.Until != "42" || opts.Stdout || !opts.Stderr || !opts.Timestamps {
		t.Fatalf("options=%+v", opts)
	}
}

func TestContainerLogsValidation(t *testing.T) {
	router := NewRouter(Config{Docker: newFakeDockerClient()})
	for _, target := range []string{"/api/v1/containers/c/logs?follow=no", "/api/v1/containers/c/logs?tail=-1", "/api/v1/containers/c/logs?tail=nope", "/api/v1/containers/c/logs?since=yesterday", "/api/v1/containers/c/logs?stream=merged"} {
		response := performRequest(router, http.MethodGet, target)
		if response.Code != http.StatusBadRequest || !bytes.Contains(response.Body.Bytes(), []byte(`"invalid_query"`)) {
			t.Errorf("%s: %d %s", target, response.Code, response.Body.String())
		}
	}
}

func TestContainerLogDownload(t *testing.T) {
	fake := newFakeDockerClient()
	fake.container = &dockerapi.ContainerDetail{Name: "/api"}
	fake.logs = &fakeLogStream{lines: []dockerapi.LogLine{{Time: time.Date(2026, 9, 7, 1, 2, 3, 0, time.UTC), Text: "hello"}}}
	router := NewRouter(Config{Docker: fake})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/containers/c1/logs?tail=0", nil)
	request.Header.Set("Accept", "text/plain")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if got := response.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("Content-Type=%q", got)
	}
	if got := response.Header().Get("Content-Disposition"); !strings.Contains(got, "attachment") || !strings.Contains(got, "api-") || !strings.HasSuffix(got, ".log") {
		t.Fatalf("Content-Disposition=%q", got)
	}
	if got := response.Body.String(); got != "hello\n" {
		t.Fatalf("body=%q", got)
	}
	if fake.logCalls[0].Timestamps {
		t.Fatalf("download unexpectedly requested timestamps")
	}
}

func TestContainerLogsCancellationClosesReader(t *testing.T) {
	fake := newFakeDockerClient()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream := &fakeLogStream{wait: ctx.Done()}
	fake.logs = stream
	request := httptest.NewRequest(http.MethodGet, "/api/v1/containers/c1/logs?follow=true", nil).WithContext(ctx)
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { NewRouter(Config{Docker: fake}).ServeHTTP(response, request); close(done) }()
	time.Sleep(10 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stream did not stop")
	}
	deadline := time.Now().Add(time.Second)
	for !stream.closed && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !stream.closed {
		t.Fatal("log reader was not closed")
	}
}

func TestStreamWithOverflowKeepsNewestAndWarning(t *testing.T) {
	started, release, produce := make(chan struct{}), make(chan struct{}), make(chan struct{})
	w := &blockingSSEWriter{header: make(http.Header), started: started, release: release}
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	done := make(chan struct{})
	go func() {
		StreamWithOverflow(c, 2, func(n int) (string, any) { return "log", map[string]any{"line": fmt.Sprintf("dropped %d", n)} }, func(send func(string, any) error) error {
			_ = send("log", map[string]string{"line": "first"})
			<-produce
			_ = send("log", map[string]string{"line": "second"})
			_ = send("log", map[string]string{"line": "third"})
			return send("log", map[string]string{"line": "fourth"})
		})
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("writer did not block")
	}
	close(produce)
	time.Sleep(10 * time.Millisecond)
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stream did not finish")
	}
	body := w.body.String()
	if !strings.Contains(body, `"line":"first"`) || !strings.Contains(body, `"line":"fourth"`) || !strings.Contains(body, `"line":"dropped 2"`) {
		t.Fatalf("body=%q", body)
	}
	if strings.Contains(body, `"line":"second"`) || strings.Contains(body, `"line":"third"`) {
		t.Fatalf("old lines retained: %q", body)
	}
}
