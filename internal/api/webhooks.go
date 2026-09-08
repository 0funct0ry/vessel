package api

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/0funct0ry/vessel/internal/store"
	"github.com/0funct0ry/vessel/internal/webhook"
	"github.com/gin-gonic/gin"
)

type webhookInput struct {
	Name        string   `json:"name"`
	URL         string   `json:"url"`
	Secret      *string  `json:"secret"`
	Enabled     *bool    `json:"enabled"`
	EventTypes  []string `json:"event_types"`
	Filters     []byte   `json:"filters"`
	Headers     []byte   `json:"headers"`
	MaxAttempts int      `json:"max_attempts"`
}
type webhookView struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	URL         string    `json:"url"`
	SecretSet   bool      `json:"secret_set"`
	Enabled     bool      `json:"enabled"`
	EventTypes  []string  `json:"event_types"`
	Filters     []byte    `json:"filters,omitempty"`
	Headers     []byte    `json:"headers,omitempty"`
	MaxAttempts int       `json:"max_attempts"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Dropped     uint64    `json:"webhook_dropped_total"`
}

func webView(w store.Webhook, dropped uint64) webhookView {
	return webhookView{w.ID, w.Name, w.URL, w.Secret != nil, w.Enabled, w.EventTypes, w.Filters, w.Headers, w.MaxAttempts, w.CreatedAt, w.UpdatedAt, dropped}
}
func webhookErr(c *gin.Context, err error) {
	if errors.Is(err, store.ErrNotFound) {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "webhook_not_found", "message": "webhook not found"}})
		return
	}
	Fail(c, err)
}
func validateWebhook(in webhookInput) error {
	if strings.TrimSpace(in.Name) == "" {
		return invalidInput("name is required")
	}
	u, err := url.ParseRequestURI(in.URL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return invalidInput("url must be an absolute HTTP URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return invalidInput("url must use http or https")
	}
	if len(in.EventTypes) == 0 {
		return invalidInput("event_types is required")
	}
	for _, p := range in.EventTypes {
		if p == "" || strings.Count(p, ".") > 1 {
			return invalidInput("invalid event type")
		}
	}
	if in.MaxAttempts < 0 || in.MaxAttempts > 20 {
		return invalidInput("max_attempts must be between 1 and 20")
	}
	return nil
}
func (s *server) dropped() uint64 {
	if s.webhooks == nil {
		return 0
	}
	return s.webhooks.Dropped()
}
func (s *server) handleWebhooks(c *gin.Context) {
	ws, err := s.store.ListWebhooks(c)
	if err != nil {
		Fail(c, err)
		return
	}
	out := make([]webhookView, 0, len(ws))
	for _, w := range ws {
		out = append(out, webView(w, s.dropped()))
	}
	c.JSON(http.StatusOK, gin.H{"webhooks": out})
}
func (s *server) handleWebhook(c *gin.Context) {
	w, err := s.store.GetWebhook(c, c.Param("id"))
	if err != nil {
		webhookErr(c, err)
		return
	}
	c.JSON(http.StatusOK, webView(w, s.dropped()))
}
func (s *server) handleWebhookCreate(c *gin.Context) {
	var in webhookInput
	if err := decodeBody(c, &in); err != nil {
		Fail(c, err)
		return
	}
	if err := validateWebhook(in); err != nil {
		Fail(c, err)
		return
	}
	now := time.Now().UTC()
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	w, err := s.store.CreateWebhook(c, store.Webhook{ID: webhookID(), Name: in.Name, URL: in.URL, Secret: in.Secret, Enabled: enabled, EventTypes: in.EventTypes, Filters: in.Filters, Headers: in.Headers, MaxAttempts: in.MaxAttempts, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		webhookErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, webView(w, s.dropped()))
}
func (s *server) handleWebhookUpdate(c *gin.Context) {
	w, err := s.store.GetWebhook(c, c.Param("id"))
	if err != nil {
		webhookErr(c, err)
		return
	}
	var in webhookInput
	if err := decodeBody(c, &in); err != nil {
		Fail(c, err)
		return
	}
	if in.Name != "" {
		w.Name = in.Name
	}
	if in.URL != "" {
		w.URL = in.URL
	}
	if in.Secret != nil {
		w.Secret = in.Secret
	}
	if in.Enabled != nil {
		w.Enabled = *in.Enabled
	}
	if in.EventTypes != nil {
		w.EventTypes = in.EventTypes
	}
	if in.Filters != nil {
		w.Filters = in.Filters
	}
	if in.Headers != nil {
		w.Headers = in.Headers
	}
	if in.MaxAttempts != 0 {
		w.MaxAttempts = in.MaxAttempts
	}
	if err := validateWebhook(webhookInput{Name: w.Name, URL: w.URL, EventTypes: w.EventTypes, MaxAttempts: w.MaxAttempts}); err != nil {
		Fail(c, err)
		return
	}
	w.UpdatedAt = time.Now().UTC()
	w, err = s.store.UpdateWebhook(c, w)
	if err != nil {
		webhookErr(c, err)
		return
	}
	c.JSON(http.StatusOK, webView(w, s.dropped()))
}
func (s *server) handleWebhookDelete(c *gin.Context) {
	if err := s.store.DeleteWebhook(c, c.Param("id")); err != nil {
		webhookErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
func (s *server) handleWebhookTest(c *gin.Context) {
	if s.webhooks == nil {
		Fail(c, invalidInput("webhook engine is unavailable"))
		return
	}
	if err := s.webhooks.TestWebhook(c, c.Param("id")); err != nil {
		webhookErr(c, err)
		return
	}
	c.Status(http.StatusAccepted)
}
func (s *server) handleDeliveries(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit < 1 || limit > 5000 {
		Fail(c, invalidQuery("limit must be between 1 and 5000"))
		return
	}
	ds, err := s.store.ListDeliveries(c, c.Param("id"), store.DeliveryQuery{Limit: limit, Cursor: c.Query("cursor")})
	if err != nil {
		webhookErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deliveries": ds})
}
func (s *server) handleRedeliver(c *gin.Context) {
	if s.webhooks == nil {
		Fail(c, invalidInput("webhook engine is unavailable"))
		return
	}
	if err := s.webhooks.Redeliver(c, c.Param("id")); err != nil {
		webhookErr(c, err)
		return
	}
	c.Status(http.StatusAccepted)
}
func webhookID() string { return "wh_" + strconv.FormatInt(time.Now().UnixNano(), 36) }

var _ = webhook.EventType
