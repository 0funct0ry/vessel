package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/0funct0ry/vessel/internal/compose"
	"github.com/0funct0ry/vessel/internal/dockerapi"
	"github.com/0funct0ry/vessel/internal/store"
	"github.com/0funct0ry/vessel/internal/store/memstore"
)

const stackYAML = "services:\n  api:\n    image: alpine:3\n  web:\n    image: nginx:1\n"

func stackRouter(t *testing.T, fake *fakeDockerClient) (http.Handler, store.Store) {
	t.Helper()
	persistence := memstore.New()
	return NewRouter(Config{Docker: fake, Store: persistence}), persistence
}

func seedStack(t *testing.T, persistence store.Store) store.Stack {
	t.Helper()
	now := time.Now().UTC()
	item, err := persistence.CreateStack(context.Background(), store.Stack{ID: "st_1", Name: "acme", ComposeYAML: stackYAML, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	return item
}

func labeled(name, service, state string) dockerapi.Container {
	return dockerapi.Container{ID: name + "-id", Names: []string{"/" + name}, State: state, Labels: map[string]string{compose.LabelProject: "acme", compose.LabelService: service}}
}

func decodeStack(t *testing.T, body string) stackView {
	t.Helper()
	var view stackView
	if err := json.Unmarshal([]byte(body), &view); err != nil {
		t.Fatalf("body is not a stack: %v\n%s", err, body)
	}
	return view
}

func TestStackCreateValidatesAndRejectsDuplicates(t *testing.T) {
	router, _ := stackRouter(t, newFakeDockerClient())

	response := performRequestBody(router, http.MethodPost, "/api/v1/stacks", `{"name":"acme","compose_yaml":`+jsonString(stackYAML)+`,"env_content":"TAG=3\n"}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", response.Code, response.Body.String())
	}
	view := decodeStack(t, response.Body.String())
	if view.Name != "acme" || view.Source != "internal" || view.ServiceCount != 2 || view.Status != compose.StatusNotDeployed {
		t.Fatalf("view = %+v", view)
	}

	response = performRequestBody(router, http.MethodPost, "/api/v1/stacks", `{"name":"acme","compose_yaml":`+jsonString(stackYAML)+`}`)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "already_exists") {
		t.Fatalf("duplicate status=%d body=%s", response.Code, response.Body.String())
	}

	response = performRequestBody(router, http.MethodPost, "/api/v1/stacks", `{"name":"broken","compose_yaml":"services: {}\n"}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid compose status=%d body=%s", response.Code, response.Body.String())
	}

	response = performRequestBody(router, http.MethodPost, "/api/v1/stacks", `{"name":"-nope","compose_yaml":`+jsonString(stackYAML)+`}`)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid_name") {
		t.Fatalf("invalid name status=%d body=%s", response.Code, response.Body.String())
	}
}

// jsonString quotes a Go string as a JSON string literal for the fixtures above.
func jsonString(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func TestStackGraphToAndFromRoundTrip(t *testing.T) {
	router, _ := stackRouter(t, newFakeDockerClient())

	response := performRequestBody(router, http.MethodPost, "/api/v1/stacks/graph/to", `{"compose_yaml":`+jsonString(stackYAML)+`}`)
	if response.Code != http.StatusOK {
		t.Fatalf("graph/to status=%d body=%s", response.Code, response.Body.String())
	}
	var toView stackGraphToView
	if err := json.Unmarshal(response.Body.Bytes(), &toView); err != nil {
		t.Fatal(err)
	}
	if len(toView.Nodes) != 2 {
		t.Fatalf("nodes = %+v", toView.Nodes)
	}

	nodesJSON, err := json.Marshal(toView.Nodes)
	if err != nil {
		t.Fatal(err)
	}
	response = performRequestBody(router, http.MethodPost, "/api/v1/stacks/graph/from", `{"nodes":`+string(nodesJSON)+`,"edges":[]}`)
	if response.Code != http.StatusOK {
		t.Fatalf("graph/from status=%d body=%s", response.Code, response.Body.String())
	}
	var fromView stackGraphFromView
	if err := json.Unmarshal(response.Body.Bytes(), &fromView); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fromView.ComposeYAML, "alpine:3") || !strings.Contains(fromView.ComposeYAML, "nginx:1") {
		t.Fatalf("compose_yaml = %s", fromView.ComposeYAML)
	}

	response = performRequestBody(router, http.MethodPost, "/api/v1/stacks/graph/to", `{"compose_yaml":""}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("empty compose_yaml status=%d body=%s", response.Code, response.Body.String())
	}
}

// TestStackGraphFromPatchesInPlaceGivenPreviousYAML confirms the handler
// wires previous_compose_yaml through to Compose.Patch: an edit that adds
// one field to one service must leave the rest of the file, including an
// unrelated service's untouched formatting, byte-identical.
func TestStackGraphFromPatchesInPlaceGivenPreviousYAML(t *testing.T) {
	router, _ := stackRouter(t, newFakeDockerClient())
	original := "services:\n  api:\n    image: alpine:3\n    environment:\n      PORT: \"8080\"\n  worker:\n    image: alpine:3\n"

	toResponse := performRequestBody(router, http.MethodPost, "/api/v1/stacks/graph/to", `{"compose_yaml":`+jsonString(original)+`}`)
	if toResponse.Code != http.StatusOK {
		t.Fatalf("graph/to status=%d body=%s", toResponse.Code, toResponse.Body.String())
	}
	var toView stackGraphToView
	if err := json.Unmarshal(toResponse.Body.Bytes(), &toView); err != nil {
		t.Fatal(err)
	}

	// Add a dependency edge: worker depends_on api.
	var apiID, workerID string
	for _, n := range toView.Nodes {
		if n.Name == "api" {
			apiID = n.ID
		}
		if n.Name == "worker" {
			workerID = n.ID
		}
	}
	edges := append(toView.Edges, graphEdge{ID: "dependency:api:worker", Kind: "dependency", From: apiID, To: workerID})

	nodesJSON, _ := json.Marshal(toView.Nodes)
	edgesJSON, _ := json.Marshal(edges)
	fromResponse := performRequestBody(router, http.MethodPost, "/api/v1/stacks/graph/from",
		`{"nodes":`+string(nodesJSON)+`,"edges":`+string(edgesJSON)+`,"previous_compose_yaml":`+jsonString(original)+`}`)
	if fromResponse.Code != http.StatusOK {
		t.Fatalf("graph/from status=%d body=%s", fromResponse.Code, fromResponse.Body.String())
	}
	var fromView stackGraphFromView
	if err := json.Unmarshal(fromResponse.Body.Bytes(), &fromView); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(fromView.ComposeYAML, `PORT: "8080"`) {
		t.Fatalf("expected original double-quote style preserved untouched, got:\n%s", fromView.ComposeYAML)
	}
	if !strings.Contains(fromView.ComposeYAML, "depends_on:") || !strings.Contains(fromView.ComposeYAML, "- api") {
		t.Fatalf("expected new depends_on entry, got:\n%s", fromView.ComposeYAML)
	}
}

func TestStackListJoinsLiveContainerStatus(t *testing.T) {
	fake := newFakeDockerClient()
	fake.containers = []dockerapi.Container{labeled("acme-api-1", "api", "running"), labeled("acme-web-1", "web", "exited")}
	router, persistence := stackRouter(t, fake)
	seedStack(t, persistence)

	response := performRequest(router, http.MethodGet, "/api/v1/stacks")
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var views []stackView
	if err := json.Unmarshal(response.Body.Bytes(), &views); err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].Status != compose.StatusPartial || views[0].ContainerCount != 2 {
		t.Fatalf("views = %+v", views)
	}
	if views[0].Services[0].ContainerName != "acme-api-1" || views[0].Services[1].State != "exited" {
		t.Fatalf("services = %+v", views[0].Services)
	}
	if filter := fake.containerCalls[0].Filters["label"]; len(filter) != 1 || filter[0] != compose.LabelProject {
		t.Fatalf("list filter = %v", fake.containerCalls[0].Filters)
	}
}

func TestStackDetailAndNotFound(t *testing.T) {
	fake := newFakeDockerClient()
	fake.containers = []dockerapi.Container{labeled("acme-api-1", "api", "running"), labeled("acme-web-1", "web", "running")}
	router, persistence := stackRouter(t, fake)
	seedStack(t, persistence)

	response := performRequest(router, http.MethodGet, "/api/v1/stacks/acme")
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if view := decodeStack(t, response.Body.String()); view.Status != compose.StatusRunning {
		t.Fatalf("status = %q", view.Status)
	}

	response = performRequest(router, http.MethodGet, "/api/v1/stacks/missing")
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "stack_not_found") {
		t.Fatalf("missing status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestStackUpdateStoresComposeAndEnv(t *testing.T) {
	router, persistence := stackRouter(t, newFakeDockerClient())
	seedStack(t, persistence)

	updated := "services:\n  api:\n    image: alpine:3.20\n"
	response := performRequestBody(router, http.MethodPut, "/api/v1/stacks/acme", `{"compose_yaml":`+jsonString(updated)+`,"env_content":"TAG=3.20\n"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	stored, err := persistence.GetStackByName(context.Background(), "acme")
	if err != nil || stored.ComposeYAML != updated || stored.EnvContent != "TAG=3.20\n" {
		t.Fatalf("stored = %#v, %v", stored, err)
	}
	if !stored.UpdatedAt.After(stored.CreatedAt) {
		t.Fatalf("updated_at was not advanced: %v", stored.UpdatedAt)
	}
}

func TestStackDeleteRefusesARunningStack(t *testing.T) {
	fake := newFakeDockerClient()
	fake.containers = []dockerapi.Container{labeled("acme-api-1", "api", "running")}
	router, persistence := stackRouter(t, fake)
	seedStack(t, persistence)

	response := performRequest(router, http.MethodDelete, "/api/v1/stacks/acme")
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "stack_running") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if _, err := persistence.GetStackByName(context.Background(), "acme"); err != nil {
		t.Fatalf("stack was deleted anyway: %v", err)
	}

	fake.containers = []dockerapi.Container{labeled("acme-api-1", "api", "exited")}
	response = performRequest(router, http.MethodDelete, "/api/v1/stacks/acme")
	if response.Code != http.StatusNoContent {
		t.Fatalf("stopped stack delete status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestStackUpStreamsOneStepPerResource(t *testing.T) {
	fake := newFakeDockerClient()
	fake.createResult = dockerapi.CreateResult{ID: "new"}
	router, persistence := stackRouter(t, fake)
	seedStack(t, persistence)

	response := performRequest(router, http.MethodPost, "/api/v1/stacks/acme/up")
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("content type = %q", got)
	}
	body := response.Body.String()
	for _, want := range []string{`"phase":"created"`, `"phase":"started"`, `"name":"acme-api-1"`, `"name":"acme-web-1"`, `"phase":"done"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("stream missing %s:\n%s", want, body)
		}
	}
}

func TestStackUpReportsSetupFailureAsJSONNotSSE(t *testing.T) {
	router, persistence := stackRouter(t, newFakeDockerClient())
	now := time.Now().UTC()
	if _, err := persistence.CreateStack(context.Background(), store.Stack{ID: "st_bad", Name: "broken", ComposeYAML: "services: {}\n", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	response := performRequest(router, http.MethodPost, "/api/v1/stacks/broken/up")
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid_request") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestStackDownStopsAndRemovesLabeledContainers(t *testing.T) {
	fake := newFakeDockerClient()
	fake.containers = []dockerapi.Container{labeled("acme-api-1", "api", "running")}
	router, persistence := stackRouter(t, fake)
	seedStack(t, persistence)

	response := performRequest(router, http.MethodPost, "/api/v1/stacks/acme/down")
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if len(fake.removedIDs) != 1 || fake.removedIDs[0] != "acme-api-1-id" {
		t.Fatalf("remove calls = %v", fake.removedIDs)
	}
	if !strings.Contains(response.Body.String(), `"phase":"removed"`) {
		t.Fatalf("stream = %s", response.Body.String())
	}
}
