package store_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/0funct0ry/vessel/internal/store"
	"github.com/0funct0ry/vessel/internal/store/memstore"
	"github.com/0funct0ry/vessel/internal/store/sqlitestore"
	_ "modernc.org/sqlite"
)

type newStore func(*testing.T) store.Store

func TestStoreContract(t *testing.T) {
	backends := map[string]newStore{
		"memory": func(t *testing.T) store.Store { return memstore.New() },
		"sqlite": func(t *testing.T) store.Store {
			s, err := sqlitestore.Open(filepath.Join(t.TempDir(), "vessel.db"))
			if err != nil {
				t.Fatal(err)
			}
			return s
		},
	}
	for name, open := range backends {
		t.Run(name, func(t *testing.T) { testStore(t, open) })
	}
}

func testStore(t *testing.T, open newStore) {
	t.Helper()
	ctx := context.Background()
	s := open(t)
	t.Cleanup(func() { _ = s.Close() })
	if err := s.SetSetting(ctx, "instance_id", "one"); err != nil {
		t.Fatal(err)
	}
	if got, err := s.GetSetting(ctx, "instance_id"); err != nil || got != "one" {
		t.Fatalf("setting = %q, %v", got, err)
	}
	if _, err := s.GetSetting(ctx, "missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing setting = %v", err)
	}

	now := time.Date(2026, 9, 8, 1, 2, 3, 0, time.UTC)
	u, err := s.CreateUser(ctx, store.User{Username: "alice", PasswordHash: "hash", Role: store.RoleAdmin, CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if u.ID == 0 {
		t.Fatal("created user has no id")
	}
	if _, err := s.CreateUser(ctx, store.User{Username: "alice", PasswordHash: "other", Role: store.RoleViewer, CreatedAt: now}); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("duplicate user = %v", err)
	}
	got, err := s.GetUserByUsername(ctx, "alice")
	if err != nil || got.ID != u.ID {
		t.Fatalf("lookup user = %#v, %v", got, err)
	}
	got.Role = store.RoleOperator
	if _, err := s.UpdateUser(ctx, got); err != nil {
		t.Fatal(err)
	}
	users, err := s.ListUsers(ctx)
	if err != nil || len(users) != 1 || users[0].Role != store.RoleOperator {
		t.Fatalf("users = %#v, %v", users, err)
	}
	if err := s.DeleteUser(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetUser(ctx, u.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("deleted user = %v", err)
	}

	w := store.Webhook{ID: "wh_one", Name: "one", URL: "https://example.test/hook", Enabled: true, EventTypes: []string{"container.die"}, Filters: []byte(`{"name":"api-*"}`), Headers: []byte(`{"X-Test":"yes"}`), MaxAttempts: 5, CreatedAt: now, UpdatedAt: now}
	if _, err := s.CreateWebhook(ctx, w); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateWebhook(ctx, w); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("duplicate webhook = %v", err)
	}
	w.Name = "renamed"
	if _, err := s.UpdateWebhook(ctx, w); err != nil {
		t.Fatal(err)
	}
	webhooks, err := s.ListWebhooks(ctx)
	if err != nil || len(webhooks) != 1 || webhooks[0].Name != "renamed" {
		t.Fatalf("webhooks = %#v, %v", webhooks, err)
	}
	d := store.Delivery{ID: "dl_one", WebhookID: w.ID, EventID: "evt_one", Payload: []byte(`{"ok":true}`), Attempt: 1, Status: store.DeliveryPending, CreatedAt: now}
	if _, err := s.CreateDelivery(ctx, d); err != nil {
		t.Fatal(err)
	}
	d.Status = store.DeliverySuccess
	code := 204
	d.StatusCode = &code
	if _, err := s.UpdateDelivery(ctx, d); err != nil {
		t.Fatal(err)
	}
	deliveries, err := s.ListDeliveries(ctx, w.ID, store.DeliveryQuery{})
	if err != nil || len(deliveries) != 1 || deliveries[0].Status != store.DeliverySuccess {
		t.Fatalf("deliveries = %#v, %v", deliveries, err)
	}
	if err := s.DeleteWebhook(ctx, w.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetDelivery(ctx, d.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cascade delete = %v", err)
	}

	e1 := store.Event{ID: "evt_one", Type: "container", Action: "start", SubjectID: "c1", Name: "api", Attrs: []byte(`{"image":"acme/api:1"}`), CreatedAt: now}
	e2 := store.Event{ID: "evt_two", Type: "image", Action: "pull", SubjectID: "i1", Name: "acme/api:2", Attrs: []byte(`{}`), CreatedAt: now.Add(time.Second)}
	if _, err := s.CreateEvent(ctx, e1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateEvent(ctx, e2); err != nil {
		t.Fatal(err)
	}
	events, err := s.ListEvents(ctx, store.EventQuery{Limit: 1, Types: []string{"image"}})
	if err != nil || len(events) != 1 || events[0].ID != e2.ID {
		t.Fatalf("events = %#v, %v", events, err)
	}
	if err := s.DeleteEvent(ctx, e1.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ListEvents(ctx, store.EventQuery{Limit: 10}); err != nil {
		t.Fatal(err)
	}
	if n, err := s.ClearEvents(ctx); err != nil || n != 1 {
		t.Fatalf("cleared = %d, %v", n, err)
	}
}

func TestMemoryDeliveryRetention(t *testing.T) {
	ctx := context.Background()
	s := memstore.New()
	now := time.Now().UTC()
	if _, err := s.CreateWebhook(ctx, store.Webhook{ID: "wh", Name: "hook", URL: "https://example.test", EventTypes: []string{}, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 201; i++ {
		_, err := s.CreateDelivery(ctx, store.Delivery{ID: fmt.Sprintf("dl_%03d", i), WebhookID: "wh", EventID: "evt", Payload: []byte("{}"), Status: store.DeliveryPending, CreatedAt: now.Add(time.Duration(i) * time.Second)})
		if err != nil {
			t.Fatal(err)
		}
	}
	items, err := s.ListDeliveries(ctx, "wh", store.DeliveryQuery{Limit: 500})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 200 {
		t.Fatalf("retained %d deliveries, want 200", len(items))
	}
	if _, err := s.GetDelivery(ctx, "dl_000"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("oldest delivery = %v", err)
	}
}

func TestSQLiteMigrationAndNewerSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vessel.db")
	s, err := sqlitestore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.SchemaVersion(context.Background())
	if err != nil || v != 2 {
		t.Fatalf("version = %d, %v", v, err)
	}
	_ = s.Close()
	if _, err := sqlitestore.Open(path); err != nil {
		t.Fatalf("reopen migrated db: %v", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE schema_version SET version=3"); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	if _, err := sqlitestore.Open(path); err == nil {
		t.Fatal("opening newer schema succeeded")
	}
}
