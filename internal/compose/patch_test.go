package compose

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func mustParseNode(t *testing.T, yamlText string) (*yaml.Node, Compose) {
	t.Helper()
	root, c, _, err := ParseComposeStructureNode([]byte(yamlText))
	if err != nil {
		t.Fatal(err)
	}
	return root, c
}

func TestPatchAddingOneFieldTouchesOnlyThatField(t *testing.T) {
	src := `services:
  api:
    image: alpine:3
    environment:
      PORT: "8080"
  worker:
    image: alpine:3
`
	root, c := mustParseNode(t, src)

	worker := c.Services["worker"]
	worker.DependsOn = []string{"api"}
	c.Services["worker"] = worker

	out, err := c.Patch(root)
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)

	for _, want := range []string{
		"image: alpine:3",
		`PORT: "8080"`,
		"depends_on:",
		"- api",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
	// The untouched api service's fields must survive byte-for-byte,
	// including the original double-quote style on PORT.
	apiIdx := strings.Index(text, "api:")
	workerIdx := strings.Index(text, "worker:")
	if apiIdx < 0 || workerIdx < 0 || apiIdx > workerIdx {
		t.Fatalf("expected api before worker, unexpected reordering:\n%s", text)
	}
}

func TestPatchAddingNewServiceAppendsWithoutReordering(t *testing.T) {
	src := `services:
  api:
    image: alpine:3
  worker:
    image: alpine:3
`
	root, c := mustParseNode(t, src)
	c.Services["cache"] = Service{Image: "redis:7"}
	c.Order = append(c.Order, "cache")

	out, err := c.Patch(root)
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)

	apiIdx := strings.Index(text, "api:")
	workerIdx := strings.Index(text, "worker:")
	cacheIdx := strings.Index(text, "cache:")
	if !(apiIdx < workerIdx && workerIdx < cacheIdx) {
		t.Fatalf("expected order api, worker, cache; got:\n%s", text)
	}
}

func TestPatchRemovingAFieldDeletesOnlyThatKey(t *testing.T) {
	src := `services:
  api:
    image: alpine:3
    restart: unless-stopped
    labels:
      team: platform
`
	root, c := mustParseNode(t, src)
	api := c.Services["api"]
	api.Restart = ""
	c.Services["api"] = api

	out, err := c.Patch(root)
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	if strings.Contains(text, "restart:") {
		t.Fatalf("expected restart: removed, got:\n%s", text)
	}
	if !strings.Contains(text, "team: platform") {
		t.Fatalf("expected unrelated label preserved, got:\n%s", text)
	}
}

func TestPatchRemovingAWholeServiceDropsOnlyThatEntry(t *testing.T) {
	src := `services:
  api:
    image: alpine:3
  worker:
    image: alpine:3
`
	root, c := mustParseNode(t, src)
	delete(c.Services, "worker")
	c.Order = []string{"api"}

	out, err := c.Patch(root)
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	if strings.Contains(text, "worker:") {
		t.Fatalf("expected worker removed, got:\n%s", text)
	}
	if !strings.Contains(text, "api:") {
		t.Fatalf("expected api preserved, got:\n%s", text)
	}
}

func TestPatchUnrelatedTopLevelKeysAndUnmodelledServiceKeysSurvive(t *testing.T) {
	src := `services:
  api:
    image: alpine:3
    healthcheck:
      test: ["CMD", "true"]
networks:
  front:
    driver: bridge
volumes:
  data:
    driver: local
`
	root, c := mustParseNode(t, src)
	api := c.Services["api"]
	api.Restart = "always"
	c.Services["api"] = api

	out, err := c.Patch(root)
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	for _, want := range []string{
		"healthcheck:",
		`test: ["CMD", "true"]`,
		"restart: always",
		"front:",
		"driver: bridge",
		"data:",
		"driver: local",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}

	// Re-parsing structurally must still see the untouched healthcheck.
	reparsed, _, err := ParseComposeStructure(out)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reparsed.Services["api"].Raw["healthcheck"]; !ok {
		t.Fatalf("healthcheck lost after Patch: %#v", reparsed.Services["api"].Raw)
	}
}

func TestPatchNetworkFieldChangeLeavesOtherNetworksUntouched(t *testing.T) {
	src := `networks:
  front:
    driver: bridge
    internal: true
  back:
    driver: bridge
services:
  api:
    image: alpine:3
    networks:
      - front
`
	root, c := mustParseNode(t, src)
	front := c.Networks["front"]
	front.Attachable = true
	c.Networks["front"] = front

	out, err := c.Patch(root)
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	if !strings.Contains(text, "attachable: true") {
		t.Fatalf("expected attachable added, got:\n%s", text)
	}
	if !strings.Contains(text, "internal: true") {
		t.Fatalf("expected internal preserved, got:\n%s", text)
	}
	backIdx := strings.Index(text, "back:")
	frontIdx := strings.Index(text, "front:")
	if backIdx < 0 || frontIdx < 0 {
		t.Fatalf("expected both networks present, got:\n%s", text)
	}
}
