package api

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/0funct0ry/vessel/internal/dockerapi"
)

func TestContainerCommitValidationAndPauseDefault(t *testing.T) {
	fake := newFakeDockerClient()
	fake.commitResult = dockerapi.CommitResult{ImageID: "sha256:new"}
	router := NewRouter(Config{Docker: fake})
	for _, body := range []string{`{"repo":"bad repo"}`, `{"repo":"repo","tag":"bad tag"}`} {
		got := mutationRequest(router, http.MethodPost, "/api/v1/containers/c1/commit", body)
		if got.Code != http.StatusBadRequest || !bytes.Contains(got.Body.Bytes(), []byte(`"invalid_image_reference"`)) {
			t.Fatalf("invalid commit status=%d body=%s", got.Code, got.Body.String())
		}
	}
	got := mutationRequest(router, http.MethodPost, "/api/v1/containers/c1/commit", `{"repo":"acme/api","tag":"snapshot"}`)
	if got.Code != http.StatusCreated {
		t.Fatalf("default pause status=%d body=%s", got.Code, got.Body.String())
	}
	if !fake.commitOptions.Pause || fake.commitOptions.Repo != "acme/api" || fake.commitOptions.Tag != "snapshot" {
		t.Fatalf("default options=%+v", fake.commitOptions)
	}
	assertJSON(t, got.Body.String(), `{"image_id":"sha256:new"}`)
	got = mutationRequest(router, http.MethodPost, "/api/v1/containers/c1/commit", `{"repo":"acme/api","pause":false}`)
	if got.Code != http.StatusCreated || fake.commitOptions.Pause {
		t.Fatalf("explicit false status=%d options=%+v", got.Code, fake.commitOptions)
	}
}

func TestContainerTopPassesDockerArraysThrough(t *testing.T) {
	fake := newFakeDockerClient()
	fake.top = &dockerapi.TopEntry{Titles: []string{"UID", "PID", "CMD"}, Processes: [][]string{{"root", "1", "/app"}, {"nobody", "7", "worker"}}}
	got := performRequest(NewRouter(Config{Docker: fake}), http.MethodGet, "/api/v1/containers/c1/top")
	if got.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", got.Code, got.Body.String())
	}
	assertJSON(t, got.Body.String(), `{"titles":["UID","PID","CMD"],"processes":[["root","1","/app"],["nobody","7","worker"]]}`)
}
