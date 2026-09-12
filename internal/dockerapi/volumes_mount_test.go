package dockerapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateVolumeSendsDriverOpts(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1.43/volumes/create" {
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			_, _ = io.WriteString(w, `{"Name":"data","Driver":"nfs"}`)
		}
	}))
	defer srv.Close()
	c, err := New("tcp://" + srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.CreateVolume(context.Background(), CreateVolumeOptions{
		Name: "data", Driver: "nfs",
		DriverOpts: map[string]string{"type": "nfs", "device": ":/export"},
	})
	if err != nil {
		t.Fatal(err)
	}
	opts, _ := gotBody["DriverOpts"].(map[string]any)
	if opts["type"] != "nfs" || opts["device"] != ":/export" {
		t.Fatalf("body=%+v", gotBody)
	}
}

func TestWithVolumeMountBindsReadWriteAndAlwaysRemoves(t *testing.T) {
	var gotBody map[string]any
	var removed bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1.43/containers/create":
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			_, _ = io.WriteString(w, `{"Id":"helper1"}`)
		case r.URL.Path == "/v1.43/containers/helper1/start":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodDelete && r.URL.Path == "/v1.43/containers/helper1":
			removed = true
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	c, err := New("tcp://" + srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	fnErr := errors.New("boom")
	err = c.WithVolumeMount(context.Background(), "data", func(containerID string) error {
		if containerID != "helper1" {
			t.Fatalf("containerID=%s", containerID)
		}
		return fnErr
	})
	if !errors.Is(err, fnErr) {
		t.Fatalf("err=%v", err)
	}
	if !removed {
		t.Fatalf("helper container was not removed after fn error")
	}
	hostConfig, _ := gotBody["HostConfig"].(map[string]any)
	mounts, _ := hostConfig["Mounts"].([]any)
	if len(mounts) != 1 {
		t.Fatalf("mounts=%+v", hostConfig)
	}
	mount := mounts[0].(map[string]any)
	if mount["Target"] != "/vessel-volume" || mount["ReadOnly"] != nil || mount["Source"] != "data" {
		t.Fatalf("mount=%+v, want read-write (no ReadOnly field) so upload/mkdir/delete/rename/edit work", mount)
	}
}

func TestCloneVolumeRemovesDestinationOnFailure(t *testing.T) {
	var createdVolumes []string
	var removedVolumes []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1.43/volumes/source":
			_, _ = io.WriteString(w, `{"Name":"source","Driver":"local"}`)
		case r.URL.Path == "/v1.43/volumes/create":
			var body struct {
				Name string `json:"Name"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			createdVolumes = append(createdVolumes, body.Name)
			_, _ = io.WriteString(w, `{"Name":"`+body.Name+`","Driver":"local"}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/v1.43/volumes/dest":
			removedVolumes = append(removedVolumes, "dest")
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/v1.43/containers/create":
			// Simulate the helper container failing to start, so the
			// exec-based copy never runs and CloneVolume must clean up.
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	c, err := New("tcp://" + srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	err = c.CloneVolume(context.Background(), "source", "dest")
	if err == nil {
		t.Fatal("expected clone error")
	}
	if len(createdVolumes) != 1 || createdVolumes[0] != "dest" {
		t.Fatalf("createdVolumes=%v", createdVolumes)
	}
	if len(removedVolumes) != 1 || removedVolumes[0] != "dest" {
		t.Fatalf("removedVolumes=%v, want destination volume removed after failure", removedVolumes)
	}
}
