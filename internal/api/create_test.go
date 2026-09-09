package api

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/0funct0ry/vessel/internal/dockerapi"
)

func TestContainerCreateValidationAndResponse(t *testing.T) {
	fake := newFakeDockerClient()
	fake.networks = []dockerapi.Network{{ID: "bridge", Name: "bridge"}, {ID: "n1", Name: "edge"}}
	fake.createResult = dockerapi.CreateResult{ID: "created", Warnings: []string{"published port already allocated"}}
	router := NewRouter(Config{Docker: fake})
	for _, tc := range []struct{ name, body, code string }{
		{"bad name", `{"name":"not valid","image":"alpine:3"}`, "invalid_name"},
		{"bad image", `{"name":"app","image":"bad reference"}`, "invalid_image_reference"},
		{"unknown network", `{"name":"app","image":"alpine:3","network":"missing"}`, "invalid_request"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := mutationRequest(router, http.MethodPost, "/api/v1/containers", tc.body)
			if got.Code != http.StatusBadRequest || !bytes.Contains(got.Body.Bytes(), []byte(tc.code)) {
				t.Fatalf("status=%d body=%s", got.Code, got.Body.String())
			}
		})
	}
	got := mutationRequest(router, http.MethodPost, "/api/v1/containers", `{"name":"app","image":"alpine:3","network":"n1","ports":[{"container":"8080","host":"18080","protocol":"tcp"}],"start":true}`)
	if got.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", got.Code, got.Body.String())
	}
	assertJSON(t, got.Body.String(), `{"id":"created","name":"app","warnings":["published port already allocated"]}`)
	if fake.createSpec.Image != "alpine:3" || !fake.createSpec.Start || fake.createSpec.Network != "n1" {
		t.Fatalf("spec=%+v", fake.createSpec)
	}
}

func TestContainerCreateStartFailureIsStillCreated(t *testing.T) {
	fake := newFakeDockerClient()
	fake.createResult = dockerapi.CreateResult{ID: "created"}
	fake.createErr = &dockerapi.StartError{Result: fake.createResult, Err: dockerapi.ErrConflict}
	router := NewRouter(Config{Docker: fake})
	got := mutationRequest(router, http.MethodPost, "/api/v1/containers", `{"image":"alpine:3","start":true}`)
	if got.Code != http.StatusCreated || !bytes.Contains(got.Body.Bytes(), []byte(`"start_error"`)) {
		t.Fatalf("status=%d body=%s", got.Code, got.Body.String())
	}
}
