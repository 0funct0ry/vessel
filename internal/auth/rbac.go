// Package auth contains authentication and authorization primitives.
package auth

import (
	"net/http"
	"strings"

	"github.com/0funct0ry/vessel/internal/store"
)

// Policy describes the minimum role required to call one API route. Policies is
// the single source of truth for Vessel's flat-role authorization model.
type Policy struct {
	Method      string
	Path        string
	Role        store.Role
	Capability  string
	Implemented bool
}

// Policies mirrors the authorization annotations in SPEC §5.1. Implemented
// distinguishes routes that are already registered from later-milestone routes;
// the latter intentionally do not appear in today's capability response.
var Policies = []Policy{
	{http.MethodPost, "/api/v1/auth/logout", store.RoleViewer, "auth.logout", true},
	{http.MethodGet, "/api/v1/auth/me", store.RoleViewer, "auth.me", true},
	{http.MethodPost, "/api/v1/auth/ws-ticket", store.RoleViewer, "auth.ws_ticket", true},
	{http.MethodGet, "/api/v1/host", store.RoleViewer, "host.read", true},
	{http.MethodGet, "/api/v1/events", store.RoleViewer, "events.read", false},
	{http.MethodGet, "/api/v1/containers", store.RoleViewer, "containers.read", true},
	{http.MethodGet, "/api/v1/containers/:id", store.RoleViewer, "containers.read", true},
	{http.MethodGet, "/api/v1/containers/:id/logs", store.RoleViewer, "containers.logs", true},
	{http.MethodGet, "/api/v1/containers/:id/stats", store.RoleViewer, "containers.stats", true},
	{http.MethodGet, "/api/v1/containers/:id/top", store.RoleViewer, "containers.top", true},
	{http.MethodPost, "/api/v1/containers/:id/start", store.RoleOperator, "containers.start", true},
	{http.MethodPost, "/api/v1/containers/:id/stop", store.RoleOperator, "containers.stop", true},
	{http.MethodPost, "/api/v1/containers/:id/restart", store.RoleOperator, "containers.restart", true},
	{http.MethodPost, "/api/v1/containers/:id/pause", store.RoleOperator, "containers.pause", true},
	{http.MethodPost, "/api/v1/containers/:id/unpause", store.RoleOperator, "containers.unpause", true},
	{http.MethodPost, "/api/v1/containers/:id/kill", store.RoleOperator, "containers.kill", true},
	{http.MethodPost, "/api/v1/containers/:id/rename", store.RoleOperator, "containers.rename", true},
	{http.MethodDelete, "/api/v1/containers/:id", store.RoleOperator, "containers.remove", true},
	{http.MethodPost, "/api/v1/containers", store.RoleOperator, "containers.create", true},
	{http.MethodGet, "/api/v1/containers/:id/files", store.RoleOperator, "containers.files.list", true},
	{http.MethodPost, "/api/v1/containers/:id/files", store.RoleOperator, "containers.files.upload", true},
	{http.MethodPost, "/api/v1/containers/:id/folders", store.RoleOperator, "containers.files.mkdir", true},
	{http.MethodGet, "/api/v1/containers/:id/files/download", store.RoleOperator, "containers.files.download", true},
	{http.MethodDelete, "/api/v1/containers/:id/files", store.RoleOperator, "containers.files.delete", true},
	{http.MethodPost, "/api/v1/containers/:id/files/rename", store.RoleOperator, "containers.files.rename", true},
	{http.MethodGet, "/api/v1/containers/:id/files/view", store.RoleOperator, "containers.files.view", true},
	{http.MethodPut, "/api/v1/containers/:id/files/content", store.RoleOperator, "containers.files.edit", true},
	{http.MethodGet, "/api/v1/containers/:id/exec", store.RoleOperator, "containers.exec", true},
	{http.MethodGet, "/api/v1/images", store.RoleViewer, "images.read", true},
	{http.MethodGet, "/api/v1/images/export", store.RoleOperator, "images.export", true},
	{http.MethodPost, "/api/v1/images/import", store.RoleOperator, "images.import", true},
	{http.MethodPost, "/api/v1/images/build", store.RoleOperator, "images.build", true},
	{http.MethodGet, "/api/v1/images/:id", store.RoleViewer, "images.read", true},
	{http.MethodGet, "/api/v1/images/:id/history", store.RoleViewer, "images.history", true},
	{http.MethodPost, "/api/v1/images/pull", store.RoleOperator, "images.pull", true},
	{http.MethodPost, "/api/v1/images/:id/tag", store.RoleOperator, "images.tag", true},
	{http.MethodDelete, "/api/v1/images/:id", store.RoleOperator, "images.remove", true},
	{http.MethodGet, "/api/v1/volumes", store.RoleViewer, "volumes.read", true},
	{http.MethodPost, "/api/v1/volumes", store.RoleOperator, "volumes.create", true},
	{http.MethodGet, "/api/v1/volumes/:name", store.RoleViewer, "volumes.read", true},
	{http.MethodDelete, "/api/v1/volumes/:name", store.RoleOperator, "volumes.remove", true},
	{http.MethodGet, "/api/v1/networks", store.RoleViewer, "networks.read", true},
	{http.MethodPost, "/api/v1/networks", store.RoleOperator, "networks.create", true},
	{http.MethodGet, "/api/v1/networks/:id", store.RoleViewer, "networks.read", true},
	{http.MethodDelete, "/api/v1/networks/:id", store.RoleOperator, "networks.remove", true},
	{http.MethodPost, "/api/v1/networks/:id/connect", store.RoleOperator, "networks.connect", true},
	{http.MethodPost, "/api/v1/networks/:id/disconnect", store.RoleOperator, "networks.disconnect", true},
	{http.MethodPost, "/api/v1/prune/:kind", store.RoleAdmin, "prune.run", true},
	{http.MethodGet, "/api/v1/webhooks", store.RoleAdmin, "webhooks.read", false},
	{http.MethodPost, "/api/v1/webhooks", store.RoleAdmin, "webhooks.create", false},
	{http.MethodGet, "/api/v1/webhooks/:id", store.RoleAdmin, "webhooks.read", false},
	{http.MethodPatch, "/api/v1/webhooks/:id", store.RoleAdmin, "webhooks.update", false},
	{http.MethodDelete, "/api/v1/webhooks/:id", store.RoleAdmin, "webhooks.remove", false},
	{http.MethodPost, "/api/v1/webhooks/:id/test", store.RoleAdmin, "webhooks.test", false},
	{http.MethodGet, "/api/v1/webhooks/:id/deliveries", store.RoleAdmin, "webhooks.deliveries", false},
	{http.MethodPost, "/api/v1/deliveries/:id/redeliver", store.RoleAdmin, "deliveries.redeliver", false},
	{http.MethodGet, "/api/v1/users", store.RoleAdmin, "users.read", false},
	{http.MethodPost, "/api/v1/users", store.RoleAdmin, "users.create", false},
	{http.MethodPatch, "/api/v1/users/:id", store.RoleAdmin, "users.update", false},
	{http.MethodDelete, "/api/v1/users/:id", store.RoleAdmin, "users.remove", false},
}

// RequiredRole returns the role policy for method and concrete or Gin-style
// route path. The bool is false for public and unregistered routes.
func RequiredRole(method, path string) (store.Role, bool) {
	for _, policy := range Policies {
		if policy.Method == method && pathMatches(policy.Path, path) {
			return policy.Role, true
		}
	}
	return "", false
}

// Allows reports whether actual meets required. Unknown roles have no access.
func Allows(actual, required store.Role) bool {
	return roleRank(actual) >= roleRank(required) && roleRank(actual) > 0
}

// Capabilities returns the action map for routes currently registered by the API.
func Capabilities(role store.Role) map[string]bool {
	capabilities := make(map[string]bool)
	for _, policy := range Policies {
		if policy.Implemented && policy.Capability != "" {
			capabilities[policy.Capability] = Allows(role, policy.Role)
		}
	}
	return capabilities
}

func roleRank(role store.Role) int {
	switch role {
	case store.RoleViewer:
		return 1
	case store.RoleOperator:
		return 2
	case store.RoleAdmin:
		return 3
	default:
		return 0
	}
}

func pathMatches(pattern, path string) bool {
	patternParts := strings.Split(strings.Trim(pattern, "/"), "/")
	pathParts := strings.Split(strings.Trim(path, "/"), "/")
	if len(patternParts) != len(pathParts) {
		return false
	}
	for i, part := range patternParts {
		if strings.HasPrefix(part, ":") {
			if pathParts[i] == "" {
				return false
			}
			continue
		}
		if part != pathParts[i] {
			return false
		}
	}
	return true
}
