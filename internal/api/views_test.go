package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/0funct0ry/vessel/internal/dockerapi"
)

func TestContainerToViewIncludesHealth(t *testing.T) {
	v := containerToView(dockerapi.Container{ID: "a", Names: []string{"/web"}, Status: "Up 2 hours (unhealthy)", Health: "unhealthy"})
	if v.Health != "unhealthy" {
		t.Fatalf("health = %q, want unhealthy", v.Health)
	}
}

func TestVersionIncludesServerModeFacts(t *testing.T) {
	router := NewRouter(Config{Docker: newFakeDockerClient(), ReadOnly: true, AllowExec: false, StoreMode: "sqlite", AuthEnabled: true})
	rec := performRequest(router, http.MethodGet, "/api/v1/version")
	if rec.Code != http.StatusOK {
		t.Fatalf("version = %d %s", rec.Code, rec.Body.String())
	}
	var body versionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.AuthMode != "on" || !body.ReadOnly || body.AllowExec || body.StoreMode != "sqlite" {
		t.Fatalf("version body = %#v", body)
	}
}
