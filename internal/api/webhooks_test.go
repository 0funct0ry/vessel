package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/0funct0ry/vessel/internal/store"
	"github.com/0funct0ry/vessel/internal/store/memstore"
	"github.com/0funct0ry/vessel/internal/webhook"
)

func newTestWebhookEngine(t *testing.T, s store.Store) *webhook.Engine {
	t.Helper()
	return webhook.New(webhook.Config{Store: s, QueueSize: 8})
}

func TestHandleWebhooksStats24h(t *testing.T) {
	persistence := memstore.New()
	ctx := context.Background()
	w, err := persistence.CreateWebhook(ctx, store.Webhook{
		ID: "wh1", Name: "acme", URL: "https://example.com/hook?token=secret",
		Enabled: true, EventTypes: []string{"container.*"}, MaxAttempts: 3,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	mk := func(id string, status store.DeliveryStatus, age time.Duration) store.Delivery {
		return store.Delivery{ID: id, WebhookID: w.ID, EventID: "evt-" + id, Payload: []byte(`{}`), Attempt: 1, Status: status, CreatedAt: now.Add(-age)}
	}
	// Inside the 24h window: 2 success, 1 failed -> sent=3, failed=1, rate=2/3.
	// memstore's ListDeliveries orders by insertion order (newest last), so
	// deliveries are created oldest-first here to match real usage.
	deliveries := []store.Delivery{
		// Outside the window: must not count.
		mk("dl0", store.DeliveryDead, 48*time.Hour),
		mk("dl3", store.DeliverySuccess, 2*time.Hour),
		mk("dl2", store.DeliveryFailed, time.Hour),
		mk("dl1", store.DeliverySuccess, 30*time.Minute),
	}
	for _, d := range deliveries {
		if _, err := persistence.CreateDelivery(ctx, d); err != nil {
			t.Fatal(err)
		}
	}

	router := NewRouter(Config{Docker: newFakeDockerClient(), Store: persistence})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/webhooks", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var body struct {
		Webhooks []struct {
			ID           string `json:"id"`
			LastDelivery *struct {
				Status    string    `json:"status"`
				CreatedAt time.Time `json:"created_at"`
			} `json:"last_delivery"`
			Stats24h struct {
				Sent        int     `json:"sent"`
				Failed      int     `json:"failed"`
				SuccessRate float64 `json:"success_rate"`
			} `json:"stats_24h"`
		} `json:"webhooks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Webhooks) != 1 {
		t.Fatalf("expected 1 webhook, got %d", len(body.Webhooks))
	}
	got := body.Webhooks[0]
	if got.Stats24h.Sent != 3 || got.Stats24h.Failed != 1 {
		t.Fatalf("stats_24h = %+v", got.Stats24h)
	}
	if got.Stats24h.SuccessRate < 0.6666 || got.Stats24h.SuccessRate > 0.6667 {
		t.Fatalf("success_rate = %v, want ~0.6667", got.Stats24h.SuccessRate)
	}
	if got.LastDelivery == nil || got.LastDelivery.Status != "success" {
		t.Fatalf("last_delivery = %+v, want most recent (dl1, success)", got.LastDelivery)
	}
}

func TestHandleWebhookCreateRejectsReservedHeaders(t *testing.T) {
	persistence := memstore.New()
	router := NewRouter(Config{Docker: newFakeDockerClient(), Store: persistence})

	for _, header := range []string{"X-Vessel-Signature", "x-vessel-delivery", "User-Agent", "USER-AGENT"} {
		body := []byte(`{"name":"acme","url":"https://example.com/hook","event_types":["container.start"],"headers":{"` + header + `":"nope"}}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("header %q: status=%d body=%s", header, rec.Code, rec.Body.String())
		}
		if !bytesContains(rec.Body.Bytes(), `"reserved_header"`) {
			t.Fatalf("header %q: body=%s, want reserved_header code", header, rec.Body.String())
		}
	}
}

func TestHandleRedeliverCreatesNewDelivery(t *testing.T) {
	persistence := memstore.New()
	ctx := context.Background()
	w, err := persistence.CreateWebhook(ctx, store.Webhook{ID: "wh1", Name: "acme", URL: "https://example.com/hook", Enabled: true, EventTypes: []string{"container.*"}, CreatedAt: time.Now(), UpdatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := persistence.CreateDelivery(ctx, store.Delivery{ID: "dl_original", WebhookID: w.ID, EventID: "evt1", Payload: []byte(`{}`), Attempt: 1, Status: store.DeliveryDead, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}

	eng := newTestWebhookEngine(t, persistence)
	router := NewRouter(Config{Docker: newFakeDockerClient(), Store: persistence, Webhooks: eng})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/deliveries/dl_original/redeliver", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		DeliveryID string `json:"delivery_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.DeliveryID == "" || out.DeliveryID == "dl_original" {
		t.Fatalf("expected a fresh delivery id, got %q", out.DeliveryID)
	}
	original, err := persistence.GetDelivery(ctx, "dl_original")
	if err != nil {
		t.Fatal(err)
	}
	if original.Status != store.DeliveryDead {
		t.Fatalf("original delivery mutated: %+v", original)
	}
	fresh, err := persistence.GetDelivery(ctx, out.DeliveryID)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Status != store.DeliveryPending {
		t.Fatalf("fresh delivery = %+v", fresh)
	}
}

func bytesContains(haystack []byte, needle string) bool {
	return bytes.Contains(haystack, []byte(needle))
}
