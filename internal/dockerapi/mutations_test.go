package dockerapi

import (
	"context"
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
	report, err := c.Prune(ctx, "volumes")
	if err != nil {
		t.Fatal(err)
	}
	if report.SpaceReclaimed != 12 || len(report.Deleted) != 1 {
		t.Fatalf("report=%+v", report)
	}
	want := []string{
		"POST /v1.43/containers/a%2Fb/stop?t=10", "POST /v1.43/containers/a/rename?name=renamed", "DELETE /v1.43/containers/a?force=1&v=1",
		"POST /v1.43/images/create?fromImage=alpine%3A3.20", "POST /v1.43/images/img/tag?repo=acme%2Fapp&tag=v1", "DELETE /v1.43/images/img?force=1&noprune=1",
		"POST /v1.43/volumes/create", "DELETE /v1.43/volumes/data?force=1", "POST /v1.43/networks/create", "DELETE /v1.43/networks/net1", "POST /v1.43/networks/net1/connect", "POST /v1.43/networks/net1/disconnect", "POST /v1.43/volumes/prune",
	}
	if strings.Join(seen, "\n") != strings.Join(want, "\n") {
		t.Fatalf("requests:\n%s\nwant:\n%s", strings.Join(seen, "\n"), strings.Join(want, "\n"))
	}
}
