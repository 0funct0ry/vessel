package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/0funct0ry/vessel/internal/auth"
	"github.com/0funct0ry/vessel/internal/store"
)

// handleBootstrapStatus reports whether the first-run admin setup screen
// should be shown: true only while the user store is completely empty.
func (s *server) handleBootstrapStatus(c *gin.Context) {
	users, err := s.store.ListUsers(c.Request.Context())
	if err != nil {
		Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"available": len(users) == 0})
}

type bootstrapRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// handleBootstrap creates the first admin account when the store has no
// users yet, then signs the caller in. It is deliberately unauthenticated —
// reachable only in the narrow window before any user exists — and rechecks
// the store immediately before writing to close the race against a
// concurrent `vessel user add` or a second bootstrap request.
func (s *server) handleBootstrap(c *gin.Context) {
	if s.store == nil || s.tokens == nil {
		Fail(c, errInternal)
		return
	}
	var in bootstrapRequest
	if err := c.ShouldBindJSON(&in); err != nil || strings.TrimSpace(in.Username) == "" {
		Fail(c, invalidInput("username and password are required"))
		return
	}
	if len(in.Password) < auth.MinimumPassword {
		Fail(c, invalidInput("password must be at least %d characters", auth.MinimumPassword))
		return
	}

	users, err := s.store.ListUsers(c.Request.Context())
	if err != nil {
		Fail(c, err)
		return
	}
	if len(users) > 0 {
		c.AbortWithStatusJSON(http.StatusConflict, errorEnvelope{Error: errorBody{
			Code: "already_bootstrapped", Message: "an admin account already exists",
		}})
		return
	}

	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		Fail(c, err)
		return
	}
	u, err := s.store.CreateUser(c.Request.Context(), store.User{
		Username:     in.Username,
		PasswordHash: hash,
		Role:         store.RoleAdmin,
		CreatedAt:    time.Now().UTC(),
	})
	if err != nil {
		userErr(c, err)
		return
	}

	token, err := s.tokens.Issue(u)
	if err != nil {
		Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"token": token, "user": userView(u)})
}
