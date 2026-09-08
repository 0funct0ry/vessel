package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0funct0ry/vessel/internal/store"
	"github.com/0funct0ry/vessel/internal/store/memstore"
)

func TestTokensLifecycleAndTampering(t *testing.T) {
	s := memstore.New()
	now := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)
	tokens, generated, err := LoadTokens(context.Background(), s, time.Hour)
	if err != nil || !generated {
		t.Fatalf("LoadTokens = %v, generated=%v", err, generated)
	}
	tokens.Now = func() time.Time { return now }
	raw, err := tokens.Issue(store.User{ID: 7, Username: "alice", Role: store.RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	claims, err := tokens.Parse(raw)
	if err != nil || claims.Subject != "7" || claims.Name != "alice" || claims.Role != store.RoleAdmin {
		t.Fatalf("claims=%+v err=%v", claims, err)
	}
	if _, err := tokens.Parse(raw + "x"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("tampered token error=%v", err)
	}
	now = now.Add(2 * time.Hour)
	if _, err := tokens.Parse(raw); !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("expired token error=%v", err)
	}
}

func TestThrottleUsesInjectedClock(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	throttle := NewThrottle()
	throttle.Now = func() time.Time { return now }
	for range 3 {
		if got := throttle.Failure("alice"); got != 0 {
			t.Fatalf("early delay=%s", got)
		}
	}
	if got := throttle.Failure("alice"); got != time.Second {
		t.Fatalf("delay=%s", got)
	}
	if got := throttle.Delay("alice"); got != time.Second {
		t.Fatalf("remaining=%s", got)
	}
	now = now.Add(time.Second)
	if got := throttle.Delay("alice"); got != 0 {
		t.Fatalf("remaining after wait=%s", got)
	}
	throttle.Success("alice")
	if got := throttle.Delay("alice"); got != 0 {
		t.Fatalf("success reset=%s", got)
	}
}

func TestTicketsAreSingleUseAndExpire(t *testing.T) {
	now := time.Now()
	tickets := NewTickets()
	tickets.Now = func() time.Time { return now }
	token, err := tickets.Issue(Claims{Name: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if c, ok := tickets.Consume(token); !ok || c.Name != "alice" {
		t.Fatalf("consume=%+v ok=%v", c, ok)
	}
	if _, ok := tickets.Consume(token); ok {
		t.Fatal("ticket consumed twice")
	}
	token, _ = tickets.Issue(Claims{})
	now = now.Add(31 * time.Second)
	if _, ok := tickets.Consume(token); ok {
		t.Fatal("expired ticket accepted")
	}
}
