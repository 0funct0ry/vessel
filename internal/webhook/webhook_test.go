package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/0funct0ry/vessel/internal/dockerapi"
	"github.com/0funct0ry/vessel/internal/store"
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
