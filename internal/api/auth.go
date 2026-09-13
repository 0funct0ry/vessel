package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

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

func (s *server) roleMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := strings.TrimPrefix(c.Request.URL.Path, s.basePath)
		required, protected := auth.RequiredRole(c.Request.Method, path)
		if !protected {
			c.Next()
			return
		}

		actual := store.RoleAdmin
		if value, ok := c.Get(claimsKey); ok {
			actual = value.(auth.Claims).Role
		}
		if !auth.Allows(actual, required) {
			c.AbortWithStatusJSON(http.StatusForbidden, errorEnvelope{Error: errorBody{
				Code: "forbidden_role", Message: "insufficient role", Required: required, Actual: actual,
			}})
			return
		}
		c.Next()
	}
}

func (s *server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.tokens == nil {
			authFailure(c, "invalid_token")
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer "))
		// Native EventSource cannot send Authorization headers. Limit the URL-token
		// escape hatch to read-only SSE routes; normal API requests remain header-only.
		if token == "" && c.Request.Method == http.MethodGet && isSSERoute(c.Request.URL.Path) {
			token = c.Query("token")
		}
		if token == "" {
			authFailure(c, "invalid_token")
			return
		}
		if auth.IsToken(token) {
			claims, err := s.authenticateToken(c, token)
			if err != nil {
				if errors.Is(err, auth.ErrExpiredToken) {
					authFailure(c, "token_expired")
					return
				}
				authFailure(c, "invalid_token")
				return
			}
			c.Set(claimsKey, claims)
			c.Next()
			return
		}
		claims, err := s.tokens.Parse(token)
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

// authenticateToken resolves a personal API token to the same auth.Claims
// shape a session JWT produces, so downstream handlers and roleMiddleware
// don't need to know which credential was used.
func (s *server) authenticateToken(c *gin.Context, raw string) (auth.Claims, error) {
	tok, err := s.store.GetTokenByHash(c.Request.Context(), auth.HashToken(raw))
	if err != nil {
		return auth.Claims{}, auth.ErrInvalidToken
	}
	if tok.ExpiresAt != nil && time.Now().UTC().After(*tok.ExpiresAt) {
		return auth.Claims{}, auth.ErrExpiredToken
	}
	u, err := s.store.GetUser(c.Request.Context(), tok.UserID)
	if err != nil {
		return auth.Claims{}, auth.ErrInvalidToken
	}
	now := time.Now().UTC()
	_ = s.store.TouchTokenLastUsed(c.Request.Context(), tok.ID, now)
	return auth.Claims{Name: u.Username, Role: u.Role, RegisteredClaims: jwt.RegisteredClaims{Subject: fmt.Sprint(u.ID)}}, nil
}

func isSSERoute(path string) bool {
	return strings.HasSuffix(path, "/logs") || strings.HasSuffix(path, "/stats") || strings.HasSuffix(path, "/events")
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
		c.JSON(http.StatusOK, gin.H{"auth": false, "user": gin.H{"role": store.RoleAdmin}, "capabilities": s.capabilities(store.RoleAdmin)})
		return
	}
	claims := value.(auth.Claims)
	c.JSON(http.StatusOK, gin.H{"auth": true, "user": gin.H{"id": claims.Subject, "username": claims.Name, "role": claims.Role}, "capabilities": s.capabilities(claims.Role)})
}

func (s *server) capabilities(role store.Role) map[string]bool {
	capabilities := auth.Capabilities(role)
	if !s.execOn {
		delete(capabilities, "containers.exec")
		delete(capabilities, "containers.files.list")
		delete(capabilities, "containers.files.mkdir")
		delete(capabilities, "containers.files.delete")
		delete(capabilities, "containers.files.rename")
		delete(capabilities, "volumes.files.list")
		delete(capabilities, "volumes.files.mkdir")
		delete(capabilities, "volumes.files.delete")
		delete(capabilities, "volumes.files.rename")
		delete(capabilities, "volumes.clone")
	}
	return capabilities
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
