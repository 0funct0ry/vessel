// Package sqlitestore implements store.Store using the CGO-free modernc driver.
package sqlitestore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/0funct0ry/vessel/internal/store"
)

const schemaVersion = 1

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err = s.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) SchemaVersion(ctx context.Context) (int, error) {
	var v int
	err := s.db.QueryRowContext(ctx, "SELECT version FROM schema_version LIMIT 1").Scan(&v)
	return v, err
}
func (s *Store) migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var v int
	err = tx.QueryRowContext(ctx, "SELECT version FROM schema_version LIMIT 1").Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("invalid schema_version table")
	}
	if err != nil {
		if !strings.Contains(err.Error(), "no such table") {
			return err
		}
		if _, err = tx.ExecContext(ctx, store.Migration0001); err != nil {
			return fmt.Errorf("apply migration 1: %w", err)
		}
		return tx.Commit()
	}
	if v > schemaVersion {
		return fmt.Errorf("database schema version %d is newer than this binary's version %d", v, schemaVersion)
	}
	return tx.Commit()
}

func dbErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return store.ErrNotFound
	}
	if strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return store.ErrConflict
	}
	return err
}
func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key=?", key).Scan(&v)
	return v, dbErr(err)
}
func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", key, value)
	return dbErr(err)
}

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }
func nullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return ts(*t)
}
func scanUser(row interface{ Scan(...any) error }) (store.User, error) {
	var u store.User
	var created string
	var last sql.NullString
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &created, &last)
	if err != nil {
		return store.User{}, dbErr(err)
	}
	var e error
	u.CreatedAt, e = time.Parse(time.RFC3339Nano, created)
	if e != nil {
		return store.User{}, e
	}
	if last.Valid {
		t, e := time.Parse(time.RFC3339Nano, last.String)
		if e != nil {
			return store.User{}, e
		}
		u.LastLoginAt = &t
	}
	return u, nil
}
func (s *Store) CreateUser(ctx context.Context, u store.User) (store.User, error) {
	r, err := s.db.ExecContext(ctx, "INSERT INTO users(username,password_hash,role,created_at,last_login_at) VALUES(?,?,?,?,?)", u.Username, u.PasswordHash, u.Role, ts(u.CreatedAt), nullableTime(u.LastLoginAt))
	if err != nil {
		return store.User{}, dbErr(err)
	}
	u.ID, _ = r.LastInsertId()
	return u, nil
}
func (s *Store) GetUser(ctx context.Context, id int64) (store.User, error) {
	return scanUser(s.db.QueryRowContext(ctx, "SELECT id,username,password_hash,role,created_at,last_login_at FROM users WHERE id=?", id))
}
func (s *Store) GetUserByUsername(ctx context.Context, n string) (store.User, error) {
	return scanUser(s.db.QueryRowContext(ctx, "SELECT id,username,password_hash,role,created_at,last_login_at FROM users WHERE username=?", n))
}
func (s *Store) ListUsers(ctx context.Context) ([]store.User, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,username,password_hash,role,created_at,last_login_at FROM users ORDER BY id")
	if err != nil {
		return nil, dbErr(err)
	}
	defer rows.Close()
	var out []store.User
	for rows.Next() {
		u, e := scanUser(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
func (s *Store) UpdateUser(ctx context.Context, u store.User) (store.User, error) {
	r, err := s.db.ExecContext(ctx, "UPDATE users SET username=?,password_hash=?,role=?,created_at=?,last_login_at=? WHERE id=?", u.Username, u.PasswordHash, u.Role, ts(u.CreatedAt), nullableTime(u.LastLoginAt), u.ID)
	if err != nil {
		return store.User{}, dbErr(err)
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return store.User{}, store.ErrNotFound
	}
	return u, nil
}
func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	r, err := s.db.ExecContext(ctx, "DELETE FROM users WHERE id=?", id)
	if err != nil {
		return dbErr(err)
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func jsonText(b []byte) any {
	if b == nil {
		return nil
	}
	return string(b)
}
func scanWebhook(row interface{ Scan(...any) error }) (store.Webhook, error) {
	var w store.Webhook
	var secret, filters, headers sql.NullString
	var enabled int
	var events, created, updated string
	err := row.Scan(&w.ID, &w.Name, &w.URL, &secret, &enabled, &events, &filters, &headers, &w.MaxAttempts, &created, &updated)
	if err != nil {
		return w, dbErr(err)
	}
	if secret.Valid {
		w.Secret = &secret.String
	}
	w.Enabled = enabled != 0
	if err := json.Unmarshal([]byte(events), &w.EventTypes); err != nil {
		return w, fmt.Errorf("decode webhook event types: %w", err)
	}
	if filters.Valid {
		w.Filters = []byte(filters.String)
	}
	if headers.Valid {
		w.Headers = []byte(headers.String)
	}
	var e error
	w.CreatedAt, e = time.Parse(time.RFC3339Nano, created)
	if e != nil {
		return w, e
	}
	w.UpdatedAt, e = time.Parse(time.RFC3339Nano, updated)
	return w, e
}
func webhookArgs(w store.Webhook) []any {
	events, err := json.Marshal(w.EventTypes)
	if err != nil {
		panic(err)
	}
	return []any{w.Name, w.URL, w.Secret, boolInt(w.Enabled), string(events), jsonText(w.Filters), jsonText(w.Headers), w.MaxAttempts, ts(w.CreatedAt), ts(w.UpdatedAt)}
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
func (s *Store) CreateWebhook(ctx context.Context, w store.Webhook) (store.Webhook, error) {
	a := append([]any{w.ID}, webhookArgs(w)...)
	_, err := s.db.ExecContext(ctx, "INSERT INTO webhooks(id,name,url,secret,enabled,event_types,filters,headers,max_attempts,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)", a...)
	return w, dbErr(err)
}
func (s *Store) GetWebhook(ctx context.Context, id string) (store.Webhook, error) {
	return scanWebhook(s.db.QueryRowContext(ctx, "SELECT id,name,url,secret,enabled,event_types,filters,headers,max_attempts,created_at,updated_at FROM webhooks WHERE id=?", id))
}
func (s *Store) ListWebhooks(ctx context.Context) ([]store.Webhook, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,name,url,secret,enabled,event_types,filters,headers,max_attempts,created_at,updated_at FROM webhooks ORDER BY id")
	if err != nil {
		return nil, dbErr(err)
	}
	defer rows.Close()
	var out []store.Webhook
	for rows.Next() {
		w, e := scanWebhook(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, w)
	}
	return out, rows.Err()
}
func (s *Store) UpdateWebhook(ctx context.Context, w store.Webhook) (store.Webhook, error) {
	a := append(webhookArgs(w), w.ID)
	r, err := s.db.ExecContext(ctx, "UPDATE webhooks SET name=?,url=?,secret=?,enabled=?,event_types=?,filters=?,headers=?,max_attempts=?,created_at=?,updated_at=? WHERE id=?", a...)
	if err != nil {
		return store.Webhook{}, dbErr(err)
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return store.Webhook{}, store.ErrNotFound
	}
	return w, nil
}
func (s *Store) DeleteWebhook(ctx context.Context, id string) error {
	r, err := s.db.ExecContext(ctx, "DELETE FROM webhooks WHERE id=?", id)
	if err != nil {
		return dbErr(err)
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func scanDelivery(row interface{ Scan(...any) error }) (store.Delivery, error) {
	var d store.Delivery
	var code, ms sql.NullInt64
	var body, problem, next sql.NullString
	var payload, created string
	err := row.Scan(&d.ID, &d.WebhookID, &d.EventID, &payload, &d.Attempt, &d.Status, &code, &ms, &body, &problem, &created, &next)
	if err != nil {
		return d, dbErr(err)
	}
	d.Payload = []byte(payload)
	if code.Valid {
		x := int(code.Int64)
		d.StatusCode = &x
	}
	if ms.Valid {
		x := int(ms.Int64)
		d.ResponseMS = &x
	}
	if body.Valid {
		d.ResponseBody = &body.String
	}
	if problem.Valid {
		d.Error = &problem.String
	}
	var e error
	d.CreatedAt, e = time.Parse(time.RFC3339Nano, created)
	if e != nil {
		return d, e
	}
	if next.Valid {
		t, e := time.Parse(time.RFC3339Nano, next.String)
		if e != nil {
			return d, e
		}
		d.NextRetryAt = &t
	}
	return d, nil
}
func deliveryArgs(d store.Delivery) []any {
	return []any{d.WebhookID, d.EventID, string(d.Payload), d.Attempt, d.Status, d.StatusCode, d.ResponseMS, d.ResponseBody, d.Error, ts(d.CreatedAt), nullableTime(d.NextRetryAt)}
}
func (s *Store) CreateDelivery(ctx context.Context, d store.Delivery) (store.Delivery, error) {
	a := append([]any{d.ID}, deliveryArgs(d)...)
	_, err := s.db.ExecContext(ctx, "INSERT INTO deliveries(id,webhook_id,event_id,payload,attempt,status,status_code,response_ms,response_body,error,created_at,next_retry_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)", a...)
	return d, dbErr(err)
}
func (s *Store) GetDelivery(ctx context.Context, id string) (store.Delivery, error) {
	return scanDelivery(s.db.QueryRowContext(ctx, "SELECT id,webhook_id,event_id,payload,attempt,status,status_code,response_ms,response_body,error,created_at,next_retry_at FROM deliveries WHERE id=?", id))
}
func (s *Store) ListDeliveries(ctx context.Context, webhookID string, q store.DeliveryQuery) ([]store.Delivery, error) {
	if _, err := s.GetWebhook(ctx, webhookID); err != nil {
		return nil, err
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 50
	}
	args := []any{webhookID}
	where := "WHERE webhook_id=?"
	if q.Cursor != "" {
		where += " AND id<?"
		args = append(args, q.Cursor)
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, "SELECT id,webhook_id,event_id,payload,attempt,status,status_code,response_ms,response_body,error,created_at,next_retry_at FROM deliveries "+where+" ORDER BY created_at DESC,id DESC LIMIT ?", args...)
	if err != nil {
		return nil, dbErr(err)
	}
	defer rows.Close()
	var out []store.Delivery
	for rows.Next() {
		d, e := scanDelivery(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
func (s *Store) UpdateDelivery(ctx context.Context, d store.Delivery) (store.Delivery, error) {
	a := append(deliveryArgs(d), d.ID)
	r, err := s.db.ExecContext(ctx, "UPDATE deliveries SET webhook_id=?,event_id=?,payload=?,attempt=?,status=?,status_code=?,response_ms=?,response_body=?,error=?,created_at=?,next_retry_at=? WHERE id=?", a...)
	if err != nil {
		return store.Delivery{}, dbErr(err)
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return store.Delivery{}, store.ErrNotFound
	}
	return d, nil
}
