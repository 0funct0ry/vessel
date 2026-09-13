package api

import (
	"encoding/json"
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
	Name        string          `json:"name"`
	URL         string          `json:"url"`
	Secret      *string         `json:"secret"`
	Enabled     *bool           `json:"enabled"`
	EventTypes  []string        `json:"event_types"`
	Filters     json.RawMessage `json:"filters"`
	Headers     json.RawMessage `json:"headers"`
	MaxAttempts int             `json:"max_attempts"`
}
type lastDeliveryView struct {
	Status    store.DeliveryStatus `json:"status"`
	CreatedAt time.Time            `json:"created_at"`
}
type stats24hView struct {
	Sent        int     `json:"sent"`
	Failed      int     `json:"failed"`
	SuccessRate float64 `json:"success_rate"`
}
type webhookView struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	URL          string            `json:"url"`
	SecretSet    bool              `json:"secret_set"`
	Enabled      bool              `json:"enabled"`
	EventTypes   []string          `json:"event_types"`
	Filters      json.RawMessage   `json:"filters,omitempty"`
	Headers      json.RawMessage   `json:"headers,omitempty"`
	MaxAttempts  int               `json:"max_attempts"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	Dropped      uint64            `json:"webhook_dropped_total"`
	LastDelivery *lastDeliveryView `json:"last_delivery,omitempty"`
	Stats24h     stats24hView      `json:"stats_24h"`
}

func webView(w store.Webhook, dropped uint64) webhookView {
	return webhookView{
		ID: w.ID, Name: w.Name, URL: w.URL, SecretSet: w.Secret != nil, Enabled: w.Enabled,
		EventTypes: w.EventTypes, Filters: w.Filters, Headers: w.Headers, MaxAttempts: w.MaxAttempts,
		CreatedAt: w.CreatedAt, UpdatedAt: w.UpdatedAt, Dropped: dropped,
	}
}

// reservedHeaders are set by the engine itself; a webhook cannot override them.
func reservedHeader(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	return n == "user-agent" || strings.HasPrefix(n, "x-vessel-")
}
func validateHeaders(raw []byte) error {
	if len(raw) == 0 {
		return nil
	}
	var headers map[string]string
	if err := json.Unmarshal(raw, &headers); err != nil {
		return invalidInput("headers must be a JSON object of header name to value")
	}
	for name := range headers {
		if reservedHeader(name) {
			return reservedHeaderErr(name)
		}
	}
	return nil
}
func reservedHeaderErr(name string) error {
	return &validationError{code: "reserved_header", message: "header " + name + " is reserved and set by the webhook engine"}
}

// statsWindow computes the trailing-24h delivery stats and the most recent
// delivery, from the same bounded delivery log the delivery-log UI reads
// (§6 already caps rows per webhook, so this stays a bounded scan).
func (s *server) statsWindow(c *gin.Context, webhookID string, now time.Time) (stats24hView, *lastDeliveryView) {
	ds, err := s.store.ListDeliveries(c, webhookID, store.DeliveryQuery{Limit: 5000})
	if err != nil || len(ds) == 0 {
		return stats24hView{}, nil
	}
	last := lastDeliveryView{Status: ds[0].Status, CreatedAt: ds[0].CreatedAt}
	cutoff := now.Add(-24 * time.Hour)
	var sent, failed int
	for _, d := range ds {
		if d.CreatedAt.Before(cutoff) {
			continue
		}
		switch d.Status {
		case store.DeliverySuccess:
			sent++
		case store.DeliveryFailed, store.DeliveryDead:
			sent++
			failed++
		}
	}
	stats := stats24hView{Sent: sent, Failed: failed}
	if sent > 0 {
		stats.SuccessRate = float64(sent-failed) / float64(sent)
	}
	return stats, &last
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
	if in.MaxAttempts != 0 && (in.MaxAttempts < 1 || in.MaxAttempts > 10) {
		return invalidInput("max_attempts must be between 1 and 10")
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
	now := time.Now().UTC()
	out := make([]webhookView, 0, len(ws))
	for _, w := range ws {
		v := webView(w, s.dropped())
		v.Stats24h, v.LastDelivery = s.statsWindow(c, w.ID, now)
		out = append(out, v)
	}
	c.JSON(http.StatusOK, gin.H{"webhooks": out})
}
func (s *server) handleWebhook(c *gin.Context) {
	w, err := s.store.GetWebhook(c, c.Param("id"))
	if err != nil {
		webhookErr(c, err)
		return
	}
	v := webView(w, s.dropped())
	v.Stats24h, v.LastDelivery = s.statsWindow(c, w.ID, time.Now().UTC())
	c.JSON(http.StatusOK, v)
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
	if err := validateHeaders(in.Headers); err != nil {
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
	if err := validateHeaders(w.Headers); err != nil {
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

type deliveryView struct {
	ID           string               `json:"id"`
	WebhookID    string               `json:"webhook_id"`
	EventID      string               `json:"event_id"`
	Payload      json.RawMessage      `json:"payload"`
	Attempt      int                  `json:"attempt"`
	Status       store.DeliveryStatus `json:"status"`
	StatusCode   *int                 `json:"status_code,omitempty"`
	ResponseMS   *int                 `json:"response_ms,omitempty"`
	ResponseBody *string              `json:"response_body,omitempty"`
	Error        *string              `json:"error,omitempty"`
	CreatedAt    time.Time            `json:"created_at"`
	NextRetryAt  *time.Time           `json:"next_retry_at,omitempty"`
}

func deliveryViewOf(d store.Delivery) deliveryView {
	return deliveryView{
		ID: d.ID, WebhookID: d.WebhookID, EventID: d.EventID, Payload: json.RawMessage(d.Payload),
		Attempt: d.Attempt, Status: d.Status, StatusCode: d.StatusCode, ResponseMS: d.ResponseMS,
		ResponseBody: d.ResponseBody, Error: d.Error, CreatedAt: d.CreatedAt, NextRetryAt: d.NextRetryAt,
	}
}
func validDeliveryStatus(s string) bool {
	switch store.DeliveryStatus(s) {
	case "", store.DeliveryPending, store.DeliverySuccess, store.DeliveryFailed, store.DeliveryDead:
		return true
	default:
		return false
	}
}
func (s *server) handleDeliveries(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit < 1 || limit > 5000 {
		Fail(c, invalidQuery("limit must be between 1 and 5000"))
		return
	}
	status := c.Query("status")
	if !validDeliveryStatus(status) {
		Fail(c, invalidQuery("status must be one of pending, success, failed, dead"))
		return
	}
	ds, err := s.store.ListDeliveries(c, c.Param("id"), store.DeliveryQuery{Limit: limit, Cursor: c.Query("cursor"), Status: store.DeliveryStatus(status)})
	if err != nil {
		webhookErr(c, err)
		return
	}
	out := make([]deliveryView, 0, len(ds))
	for _, d := range ds {
		out = append(out, deliveryViewOf(d))
	}
	c.JSON(http.StatusOK, gin.H{"deliveries": out})
}
func (s *server) handleRedeliver(c *gin.Context) {
	if s.webhooks == nil {
		Fail(c, invalidInput("webhook engine is unavailable"))
		return
	}
	id, err := s.webhooks.Redeliver(c, c.Param("id"))
	if err != nil {
		webhookErr(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"delivery_id": id})
}
func webhookID() string { return "wh_" + strconv.FormatInt(time.Now().UnixNano(), 36) }

var _ = webhook.EventType
