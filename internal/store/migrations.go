package store

import _ "embed"

// Migration0001 is the initial persisted schema.
//
//go:embed migrations/0001_initial.sql
var Migration0001 string
