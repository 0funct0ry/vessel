package dockerapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestMutations_EngineRequests(t *testing.T) {
	seen := make([]string, 0, 12)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.RequestURI())
		switch r.URL.Path {
		case "/v1.43/volumes/create":
			_, _ = io.WriteString(w, `{"Name":"data","Driver":"local"}`)
		case "/v1.43/networks/create":
			_, _ = io.WriteString(w, `{"Id":"net1"}`)
		case "/v1.43/images/create":
			_, _ = io.WriteString(w, `{"id":"layer","status":"Downloading","progressDetail":{"current":1,"total":2}}`+"\n")
		case "/v1.43/commit":
			_, _ = io.WriteString(w, `{"Id":"sha256:committed"}`)
		case "/v1.43/volumes/prune":
			_, _ = io.WriteString(w, `{"VolumesDeleted":["data"],"SpaceReclaimed":12}`)
		}
	}))
	defer srv.Close()
	c, err := New("tcp://" + srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := c.Lifecycle(ctx, "a/b", "stop", url.Values{"t": {"10"}}); err != nil {
		t.Fatal(err)
	}
	if err := c.RenameContainer(ctx, "a", "renamed"); err != nil {
		t.Fatal(err)
	}
	if err := c.RemoveContainer(ctx, "a", RemoveContainerOptions{Force: true, Volumes: true}); err != nil {
		t.Fatal(err)
	}
	pull, err := c.PullImage(ctx, "alpine:3.20")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pull.Next(); err != nil {
		t.Fatal(err)
	}
	_ = pull.Close()
	if err := c.TagImage(ctx, "img", "acme/app", "v1"); err != nil {
		t.Fatal(err)
	}
	committed, err := c.CommitContainer(ctx, "a/b", CommitOptions{Repo: "acme/app", Tag: "snapshot", Comment: "before upgrade", Pause: false})
	if err != nil {
		t.Fatal(err)
	}
	if committed.ImageID != "sha256:committed" {
		t.Fatalf("commit result=%+v", committed)
	}
	if err := c.RemoveImage(ctx, "img", RemoveImageOptions{Force: true, NoPrune: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CreateVolume(ctx, CreateVolumeOptions{Name: "data", Driver: "local"}); err != nil {
		t.Fatal(err)
	}
	if err := c.RemoveVolume(ctx, "data", true); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CreateNetwork(ctx, CreateNetworkOptions{Name: "edge", Driver: "bridge", Subnet: "172.30.0.0/16", Gateway: "172.30.0.1"}); err != nil {
		t.Fatal(err)
	}
	if err := c.RemoveNetwork(ctx, "net1"); err != nil {
		t.Fatal(err)
	}
	if err := c.NetworkConnect(ctx, "net1", "app", false); err != nil {
		t.Fatal(err)
	}
	if err := c.NetworkConnect(ctx, "net1", "app", true); err != nil {
		t.Fatal(err)
	}
	report, err := c.Prune(ctx, "volumes", nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.SpaceReclaimed != 12 || len(report.Deleted) != 1 {
		t.Fatalf("report=%+v", report)
	}
	want := []string{
		"POST /v1.43/containers/a%2Fb/stop?t=10", "POST /v1.43/containers/a/rename?name=renamed", "DELETE /v1.43/containers/a?force=1&v=1",
		"POST /v1.43/images/create?fromImage=alpine%3A3.20", "POST /v1.43/images/img/tag?repo=acme%2Fapp&tag=v1", "POST /v1.43/commit?comment=before+upgrade&container=a%2Fb&pause=false&repo=acme%2Fapp&tag=snapshot", "DELETE /v1.43/images/img?force=1&noprune=1",
		"POST /v1.43/volumes/create", "DELETE /v1.43/volumes/data?force=1", "POST /v1.43/networks/create", "DELETE /v1.43/networks/net1", "POST /v1.43/networks/net1/connect", "POST /v1.43/networks/net1/disconnect", "POST /v1.43/volumes/prune",
	}
	if strings.Join(seen, "\n") != strings.Join(want, "\n") {
		t.Fatalf("requests:\n%s\nwant:\n%s", strings.Join(seen, "\n"), strings.Join(want, "\n"))
	}
}

func TestPrune_FiltersQuery(t *testing.T) {
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		_, _ = io.WriteString(w, `{"ImagesDeleted":[{"Deleted":"sha256:a"},{"Untagged":"acme/app:old"}],"SpaceReclaimed":5}`)
	}))
	defer srv.Close()
	c, err := New("tcp://" + srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	report, err := c.Prune(ctx, "images", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Docker's real /images/prune response shapes ImagesDeleted as objects
	// ({"Deleted": "sha256:..."} / {"Untagged": "repo:tag"}), not plain
	// strings; decoding must pull the id out of either field.
	if len(report.Deleted) != 2 || report.Deleted[0] != "sha256:a" || report.Deleted[1] != "acme/app:old" {
		t.Fatalf("deleted=%v", report.Deleted)
	}
	if gotQuery.Get("filters") != "" {
		t.Fatalf("expected no filters query when none given, got %q", gotQuery.Get("filters"))
	}

	if _, err := c.Prune(ctx, "images", map[string][]string{"dangling": {"false"}}); err != nil {
		t.Fatal(err)
	}
	var decoded map[string][]string
	if err := json.Unmarshal([]byte(gotQuery.Get("filters")), &decoded); err != nil {
		t.Fatalf("decoding filters query: %v", err)
	}
	if len(decoded["dangling"]) != 1 || decoded["dangling"][0] != "false" {
		t.Fatalf("filters=%v", decoded)
	}
}

func TestCreateNetwork_FullOptions(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		_, _ = io.WriteString(w, `{"Id":"net1"}`)
	}))
	defer srv.Close()
	c, err := New("tcp://" + srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.CreateNetwork(context.Background(), CreateNetworkOptions{
		Name: "edge", Driver: "bridge",
		Internal: true, Attachable: true, EnableIPv6: true,
		IPAMDriver: "default", Subnet: "172.30.0.0/16", Gateway: "172.30.0.1", IPRange: "172.30.1.0/24",
		AuxAddresses: map[string]string{"host": "172.30.0.2"},
		IPAMOptions:  map[string]string{"foo": "bar"},
		DriverOpts:   map[string]string{"com.docker.network.bridge.name": "br-edge"},
		Labels:       map[string]string{"env": "prod"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"Name":"edge","Driver":"bridge","Internal":true,"Attachable":true,"EnableIPv6":true,"IPAM":{"Driver":"default","Config":[{"Subnet":"172.30.0.0/16","Gateway":"172.30.0.1","IPRange":"172.30.1.0/24","AuxiliaryAddresses":{"host":"172.30.0.2"}}],"Options":{"foo":"bar"}},"Options":{"com.docker.network.bridge.name":"br-edge"},"Labels":{"env":"prod"}}`
	if got := strings.TrimSpace(string(body)); got != want {
		t.Fatalf("request body:\n%s\nwant:\n%s", got, want)
	}
}
