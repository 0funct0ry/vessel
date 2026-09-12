package api

import (
	"testing"

	"github.com/0funct0ry/vessel/internal/dockerapi"
)

func TestContainerToViewIncludesHealth(t *testing.T) {
	v := containerToView(dockerapi.Container{ID: "a", Names: []string{"/web"}, Status: "Up 2 hours (unhealthy)", Health: "unhealthy"})
	if v.Health != "unhealthy" {
		t.Fatalf("health = %q, want unhealthy", v.Health)
	}
}
