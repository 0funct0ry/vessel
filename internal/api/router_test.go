package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
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
	history        []dockerapi.HistoryLayer
	export         io.ReadCloser
	importStream   dockerapi.ImportStream
	importBody     []byte
	buildStream    dockerapi.BuildStream
	buildBody      []byte
	buildTags      []string
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
	pruneCalls     []pruneCall
	stats          dockerapi.Stats
	statsByID      map[string]dockerapi.Stats
	statsStream    dockerapi.StatsStream
	statsCalls     []string
	execCalls      []dockerapi.ExecOptions
	execCreate     func(dockerapi.ExecOptions) (string, error)
	files          []dockerapi.FileEntry
	uploadBody     []byte
	uploadPath     string
	download       io.ReadCloser
	createSpec     dockerapi.Spec
	createResult   dockerapi.CreateResult
	createErr      error
	commitOptions  dockerapi.CommitOptions
	commitResult   dockerapi.CommitResult
	removeCalls    []string
	removedIDs     []string
	renameCalls    [][2]string
	readFileName   string
	readFileData   []byte
	writeFilePath  string
	writeFileData  []byte
	events         *dockerapi.EventReader
	eventsOptions  []dockerapi.EventsOptions
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

func (f *fakeDockerClient) Stats(_ context.Context, id string) (dockerapi.Stats, error) {
	if s, ok := f.statsByID[id]; ok {
		return s, f.err
	}
	return f.stats, f.err
}

func (f *fakeDockerClient) Events(_ context.Context, opts dockerapi.EventsOptions) (*dockerapi.EventReader, error) {
	f.eventsOptions = append(f.eventsOptions, opts)
	return f.events, f.err
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

func (f *fakeDockerClient) History(context.Context, string) ([]dockerapi.HistoryLayer, error) {
	return f.history, f.err
}
func (f *fakeDockerClient) ExportImages(context.Context, []string) (io.ReadCloser, error) {
	return f.export, f.err
}
func (f *fakeDockerClient) ImportImages(_ context.Context, tar io.Reader) (dockerapi.ImportStream, error) {
	f.importBody, _ = io.ReadAll(tar)
	return f.importStream, f.err
}
func (f *fakeDockerClient) BuildImage(_ context.Context, tar io.Reader, tags []string) (dockerapi.BuildStream, error) {
	f.buildBody, _ = io.ReadAll(tar)
	f.buildTags = append([]string(nil), tags...)
	return f.buildStream, f.err
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
func (f *fakeDockerClient) RemoveContainer(_ context.Context, id string, _ dockerapi.RemoveContainerOptions) error {
	if f.err == nil {
		f.removedIDs = append(f.removedIDs, id)
	}
	return f.err
}
func (f *fakeDockerClient) CreateContainer(_ context.Context, spec dockerapi.Spec) (dockerapi.CreateResult, error) {
	f.createSpec = spec
	if f.createResult.ID == "" {
		f.createResult.ID = "new-container"
	}
	if f.createErr != nil {
		return f.createResult, f.createErr
	}
	return f.createResult, f.err
}
func (f *fakeDockerClient) RecreateContainer(_ context.Context, _ string, spec dockerapi.Spec) (dockerapi.CreateResult, error) {
	f.createSpec = spec
	if f.createResult.ID == "" {
		f.createResult.ID = "new-container"
	}
	if f.createErr != nil {
		return f.createResult, f.createErr
	}
	return f.createResult, f.err
}
func (f *fakeDockerClient) CommitContainer(_ context.Context, _ string, opts dockerapi.CommitOptions) (dockerapi.CommitResult, error) {
	f.commitOptions = opts
	if f.commitResult.ImageID == "" {
		f.commitResult.ImageID = "new-image"
	}
	return f.commitResult, f.err
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
func (f *fakeDockerClient) WithVolumeMount(_ context.Context, _ string, fn func(string) error) error {
	if f.err != nil {
		return f.err
	}
	return fn("mount-helper")
}
func (f *fakeDockerClient) CloneVolume(context.Context, string, string) error { return f.err }
func (f *fakeDockerClient) CreateNetwork(context.Context, dockerapi.CreateNetworkOptions) (*dockerapi.Network, error) {
	return f.network, f.err
}
func (f *fakeDockerClient) RemoveNetwork(context.Context, string) error                { return f.err }
func (f *fakeDockerClient) NetworkConnect(context.Context, string, string, bool) error { return f.err }

type pruneCall struct {
	kind    string
	filters map[string][]string
}

func (f *fakeDockerClient) Prune(_ context.Context, kind string, filters map[string][]string) (*dockerapi.PruneReport, error) {
	f.pruneCalls = append(f.pruneCalls, pruneCall{kind: kind, filters: filters})
	return f.prune, f.err
}
func (f *fakeDockerClient) CreateExec(_ context.Context, _ string, opts dockerapi.ExecOptions) (string, error) {
	f.execCalls = append(f.execCalls, opts)
	if f.execCreate != nil {
		return f.execCreate(opts)
	}
	return "exec1", f.err
}
func (f *fakeDockerClient) StartExec(context.Context, string, bool) (dockerapi.ExecSession, error) {
	return nil, f.err
}
func (f *fakeDockerClient) ResizeExec(context.Context, string, int, int) error { return f.err }
func (f *fakeDockerClient) ListDirectory(context.Context, string, string) ([]dockerapi.FileEntry, error) {
	return f.files, f.err
}
func (f *fakeDockerClient) UploadFiles(_ context.Context, _ string, dir string, body io.Reader) error {
	f.uploadPath = dir
	f.uploadBody, _ = io.ReadAll(body)
	return f.err
}
func (f *fakeDockerClient) CreateDirectory(context.Context, string, string) error { return f.err }
func (f *fakeDockerClient) DownloadPath(context.Context, string, string) (io.ReadCloser, error) {
	return f.download, f.err
}
func (f *fakeDockerClient) RemovePath(_ context.Context, _ string, path string) error {
	f.removeCalls = append(f.removeCalls, path)
	return f.err
}
func (f *fakeDockerClient) RenamePath(_ context.Context, _ string, from, to string) error {
	f.renameCalls = append(f.renameCalls, [2]string{from, to})
	return f.err
}
func (f *fakeDockerClient) ReadFile(context.Context, string, string) (string, []byte, error) {
	return f.readFileName, f.readFileData, f.err
}
func (f *fakeDockerClient) WriteFile(_ context.Context, _ string, path string, data []byte) error {
	f.writeFilePath = path
	f.writeFileData = data
	return f.err
}

func performRequest(router http.Handler, method, target string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, nil)
	router.ServeHTTP(recorder, request)
	return recorder
}

func performRequestBody(router http.Handler, method, target, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
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
	assertJSON(t, response.Body.String(), `[{"id":"abc123","name":"api","image":"acme/api:1","image_id":"sha256:image","command":"/app","created":42,"state":"running","status":"Up 2 minutes","health":"","ports":[{"ip":"127.0.0.1","private_port":8080,"public_port":7373,"type":"tcp"}],"labels":{"env":"test"}}]`)
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

func TestImageHistoryJSON(t *testing.T) {
	fake := newFakeDockerClient()
	fake.history = []dockerapi.HistoryLayer{
		{ID: "sha256:top", Created: 200, CreatedBy: `CMD ["sh"]`, Size: 10, Tags: []string{"acme/app:1"}},
		{ID: "<missing>", Created: 100, CreatedBy: "ADD file:abc in /", Size: 5},
	}
	router := NewRouter(Config{Docker: fake})

	response := performRequest(router, http.MethodGet, "/api/v1/images/img/history")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	assertJSON(t, response.Body.String(), `[
		{"id":"sha256:top","created":200,"created_by":"CMD [\"sh\"]","size":10,"comment":"","tags":["acme/app:1"]},
		{"id":"<missing>","created":100,"created_by":"ADD file:abc in /","size":5,"comment":"","tags":[]}
	]`)
}

func TestImageDockerfileJSON(t *testing.T) {
	fake := newFakeDockerClient()
	fake.image = &dockerapi.ImageDetail{Config: dockerapi.ImageConfig{Env: []string{"PORT=8080"}, WorkingDir: "/app", Cmd: []string{"server"}}}
	fake.history = []dockerapi.HistoryLayer{{ID: "sha256:top", CreatedBy: "/bin/sh -c make build"}, {ID: "<missing>", CreatedBy: "ADD file:abc in /"}}
	router := NewRouter(Config{Docker: fake})

	response := performRequest(router, http.MethodGet, "/api/v1/images/img/dockerfile")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	assertJSON(t, response.Body.String(), `{"dockerfile":"# FROM unknown — base image cannot be recovered from history\n# <missing> layer has no local metadata\n# ADD file:abc in /\nmake build\nENV PORT=8080\nWORKDIR /app\nCMD [\"server\"]\n","approximate":true}`)
}

func TestImageDockerfileNotFound(t *testing.T) {
	fake := newFakeDockerClient()
	fake.err = fmt.Errorf("%w: daemon detail", dockerapi.ErrNotFound)
	router := NewRouter(Config{Docker: fake})

	response := performRequest(router, http.MethodGet, "/api/v1/images/missing/dockerfile")
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", response.Code, response.Body.String())
	}
	assertJSON(t, response.Body.String(), `{"error":{"code":"image_not_found","message":"no such image: missing","docker_status":404}}`)
}

func TestVolumeListJoinExactJSON(t *testing.T) {
	fake := newFakeDockerClient()
	diskVolume := dockerapi.DiskVolume{Name: "data", UsageKnown: true}
	diskVolume.UsageData.Size = 42
	diskVolume.UsageData.RefCount = 1
	fake.disk = &dockerapi.DiskUsageInfo{Volumes: []dockerapi.DiskVolume{diskVolume}}
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
	assertJSON(t, response.Body.String(), `[{"name":"data","driver":"local","mountpoint":"/var/lib/docker/volumes/data/_data","size_bytes":42,"created_at":"2026-09-07T00:00:00Z","labels":{"tier":"db"},"scope":"local","used_by":[{"container_id":"c1","container_name":"postgres","mount_path":"/var/lib/postgresql/data","rw":true}]}]`)
}

func TestVolumeSizeOmittedWhenUsageUnknown(t *testing.T) {
	fake := newFakeDockerClient()
	fake.volumes = []dockerapi.Volume{{Name: "data", Driver: "local"}}
	fake.disk = &dockerapi.DiskUsageInfo{Volumes: []dockerapi.DiskVolume{{Name: "data"}}}
	response := performRequest(NewRouter(Config{Docker: fake}), http.MethodGet, "/api/v1/volumes")
	if response.Code != http.StatusOK || bytes.Contains(response.Body.Bytes(), []byte(`size_bytes`)) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestVolumeCreateRejectsEmptyDriverOptKey(t *testing.T) {
	fake := newFakeDockerClient()
	router := NewRouter(Config{Docker: fake})
	response := performRequestBody(router, http.MethodPost, "/api/v1/volumes", `{"name":"data","driver":"nfs","driver_opts":{"":"type=nfs"}}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", response.Code, response.Body.String())
	}
	if !bytes.Contains(response.Body.Bytes(), []byte("invalid_request")) {
		t.Fatalf("body = %s, want invalid_request", response.Body.String())
	}
}

func TestVolumeFilesDownloadExportAndClone(t *testing.T) {
	fake := newFakeDockerClient()
	fake.files = []dockerapi.FileEntry{{Name: "a.txt", Path: "/vessel-volume/a.txt", Type: "file", Size: 3}}
	fake.download = io.NopCloser(strings.NewReader("tar-bytes"))
	fake.volume = &dockerapi.Volume{Name: "data-copy", Driver: "local"}
	router := NewRouter(Config{Docker: fake, AllowExec: true})

	response := performRequest(router, http.MethodGet, "/api/v1/volumes/data/files")
	if response.Code != http.StatusOK {
		t.Fatalf("files status = %d: %s", response.Code, response.Body.String())
	}
	assertJSON(t, response.Body.String(), `[{"name":"a.txt","path":"/a.txt","type":"file","size":3,"mode":"","modified_at":"0001-01-01T00:00:00Z"}]`)

	response = performRequest(router, http.MethodGet, "/api/v1/volumes/data/files/download?path=/a.txt")
	if response.Code != http.StatusOK || response.Body.String() != "tar-bytes" {
		t.Fatalf("download = %d %s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Disposition"); got != `attachment; filename="a.txt.tar"` {
		t.Fatalf("content-disposition = %q", got)
	}

	fake.download = io.NopCloser(strings.NewReader("full-tar"))
	response = performRequest(router, http.MethodGet, "/api/v1/volumes/data/export")
	if response.Code != http.StatusOK || response.Body.String() != "full-tar" {
		t.Fatalf("export = %d %s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Disposition"); got != `attachment; filename="data.tar"` {
		t.Fatalf("content-disposition = %q", got)
	}

	response = performRequestBody(router, http.MethodPost, "/api/v1/volumes/data/clone", `{"name":"data-copy"}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("clone status = %d: %s", response.Code, response.Body.String())
	}
}

// TestVolumeFilesListedPathDeletesCorrectly guards against a regression where
// handleVolumeFiles returned entries with the internal /vessel-volume mount
// prefix still attached; a client then round-tripping that path back into
// delete/rename/download/navigate calls would have it prefixed a second time
// (/vessel-volume/vessel-volume/...), so the real file was never touched even
// though the request succeeded — a silent no-op, not an error.
func TestVolumeFilesListedPathDeletesCorrectly(t *testing.T) {
	fake := newFakeDockerClient()
	fake.files = []dockerapi.FileEntry{{Name: "vessel.md", Path: "/vessel-volume/vessel.md", Type: "file", Size: 5}}
	router := NewRouter(Config{Docker: fake, AllowExec: true})

	response := performRequest(router, http.MethodGet, "/api/v1/volumes/data/files")
	if response.Code != http.StatusOK {
		t.Fatalf("files status = %d: %s", response.Code, response.Body.String())
	}
	var entries []dockerapi.FileEntry
	if err := json.Unmarshal(response.Body.Bytes(), &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Path != "/vessel.md" {
		t.Fatalf("entries = %+v, want a single entry with the volume-relative path /vessel.md", entries)
	}

	response = performRequest(router, http.MethodDelete, "/api/v1/volumes/data/files?path="+url.QueryEscape(entries[0].Path))
	if response.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d: %s", response.Code, response.Body.String())
	}
	if len(fake.removeCalls) != 1 || fake.removeCalls[0] != "/vessel-volume/vessel.md" {
		t.Fatalf("removeCalls = %v, want exactly one call at /vessel-volume/vessel.md (not doubled)", fake.removeCalls)
	}
}

func TestVolumeFilesRequiresExec(t *testing.T) {
	fake := newFakeDockerClient()
	router := NewRouter(Config{Docker: fake, AllowExec: false})
	response := performRequest(router, http.MethodGet, "/api/v1/volumes/data/files")
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 route not registered: %s", response.Code, response.Body.String())
	}
}

func TestVolumeFilesWriteActions(t *testing.T) {
	fake := newFakeDockerClient()
	router := NewRouter(Config{Docker: fake, AllowExec: true})

	var form bytes.Buffer
	writer := multipart.NewWriter(&form)
	if err := writer.WriteField("path", "/"); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("files", "note.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("hi")); err != nil {
		t.Fatal(err)
	}
	writer.Close()
	uploadReq := httptest.NewRequest(http.MethodPost, "/api/v1/volumes/data/files", &form)
	uploadReq.Header.Set("Content-Type", writer.FormDataContentType())
	uploadRec := httptest.NewRecorder()
	router.ServeHTTP(uploadRec, uploadReq)
	if uploadRec.Code != http.StatusNoContent {
		t.Fatalf("upload status = %d: %s", uploadRec.Code, uploadRec.Body.String())
	}
	if fake.uploadPath != "/vessel-volume" {
		t.Fatalf("uploadPath = %q", fake.uploadPath)
	}

	response := performRequestBody(router, http.MethodPost, "/api/v1/volumes/data/folders", `{"path":"/reports"}`)
	if response.Code != http.StatusNoContent {
		t.Fatalf("folder status = %d: %s", response.Code, response.Body.String())
	}

	response = performRequest(router, http.MethodDelete, "/api/v1/volumes/data/files?path=/note.txt")
	if response.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d: %s", response.Code, response.Body.String())
	}
	if len(fake.removeCalls) != 1 || fake.removeCalls[0] != "/vessel-volume/note.txt" {
		t.Fatalf("removeCalls = %v", fake.removeCalls)
	}

	response = performRequestBody(router, http.MethodPost, "/api/v1/volumes/data/files/rename", `{"path":"/reports","name":"archive"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("rename status = %d: %s", response.Code, response.Body.String())
	}
	if len(fake.renameCalls) != 1 || fake.renameCalls[0] != [2]string{"/vessel-volume/reports", "/vessel-volume/archive"} {
		t.Fatalf("renameCalls = %v", fake.renameCalls)
	}

	fake.readFileName = "note.txt"
	fake.readFileData = []byte("hello")
	response = performRequest(router, http.MethodGet, "/api/v1/volumes/data/files/view?path=/note.txt")
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"content":"hello"`)) {
		t.Fatalf("view = %d %s", response.Code, response.Body.String())
	}

	response = performRequestBody(router, http.MethodPut, "/api/v1/volumes/data/files/content", `{"path":"/note.txt","content":"updated"}`)
	if response.Code != http.StatusNoContent {
		t.Fatalf("write status = %d: %s", response.Code, response.Body.String())
	}
	if fake.writeFilePath != "/vessel-volume/note.txt" || string(fake.writeFileData) != "updated" {
		t.Fatalf("writeFilePath=%q writeFileData=%q", fake.writeFilePath, fake.writeFileData)
	}
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
	assertJSON(t, response.Body.String(), `{"id":"n1","name":"edge","driver":"bridge","scope":"local","internal":false,"attachable":false,"enable_ipv6":false,"ipam_driver":"","ipam":[{"subnet":"172.20.0.0/16","gateway":"172.20.0.1"}],"ipam_options":{},"driver_opts":{},"labels":{},"containers":[{"container_id":"c1","container_name":"api","ipv4_address":"172.20.0.2/16","ipv6_address":"fd00::2/64"},{"container_id":"c2","container_name":"worker","ipv4_address":"172.20.0.3/16","ipv6_address":""}],"raw":{"Id":"n1"}}`)

	response = performRequest(router, http.MethodGet, "/api/v1/containers/c1")
	assertJSON(t, response.Body.String(), `{"id":"c1","name":"api","image":"acme/api:1","command":[],"created":"now","state":"running","status":"running","exit_code":0,"health":"","restart_policy":"","mounts":[],"networks":{},"env":[],"labels":{},"ports":[],"security":{"privileged":false,"readonly_rootfs":false,"user":"","userns_mode":"","apparmor_profile":""},"resources":{"cpu_shares":0,"cpus":0,"memory":0,"memory_swap":0,"memory_reservation":0,"pids_limit":0,"oom_kill_disable":false,"cpu_period":0,"cpu_quota":0,"cgroup_parent":"","cgroupns_mode":""},"raw":{"Id":"c1"}}`)
}

func TestNetworksListReflectsConnectedContainers(t *testing.T) {
	fake := newFakeDockerClient()
	fake.networks = []dockerapi.Network{
		{ID: "n1", Name: "bridge", Driver: "bridge", Scope: "local"},
		{ID: "n2", Name: "isolated", Driver: "bridge", Scope: "local"},
	}
	fake.containers = []dockerapi.Container{{
		ID: "c1", Names: []string{"/api"},
	}, {
		ID: "c2", Names: []string{"/stale"},
	}}
	fake.containers[0].NetworkSettings.Networks = map[string]dockerapi.ContainerNetwork{
		"bridge": {NetworkID: "n1", IPAddress: "172.17.0.2"},
	}
	// A stopped container can retain a stale network entry with no live
	// endpoint (no IP) — it must not be counted as attached.
	fake.containers[1].NetworkSettings.Networks = map[string]dockerapi.ContainerNetwork{
		"isolated": {NetworkID: "", IPAddress: ""},
	}
	router := NewRouter(Config{Docker: fake})

	response := performRequest(router, http.MethodGet, "/api/v1/networks")
	assertJSON(t, response.Body.String(), `[{"id":"n1","name":"bridge","driver":"bridge","scope":"local","internal":false,"attachable":false,"enable_ipv6":false,"ipam_driver":"","ipam":[],"ipam_options":{},"driver_opts":{},"labels":{},"containers":[{"container_id":"c1","container_name":"api","ipv4_address":"172.17.0.2","ipv6_address":""}]},{"id":"n2","name":"isolated","driver":"bridge","scope":"local","internal":false,"attachable":false,"enable_ipv6":false,"ipam_driver":"","ipam":[],"ipam_options":{},"driver_opts":{},"labels":{},"containers":[]}]`)
}

func TestNetworkCreateRejectsEmptyMapKeys(t *testing.T) {
	fake := newFakeDockerClient()
	fake.network = &dockerapi.Network{ID: "n1", Name: "edge"}
	router := NewRouter(Config{Docker: fake})

	for _, body := range []string{
		`{"name":"edge","aux_addresses":{"":"10.0.0.1"}}`,
		`{"name":"edge","ipam_options":{"":"x"}}`,
		`{"name":"edge","driver_opts":{"":"x"}}`,
	} {
		response := performRequestBody(router, http.MethodPost, "/api/v1/networks", body)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("body=%s status=%d: %s", body, response.Code, response.Body.String())
		}
	}
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
	assertJSON(t, response.Body.String(), `{"id":"host1","server_version":"27.1","api_version":"1.47","min_api_version":"1.24","operating_system":"Linux","os_type":"linux","architecture":"arm64","kernel_version":"6.8","cpus":8,"memory_bytes":17179869184,"cpu_pct":0,"memory":{"used":0,"limit":0},"containers":{"total":4,"running":2,"paused":1,"stopped":1},"images":7,"disk":{"images":920,"containers":10,"volumes":0,"build_cache":3,"reclaimable":933,"images_reclaimable":920,"containers_reclaimable":10,"volumes_reclaimable":0,"build_cache_reclaimable":3},"top_cpu":[],"top_mem":[]}`)

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
	assertJSON(t, response.Body.String(), `{"id":"img","repo_tags":["acme/api:1"],"repo_digests":[],"created":"now","size":5,"architecture":"arm64","os":"linux","env":[],"entrypoint":[],"cmd":[],"labels":{},"used_by_count":1,"used_by":[{"container_id":"c1","container_name":"api","state":""}],"dangling":false,"raw":{"Id":"img"}}`)
	response = performRequest(router, http.MethodGet, "/api/v1/volumes/data")
	assertJSON(t, response.Body.String(), `{"name":"data","driver":"local","mountpoint":"/data","created_at":"now","labels":{},"scope":"local","used_by":[{"container_id":"c1","container_name":"api","mount_path":"/srv/data","rw":true}],"raw":{"Name":"data"}}`)
	response = performRequest(router, http.MethodGet, "/api/v1/networks")
	assertJSON(t, response.Body.String(), `[{"id":"net","name":"bridge","driver":"bridge","scope":"local","internal":false,"attachable":false,"enable_ipv6":false,"ipam_driver":"","ipam":[],"ipam_options":{},"driver_opts":{},"labels":{},"containers":[]}]`)
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
