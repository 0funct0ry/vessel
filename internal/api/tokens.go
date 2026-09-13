package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oklog/ulid/v2"

	"github.com/0funct0ry/vessel/internal/auth"
	"github.com/0funct0ry/vessel/internal/store"
)

type tokenView struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

func toTokenView(t store.Token) tokenView {
	return tokenView{ID: t.ID, Name: t.Name, CreatedAt: t.CreatedAt, LastUsedAt: t.LastUsedAt, ExpiresAt: t.ExpiresAt}
}

func (s *server) callerUserID(c *gin.Context) (int64, bool) {
	claims, ok := callerClaims(c)
	if !ok {
		return 0, false
	}
	id, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}

func (s *server) handleTokens(c *gin.Context) {
	userID, ok := s.callerUserID(c)
	if !ok {
		Fail(c, invalidInput("authentication required"))
		return
	}
	tokens, err := s.store.ListTokensByUser(c.Request.Context(), userID)
	if err != nil {
		Fail(c, err)
		return
	}
	out := make([]tokenView, 0, len(tokens))
	for _, t := range tokens {
		out = append(out, toTokenView(t))
	}
	c.JSON(http.StatusOK, gin.H{"tokens": out})
}

type tokenCreateInput struct {
	Name          string `json:"name"`
	ExpiresInDays int    `json:"expires_in_days"`
}

func (s *server) handleTokenCreate(c *gin.Context) {
	userID, ok := s.callerUserID(c)
	if !ok {
		Fail(c, invalidInput("authentication required"))
		return
	}
	var in tokenCreateInput
	if err := decodeBody(c, &in); err != nil {
		Fail(c, err)
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		Fail(c, invalidInput("name is required"))
		return
	}
	if in.ExpiresInDays < 0 {
		Fail(c, invalidInput("expires_in_days must not be negative"))
		return
	}
	raw, hash, err := auth.GenerateToken()
	if err != nil {
		Fail(c, err)
		return
	}
	now := time.Now().UTC()
	var expiresAt *time.Time
	if in.ExpiresInDays > 0 {
		t := now.AddDate(0, 0, in.ExpiresInDays)
		expiresAt = &t
	}
	tok := store.Token{ID: ulid.Make().String(), UserID: userID, Name: in.Name, Hash: hash, CreatedAt: now, ExpiresAt: expiresAt}
	tok, err = s.store.CreateToken(c.Request.Context(), tok)
	if err != nil {
		Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"token": raw, "id": tok.ID, "name": tok.Name, "created_at": tok.CreatedAt, "expires_at": tok.ExpiresAt})
}

func (s *server) handleTokenDelete(c *gin.Context) {
	userID, ok := s.callerUserID(c)
	if !ok {
		Fail(c, invalidInput("authentication required"))
		return
	}
	if err := s.store.DeleteToken(c.Request.Context(), c.Param("id"), userID); err != nil {
		if err == store.ErrNotFound {
			c.AbortWithStatusJSON(http.StatusNotFound, errorEnvelope{Error: errorBody{Code: "token_not_found", Message: "token not found"}})
			return
		}
		Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
