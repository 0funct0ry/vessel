package dockerapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRecreateContainerCallSequence(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1.43/containers/old/json":
			calls = append(calls, "inspect")
			_, _ = io.WriteString(w, `{"Id":"old","Name":"/app","State":{"Status":"running"}}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/v1.43/containers/old":
			calls = append(calls, "remove")
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/v1.43/containers/create":
			calls = append(calls, "create")
			if r.URL.RawQuery != "name=app" {
				t.Fatalf("create query = %s, want name=app", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `{"Id":"new"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1.43/containers/new/start":
			calls = append(calls, "start")
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	c, err := New("tcp://" + srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	result, err := c.RecreateContainer(context.Background(), "old", Spec{Image: "alpine:3"})
	if err != nil {
		t.Fatalf("RecreateContainer error: %v", err)
	}
	if result.ID != "new" {
		t.Fatalf("result=%+v", result)
	}
	want := []string{"inspect", "remove", "create", "start"}
	if len(calls) != len(want) {
		t.Fatalf("calls=%v want=%v", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("calls=%v want=%v", calls, want)
		}
	}
}

func TestRecreateContainerKeepsStoppedContainerStopped(t *testing.T) {
	started := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1.43/containers/old/json":
			_, _ = io.WriteString(w, `{"Id":"old","Name":"/app","State":{"Status":"exited"}}`)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/v1.43/containers/create":
			_, _ = io.WriteString(w, `{"Id":"new"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1.43/containers/new/start":
			started = true
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	c, _ := New("tcp://" + srv.Listener.Addr().String())

	if _, err := c.RecreateContainer(context.Background(), "old", Spec{Image: "alpine:3"}); err != nil {
		t.Fatalf("RecreateContainer error: %v", err)
	}
	if started {
		t.Fatal("a stopped container should not be started after recreate")
	}
}

func TestRecreateContainerCreateFailureReturnsRecreateFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1.43/containers/old/json":
			_, _ = io.WriteString(w, `{"Id":"old","Name":"/app","State":{"Status":"running"}}`)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/v1.43/containers/create":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `{"message":"boom"}`)
		}
	}))
	defer srv.Close()
	c, _ := New("tcp://" + srv.Listener.Addr().String())

	_, err := c.RecreateContainer(context.Background(), "old", Spec{Image: "alpine:3"})
	recreateErr, ok := err.(*RecreateFailed)
	if !ok {
		t.Fatalf("err=%T %v, want *RecreateFailed", err, err)
	}
	if recreateErr.FreedName != "app" {
		t.Fatalf("FreedName=%q, want app", recreateErr.FreedName)
	}
}
