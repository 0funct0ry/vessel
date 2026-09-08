package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/0funct0ry/vessel/internal/auth"
	"github.com/0funct0ry/vessel/internal/store"
	"github.com/0funct0ry/vessel/internal/store/memstore"
)

func authenticatedRouter(t *testing.T) (http.Handler, *auth.Tokens) {
	t.Helper()
	s := memstore.New()
	hash, err := auth.HashPassword("password1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.CreateUser(context.Background(), store.User{Username: "alice", PasswordHash: hash, Role: store.RoleAdmin, CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	tokens, _, err := auth.LoadTokens(context.Background(), s, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return NewRouter(Config{Docker: newFakeDockerClient(), Store: s, AuthEnabled: true, Tokens: tokens}), tokens
}
func TestAuthenticationRoutesAndMiddleware(t *testing.T) {
	router, _ := authenticatedRouter(t)
	if r := performRequest(router, http.MethodGet, "/api/v1/health"); r.Code != http.StatusOK {
		t.Fatalf("health=%d", r.Code)
	}
	if r := performRequest(router, http.MethodGet, "/api/v1/containers"); r.Code != http.StatusUnauthorized {
		t.Fatalf("unauthed=%d", r.Code)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"alice","password":"password1"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login=%d %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+payload.Token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("me=%d %s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/ws-ticket", nil)
	req.Header.Set("Authorization", "Bearer "+payload.Token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ticket=%d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+payload.Token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout=%d", rec.Code)
	}
}
func TestExpiredTokenHasStableError(t *testing.T) {
	router, tokens := authenticatedRouter(t)
	now := time.Now()
	tokens.Now = func() time.Time { return now }
	raw, err := tokens.Issue(store.User{ID: 1, Username: "alice", Role: store.RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Hour)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/containers", nil)
	req.Header.Set("Authorization", "Bearer "+raw)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != 401 || !strings.Contains(rec.Body.String(), "token_expired") {
		t.Fatalf("response=%d %s", rec.Code, rec.Body.String())
	}
}
