package cmd

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/0funct0ry/vessel/internal/store"
	"github.com/0funct0ry/vessel/internal/store/sqlitestore"
)

func TestFirstPersistentUserImpliesAuth(t *testing.T) {
	s, err := sqlitestore.Open(filepath.Join(t.TempDir(), "vessel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if enabled, implied, count, err := effectiveAuth(context.Background(), false, s); err != nil || enabled || implied || count != 0 {
		t.Fatalf("before user = enabled=%v implied=%v count=%v err=%v", enabled, implied, count, err)
	}
	if _, err := s.CreateUser(context.Background(), store.User{Username: "alice", PasswordHash: "hash", Role: store.RoleAdmin, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if enabled, implied, count, err := effectiveAuth(context.Background(), false, s); err != nil || !enabled || !implied || count != 1 {
		t.Fatalf("after user = enabled=%v implied=%v count=%v err=%v", enabled, implied, count, err)
	}
}
