package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/0funct0ry/vessel/internal/auth"
	"github.com/0funct0ry/vessel/internal/store"
)

type userDetailView struct {
	ID          int64      `json:"id"`
	Username    string     `json:"username"`
	Role        store.Role `json:"role"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
}

func toUserDetailView(u store.User) userDetailView {
	return userDetailView{ID: u.ID, Username: u.Username, Role: u.Role, CreatedAt: u.CreatedAt, LastLoginAt: u.LastLoginAt}
}

func userErr(c *gin.Context, err error) {
	if err == store.ErrNotFound {
		c.AbortWithStatusJSON(http.StatusNotFound, errorEnvelope{Error: errorBody{Code: "user_not_found", Message: "user not found"}})
		return
	}
	Fail(c, err)
}

// callerClaims reads the authenticated caller's claims. It is only called on
// routes roleMiddleware has already required auth for.
func callerClaims(c *gin.Context) (auth.Claims, bool) {
	v, ok := c.Get(claimsKey)
	if !ok {
		return auth.Claims{}, false
	}
	return v.(auth.Claims), true
}

// effectiveRole mirrors roleMiddleware's own fallback: no claims means auth is
// off, which the rest of the app already treats as an implicit admin.
func effectiveRole(c *gin.Context) store.Role {
	claims, ok := callerClaims(c)
	if !ok {
		return store.RoleAdmin
	}
	return claims.Role
}

func validRole(r store.Role) bool {
	switch r {
	case store.RoleAdmin, store.RoleOperator, store.RoleViewer:
		return true
	}
	return false
}

// guardLastAdmin refuses to remove or demote the only remaining admin, the
// RBAC-layer counterpart to GuardBind's network-layer self-lockout guard.
func guardLastAdmin(ctx context.Context, s store.Store, target store.User, removing bool, newRole store.Role) error {
	if target.Role != store.RoleAdmin {
		return nil
	}
	if !removing && newRole == store.RoleAdmin {
		return nil
	}
	users, err := s.ListUsers(ctx)
	if err != nil {
		return err
	}
	admins := 0
	for _, u := range users {
		if u.Role == store.RoleAdmin {
			admins++
		}
	}
	if admins <= 1 {
		return lastAdminError{}
	}
	return nil
}

func (s *server) handleUsers(c *gin.Context) {
	users, err := s.store.ListUsers(c.Request.Context())
	if err != nil {
		Fail(c, err)
		return
	}
	out := make([]userDetailView, 0, len(users))
	for _, u := range users {
		out = append(out, toUserDetailView(u))
	}
	c.JSON(http.StatusOK, gin.H{"users": out})
}

type userCreateInput struct {
	Username string     `json:"username"`
	Password string     `json:"password"`
	Role     store.Role `json:"role"`
}

func (s *server) handleUserCreate(c *gin.Context) {
	var in userCreateInput
	if err := decodeBody(c, &in); err != nil {
		Fail(c, err)
		return
	}
	if strings.TrimSpace(in.Username) == "" {
		Fail(c, invalidInput("username is required"))
		return
	}
	if len(in.Password) < auth.MinimumPassword {
		Fail(c, invalidInput("password must be at least %d characters", auth.MinimumPassword))
		return
	}
	if !validRole(in.Role) {
		Fail(c, invalidInput("role must be one of admin, operator, viewer"))
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		Fail(c, err)
		return
	}
	u, err := s.store.CreateUser(c.Request.Context(), store.User{Username: in.Username, PasswordHash: hash, Role: in.Role, CreatedAt: time.Now().UTC()})
	if err != nil {
		userErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, toUserDetailView(u))
}

type userUpdateInput struct {
	Role store.Role `json:"role"`
}

func (s *server) handleUserUpdate(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		Fail(c, invalidInput("invalid user id"))
		return
	}
	var in userUpdateInput
	if err := decodeBody(c, &in); err != nil {
		Fail(c, err)
		return
	}
	if !validRole(in.Role) {
		Fail(c, invalidInput("role must be one of admin, operator, viewer"))
		return
	}
	u, err := s.store.GetUser(c.Request.Context(), id)
	if err != nil {
		userErr(c, err)
		return
	}
	if err := guardLastAdmin(c.Request.Context(), s.store, u, false, in.Role); err != nil {
		Fail(c, err)
		return
	}
	u.Role = in.Role
	u, err = s.store.UpdateUser(c.Request.Context(), u)
	if err != nil {
		userErr(c, err)
		return
	}
	c.JSON(http.StatusOK, toUserDetailView(u))
}

func (s *server) handleUserDelete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		Fail(c, invalidInput("invalid user id"))
		return
	}
	u, err := s.store.GetUser(c.Request.Context(), id)
	if err != nil {
		userErr(c, err)
		return
	}
	if err := guardLastAdmin(c.Request.Context(), s.store, u, true, ""); err != nil {
		Fail(c, err)
		return
	}
	if err := s.store.DeleteUser(c.Request.Context(), id); err != nil {
		userErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

type userPasswordInput struct {
	// Admin reset path: set a user's password without knowing the old one.
	Password string `json:"password"`
	// Self-service path: change the caller's own password.
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (s *server) handleUserSetPassword(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		Fail(c, invalidInput("invalid user id"))
		return
	}
	var in userPasswordInput
	if err := decodeBody(c, &in); err != nil {
		Fail(c, err)
		return
	}
	claims, authed := callerClaims(c)
	callerID, _ := strconv.ParseInt(claims.Subject, 10, 64)
	selfService := in.CurrentPassword != "" || in.NewPassword != ""

	u, err := s.store.GetUser(c.Request.Context(), id)
	if err != nil {
		userErr(c, err)
		return
	}

	var newPassword string
	if selfService {
		if !authed || callerID != id {
			Fail(c, invalidInput("current_password can only be used to change your own password"))
			return
		}
		if !auth.VerifyPassword(u.PasswordHash, in.CurrentPassword) {
			Fail(c, invalidInput("current password is incorrect"))
			return
		}
		newPassword = in.NewPassword
	} else {
		// Admin reset of someone else's (or their own) password without the old one.
		if effectiveRole(c) != store.RoleAdmin {
			Fail(c, invalidInput("only an admin may reset another user's password"))
			return
		}
		newPassword = in.Password
	}
	if len(newPassword) < auth.MinimumPassword {
		Fail(c, invalidInput("password must be at least %d characters", auth.MinimumPassword))
		return
	}
	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		Fail(c, err)
		return
	}
	u.PasswordHash = hash
	if _, err := s.store.UpdateUser(c.Request.Context(), u); err != nil {
		userErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
