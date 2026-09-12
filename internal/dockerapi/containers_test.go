package dockerapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseHealthFromStatus(t *testing.T) {
	cases := map[string]string{
		"Up 2 hours (healthy)":             "healthy",
		"Up 3 seconds (unhealthy)":         "unhealthy",
		"Up 10 seconds (health: starting)": "starting",
		"Up 2 hours":                       "",
		"Exited (137) 4 minutes ago":       "",
	}
	for status, want := range cases {
		if got := parseHealthFromStatus(status); got != want {
			t.Errorf("parseHealthFromStatus(%q) = %q, want %q", status, got, want)
		}
	}
}

func TestListContainersPopulatesHealth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"Id":"a","Status":"Up 2 hours (healthy)"},{"Id":"b","Status":"Up 2 hours (unhealthy)"},{"Id":"c","Status":"Up 2 hours"}]`))
	}))
	defer server.Close()

	client, err := New("tcp://" + server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	containers, err := client.ListContainers(context.Background(), ListContainersOptions{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(containers) != 3 || containers[0].Health != "healthy" || containers[1].Health != "unhealthy" || containers[2].Health != "" {
		t.Fatalf("containers = %#v", containers)
	}
}
