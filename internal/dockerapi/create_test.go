package dockerapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestCreateContainerBuildsExactDockerRequest(t *testing.T) {
	var createPath string
	var gotBody any
	var started bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1.43/containers/create" {
			createPath = r.URL.RequestURI()
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Fatal(err)
			}
			_, _ = io.WriteString(w, `{"Id":"c1","Warnings":["warn"]}`)
			return
		}
		if r.URL.Path == "/v1.43/containers/c1/start" {
			started = true
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	c, err := New("tcp://" + srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.CreateContainer(context.Background(), Spec{Name: "app", Image: "alpine:3", Command: []string{"sleep", "10"}, Entrypoint: []string{"/bin/sh"}, Env: []string{"A=B"}, Ports: []PortSpec{{Container: "8080", Host: "18080", Protocol: "tcp"}, {Container: "53", Protocol: "udp"}}, Mounts: []MountSpec{{Type: "volume", Source: "data", Target: "/data"}, {Type: "bind", Source: "/tmp/config", Target: "/config", ReadOnly: true}}, Network: "net1", RestartPolicy: "unless-stopped", Labels: map[string]string{"tier": "api"}, Start: true})
	if err != nil || result.ID != "c1" || !started {
		t.Fatalf("result=%+v err=%v started=%v", result, err, started)
	}
	if createPath != "/v1.43/containers/create?name=app" {
		t.Fatalf("path=%s", createPath)
	}
	want := `{"Image":"alpine:3","Cmd":["sleep","10"],"Entrypoint":["/bin/sh"],"Env":["A=B"],"ExposedPorts":{"53/udp":{},"8080/tcp":{}},"Labels":{"tier":"api"},"HostConfig":{"PortBindings":{"8080/tcp":[{"HostPort":"18080"}]},"Binds":["/tmp/config:/config:ro"],"Mounts":[{"Type":"volume","Source":"data","Target":"/data"}],"NetworkMode":"net1","RestartPolicy":{"Name":"unless-stopped"}}}`
	var expected any
	if err := json.Unmarshal([]byte(want), &expected); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotBody, expected) {
		actual, _ := json.Marshal(gotBody)
		t.Fatalf("body=%s\nwant=%s", actual, want)
	}
}

func TestCreateContainerStartFailureRetainsResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1.43/containers/create" {
			_, _ = io.WriteString(w, `{"Id":"c1"}`)
			return
		}
		w.WriteHeader(http.StatusConflict)
	}))
	defer srv.Close()
	c, _ := New("tcp://" + srv.Listener.Addr().String())
	result, err := c.CreateContainer(context.Background(), Spec{Image: "alpine", Start: true})
	if result.ID != "c1" {
		t.Fatalf("result=%+v", result)
	}
	if _, ok := err.(*StartError); !ok {
		t.Fatalf("err=%T %v", err, err)
	}
}
