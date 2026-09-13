package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/0funct0ry/vessel/internal/auth"
	"github.com/0funct0ry/vessel/internal/store"
	"github.com/0funct0ry/vessel/internal/store/memstore"
	"github.com/0funct0ry/vessel/internal/store/sqlitestore"
)

func testBootstrapStores(t *testing.T) map[string]store.Store {
	t.Helper()
	sqlite, err := sqlitestore.Open(filepath.Join(t.TempDir(), "vessel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlite.Close() })
	return map[string]store.Store{
		"memstore":    memstore.New(),
		"sqlitestore": sqlite,
	}
}

func TestBootstrapStatusAndCreate(t *testing.T) {
	for name, persistence := range testBootstrapStores(t) {
		t.Run(name, func(t *testing.T) {
			tokens, _, err := auth.LoadTokens(context.Background(), persistence, time.Hour)
			if err != nil {
				t.Fatal(err)
			}
			router := NewRouter(Config{Docker: newFakeDockerClient(), Store: persistence, AuthEnabled: true, Tokens: tokens})

			// Empty store: bootstrap is available.
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/bootstrap", nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("status GET#1=%d body=%s", rec.Code, rec.Body.String())
			}
			var status struct {
				Available bool `json:"available"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
				t.Fatal(err)
			}
			if !status.Available {
				t.Fatalf("expected available=true on empty store")
			}

			// Create the first admin via bootstrap.
			body, _ := json.Marshal(map[string]string{"username": "alice", "password": "supersecret"})
			rec = httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/bootstrap", bytes.NewReader(body)))
			if rec.Code != http.StatusCreated {
				t.Fatalf("status POST#1=%d body=%s", rec.Code, rec.Body.String())
			}
			var created struct {
				Token string `json:"token"`
				User  struct {
					Username string     `json:"username"`
					Role     store.Role `json:"role"`
				} `json:"user"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
				t.Fatal(err)
			}
			if created.Token == "" {
				t.Fatalf("expected a session token in bootstrap response")
			}
			if created.User.Username != "alice" || created.User.Role != store.RoleAdmin {
				t.Fatalf("unexpected user in bootstrap response: %+v", created.User)
			}

			// Status now reports unavailable.
			rec = httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/bootstrap", nil))
			status.Available = true
			if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
				t.Fatal(err)
			}
			if status.Available {
				t.Fatalf("expected available=false once a user exists")
			}

			// A second bootstrap attempt is rejected with 409, regardless of role requested.
			body, _ = json.Marshal(map[string]string{"username": "mallory", "password": "supersecret"})
			rec = httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/bootstrap", bytes.NewReader(body)))
			if rec.Code != http.StatusConflict {
				t.Fatalf("status POST#2=%d body=%s", rec.Code, rec.Body.String())
			}

			users, err := persistence.ListUsers(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(users) != 1 {
				t.Fatalf("expected exactly one user after a rejected second bootstrap, got %d", len(users))
			}
		})
	}
}
