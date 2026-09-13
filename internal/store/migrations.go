package store

import _ "embed"

// Migration0001 is the initial persisted schema.
//
//go:embed migrations/0001_initial.sql
var Migration0001 string

//go:embed migrations/0002_events.sql
var Migration0002 string

//go:embed migrations/0003_stacks.sql
var Migration0003 string

//go:embed migrations/0004_tokens.sql
var Migration0004 string
