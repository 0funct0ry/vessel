package api

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/0funct0ry/vessel/internal/dockerapi"
)

type trackingEventBody struct {
	io.Reader
	closed bool
}

func (b *trackingEventBody) Close() error { b.closed = true; return nil }

func TestEventsStreamsNormalizedDockerEventAndFilters(t *testing.T) {
	body := &trackingEventBody{Reader: strings.NewReader(`{"Type":"container","Action":"die","Actor":{"ID":"abc123","Attributes":{"name":"worker","exitCode":"137"}},"time":1720000000}` + "\n")}
	fake := newFakeDockerClient()
	fake.events = dockerapi.NewEventReader(body)
	router := NewRouter(Config{Docker: fake})

	response := performRequest(router, http.MethodGet, "/api/v1/events?type=container&type=image&since=1720000000")
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "event: docker") || !strings.Contains(response.Body.String(), `"name":"worker"`) || !strings.Contains(response.Body.String(), `"timestamp":"2024-07-03T09:46:40Z"`) {
		t.Fatalf("unexpected SSE body: %s", response.Body.String())
	}
	if len(fake.eventsOptions) != 1 || strings.Join(fake.eventsOptions[0].Filters["type"], ",") != "container,image" || fake.eventsOptions[0].Since != "1720000000" {
		t.Fatalf("filters=%+v", fake.eventsOptions)
	}
	if !body.closed {
		t.Fatal("event reader body was not closed")
	}
}

func TestEventsRejectInvalidTypeAndMapsDockerErrors(t *testing.T) {
	fake := newFakeDockerClient()
	router := NewRouter(Config{Docker: fake})
	response := performRequest(router, http.MethodGet, "/api/v1/events?type=service")
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid_query") {
		t.Fatalf("invalid type response=%d %s", response.Code, response.Body.String())
	}
	fake.err = dockerapi.ErrUnreachable
	response = performRequest(router, http.MethodGet, "/api/v1/events")
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "docker_unreachable") {
		t.Fatalf("docker error response=%d %s", response.Code, response.Body.String())
	}
}
