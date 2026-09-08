package dockerapi

import (
	"context"
	"encoding/json"
	"errors"
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

// A daemon that has raised its minimum supported API version past vessel's
// compiled-in default reports the mirror-image error ("too old" rather than
// "too new"); negotiation must handle both.
func TestClient_NegotiatesVersionOnTooOldClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/version":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"Version":    "29.2.0",
				"ApiVersion": "1.53",
			})
		case r.URL.Path == "/v1.43/containers/json":
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"message": "client version 1.43 is too old. Minimum supported API version is 1.44, please upgrade your client to a newer version",
			})
		case r.URL.Path == "/v1.53/containers/json":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode([]Container{{ID: "def456"}})
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
	if len(containers) != 1 || containers[0].ID != "def456" {
		t.Fatalf("containers = %+v, want one container def456", containers)
	}
	if c.APIVersion() != "v1.53" {
		t.Fatalf("APIVersion = %s, want v1.53", c.APIVersion())
	}
}

// Some daemons' /info endpoint, when pinned below their declared minimum API
// version, returns 400 with a body that decodes as a near-empty Info struct
// and no "message" field at all — no text for a regex to match. This must
// still be treated as a version issue and renegotiated.
func TestClient_NegotiatesVersionOnMessagelessInfo400(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/version":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"Version":    "29.2.0",
				"ApiVersion": "1.53",
			})
		case r.URL.Path == "/v1.43/info":
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"ID": "", "Containers": 0})
		case r.URL.Path == "/v1.53/info":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"ID": "real-host-id", "Containers": 3})
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c, err := New("tcp://" + srv.Listener.Addr().String())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	info, err := c.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.ID != "real-host-id" {
		t.Fatalf("Info.ID = %q, want real-host-id", info.ID)
	}
	if c.APIVersion() != "v1.53" {
		t.Fatalf("APIVersion = %s, want v1.53", c.APIVersion())
	}
}

// A genuine validation 400 (with a normal Docker {"message": "..."} body
// that is not version-related) must surface as a real error, not be
// swallowed as a version-negotiation retry.
func TestClient_GenuineBadRequestIsNotTreatedAsVersionMismatch(t *testing.T) {
	var versionCalls int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/version":
			versionCalls++
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"Version":    "27.3.1",
				"ApiVersion": "1.43",
			})
		case r.URL.Path == "/v1.43/containers/json":
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"message": "invalid filter value",
			})
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c, err := New("tcp://" + srv.Listener.Addr().String())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = c.ListContainers(context.Background(), ListContainersOptions{})
	if err == nil {
		t.Fatal("ListContainers: want an error, got nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *APIError", err)
	}
	if apiErr.Status != http.StatusBadRequest || apiErr.Message != "invalid filter value" {
		t.Fatalf("apiErr = %+v, want status 400 message %q", apiErr, "invalid filter value")
	}
	if versionCalls != 0 {
		t.Fatalf("/version called %d times, want 0 (should not have renegotiated)", versionCalls)
	}
}
