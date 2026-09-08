package dockerapi

import (
	"bufio"
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

func TestExecSessionDemultiplexesNonTTYOutput(t *testing.T) {
	frame := make([]byte, 8+5)
	frame[0] = byte(StreamStdout)
	binary.BigEndian.PutUint32(frame[4:8], 5)
	copy(frame[8:], "hello")
	s := &execSession{reader: bufio.NewReader(bytes.NewReader(frame)), mux: true}
	buf := make([]byte, 2)
	var got []byte
	for len(got) < 5 {
		n, err := s.Read(buf)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, buf[:n]...)
	}
	if string(got) != "hello" {
		t.Fatalf("output=%q", got)
	}
}

func TestExecCreateStartAndResize(t *testing.T) {
	var created struct {
		Cmd  []string `json:"Cmd"`
		User string   `json:"User"`
		TTY  bool     `json:"Tty"`
	}
	var resized bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1.43/containers/c1/exec":
			if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
				t.Error(err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"Id":"e1"}`)
		case r.URL.Path == "/v1.43/exec/e1/resize":
			resized = r.URL.Query().Get("w") == "120" && r.URL.Query().Get("h") == "34"
			w.WriteHeader(http.StatusCreated)
		case r.URL.Path == "/v1.43/exec/e1/start":
			_, _ = io.Copy(io.Discard, r.Body)
			h, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("response writer cannot hijack")
			}
			conn, rw, err := h.Hijack()
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\nready")
			_ = rw.Flush()
			buf := make([]byte, 4)
			_, _ = conn.Read(buf)
			if string(buf) != "ping" {
				t.Errorf("stdin=%q", buf)
			}
		}
	}))
	defer srv.Close()
	c, err := New("tcp://" + strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	id, err := c.CreateExec(context.Background(), "c1", ExecOptions{Cmd: []string{"/bin/sh"}, User: "root", TTY: true})
	if err != nil {
		t.Fatal(err)
	}
	if id != "e1" || !created.TTY || created.User != "root" || len(created.Cmd) != 1 {
		t.Fatalf("created=%+v id=%q", created, id)
	}
	session, err := c.StartExec(context.Background(), id, true)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	buf := make([]byte, 5)
	if _, err := io.ReadFull(session, buf); err != nil || string(buf) != "ready" {
		t.Fatalf("output=%q err=%v", buf, err)
	}
	if _, err := session.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	if err := c.ResizeExec(context.Background(), id, 120, 34); err != nil {
		t.Fatal(err)
	}
	if !resized {
		t.Fatal("resize query not received")
	}
}
