CREATE TABLE schema_version (version INTEGER NOT NULL);
INSERT INTO schema_version (version) VALUES (1);

CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE users (
  id INTEGER PRIMARY KEY,
  username TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  role TEXT NOT NULL CHECK (role IN ('admin','operator','viewer')),
  created_at TEXT NOT NULL,
  last_login_at TEXT
);
CREATE TABLE webhooks (
  id TEXT PRIMARY KEY, name TEXT NOT NULL, url TEXT NOT NULL, secret TEXT,
  enabled INTEGER NOT NULL DEFAULT 1, event_types TEXT NOT NULL, filters TEXT,
  headers TEXT, max_attempts INTEGER NOT NULL DEFAULT 5,
  created_at TEXT NOT NULL, updated_at TEXT NOT NULL
);
CREATE TABLE deliveries (
  id TEXT PRIMARY KEY,
  webhook_id TEXT NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
  event_id TEXT NOT NULL, payload TEXT NOT NULL, attempt INTEGER NOT NULL,
  status TEXT NOT NULL, status_code INTEGER, response_ms INTEGER,
  response_body TEXT, error TEXT, created_at TEXT NOT NULL, next_retry_at TEXT
);
CREATE INDEX deliveries_by_webhook ON deliveries(webhook_id, created_at DESC);
