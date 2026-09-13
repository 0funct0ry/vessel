package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/0funct0ry/vessel/internal/auth"
	"github.com/0funct0ry/vessel/internal/store"
	"github.com/0funct0ry/vessel/internal/store/memstore"
)

func TestUsersLastAdminGuard(t *testing.T) {
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

	loginRec := performRequestBody(router, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"password1"}`)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login: %d %s", loginRec.Code, loginRec.Body.String())
	}
	var payload struct {
		Token string       `json:"token"`
		User  userResponse `json:"user"`
	}
	if err := json.Unmarshal(loginRec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	adminID := strconv.FormatInt(payload.User.ID, 10)

	bearer := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Authorization", "Bearer "+payload.Token)
		router.ServeHTTP(rec, req)
		return rec
	}

	// Removing the only admin is refused.
	if rec := bearer(http.MethodDelete, "/api/v1/users/"+adminID, ""); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "last_admin") {
		t.Fatalf("delete last admin = %d %s", rec.Code, rec.Body.String())
	}
	// Demoting the only admin is refused.
	if rec := bearer(http.MethodPatch, "/api/v1/users/"+adminID, `{"role":"viewer"}`); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "last_admin") {
		t.Fatalf("demote last admin = %d %s", rec.Code, rec.Body.String())
	}

	// Add a second admin; now both operations succeed.
	if rec := bearer(http.MethodPost, "/api/v1/users", `{"username":"second","password":"password2","role":"admin"}`); rec.Code != http.StatusCreated {
		t.Fatalf("create second admin = %d %s", rec.Code, rec.Body.String())
	}
	if rec := bearer(http.MethodPatch, "/api/v1/users/"+adminID, `{"role":"viewer"}`); rec.Code != http.StatusOK {
		t.Fatalf("demote after second admin = %d %s", rec.Code, rec.Body.String())
	}
}

func TestUserPasswordSelfServiceAndAdminReset(t *testing.T) {
	s := memstore.New()
	adminHash, err := auth.HashPassword("password1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser(context.Background(), store.User{Username: "admin", PasswordHash: adminHash, Role: store.RoleAdmin, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	viewerHash, err := auth.HashPassword("viewerpass")
	if err != nil {
		t.Fatal(err)
	}
	viewer, err := s.CreateUser(context.Background(), store.User{Username: "viewer", PasswordHash: viewerHash, Role: store.RoleViewer, CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	tokens, _, err := auth.LoadTokens(context.Background(), s, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(Config{Docker: newFakeDockerClient(), Store: s, AuthEnabled: true, Tokens: tokens})

	adminToken := loginFor(t, router, "admin", "password1")
	viewerToken := loginFor(t, router, "viewer", "viewerpass")
	viewerID := strconv.FormatInt(viewer.ID, 10)

	send := func(bearer, method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+bearer)
		router.ServeHTTP(rec, req)
		return rec
	}

	// A non-admin cannot reset someone else's password.
	if rec := send(viewerToken, http.MethodPost, "/api/v1/users/"+viewerID+"/password", `{"password":"whatever1"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("non-admin reset = %d %s", rec.Code, rec.Body.String())
	}
	// The admin can reset the viewer's password without the old one.
	if rec := send(adminToken, http.MethodPost, "/api/v1/users/"+viewerID+"/password", `{"password":"newpassword1"}`); rec.Code != http.StatusNoContent {
		t.Fatalf("admin reset = %d %s", rec.Code, rec.Body.String())
	}
	// The viewer can change their own password given the current one.
	if rec := send(viewerToken, http.MethodPost, "/api/v1/users/"+viewerID+"/password", `{"current_password":"newpassword1","new_password":"anotherpass1"}`); rec.Code != http.StatusNoContent {
		t.Fatalf("self change = %d %s", rec.Code, rec.Body.String())
	}
	// Wrong current password is refused.
	if rec := send(viewerToken, http.MethodPost, "/api/v1/users/"+viewerID+"/password", `{"current_password":"wrong","new_password":"anotherpass2"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("wrong current password = %d %s", rec.Code, rec.Body.String())
	}
}

func loginFor(t *testing.T, router http.Handler, username, password string) string {
	t.Helper()
	rec := performRequestBody(router, http.MethodPost, "/api/v1/auth/login", `{"username":"`+username+`","password":"`+password+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("login %s: %d %s", username, rec.Code, rec.Body.String())
	}
	var payload struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	return payload.Token
}
