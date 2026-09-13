CREATE TABLE stacks (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  compose_yaml TEXT NOT NULL,
  env_content TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
UPDATE schema_version SET version=3;
