package dockerapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDo_MapsStatusCodesToSentinels(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   error
	}{
		{"not found", http.StatusNotFound, ErrNotFound},
		{"conflict", http.StatusConflict, ErrConflict},
		{"not modified", http.StatusNotModified, ErrNotModified},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				if tt.status != http.StatusNotModified {
					_, _ = w.Write([]byte(`{"message":"boom"}`))
				}
			}))
			defer srv.Close()

			c, err := New("tcp://" + srv.Listener.Addr().String())
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			_, err = c.do(context.Background(), http.MethodGet, "/whatever", nil)
			if !errors.Is(err, tt.want) {
				t.Fatalf("do error = %v, want errors.Is(_, %v)", err, tt.want)
			}
		})
	}
}

func TestDo_UnreachableSocket(t *testing.T) {
	c, err := New("unix:///nonexistent/does/not/exist.sock")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = c.do(context.Background(), http.MethodGet, "/whatever", nil)
	if !errors.Is(err, ErrUnreachable) {
		t.Fatalf("do error = %v, want errors.Is(_, ErrUnreachable)", err)
	}
}

func TestDo_GenericAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"internal explosion"}`))
	}))
	defer srv.Close()

	c, err := New("tcp://" + srv.Listener.Addr().String())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = c.do(context.Background(), http.MethodGet, "/whatever", nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("do error = %v, want *APIError", err)
	}
	if apiErr.Status != 500 || apiErr.Message != "internal explosion" {
		t.Fatalf("apiErr = %+v", apiErr)
	}
}
