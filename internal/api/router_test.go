package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/0funct0ry/vessel/internal/dockerapi"
)

type fakeDockerClient struct {
	info           *dockerapi.Info
	version        *dockerapi.VersionInfo
	disk           *dockerapi.DiskUsageInfo
	containers     []dockerapi.Container
	container      *dockerapi.ContainerDetail
	top            *dockerapi.TopEntry
	images         []dockerapi.Image
	image          *dockerapi.ImageDetail
	volumes        []dockerapi.Volume
	volume         *dockerapi.Volume
	networks       []dockerapi.Network
	network        *dockerapi.Network
	err            error
	panicList      bool
	containerCalls []dockerapi.ListContainersOptions
	pull           dockerapi.PullStream
	logs           dockerapi.LogStream
	logCalls       []dockerapi.LogsOptions
	prune          *dockerapi.PruneReport
	stats          dockerapi.Stats
	statsStream    dockerapi.StatsStream
	statsCalls     []string
}

func newFakeDockerClient() *fakeDockerClient {
	return &fakeDockerClient{
		info:      &dockerapi.Info{},
		version:   &dockerapi.VersionInfo{},
		disk:      &dockerapi.DiskUsageInfo{},
		container: &dockerapi.ContainerDetail{},
		top:       &dockerapi.TopEntry{},
		image:     &dockerapi.ImageDetail{},
		volume:    &dockerapi.Volume{},
		network:   &dockerapi.Network{},
	}
}

func (f *fakeDockerClient) Info(context.Context) (*dockerapi.Info, error) {
	return f.info, f.err
}

func (f *fakeDockerClient) Version(context.Context) (*dockerapi.VersionInfo, error) {
	return f.version, f.err
}

func (f *fakeDockerClient) DiskUsage(context.Context) (*dockerapi.DiskUsageInfo, error) {
	return f.disk, f.err
}

func (f *fakeDockerClient) ListContainers(_ context.Context, opts dockerapi.ListContainersOptions) ([]dockerapi.Container, error) {
	if f.panicList {
		panic("test panic")
	}
	f.containerCalls = append(f.containerCalls, opts)
	return f.containers, f.err
}

func (f *fakeDockerClient) InspectContainer(context.Context, string) (*dockerapi.ContainerDetail, error) {
	return f.container, f.err
}

func (f *fakeDockerClient) LogStream(_ context.Context, _ string, opts dockerapi.LogsOptions) (dockerapi.LogStream, error) {
	f.logCalls = append(f.logCalls, opts)
	return f.logs, f.err
}

func (f *fakeDockerClient) StatsStream(_ context.Context, id string) (dockerapi.StatsStream, error) {
	f.statsCalls = append(f.statsCalls, id)
	return f.statsStream, f.err
}

func (f *fakeDockerClient) Stats(context.Context, string) (dockerapi.Stats, error) {
	return f.stats, f.err
}

func (f *fakeDockerClient) Top(context.Context, string, string) (*dockerapi.TopEntry, error) {
	return f.top, f.err
}

func (f *fakeDockerClient) ListImages(context.Context, bool) ([]dockerapi.Image, error) {
	return f.images, f.err
}

func (f *fakeDockerClient) InspectImage(context.Context, string) (*dockerapi.ImageDetail, error) {
	return f.image, f.err
}

func (f *fakeDockerClient) ListVolumes(context.Context) ([]dockerapi.Volume, error) {
	return f.volumes, f.err
}

func (f *fakeDockerClient) InspectVolume(context.Context, string) (*dockerapi.Volume, error) {
	return f.volume, f.err
}

func (f *fakeDockerClient) ListNetworks(context.Context) ([]dockerapi.Network, error) {
	return f.networks, f.err
}

func (f *fakeDockerClient) InspectNetwork(context.Context, string) (*dockerapi.Network, error) {
	return f.network, f.err
}

func (f *fakeDockerClient) Lifecycle(context.Context, string, string, url.Values) error { return f.err }
func (f *fakeDockerClient) RenameContainer(context.Context, string, string) error       { return f.err }
func (f *fakeDockerClient) RemoveContainer(context.Context, string, dockerapi.RemoveContainerOptions) error {
	return f.err
}
func (f *fakeDockerClient) PullImage(context.Context, string) (dockerapi.PullStream, error) {
	return f.pull, f.err
}
func (f *fakeDockerClient) TagImage(context.Context, string, string, string) error { return f.err }
func (f *fakeDockerClient) RemoveImage(context.Context, string, dockerapi.RemoveImageOptions) error {
	return f.err
}
func (f *fakeDockerClient) CreateVolume(context.Context, dockerapi.CreateVolumeOptions) (*dockerapi.Volume, error) {
	return f.volume, f.err
}
func (f *fakeDockerClient) RemoveVolume(context.Context, string, bool) error { return f.err }
func (f *fakeDockerClient) CreateNetwork(context.Context, dockerapi.CreateNetworkOptions) (*dockerapi.Network, error) {
	return f.network, f.err
}
func (f *fakeDockerClient) RemoveNetwork(context.Context, string) error                { return f.err }
func (f *fakeDockerClient) NetworkConnect(context.Context, string, string, bool) error { return f.err }
func (f *fakeDockerClient) Prune(context.Context, string) (*dockerapi.PruneReport, error) {
	return f.prune, f.err
}

func performRequest(router http.Handler, method, target string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, nil)
	router.ServeHTTP(recorder, request)
	return recorder
}

func assertJSON(t *testing.T, got, want string) {
	t.Helper()
	var gotValue any
	if err := json.Unmarshal([]byte(got), &gotValue); err != nil {
		t.Fatalf("response is not JSON: %v\n%s", err, got)
	}
	var wantValue any
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("invalid expected JSON: %v", err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("JSON mismatch\n got: %s\nwant: %s", got, want)
	}
}

func TestContainerListExactJSON(t *testing.T) {
	fake := newFakeDockerClient()
	fake.containers = []dockerapi.Container{{
		ID: "abc123", Names: []string{"/api"}, Image: "acme/api:1", ImageID: "sha256:image",
		Command: "/app", Created: 42, State: "running", Status: "Up 2 minutes",
		Ports:  []dockerapi.Port{{IP: "127.0.0.1", PrivatePort: 8080, PublicPort: 7373, Type: "tcp"}},
		Labels: map[string]string{"env": "test"},
	}}
	router := NewRouter(Config{Docker: fake, Logger: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))})

	response := performRequest(router, http.MethodGet, "/api/v1/containers?all=true")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	assertJSON(t, response.Body.String(), `[{"id":"abc123","name":"api","image":"acme/api:1","image_id":"sha256:image","command":"/app","created":42,"state":"running","status":"Up 2 minutes","ports":[{"ip":"127.0.0.1","private_port":8080,"public_port":7373,"type":"tcp"}],"labels":{"env":"test"}}]`)
	if len(fake.containerCalls) != 1 || !fake.containerCalls[0].All {
		t.Fatalf("ListContainers calls = %+v, want one all=true call", fake.containerCalls)
	}
}

func TestImageListEnrichmentExactJSON(t *testing.T) {
	fake := newFakeDockerClient()
	fake.images = []dockerapi.Image{
		{ID: "sha256:used", RepoTags: []string{"acme/api:1"}, RepoDigests: nil, Created: 100, Size: 512, Labels: nil},
		{ID: "sha256:loose", RepoTags: []string{"<none>:<none>"}, Created: 90, Size: 256},
	}
	fake.containers = []dockerapi.Container{{ID: "c1", ImageID: "sha256:used", Image: "acme/api:1"}}
	router := NewRouter(Config{Docker: fake})

	response := performRequest(router, http.MethodGet, "/api/v1/images")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	assertJSON(t, response.Body.String(), `[
		{"id":"sha256:used","repo_tags":["acme/api:1"],"repo_digests":[],"created":100,"size":512,"labels":{},"used_by_count":1,"dangling":false},
		{"id":"sha256:loose","repo_tags":["<none>:<none>"],"repo_digests":[],"created":90,"size":256,"labels":{},"used_by_count":0,"dangling":true}
	]`)
	if len(fake.containerCalls) != 1 || !fake.containerCalls[0].All {
		t.Fatalf("enrichment did not request all containers: %+v", fake.containerCalls)
	}
}

func TestVolumeListJoinExactJSON(t *testing.T) {
	fake := newFakeDockerClient()
	fake.volumes = []dockerapi.Volume{{
		Name: "data", Driver: "local", Mountpoint: "/var/lib/docker/volumes/data/_data",
		CreatedAt: "2026-09-07T00:00:00Z", Labels: map[string]string{"tier": "db"}, Scope: "local",
	}}
	fake.containers = []dockerapi.Container{{
		ID: "c1", Names: []string{"/postgres"}, Mounts: []dockerapi.ContainerMount{{
			Type: "volume", Name: "data", Destination: "/var/lib/postgresql/data", RW: true,
		}},
	}}
	router := NewRouter(Config{Docker: fake})

	response := performRequest(router, http.MethodGet, "/api/v1/volumes")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	assertJSON(t, response.Body.String(), `[{"name":"data","driver":"local","mountpoint":"/var/lib/docker/volumes/data/_data","created_at":"2026-09-07T00:00:00Z","labels":{"tier":"db"},"scope":"local","used_by":[{"container_id":"c1","container_name":"postgres","mount_path":"/var/lib/postgresql/data","rw":true}]}]`)
}

func TestNetworkDetailAndContainerDetailJSON(t *testing.T) {
	fake := newFakeDockerClient()
	network := dockerapi.Network{ID: "n1", Name: "edge", Driver: "bridge", Scope: "local", Labels: nil, Raw: json.RawMessage(`{"Id":"n1"}`)}
	network.IPAM.Config = []dockerapi.IPAMConfig{{Subnet: "172.20.0.0/16", Gateway: "172.20.0.1"}}
	network.Containers = map[string]dockerapi.NetworkContainer{
		"c2": {Name: "worker", IPv4Address: "172.20.0.3/16"},
		"c1": {Name: "api", IPv4Address: "172.20.0.2/16", IPv6Address: "fd00::2/64"},
	}
	fake.network = &network
	fake.container = &dockerapi.ContainerDetail{
		ID: "c1", Name: "/api", Image: "acme/api:1", Command: nil, Created: "now", State: "running", Status: "running",
		Mounts: nil, Networks: nil, Env: nil, Labels: nil, Raw: json.RawMessage(`{"Id":"c1"}`),
	}
	router := NewRouter(Config{Docker: fake})

	response := performRequest(router, http.MethodGet, "/api/v1/networks/n1")
	assertJSON(t, response.Body.String(), `{"id":"n1","name":"edge","driver":"bridge","scope":"local","ipam":[{"subnet":"172.20.0.0/16","gateway":"172.20.0.1"}],"labels":{},"containers":[{"container_id":"c1","container_name":"api","ipv4_address":"172.20.0.2/16","ipv6_address":"fd00::2/64"},{"container_id":"c2","container_name":"worker","ipv4_address":"172.20.0.3/16","ipv6_address":""}],"raw":{"Id":"n1"}}`)

	response = performRequest(router, http.MethodGet, "/api/v1/containers/c1")
	assertJSON(t, response.Body.String(), `{"id":"c1","name":"api","image":"acme/api:1","command":[],"created":"now","state":"running","status":"running","exit_code":0,"health":"","restart_policy":"","mounts":[],"networks":{},"env":[],"labels":{},"raw":{"Id":"c1"}}`)
}

func TestHostAndTopJSON(t *testing.T) {
	fake := newFakeDockerClient()
	fake.info = &dockerapi.Info{
		ID: "host1", Containers: 4, ContainersRunning: 2, ContainersPaused: 1, ContainersStopped: 1,
		Images: 7, OperatingSystem: "Linux", OSType: "linux", Architecture: "arm64", NCPU: 8,
		MemTotal: 16 << 30, ServerVersion: "27.1",
	}
	fake.version = &dockerapi.VersionInfo{Version: "27.1", APIVersion: "1.47", MinAPI: "1.24", Os: "linux", Arch: "arm64", KernelVer: "6.8"}
	fake.disk = &dockerapi.DiskUsageInfo{Images: []dockerapi.DiskImage{{Size: 900}, {Size: 20}}, Containers: []dockerapi.DiskContainer{{SizeRW: 10}}, Volumes: []dockerapi.DiskVolume{{}, {}, {}}, BuildCache: []dockerapi.DiskBuildCache{{Size: 3}}}
	fake.top = &dockerapi.TopEntry{Titles: []string{"PID", "CMD"}, Processes: [][]string{{"1", "/app"}}}
	router := NewRouter(Config{Docker: fake})

	response := performRequest(router, http.MethodGet, "/api/v1/host")
	assertJSON(t, response.Body.String(), `{"id":"host1","server_version":"27.1","api_version":"1.47","min_api_version":"1.24","operating_system":"Linux","os_type":"linux","architecture":"arm64","kernel_version":"6.8","cpus":8,"memory_bytes":17179869184,"cpu_pct":0,"memory":{"used":0,"limit":0},"containers":{"total":4,"running":2,"paused":1,"stopped":1},"images":7,"disk":{"images":920,"containers":10,"volumes":0,"build_cache":3,"reclaimable":933}}`)

	response = performRequest(router, http.MethodGet, "/api/v1/containers/c1/top?ps_args=aux")
	assertJSON(t, response.Body.String(), `{"titles":["PID","CMD"],"processes":[["1","/app"]]}`)
}

func TestCollectionFilteringSortingAndValidation(t *testing.T) {
	fake := newFakeDockerClient()
	fake.containers = []dockerapi.Container{
		{ID: "b", Names: []string{"/zeta"}, Image: "redis:7", Created: 10, State: "stopped", Status: "Exited"},
		{ID: "a", Names: []string{"/alpha"}, Image: "acme/api:1", Created: 20, State: "running", Status: "Up"},
		{ID: "c", Names: []string{"/beta"}, Image: "acme/worker:1", Created: 30, State: "running", Status: "Up"},
	}
	router := NewRouter(Config{Docker: fake})

	response := performRequest(router, http.MethodGet, "/api/v1/containers?q=acme&status=RUNNING&sort=name")
	var items []containerView
	if err := json.Unmarshal(response.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if got := []string{items[0].ID, items[1].ID}; !reflect.DeepEqual(got, []string{"a", "c"}) {
		t.Fatalf("filtered IDs = %v, want [a c]", got)
	}

	response = performRequest(router, http.MethodGet, "/api/v1/containers?sort=created")
	if err := json.Unmarshal(response.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if got := []string{items[0].ID, items[1].ID, items[2].ID}; !reflect.DeepEqual(got, []string{"c", "a", "b"}) {
		t.Fatalf("created sort IDs = %v", got)
	}

	response = performRequest(router, http.MethodGet, "/api/v1/containers?sort=cpu")
	if err := json.Unmarshal(response.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if got := []string{items[0].ID, items[1].ID, items[2].ID}; !reflect.DeepEqual(got, []string{"b", "a", "c"}) {
		t.Fatalf("CPU stable-order IDs = %v", got)
	}

	response = performRequest(router, http.MethodGet, "/api/v1/containers?sort=state")
	if err := json.Unmarshal(response.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if got := []string{items[0].ID, items[1].ID, items[2].ID}; !reflect.DeepEqual(got, []string{"a", "c", "b"}) {
		t.Fatalf("state sort IDs = %v", got)
	}

	for _, target := range []string{
		"/api/v1/containers?all=perhaps",
		"/api/v1/containers?sort=size",
		"/api/v1/images?status=running",
		"/api/v1/networks?sort=created",
	} {
		response = performRequest(router, http.MethodGet, target)
		if response.Code != http.StatusBadRequest {
			t.Errorf("%s status = %d, want 400", target, response.Code)
		}
		assertJSON(t, response.Body.String(), fmt.Sprintf(`{"error":{"code":"invalid_query","message":%q}}`, errorMessage(response.Body.Bytes())))
	}
}

func TestOtherCollectionFiltersAndSorts(t *testing.T) {
	images := []imageView{
		{ID: "z", RepoTags: []string{"zeta:1"}, Created: 10},
		{ID: "a", RepoTags: []string{"alpha:1"}, Created: 20},
	}
	query, err := parseCollectionQuery(map[string][]string{"sort": {"name"}, "q": {"aLpHa"}}, "images")
	if err != nil {
		t.Fatal(err)
	}
	if got := filterSortImages(images, query); len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("filtered images = %+v", got)
	}
	query, err = parseCollectionQuery(map[string][]string{"sort": {"created"}}, "images")
	if err != nil {
		t.Fatal(err)
	}
	if got := filterSortImages(images, query); got[0].ID != "a" {
		t.Fatalf("created image sort = %+v", got)
	}

	volumes := []volumeView{
		{Name: "zeta", Driver: "local", CreatedAt: "2025-01-01"},
		{Name: "alpha", Driver: "nfs", CreatedAt: "2026-01-01"},
	}
	query, err = parseCollectionQuery(map[string][]string{"sort": {"name"}, "q": {"LOCAL"}}, "volumes")
	if err != nil {
		t.Fatal(err)
	}
	if got := filterSortVolumes(volumes, query); len(got) != 1 || got[0].Name != "zeta" {
		t.Fatalf("filtered volumes = %+v", got)
	}
	query, err = parseCollectionQuery(map[string][]string{"sort": {"created"}}, "volumes")
	if err != nil {
		t.Fatal(err)
	}
	if got := filterSortVolumes(volumes, query); got[0].Name != "alpha" {
		t.Fatalf("created volume sort = %+v", got)
	}

	networks := []networkView{{ID: "z", Name: "zeta", Driver: "bridge"}, {ID: "a", Name: "alpha", Driver: "overlay"}}
	query, err = parseCollectionQuery(map[string][]string{"sort": {"name"}, "q": {"OVER"}}, "networks")
	if err != nil {
		t.Fatal(err)
	}
	if got := filterSortNetworks(networks, query); len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("filtered networks = %+v", got)
	}
}

func TestRemainingReadRoutes(t *testing.T) {
	fake := newFakeDockerClient()
	fake.image = &dockerapi.ImageDetail{
		ID: "img", RepoTags: []string{"acme/api:1"}, RepoDigests: nil, Created: "now", Size: 5,
		Architecture: "arm64", Os: "linux", Env: nil, Entrypoint: nil, Cmd: nil, Labels: nil,
		Raw: json.RawMessage(`{"Id":"img"}`),
	}
	fake.volume = &dockerapi.Volume{
		Name: "data", Driver: "local", Mountpoint: "/data", CreatedAt: "now", Scope: "local",
		Raw: json.RawMessage(`{"Name":"data"}`),
	}
	network := dockerapi.Network{ID: "net", Name: "bridge", Driver: "bridge", Scope: "local"}
	fake.networks = []dockerapi.Network{network}
	fake.containers = []dockerapi.Container{{
		ID: "c1", Names: []string{"/api"}, ImageID: "img", Mounts: []dockerapi.ContainerMount{{
			Type: "volume", Name: "data", Destination: "/srv/data", RW: true,
		}},
	}}
	router := NewRouter(Config{Docker: fake})

	for _, target := range []string{"/api/v1/health", "/api/v1/version", "/api/v1/images/img", "/api/v1/volumes/data", "/api/v1/networks"} {
		response := performRequest(router, http.MethodGet, target)
		if response.Code != http.StatusOK {
			t.Errorf("%s status = %d, want 200: %s", target, response.Code, response.Body.String())
		}
	}

	response := performRequest(router, http.MethodGet, "/api/v1/images/img")
	assertJSON(t, response.Body.String(), `{"id":"img","repo_tags":["acme/api:1"],"repo_digests":[],"created":"now","size":5,"architecture":"arm64","os":"linux","env":[],"entrypoint":[],"cmd":[],"labels":{},"used_by_count":1,"dangling":false,"raw":{"Id":"img"}}`)
	response = performRequest(router, http.MethodGet, "/api/v1/volumes/data")
	assertJSON(t, response.Body.String(), `{"name":"data","driver":"local","mountpoint":"/data","created_at":"now","labels":{},"scope":"local","used_by":[{"container_id":"c1","container_name":"api","mount_path":"/srv/data","rw":true}],"raw":{"Name":"data"}}`)
	response = performRequest(router, http.MethodGet, "/api/v1/networks")
	assertJSON(t, response.Body.String(), `[{"id":"net","name":"bridge","driver":"bridge","scope":"local","ipam":[],"labels":{},"containers":[]}]`)
}

func errorMessage(body []byte) string {
	var envelope errorEnvelope
	_ = json.Unmarshal(body, &envelope)
	return envelope.Error.Message
}

func TestErrorMapping(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		path       string
		wantStatus int
		want       string
	}{
		{"resource not found", fmt.Errorf("%w: daemon detail", dockerapi.ErrNotFound), "/api/v1/containers/missing", 404, `{"error":{"code":"container_not_found","message":"no such container: missing","docker_status":404}}`},
		{"unreachable", fmt.Errorf("%w: socket", dockerapi.ErrUnreachable), "/api/v1/containers", 503, `{"error":{"code":"docker_unreachable","message":"Docker Engine is unreachable"}}`},
		{"conflict", fmt.Errorf("%w: already running", dockerapi.ErrConflict), "/api/v1/containers", 409, `{"error":{"code":"already_in_state","message":"already running","docker_status":409}}`},
		{"Docker API", &dockerapi.APIError{Status: 500, Message: "daemon failed"}, "/api/v1/containers", 500, `{"error":{"code":"docker_error","message":"daemon failed","docker_status":500}}`},
		{"internal", errors.New("secret detail"), "/api/v1/containers", 500, `{"error":{"code":"internal_error","message":"internal server error"}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := newFakeDockerClient()
			fake.err = test.err
			response := performRequest(NewRouter(Config{Docker: fake}), http.MethodGet, test.path)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
			assertJSON(t, response.Body.String(), test.want)
		})
	}
}

func TestMiddlewareAndBasePath(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	fake := newFakeDockerClient()
	router := NewRouter(Config{Docker: fake, ReadOnly: true, BasePath: "/vessel/", Logger: logger})

	request := httptest.NewRequest(http.MethodGet, "/vessel/api/v1/health", nil)
	request.Header.Set("X-Request-ID", "caller-id")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("X-Request-ID") != "caller-id" {
		t.Fatalf("health response = %d, request ID %q", response.Code, response.Header().Get("X-Request-ID"))
	}
	logLine := logs.String()
	for _, expected := range []string{`"request_id":"caller-id"`, `"method":"GET"`, `"path":"/vessel/api/v1/health"`, `"status":200`, `"duration":`} {
		if !strings.Contains(logLine, expected) {
			t.Errorf("access log %q does not contain %q", logLine, expected)
		}
	}

	response = performRequest(router, http.MethodPost, "/vessel/api/v1/containers/c1/start")
	if response.Code != http.StatusForbidden {
		t.Fatalf("read-only status = %d, want 403", response.Code)
	}
	assertJSON(t, response.Body.String(), `{"error":{"code":"read_only","message":"Vessel is running in read-only mode"}}`)
}

func TestRecoveryReturnsEnvelope(t *testing.T) {
	fake := newFakeDockerClient()
	fake.panicList = true
	response := performRequest(NewRouter(Config{Docker: fake}), http.MethodGet, "/api/v1/containers")
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", response.Code)
	}
	assertJSON(t, response.Body.String(), `{"error":{"code":"internal_error","message":"internal server error"}}`)
}
