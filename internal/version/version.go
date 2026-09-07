// Package version holds build metadata injected via -ldflags.
package version

// Set at build time via:
//
//	-ldflags "-X github.com/0funct0ry/vessel/internal/version.Version=... \
//	           -X github.com/0funct0ry/vessel/internal/version.Commit=... \
//	           -X github.com/0funct0ry/vessel/internal/version.Date=..."
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// Info is the machine-readable view of the build metadata.
type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

// Get returns the current build info.
func Get() Info {
	return Info{Version: Version, Commit: Commit, Date: Date}
}

// String returns a short human-readable version string, e.g. "v0.1.0".
func String() string {
	return "v" + trimV(Version)
}

func trimV(s string) string {
	if len(s) > 0 && (s[0] == 'v' || s[0] == 'V') {
		return s[1:]
	}
	return s
}
