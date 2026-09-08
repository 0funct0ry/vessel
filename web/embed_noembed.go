//go:build !embed

// Package web embeds the built React app (web/dist) into the Vessel binary.
// This file backs plain `go build ./...` (no frontend built, no dist/
// directory required to exist).
package web

import (
	"errors"
	"io/fs"
)

// Available reports whether the frontend was built into this binary.
const Available = false

// Dist returns an error: this binary was built without the `embed` tag, so
// no frontend assets are available.
func Dist() (fs.FS, error) {
	return nil, errors.New("web: built without the embed tag; run make build")
}
