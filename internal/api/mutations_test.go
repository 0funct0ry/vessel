package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func mutationRequest(router http.Handler, method, target, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func TestMutationRoutesAndValidation(t *testing.T) {
	fake := newFakeDockerClient()
	router := NewRouter(Config{Docker: fake})
	if got := mutationRequest(router, http.MethodPost, "/api/v1/containers/c1/start", ""); got.Code != http.StatusNoContent {
		t.Fatalf("start=%d %s", got.Code, got.Body.String())
	}
	if got := mutationRequest(router, http.MethodPost, "/api/v1/containers/c1/stop?t=no", ""); got.Code != http.StatusBadRequest {
		t.Fatalf("bad stop=%d", got.Code)
	}
	if got := mutationRequest(router, http.MethodPost, "/api/v1/volumes", `{"name":"not valid"}`); got.Code != http.StatusBadRequest || !bytes.Contains(got.Body.Bytes(), []byte(`"invalid_name"`)) {
		t.Fatalf("bad volume=%d %s", got.Code, got.Body.String())
	}
	if got := mutationRequest(router, http.MethodPost, "/api/v1/images/pull", `{"reference":"bad reference"}`); got.Code != http.StatusBadRequest || !bytes.Contains(got.Body.Bytes(), []byte(`"invalid_name"`)) {
		t.Fatalf("bad pull=%d %s", got.Code, got.Body.String())
	}
	if got := mutationRequest(router, http.MethodPost, "/api/v1/networks/n/connect", `{"container":""}`); got.Code != http.StatusBadRequest {
		t.Fatalf("empty connection=%d", got.Code)
	}
	if got := mutationRequest(router, http.MethodPost, "/api/v1/prune/nope", ""); got.Code != http.StatusBadRequest {
		t.Fatalf("bad prune=%d", got.Code)
	}
}

func TestMutationReadOnly(t *testing.T) {
	got := mutationRequest(NewRouter(Config{Docker: newFakeDockerClient(), ReadOnly: true}), http.MethodPost, "/api/v1/containers/c1/start", "")
	if got.Code != http.StatusForbidden {
		t.Fatalf("status=%d", got.Code)
	}
}

func TestStreamWritesNamedEvent(t *testing.T) {
	router := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, _ := gin.CreateTestContext(w)
		c.Request = r
		Stream(c, func(send func(string, any) error) error { return send("pull", map[string]string{"id": "layer"}) })
	})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if got := w.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("content type=%q", got)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("event: pull\ndata: {\"id\":\"layer\"}")) {
		t.Fatalf("body=%q", w.Body.String())
	}
}
