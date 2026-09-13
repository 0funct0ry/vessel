package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/0funct0ry/vessel/internal/auth"
	"github.com/0funct0ry/vessel/internal/store"
	"github.com/0funct0ry/vessel/internal/store/memstore"
)

type protectedRoute struct {
	method string
	path   string
	body   string
	role   store.Role
}

var protectedRoutes = []protectedRoute{
	{http.MethodPost, "/api/v1/auth/logout", "", store.RoleViewer},
	{http.MethodGet, "/api/v1/auth/me", "", store.RoleViewer},
	{http.MethodPost, "/api/v1/auth/ws-ticket", "", store.RoleViewer},
	{http.MethodGet, "/api/v1/host", "", store.RoleViewer},
	{http.MethodGet, "/api/v1/containers", "", store.RoleViewer},
	{http.MethodGet, "/api/v1/containers/c1", "", store.RoleViewer},
	{http.MethodGet, "/api/v1/containers/c1/logs", "", store.RoleViewer},
	{http.MethodGet, "/api/v1/containers/c1/stats", "", store.RoleViewer},
	{http.MethodGet, "/api/v1/containers/c1/top", "", store.RoleViewer},
	{http.MethodPost, "/api/v1/containers/c1/start", "", store.RoleOperator},
	{http.MethodPost, "/api/v1/containers/c1/stop", "", store.RoleOperator},
	{http.MethodPost, "/api/v1/containers/c1/restart", "", store.RoleOperator},
	{http.MethodPost, "/api/v1/containers/c1/pause", "", store.RoleOperator},
	{http.MethodPost, "/api/v1/containers/c1/unpause", "", store.RoleOperator},
	{http.MethodPost, "/api/v1/containers/c1/kill", "", store.RoleOperator},
	{http.MethodPost, "/api/v1/containers/c1/rename", `{"name":"new"}`, store.RoleOperator},
	{http.MethodDelete, "/api/v1/containers/c1", "", store.RoleOperator},
	{http.MethodPost, "/api/v1/containers", `{"image":"repo:tag"}`, store.RoleOperator},
	{http.MethodPost, "/api/v1/containers/c1/commit", `{"repo":"repo","tag":"tag"}`, store.RoleOperator},
	{http.MethodGet, "/api/v1/containers/c1/files", "", store.RoleOperator},
	{http.MethodPost, "/api/v1/containers/c1/files", "", store.RoleOperator},
	{http.MethodPost, "/api/v1/containers/c1/folders", `{"path":"/tmp/x"}`, store.RoleOperator},
	{http.MethodGet, "/api/v1/containers/c1/files/download", "", store.RoleOperator},
	{http.MethodDelete, "/api/v1/containers/c1/files", "", store.RoleOperator},
	{http.MethodPost, "/api/v1/containers/c1/files/rename", `{"path":"/tmp/a","name":"b"}`, store.RoleOperator},
	{http.MethodGet, "/api/v1/containers/c1/files/view", "", store.RoleOperator},
	{http.MethodPut, "/api/v1/containers/c1/files/content", `{"path":"/tmp/a","content":""}`, store.RoleOperator},
	{http.MethodGet, "/api/v1/images", "", store.RoleViewer},
	{http.MethodGet, "/api/v1/images/i1", "", store.RoleViewer},
	{http.MethodGet, "/api/v1/images/i1/history", "", store.RoleViewer},
	{http.MethodGet, "/api/v1/images/i1/dockerfile", "", store.RoleViewer},
	{http.MethodPost, "/api/v1/images/build", "", store.RoleOperator},
	{http.MethodPost, "/api/v1/images/pull", `{"reference":"repo:tag"}`, store.RoleOperator},
	{http.MethodPost, "/api/v1/images/i1/tag", `{"repo":"repo","tag":"tag"}`, store.RoleOperator},
	{http.MethodDelete, "/api/v1/images/i1", "", store.RoleOperator},
	{http.MethodGet, "/api/v1/volumes", "", store.RoleViewer},
	{http.MethodPost, "/api/v1/volumes", `{"name":"data"}`, store.RoleOperator},
	{http.MethodGet, "/api/v1/volumes/data", "", store.RoleViewer},
	{http.MethodDelete, "/api/v1/volumes/data", "", store.RoleOperator},
	{http.MethodGet, "/api/v1/volumes/data/files", "", store.RoleOperator},
	{http.MethodPost, "/api/v1/volumes/data/files", "", store.RoleOperator},
	{http.MethodPost, "/api/v1/volumes/data/folders", `{"path":"/tmp/x"}`, store.RoleOperator},
	{http.MethodGet, "/api/v1/volumes/data/files/download", "", store.RoleOperator},
	{http.MethodDelete, "/api/v1/volumes/data/files", "", store.RoleOperator},
	{http.MethodPost, "/api/v1/volumes/data/files/rename", `{"path":"/tmp/a","name":"b"}`, store.RoleOperator},
	{http.MethodGet, "/api/v1/volumes/data/files/view", "", store.RoleOperator},
	{http.MethodPut, "/api/v1/volumes/data/files/content", `{"path":"/tmp/a","content":""}`, store.RoleOperator},
	{http.MethodGet, "/api/v1/volumes/data/export", "", store.RoleViewer},
	{http.MethodPost, "/api/v1/volumes/data/clone", `{"name":"data-copy"}`, store.RoleOperator},
	{http.MethodGet, "/api/v1/networks", "", store.RoleViewer},
	{http.MethodPost, "/api/v1/networks", `{"name":"edge"}`, store.RoleOperator},
	{http.MethodGet, "/api/v1/networks/n1", "", store.RoleViewer},
	{http.MethodDelete, "/api/v1/networks/n1", "", store.RoleOperator},
	{http.MethodPost, "/api/v1/networks/n1/connect", `{"container":"c1"}`, store.RoleOperator},
	{http.MethodPost, "/api/v1/networks/n1/disconnect", `{"container":"c1"}`, store.RoleOperator},
	{http.MethodGet, "/api/v1/stacks", "", store.RoleViewer},
	{http.MethodPost, "/api/v1/stacks", `{"name":"acme","compose_yaml":"services:\n  api:\n    image: alpine:3\n"}`, store.RoleOperator},
	{http.MethodPost, "/api/v1/stacks/graph/to", `{"compose_yaml":"services:\n  api:\n    image: alpine:3\n"}`, store.RoleViewer},
	{http.MethodPost, "/api/v1/stacks/graph/from", `{"nodes":[{"id":"service:api","kind":"service","name":"api","service":{"image":"alpine:3"}}],"edges":[]}`, store.RoleViewer},
	{http.MethodGet, "/api/v1/stacks/acme", "", store.RoleViewer},
	{http.MethodPut, "/api/v1/stacks/acme", `{"compose_yaml":"services:\n  api:\n    image: alpine:3\n"}`, store.RoleOperator},
	{http.MethodDelete, "/api/v1/stacks/acme", "", store.RoleOperator},
	{http.MethodGet, "/api/v1/stacks/acme/logs", "", store.RoleViewer},
	{http.MethodPost, "/api/v1/stacks/acme/up", "", store.RoleOperator},
	{http.MethodPost, "/api/v1/stacks/acme/down", "", store.RoleOperator},
	{http.MethodPost, "/api/v1/stacks/acme/redeploy", "", store.RoleOperator},
	{http.MethodPost, "/api/v1/prune/images", "", store.RoleAdmin},
	{http.MethodGet, "/api/v1/users", "", store.RoleAdmin},
	{http.MethodPost, "/api/v1/users", `{"username":"new","password":"password1","role":"viewer"}`, store.RoleAdmin},
	{http.MethodPatch, "/api/v1/users/u1", `{"role":"viewer"}`, store.RoleAdmin},
	{http.MethodDelete, "/api/v1/users/u1", "", store.RoleAdmin},
	{http.MethodPost, "/api/v1/users/u1/password", `{"password":"password1"}`, store.RoleViewer},
	{http.MethodGet, "/api/v1/tokens", "", store.RoleViewer},
	{http.MethodPost, "/api/v1/tokens", `{"name":"ci"}`, store.RoleViewer},
	{http.MethodDelete, "/api/v1/tokens/t1", "", store.RoleViewer},
}

func TestRoleMiddlewareMatrix(t *testing.T) {
	persistence := memstore.New()
	tokens, _, err := auth.LoadTokens(context.Background(), persistence, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(Config{Docker: newFakeDockerClient(), Store: persistence, AuthEnabled: true, AllowExec: true, Tokens: tokens})
	roles := []store.Role{store.RoleViewer, store.RoleOperator, store.RoleAdmin}
	for _, route := range protectedRoutes {
		for _, actual := range roles {
			t.Run(route.method+" "+route.path+" as "+string(actual), func(t *testing.T) {
				token, err := tokens.Issue(store.User{ID: 1, Username: string(actual), Role: actual})
				if err != nil {
					t.Fatal(err)
				}
				if auth.Allows(actual, route.role) {
					return // The pure RBAC matrix verifies this permitted combination.
				}
				response := roleRequest(router, route, token)
				if response.Code != http.StatusForbidden {
					t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
				}
				want := `{"error":{"code":"forbidden_role","message":"insufficient role","required":"` + string(route.role) + `","actual":"` + string(actual) + `"}}`
				assertJSON(t, response.Body.String(), want)
			})
		}
	}
}

func TestRoleMiddlewareAuthOffIsAdmin(t *testing.T) {
	for _, route := range protectedRoutes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			if !auth.Allows(store.RoleAdmin, route.role) {
				t.Fatalf("admin cannot access route requiring %q", route.role)
			}
		})
	}
	// The middleware path is also exercised without claims; a protected admin
	// route must reach its handler rather than returning forbidden_role.
	response := roleRequest(NewRouter(Config{Docker: newFakeDockerClient()}), protectedRoute{method: http.MethodPost, path: "/api/v1/prune/images"}, "")
	if response.Code == http.StatusForbidden {
		t.Fatalf("auth-off request was role-denied: %s", response.Body.String())
	}
}

func TestMeIncludesCapabilities(t *testing.T) {
	persistence := memstore.New()
	tokens, _, err := auth.LoadTokens(context.Background(), persistence, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(Config{Docker: newFakeDockerClient(), Store: persistence, AuthEnabled: true, AllowExec: true, Tokens: tokens})
	token, err := tokens.Issue(store.User{ID: 1, Username: "viewer", Role: store.RoleViewer})
	if err != nil {
		t.Fatal(err)
	}
	response := roleRequest(router, protectedRoute{method: http.MethodGet, path: "/api/v1/auth/me"}, token)
	for _, capability := range []string{"containers.files.list", "containers.files.upload", "containers.files.mkdir", "containers.files.download", "containers.files.delete", "containers.files.rename", "containers.files.view", "containers.files.edit"} {
		if !strings.Contains(response.Body.String(), `"`+capability+`":false`) {
			t.Fatalf("capability %q missing/not false for viewer: %d %s", capability, response.Code, response.Body.String())
		}
	}
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}

	response = roleRequest(NewRouter(Config{Docker: newFakeDockerClient()}), protectedRoute{method: http.MethodGet, path: "/api/v1/auth/me"}, "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"auth":false`) || !strings.Contains(response.Body.String(), `"role":"admin"`) || !strings.Contains(response.Body.String(), `"images.remove":true`) {
		t.Fatalf("auth-off /me = %d %s", response.Code, response.Body.String())
	}
}

func TestMeCapabilitiesExecOffAsymmetry(t *testing.T) {
	persistence := memstore.New()
	tokens, _, err := auth.LoadTokens(context.Background(), persistence, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	// AllowExec is false but the caller is an operator, so containers.files.upload
	// and containers.files.download (archive endpoints, no exec involved) must stay
	// true while containers.files.list and containers.files.mkdir (exec-backed) must
	// flip false. This is the asymmetry PROMPTS.md M15.4 calls out as easy to miss.
	router := NewRouter(Config{Docker: newFakeDockerClient(), Store: persistence, AuthEnabled: true, AllowExec: false, Tokens: tokens})
	token, err := tokens.Issue(store.User{ID: 1, Username: "op", Role: store.RoleOperator})
	if err != nil {
		t.Fatal(err)
	}
	response := roleRequest(router, protectedRoute{method: http.MethodGet, path: "/api/v1/auth/me"}, token)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	// Exec-gated capabilities are removed from the map entirely (see
	// server.capabilities), not set to false, so assert their key is absent.
	for _, capability := range []string{"containers.files.list", "containers.files.mkdir", "containers.files.delete", "containers.files.rename", "volumes.files.list", "volumes.files.mkdir", "volumes.files.delete", "volumes.files.rename", "volumes.clone"} {
		if strings.Contains(body, `"`+capability+`"`) {
			t.Fatalf("capability %q present with exec off, want absent: %s", capability, body)
		}
	}
	for _, capability := range []string{"containers.files.upload", "containers.files.download", "containers.files.view", "containers.files.edit", "volumes.files.upload", "volumes.files.download", "volumes.files.view", "volumes.files.edit", "volumes.export"} {
		if !strings.Contains(body, `"`+capability+`":true`) {
			t.Fatalf("capability %q missing/false with exec off, want true: %s", capability, body)
		}
	}
}

func roleRequest(router http.Handler, route protectedRoute, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(route.method, route.path, strings.NewReader(route.body))
	if route.body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}
