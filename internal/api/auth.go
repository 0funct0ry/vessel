package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/0funct0ry/vessel/internal/auth"
	"github.com/0funct0ry/vessel/internal/store"
)

const claimsKey = "auth_claims"

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}
type userResponse struct {
	ID       int64      `json:"id"`
	Username string     `json:"username"`
	Role     store.Role `json:"role"`
}

func userView(u store.User) userResponse {
	return userResponse{ID: u.ID, Username: u.Username, Role: u.Role}
}
func authFailure(c *gin.Context, code string) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, errorEnvelope{Error: errorBody{Code: code, Message: "authentication required"}})
}

func (s *server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.tokens == nil {
			authFailure(c, "invalid_token")
			return
		}
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") || strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")) == "" {
			authFailure(c, "invalid_token")
			return
		}
		claims, err := s.tokens.Parse(strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")))
		if errors.Is(err, auth.ErrExpiredToken) {
			authFailure(c, "token_expired")
			return
		}
		if err != nil {
			authFailure(c, "invalid_token")
			return
		}
		c.Set(claimsKey, claims)
		c.Next()
	}
}

func (s *server) handleLogin(c *gin.Context) {
	var input loginRequest
	if err := c.ShouldBindJSON(&input); err != nil || strings.TrimSpace(input.Username) == "" || input.Password == "" {
		Fail(c, invalidInput("username and password are required"))
		return
	}
	if delay := s.throttle.Delay(input.Username); delay > 0 {
		time.Sleep(delay)
	}
	// Always run bcrypt verification, including for unknown names, to reduce account enumeration timing signals.
	if s.store == nil || s.tokens == nil {
		Fail(c, errInternal)
		return
	}
	u, err := s.store.GetUserByUsername(c.Request.Context(), input.Username)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		Fail(c, err)
		return
	}
	valid := err == nil && auth.VerifyPassword(u.PasswordHash, input.Password)
	if err != nil && errors.Is(err, store.ErrNotFound) {
		_, _ = auth.HashPassword("constant-time-placeholder")
	}
	if !valid {
		s.throttle.Failure(input.Username)
		c.AbortWithStatusJSON(http.StatusUnauthorized, errorEnvelope{Error: errorBody{Code: "invalid_credentials", Message: "invalid username or password"}})
		return
	}
	s.throttle.Success(input.Username)
	now := time.Now().UTC()
	u.LastLoginAt = &now
	if _, err = s.store.UpdateUser(c.Request.Context(), u); err != nil {
		Fail(c, err)
		return
	}
	token, err := s.tokens.Issue(u)
	if err != nil {
		Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token, "user": userView(u)})
}
func (s *server) handleLogout(c *gin.Context) { c.Status(http.StatusNoContent) }
func (s *server) handleMe(c *gin.Context) {
	value, ok := c.Get(claimsKey)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"auth": false, "user": gin.H{"role": store.RoleAdmin}})
		return
	}
	claims := value.(auth.Claims)
	c.JSON(http.StatusOK, gin.H{"user": gin.H{"id": claims.Subject, "username": claims.Name, "role": claims.Role}})
}
func (s *server) handleWSTicket(c *gin.Context) {
	value, ok := c.Get(claimsKey)
	if !ok {
		authFailure(c, "invalid_token")
		return
	}
	claims := value.(auth.Claims)
	ticket, err := s.tickets.Issue(claims)
	if err != nil {
		Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ticket": ticket, "expires_in": 30})
}
