//go:build embed

// Package web embeds the built React app (web/dist) into the Vessel binary.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// Available reports whether the frontend was built into this binary.
const Available = true

// Dist returns the embedded dist/ tree, rooted so paths are relative to
// dist (e.g. "index.html", "assets/app.js").
func Dist() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}
