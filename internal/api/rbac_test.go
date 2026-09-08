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
	{http.MethodGet, "/api/v1/images", "", store.RoleViewer},
	{http.MethodGet, "/api/v1/images/i1", "", store.RoleViewer},
	{http.MethodPost, "/api/v1/images/pull", `{"reference":"repo:tag"}`, store.RoleOperator},
	{http.MethodPost, "/api/v1/images/i1/tag", `{"repo":"repo","tag":"tag"}`, store.RoleOperator},
	{http.MethodDelete, "/api/v1/images/i1", "", store.RoleOperator},
	{http.MethodGet, "/api/v1/volumes", "", store.RoleViewer},
	{http.MethodPost, "/api/v1/volumes", `{"name":"data"}`, store.RoleOperator},
	{http.MethodGet, "/api/v1/volumes/data", "", store.RoleViewer},
	{http.MethodDelete, "/api/v1/volumes/data", "", store.RoleOperator},
	{http.MethodGet, "/api/v1/networks", "", store.RoleViewer},
	{http.MethodPost, "/api/v1/networks", `{"name":"edge"}`, store.RoleOperator},
	{http.MethodGet, "/api/v1/networks/n1", "", store.RoleViewer},
	{http.MethodDelete, "/api/v1/networks/n1", "", store.RoleOperator},
	{http.MethodPost, "/api/v1/networks/n1/connect", `{"container":"c1"}`, store.RoleOperator},
	{http.MethodPost, "/api/v1/networks/n1/disconnect", `{"container":"c1"}`, store.RoleOperator},
	{http.MethodPost, "/api/v1/prune/images", "", store.RoleAdmin},
}

func TestRoleMiddlewareMatrix(t *testing.T) {
	persistence := memstore.New()
	tokens, _, err := auth.LoadTokens(context.Background(), persistence, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(Config{Docker: newFakeDockerClient(), Store: persistence, AuthEnabled: true, Tokens: tokens})
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
	router := NewRouter(Config{Docker: newFakeDockerClient(), Store: persistence, AuthEnabled: true, Tokens: tokens})
	token, err := tokens.Issue(store.User{ID: 1, Username: "viewer", Role: store.RoleViewer})
	if err != nil {
		t.Fatal(err)
	}
	response := roleRequest(router, protectedRoute{method: http.MethodGet, path: "/api/v1/auth/me"}, token)
	assertJSON(t, response.Body.String(), `{"auth":true,"user":{"id":"1","username":"viewer","role":"viewer"},"capabilities":{"auth.logout":true,"auth.me":true,"auth.ws_ticket":true,"host.read":true,"containers.read":true,"containers.logs":true,"containers.stats":true,"containers.top":true,"containers.start":false,"containers.stop":false,"containers.restart":false,"containers.pause":false,"containers.unpause":false,"containers.kill":false,"containers.rename":false,"containers.remove":false,"images.read":true,"images.pull":false,"images.tag":false,"images.remove":false,"volumes.read":true,"volumes.create":false,"volumes.remove":false,"networks.read":true,"networks.create":false,"networks.remove":false,"networks.connect":false,"networks.disconnect":false,"prune.run":false}}`)

	response = roleRequest(NewRouter(Config{Docker: newFakeDockerClient()}), protectedRoute{method: http.MethodGet, path: "/api/v1/auth/me"}, "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"auth":false`) || !strings.Contains(response.Body.String(), `"role":"admin"`) || !strings.Contains(response.Body.String(), `"images.remove":true`) {
		t.Fatalf("auth-off /me = %d %s", response.Code, response.Body.String())
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
