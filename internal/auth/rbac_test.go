package auth

import (
	"net/http"
	"testing"

	"github.com/0funct0ry/vessel/internal/store"
)

func TestRequiredRole(t *testing.T) {
	tests := []struct {
		method, path string
		want         store.Role
		found        bool
	}{
		{http.MethodGet, "/api/v1/containers/abc/logs", store.RoleViewer, true},
		{http.MethodPost, "/api/v1/containers/abc/stop", store.RoleOperator, true},
		{http.MethodGet, "/api/v1/images/export", store.RoleOperator, true},
		{http.MethodPost, "/api/v1/images/import", store.RoleOperator, true},
		{http.MethodPost, "/api/v1/prune/images", store.RoleAdmin, true},
		{http.MethodGet, "/api/v1/health", "", false},
		{http.MethodPut, "/api/v1/containers/abc", "", false},
	}
	for _, test := range tests {
		t.Run(test.method+test.path, func(t *testing.T) {
			got, found := RequiredRole(test.method, test.path)
			if got != test.want || found != test.found {
				t.Fatalf("RequiredRole(%s, %s) = %q, %t; want %q, %t", test.method, test.path, got, found, test.want, test.found)
			}
		})
	}
}

func TestAllowsAndCapabilities(t *testing.T) {
	if !Allows(store.RoleAdmin, store.RoleOperator) || !Allows(store.RoleOperator, store.RoleViewer) || Allows(store.RoleViewer, store.RoleOperator) || Allows(store.Role("other"), store.RoleViewer) {
		t.Fatal("unexpected role ordering")
	}
	viewer := Capabilities(store.RoleViewer)
	if viewer["containers.stop"] {
		t.Fatal("viewer unexpectedly can stop containers")
	}
	if !viewer["containers.read"] || viewer["images.remove"] {
		t.Fatalf("viewer capabilities = %#v", viewer)
	}
	operator := Capabilities(store.RoleOperator)
	if !operator["containers.stop"] || operator["prune.run"] {
		t.Fatalf("operator capabilities = %#v", operator)
	}
	if !operator["images.export"] || !operator["images.import"] || viewer["images.export"] || viewer["images.import"] {
		t.Fatalf("image archive capabilities incorrect: operator=%#v viewer=%#v", operator, viewer)
	}
	admin := Capabilities(store.RoleAdmin)
	if !admin["prune.run"] || !admin["images.remove"] {
		t.Fatalf("admin capabilities = %#v", admin)
	}
	if _, exists := admin["webhooks.create"]; exists {
		t.Fatal("future route leaked into current capabilities")
	}
}

func TestPolicyRoleMatrix(t *testing.T) {
	roles := []store.Role{store.RoleViewer, store.RoleOperator, store.RoleAdmin}
	for _, policy := range Policies {
		for _, actual := range roles {
			want := roleRank(actual) >= roleRank(policy.Role)
			if got := Allows(actual, policy.Role); got != want {
				t.Fatalf("%s %s as %s: allows=%t, want %t", policy.Method, policy.Path, actual, got, want)
			}
			if !Allows(store.RoleAdmin, policy.Role) {
				t.Fatalf("auth-off admin denied %s %s", policy.Method, policy.Path)
			}
		}
	}
}
