package compose

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/0funct0ry/vessel/internal/dockerapi"
)

const twoServices = `
services:
  api:
    image: alpine:3
    networks: [back]
    volumes:
      - data:/var/lib/data
    ports:
      - "8080:80"
  web:
    image: nginx:1
networks:
  back:
    driver: bridge
volumes:
  data:
`

// engineFixture wires a real dockerapi.Client at a fake Engine, the same
// httptest harness internal/dockerapi's own tests use.
type engineFixture struct {
	created         []map[string]any
	names           []string
	started         []string
	stopped         []string
	removed         []string
	networks        []string
	volumes         []string
	rmVolumes       []string
	listed          []string
	existing        []dockerapi.Container
	failOn          string   // service name whose container/create must fail
	missingImageOn  string   // service name whose first create 404s as "no such image"
	pulled          []string // images PullImage was asked to fetch
	createAttempts  map[string]int
	conflictNetwork bool // /networks/create 409s as "already exists"
	conflictVolume  bool // /volumes/create 409s as "already exists"
	rmNetworks      []string
}

func newEngine(t *testing.T, fixture *engineFixture) (*dockerapi.Client, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/v1.43")
		switch {
		case path == "/containers/json":
			fixture.listed = append(fixture.listed, r.URL.Query().Get("filters"))
			_ = json.NewEncoder(w).Encode(fixture.existing)
		case path == "/networks/create":
			var body struct{ Name string }
			_ = json.NewDecoder(r.Body).Decode(&body)
			if fixture.conflictNetwork {
				w.WriteHeader(http.StatusConflict)
				_, _ = io.WriteString(w, `{"message":"network with name `+body.Name+` already exists"}`)
				return
			}
			fixture.networks = append(fixture.networks, body.Name)
			_, _ = io.WriteString(w, `{"Id":"n1","Name":"`+body.Name+`"}`)
		case path == "/volumes/create":
			var body struct{ Name string }
			_ = json.NewDecoder(r.Body).Decode(&body)
			if fixture.conflictVolume {
				w.WriteHeader(http.StatusConflict)
				_, _ = io.WriteString(w, `{"message":"volume `+body.Name+` already exists"}`)
				return
			}
			fixture.volumes = append(fixture.volumes, body.Name)
			_, _ = io.WriteString(w, `{"Name":"`+body.Name+`"}`)
		case path == "/containers/create":
			name := r.URL.Query().Get("name")
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if fixture.failOn != "" && strings.Contains(name, "-"+fixture.failOn+"-") {
				w.WriteHeader(http.StatusConflict)
				_, _ = io.WriteString(w, `{"message":"Conflict. The container name is already in use"}`)
				return
			}
			if fixture.missingImageOn != "" && strings.Contains(name, "-"+fixture.missingImageOn+"-") {
				if fixture.createAttempts == nil {
					fixture.createAttempts = map[string]int{}
				}
				fixture.createAttempts[name]++
				if fixture.createAttempts[name] == 1 {
					w.WriteHeader(http.StatusNotFound)
					_, _ = io.WriteString(w, `{"message":"No such image: `+body["Image"].(string)+`"}`)
					return
				}
			}
			fixture.names = append(fixture.names, name)
			fixture.created = append(fixture.created, body)
			_, _ = io.WriteString(w, `{"Id":"`+name+`-id"}`)
		case path == "/images/create":
			fixture.pulled = append(fixture.pulled, r.URL.Query().Get("fromImage"))
			_, _ = io.WriteString(w, `{"status":"Pull complete"}`)
		case strings.HasSuffix(path, "/start"):
			fixture.started = append(fixture.started, strings.Split(strings.TrimPrefix(path, "/containers/"), "/")[0])
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(path, "/stop"):
			fixture.stopped = append(fixture.stopped, strings.Split(strings.TrimPrefix(path, "/containers/"), "/")[0])
			w.WriteHeader(http.StatusNoContent)
		case strings.HasPrefix(path, "/volumes/") && r.Method == http.MethodDelete:
			fixture.rmVolumes = append(fixture.rmVolumes, strings.TrimPrefix(path, "/volumes/"))
			w.WriteHeader(http.StatusNoContent)
		case strings.HasPrefix(path, "/networks/") && r.Method == http.MethodDelete:
			fixture.rmNetworks = append(fixture.rmNetworks, strings.TrimPrefix(path, "/networks/"))
			w.WriteHeader(http.StatusNoContent)
		case strings.HasPrefix(path, "/containers/") && r.Method == http.MethodDelete:
			fixture.removed = append(fixture.removed, strings.TrimPrefix(path, "/containers/")+"?"+r.URL.RawQuery)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	client, err := dockerapi.New("tcp://" + srv.Listener.Addr().String())
	if err != nil {
		srv.Close()
		t.Fatal(err)
	}
	return client, srv.Close
}

func drain(t *testing.T, call func() (<-chan Event, error)) []Event {
	t.Helper()
	events, err := call()
	if err != nil {
		t.Fatal(err)
	}
	out := []Event{}
	for event := range events {
		out = append(out, event)
	}
	return out
}

func phaseOf(events []Event, kind, name string) []string {
	out := []string{}
	for _, event := range events {
		if event.Kind == kind && event.Name == name {
			out = append(out, event.Phase)
		}
	}
	return out
}

func TestUpCreatesNetworksVolumesAndLabeledContainers(t *testing.T) {
	fixture := &engineFixture{}
	client, done := newEngine(t, fixture)
	defer done()

	events := drain(t, func() (<-chan Event, error) {
		return Up(context.Background(), client, Stack{Name: "acme", ComposeYAML: twoServices})
	})

	if !reflect.DeepEqual(fixture.networks, []string{"acme_back"}) {
		t.Fatalf("networks = %v", fixture.networks)
	}
	if !reflect.DeepEqual(fixture.volumes, []string{"acme_data"}) {
		t.Fatalf("volumes = %v", fixture.volumes)
	}
	if !reflect.DeepEqual(fixture.names, []string{"acme-api-1", "acme-web-1"}) {
		t.Fatalf("container names = %v", fixture.names)
	}
	if !reflect.DeepEqual(fixture.started, []string{"acme-api-1-id", "acme-web-1-id"}) {
		t.Fatalf("started = %v", fixture.started)
	}
	if got := fixture.listed[0]; !strings.Contains(got, LabelProject+"=acme") {
		t.Fatalf("list filter = %s", got)
	}

	labels, _ := fixture.created[0]["Labels"].(map[string]any)
	if !reflect.DeepEqual(labels, map[string]any{LabelProject: "acme", LabelService: "api"}) {
		t.Fatalf("api labels = %#v", labels)
	}
	labels, _ = fixture.created[1]["Labels"].(map[string]any)
	if !reflect.DeepEqual(labels, map[string]any{LabelProject: "acme", LabelService: "web"}) {
		t.Fatalf("web labels = %#v", labels)
	}

	host, _ := fixture.created[0]["HostConfig"].(map[string]any)
	if host["NetworkMode"] != "acme_back" {
		t.Fatalf("network mode = %#v", host["NetworkMode"])
	}
	mounts, _ := host["Mounts"].([]any)
	first, _ := mounts[0].(map[string]any)
	if len(mounts) != 1 || first["Source"] != "acme_data" || first["Target"] != "/var/lib/data" {
		t.Fatalf("mounts = %#v", mounts)
	}
	bindings, _ := host["PortBindings"].(map[string]any)
	if _, ok := bindings["80/tcp"]; !ok {
		t.Fatalf("port bindings = %#v", bindings)
	}

	// A service must be reachable by its own short name — real compose's own
	// behavior, and what every service's env config (DB_HOST=db, etc.)
	// assumes. Without a network alias, Docker's embedded DNS only resolves a
	// container by its (project-prefixed) container name.
	netConfig, _ := fixture.created[0]["NetworkingConfig"].(map[string]any)
	endpoints, _ := netConfig["EndpointsConfig"].(map[string]any)
	acmeBack, _ := endpoints["acme_back"].(map[string]any)
	aliases, _ := acmeBack["Aliases"].([]any)
	if len(aliases) != 1 || aliases[0] != "api" {
		t.Fatalf("network alias for api service = %#v", acmeBack)
	}

	last := events[len(events)-1]
	if last.Kind != "stack" || last.Phase != "done" || last.Detail != "2 of 2 services running" {
		t.Fatalf("summary = %+v", last)
	}
}

func TestUpContinuesAfterAFailedServiceAndReportsPriorSuccesses(t *testing.T) {
	fixture := &engineFixture{failOn: "api"}
	client, done := newEngine(t, fixture)
	defer done()

	events := drain(t, func() (<-chan Event, error) {
		return Up(context.Background(), client, Stack{Name: "acme", ComposeYAML: twoServices})
	})

	if !reflect.DeepEqual(fixture.names, []string{"acme-web-1"}) {
		t.Fatalf("web should still be created: %v", fixture.names)
	}
	if got := phaseOf(events, "container", "acme-api-1"); !reflect.DeepEqual(got, []string{"creating", "error"}) {
		t.Fatalf("api phases = %v", got)
	}
	if got := phaseOf(events, "container", "acme-web-1"); !reflect.DeepEqual(got, []string{"creating", "created", "starting", "started"}) {
		t.Fatalf("web phases = %v", got)
	}
	last := events[len(events)-1]
	if last.Phase != "error" || !strings.Contains(last.Detail, "1 of 2 services running") || !strings.Contains(last.Detail, "failed: api") {
		t.Fatalf("summary = %+v", last)
	}
	var failure Event
	for _, event := range events {
		if event.Phase == "error" && event.Kind == "container" {
			failure = event
		}
	}
	if !strings.Contains(failure.Detail, "already in use") {
		t.Fatalf("raw daemon error not forwarded: %q", failure.Detail)
	}
}

func TestUpSkipsServicesThatAreAlreadyDeployed(t *testing.T) {
	fixture := &engineFixture{existing: []dockerapi.Container{{ID: "c1", Names: []string{"/acme-api-1"}, State: "running", Labels: map[string]string{LabelProject: "acme", LabelService: "api"}}}}
	client, done := newEngine(t, fixture)
	defer done()

	events := drain(t, func() (<-chan Event, error) {
		return Up(context.Background(), client, Stack{Name: "acme", ComposeYAML: twoServices})
	})
	if !reflect.DeepEqual(fixture.names, []string{"acme-web-1"}) {
		t.Fatalf("created = %v", fixture.names)
	}
	if got := phaseOf(events, "container", "acme-api-1"); !reflect.DeepEqual(got, []string{"exists"}) {
		t.Fatalf("api phases = %v", got)
	}
}

func TestUpPullsAMissingImageAndRetriesCreate(t *testing.T) {
	fixture := &engineFixture{missingImageOn: "api"}
	client, done := newEngine(t, fixture)
	defer done()

	events := drain(t, func() (<-chan Event, error) {
		return Up(context.Background(), client, Stack{Name: "acme", ComposeYAML: twoServices})
	})

	if !reflect.DeepEqual(fixture.pulled, []string{"alpine:3"}) {
		t.Fatalf("pulled = %v, want the missing service's image pulled once", fixture.pulled)
	}
	if !reflect.DeepEqual(fixture.names, []string{"acme-api-1", "acme-web-1"}) {
		t.Fatalf("both services should end up created: %v", fixture.names)
	}
	if got := phaseOf(events, "image", "alpine:3"); !reflect.DeepEqual(got, []string{"pulling", "pulled"}) {
		t.Fatalf("image phases = %v", got)
	}
	if got := phaseOf(events, "container", "acme-api-1"); !reflect.DeepEqual(got, []string{"creating", "creating", "created", "starting", "started"}) {
		t.Fatalf("api phases = %v", got)
	}
	last := events[len(events)-1]
	if last.Phase != "done" || last.Detail != "2 of 2 services running" {
		t.Fatalf("summary = %+v", last)
	}
}

func TestDownStopsRemovesAndOptionallyDropsNamedVolumes(t *testing.T) {
	existing := []dockerapi.Container{
		{ID: "c1", Names: []string{"/acme-api-1"}, State: "running", Labels: map[string]string{LabelProject: "acme", LabelService: "api"}},
		{ID: "c2", Names: []string{"/acme-web-1"}, State: "exited", Labels: map[string]string{LabelProject: "acme", LabelService: "web"}},
	}

	fixture := &engineFixture{existing: existing}
	client, done := newEngine(t, fixture)
	drain(t, func() (<-chan Event, error) {
		return Down(context.Background(), client, Stack{Name: "acme", ComposeYAML: twoServices}, false)
	})
	done()
	if !reflect.DeepEqual(fixture.stopped, []string{"c1", "c2"}) || !reflect.DeepEqual(fixture.removed, []string{"c1?force=1", "c2?force=1"}) {
		t.Fatalf("stopped=%v removed=%v", fixture.stopped, fixture.removed)
	}
	if len(fixture.rmVolumes) != 0 {
		t.Fatalf("volumes removed without the flag: %v", fixture.rmVolumes)
	}
	// Networks are removed regardless of removeVolumes, matching `docker
	// compose down`'s default — only named volumes need the extra flag.
	if !reflect.DeepEqual(fixture.rmNetworks, []string{"acme_back"}) {
		t.Fatalf("removed networks = %v", fixture.rmNetworks)
	}

	fixture = &engineFixture{existing: existing}
	client, done = newEngine(t, fixture)
	defer done()
	events := drain(t, func() (<-chan Event, error) {
		return Down(context.Background(), client, Stack{Name: "acme", ComposeYAML: twoServices}, true)
	})
	if !reflect.DeepEqual(fixture.rmVolumes, []string{"acme_data"}) {
		t.Fatalf("removed volumes = %v", fixture.rmVolumes)
	}
	if !reflect.DeepEqual(fixture.rmNetworks, []string{"acme_back"}) {
		t.Fatalf("removed networks = %v", fixture.rmNetworks)
	}
	last := events[len(events)-1]
	if last.Kind != "stack" || last.Detail != "removed 2 container(s)" {
		t.Fatalf("summary = %+v", last)
	}
}

// TestUpTreatsAnAlreadyExistingNetworkOrVolumeAsExistsNotError covers the
// gitea-repro bug: a network/volume left over from an earlier partial Up (or
// a redeploy that hasn't gone through Down yet) must not fail the run — it's
// still the resource this stack wants.
func TestUpTreatsAnAlreadyExistingNetworkOrVolumeAsExistsNotError(t *testing.T) {
	fixture := &engineFixture{conflictNetwork: true, conflictVolume: true}
	client, done := newEngine(t, fixture)
	defer done()

	events := drain(t, func() (<-chan Event, error) {
		return Up(context.Background(), client, Stack{Name: "acme", ComposeYAML: twoServices})
	})

	if got := phaseOf(events, "network", "acme_back"); !reflect.DeepEqual(got, []string{"creating", "exists"}) {
		t.Fatalf("network phases = %v", got)
	}
	if got := phaseOf(events, "volume", "acme_data"); !reflect.DeepEqual(got, []string{"creating", "exists"}) {
		t.Fatalf("volume phases = %v", got)
	}
	if !reflect.DeepEqual(fixture.names, []string{"acme-api-1", "acme-web-1"}) {
		t.Fatalf("containers should still deploy: %v", fixture.names)
	}
	last := events[len(events)-1]
	if last.Phase != "done" || last.Detail != "2 of 2 services running" {
		t.Fatalf("a pre-existing network/volume must not fail the run: %+v", last)
	}
}

func TestRedeployRemovesThenRecreatesOverOneStream(t *testing.T) {
	fixture := &engineFixture{existing: []dockerapi.Container{{ID: "c1", Names: []string{"/acme-api-1"}, State: "running", Labels: map[string]string{LabelProject: "acme", LabelService: "api"}}}}
	client, done := newEngine(t, fixture)
	defer done()

	events := drain(t, func() (<-chan Event, error) {
		return Redeploy(context.Background(), client, Stack{Name: "acme", ComposeYAML: twoServices})
	})
	if !reflect.DeepEqual(fixture.removed, []string{"c1?force=1"}) {
		t.Fatalf("removed = %v", fixture.removed)
	}
	// Up re-lists, and the fake still reports c1, so api is seen as deployed;
	// what matters is that exactly one summary event closes the stream.
	summaries := 0
	for _, event := range events {
		if event.Kind == "stack" {
			summaries++
		}
	}
	if summaries != 1 || events[len(events)-1].Kind != "stack" {
		t.Fatalf("summaries = %d, last = %+v", summaries, events[len(events)-1])
	}
}

func TestStatusCountsRunningAgainstDeclaredServices(t *testing.T) {
	cases := []struct {
		name     string
		existing []dockerapi.Container
		want     string
	}{
		{"none", nil, StatusNotDeployed},
		{"all running", []dockerapi.Container{{State: "running"}, {State: "running"}}, StatusRunning},
		{"one stopped", []dockerapi.Container{{State: "running"}, {State: "exited"}}, StatusPartial},
		{"all stopped", []dockerapi.Container{{State: "exited"}, {State: "exited"}}, StatusStopped},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := &engineFixture{existing: tc.existing}
			client, done := newEngine(t, fixture)
			defer done()
			got, err := Status(context.Background(), client, "acme", 2)
			if err != nil || got != tc.want {
				t.Fatalf("status = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestUpRejectsAnUnparseableStackBeforeTouchingDocker(t *testing.T) {
	fixture := &engineFixture{}
	client, done := newEngine(t, fixture)
	defer done()
	if _, err := Up(context.Background(), client, Stack{Name: "acme", ComposeYAML: "services: {}\n"}); err == nil {
		t.Fatal("expected a parse error")
	}
	if len(fixture.listed) != 0 {
		t.Fatalf("Docker was called: %v", fixture.listed)
	}
}
