package dockerapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_NegotiatesVersionOnTooNewClient(t *testing.T) {
	var calls int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/version":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"Version":    "24.0.0",
				"ApiVersion": "1.41",
			})
		case r.URL.Path == "/v1.43/containers/json":
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"message": "client version 1.43 is too new. Maximum supported API version is 1.41",
			})
		case r.URL.Path == "/v1.41/containers/json":
			calls++
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode([]Container{{ID: "abc123"}})
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c, err := New("tcp://" + srv.Listener.Addr().String())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	containers, err := c.ListContainers(context.Background(), ListContainersOptions{})
	if err != nil {
		t.Fatalf("ListContainers: %v", err)
	}
	if len(containers) != 1 || containers[0].ID != "abc123" {
		t.Fatalf("containers = %+v, want one container abc123", containers)
	}
	if c.APIVersion() != "v1.41" {
		t.Fatalf("APIVersion = %s, want v1.41", c.APIVersion())
	}
	if calls != 1 {
		t.Fatalf("negotiated endpoint called %d times, want 1", calls)
	}
}
