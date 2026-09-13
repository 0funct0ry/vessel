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

// A container must be reachable by a caller-meaningful name (a compose
// service's short name, say), not only by the name Docker gave it — Docker's
// embedded DNS otherwise only resolves the latter. This must hold with no
// MacAddress set, since almost nothing sets one.
func TestCreateContainerSendsNetworkAliasesOnPrimaryAndAdditionalNetworks(t *testing.T) {
	var createBody any
	var connectBody any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1.43/containers/create":
			_ = json.NewDecoder(r.Body).Decode(&createBody)
			_, _ = io.WriteString(w, `{"Id":"c1"}`)
		case "/v1.43/networks/extra/connect":
			_ = json.NewDecoder(r.Body).Decode(&connectBody)
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()
	c, err := New("tcp://" + srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.CreateContainer(context.Background(), Spec{Image: "alpine:3", Network: "back", AdditionalNetworks: []string{"extra"}, NetworkAliases: []string{"db"}}); err != nil {
		t.Fatal(err)
	}

	want := map[string]any{"EndpointsConfig": map[string]any{"back": map[string]any{"Aliases": []any{"db"}}}}
	body, _ := createBody.(map[string]any)
	if !reflect.DeepEqual(body["NetworkingConfig"], want) {
		t.Fatalf("primary network alias: NetworkingConfig = %#v, want %#v", body["NetworkingConfig"], want)
	}

	connect, _ := connectBody.(map[string]any)
	endpointConfig, _ := connect["EndpointConfig"].(map[string]any)
	aliases, _ := endpointConfig["Aliases"].([]any)
	if len(aliases) != 1 || aliases[0] != "db" {
		t.Fatalf("additional network alias: EndpointConfig = %#v", connect["EndpointConfig"])
	}
}

func TestCreateContainerAdditionalNetworkFailureBecomesWarning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1.43/containers/create":
			_, _ = io.WriteString(w, `{"Id":"c1"}`)
		case r.URL.Path == "/v1.43/networks/extra/connect":
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"message":"network not found"}`)
		}
	}))
	defer srv.Close()
	c, err := New("tcp://" + srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.CreateContainer(context.Background(), Spec{Image: "alpine:3", AdditionalNetworks: []string{"extra"}})
	if err != nil {
		t.Fatalf("CreateContainer returned a fatal error for a failed additional-network attach: %v", err)
	}
	if result.ID != "c1" {
		t.Fatalf("result=%+v", result)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings=%v, want exactly one", result.Warnings)
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
