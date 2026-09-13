package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/0funct0ry/vessel/internal/dockerapi"
	"github.com/0funct0ry/vessel/internal/store"
	"github.com/0funct0ry/vessel/internal/store/memstore"
)

func TestMatches(t *testing.T) {
	e := dockerapi.Event{Type: "container", Action: "die", Actor: dockerapi.EventActor{Attributes: map[string]string{"name": "api-1", "image": "redis:7", "label.team": "platform"}}}
	for _, tc := range []struct {
		name string
		w    store.Webhook
		want bool
	}{
		{"wildcard", store.Webhook{Enabled: true, EventTypes: []string{"container.*"}}, true},
		{"all filters", store.Webhook{Enabled: true, EventTypes: []string{"container.die"}, Filters: []byte(`{"name":"api-*","image":"redis*","label":{"team":"platform"}}`)}, true},
		{"and filters", store.Webhook{Enabled: true, EventTypes: []string{"container.die"}, Filters: []byte(`{"name":"worker-*"}`)}, false},
		{"wrong event", store.Webhook{Enabled: true, EventTypes: []string{"image.*"}}, false},
	} {
		if got := Matches(tc.w, e); got != tc.want {
			t.Errorf("%s: got %t", tc.name, got)
		}
	}
}

func TestSignatureIndependentVerification(t *testing.T) {
	body := []byte(`{"type":"container.start"}`)
	got := Signature("secret", 123, body)
	m := hmac.New(sha256.New, []byte("secret"))
	m.Write([]byte("123."))
	m.Write(body)
	want := "t=123,v1=" + hex.EncodeToString(m.Sum(nil))
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestRetryDelays(t *testing.T) {
	want := []int64{1, 5, 25, 120, 600}
	for i, s := range want {
		if retryDelay(i).Seconds() != float64(s) {
			t.Fatalf("retry %d", i)
		}
	}
}

// TestRedeliverCreatesNewDeliveryNotMutation verifies the delivery log stays
// an append-only audit trail: Redeliver must enqueue a brand-new delivery ID
// and leave the original terminal delivery row untouched.
func TestRedeliverCreatesNewDeliveryNotMutation(t *testing.T) {
	ctx := context.Background()
	s := memstore.New()
	w, err := s.CreateWebhook(ctx, store.Webhook{ID: "wh1", Name: "acme", URL: "https://example.com/hook", Enabled: true, EventTypes: []string{"container.*"}, MaxAttempts: 3, CreatedAt: time.Now(), UpdatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	original := store.Delivery{
		ID: "dl_original", WebhookID: w.ID, EventID: "evt1", Payload: []byte(`{"type":"container.start"}`),
		Attempt: 3, Status: store.DeliveryFailed, CreatedAt: time.Now().Add(-time.Hour),
	}
	code := 500
	original.StatusCode = &code
	if _, err := s.CreateDelivery(ctx, original); err != nil {
		t.Fatal(err)
	}

	e := New(Config{Store: s, QueueSize: 8})
	newID, err := e.Redeliver(ctx, "dl_original")
	if err != nil {
		t.Fatal(err)
	}
	if newID == "" || newID == "dl_original" {
		t.Fatalf("expected a fresh delivery id, got %q", newID)
	}

	// The original row must be unchanged.
	got, err := s.GetDelivery(ctx, "dl_original")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != store.DeliveryFailed || got.Attempt != 3 || got.StatusCode == nil || *got.StatusCode != 500 {
		t.Fatalf("original delivery was mutated: %+v", got)
	}

	// The new row must exist as a fresh pending attempt.
	fresh, err := s.GetDelivery(ctx, newID)
	if err != nil {
		t.Fatalf("new delivery not found: %v", err)
	}
	if fresh.Status != store.DeliveryPending || fresh.Attempt != 1 || fresh.WebhookID != w.ID || fresh.EventID != original.EventID {
		t.Fatalf("unexpected fresh delivery: %+v", fresh)
	}
}
