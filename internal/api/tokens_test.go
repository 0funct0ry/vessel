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

func TestTokenLifecycle(t *testing.T) {
	s := memstore.New()
	hash, err := auth.HashPassword("password1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser(context.Background(), store.User{Username: "admin", PasswordHash: hash, Role: store.RoleAdmin, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	tokens, _, err := auth.LoadTokens(context.Background(), s, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(Config{Docker: newFakeDockerClient(), Store: s, AuthEnabled: true, Tokens: tokens})
	sessionToken := loginFor(t, router, "admin", "password1")

	bearer := func(token, method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Authorization", "Bearer "+token)
		router.ServeHTTP(rec, req)
		return rec
	}

	createRec := bearer(sessionToken, http.MethodPost, "/api/v1/tokens", `{"name":"ci"}`)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create token = %d %s", createRec.Code, createRec.Body.String())
	}
	var created struct {
		Token string `json:"token"`
		ID    string `json:"id"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(created.Token, auth.TokenPrefix) {
		t.Fatalf("token missing prefix: %s", created.Token)
	}

	// The stored token is hashed, never the raw value.
	stored, err := s.GetTokenByHash(context.Background(), auth.HashToken(created.Token))
	if err != nil {
		t.Fatal(err)
	}
	if stored.Hash == created.Token {
		t.Fatal("token stored in plaintext")
	}

	// The raw token authenticates like a session token.
	if rec := bearer(created.Token, http.MethodGet, "/api/v1/tokens", ""); rec.Code != http.StatusOK {
		t.Fatalf("authenticate with token = %d %s", rec.Code, rec.Body.String())
	}

	// Revoke it, then it no longer authenticates.
	if rec := bearer(sessionToken, http.MethodDelete, "/api/v1/tokens/"+created.ID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("revoke = %d %s", rec.Code, rec.Body.String())
	}
	if rec := bearer(created.Token, http.MethodGet, "/api/v1/tokens", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked token still authenticates: %d %s", rec.Code, rec.Body.String())
	}
}

func TestExpiredPersonalTokenRejected(t *testing.T) {
	s := memstore.New()
	hash, err := auth.HashPassword("password1")
	if err != nil {
		t.Fatal(err)
	}
	u, err := s.CreateUser(context.Background(), store.User{Username: "admin", PasswordHash: hash, Role: store.RoleAdmin, CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	tokens, _, err := auth.LoadTokens(context.Background(), s, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	raw, tokenHash, err := auth.GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour)
	if _, err := s.CreateToken(context.Background(), store.Token{ID: "tok_expired", UserID: u.ID, Name: "old", Hash: tokenHash, CreatedAt: time.Now().Add(-2 * time.Hour), ExpiresAt: &past}); err != nil {
		t.Fatal(err)
	}
	router := NewRouter(Config{Docker: newFakeDockerClient(), Store: s, AuthEnabled: true, Tokens: tokens})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tokens", nil)
	req.Header.Set("Authorization", "Bearer "+raw)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "token_expired") {
		t.Fatalf("expired token = %d %s", rec.Code, rec.Body.String())
	}
}
