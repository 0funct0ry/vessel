CREATE TABLE events (
  id TEXT PRIMARY KEY,
  type TEXT NOT NULL,
  action TEXT NOT NULL,
  subject_id TEXT NOT NULL,
  name TEXT NOT NULL,
  attrs TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX events_by_created ON events(created_at DESC, id DESC);
UPDATE schema_version SET version=2;
