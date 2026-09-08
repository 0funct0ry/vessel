package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
)

func newStaticTestFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":    {Data: []byte(`<meta name="vessel-base-path" content="__VESSEL_BASE_PATH__">`)},
		"assets/app.js": {Data: []byte(`console.log("hi")`)},
	}
}

func TestMountStaticServesIndex(t *testing.T) {
	r := newTestEngine()
	mountStatic(r, "", newStaticTestFS(), nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Body.String(); got != `<meta name="vessel-base-path" content="/">` {
		t.Fatalf("index.html not templated correctly, got %q", got)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", cc)
	}
}

func TestMountStaticBasePathTemplating(t *testing.T) {
	r := newTestEngine()
	mountStatic(r, "/vessel", newStaticTestFS(), nil)

	req := httptest.NewRequest(http.MethodGet, "/vessel/containers", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Body.String(); got != `<meta name="vessel-base-path" content="/vessel">` {
		t.Fatalf("index.html not templated correctly, got %q", got)
	}
}

func TestMountStaticServesAsset(t *testing.T) {
	r := newTestEngine()
	mountStatic(r, "", newStaticTestFS(), nil)

	req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Fatalf("Cache-Control = %q", cc)
	}
}

func TestMountStaticSPAFallback(t *testing.T) {
	r := newTestEngine()
	mountStatic(r, "", newStaticTestFS(), nil)

	req := httptest.NewRequest(http.MethodGet, "/containers/abc123", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Body.String(); got != `<meta name="vessel-base-path" content="/">` {
		t.Fatalf("unknown path did not fall back to index.html, got %q", got)
	}
}

func TestMountStaticAPIPathNotFound(t *testing.T) {
	r := newTestEngine()
	mountStatic(r, "", newStaticTestFS(), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nope", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestMountStaticNotBuilt(t *testing.T) {
	r := newTestEngine()
	mountStatic(r, "", nil, errors.New("not built"))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Body.String(); got == "" {
		t.Fatalf("expected fallback page body")
	}
}

func newTestEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}
