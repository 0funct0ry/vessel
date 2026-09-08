package cmd

import (
	"os"
	"path/filepath"
	"runtime"
)

// defaultDockerHost is the last resort when nothing else resolves: the
// classic Linux/older-Docker-Desktop socket location, matching SPEC §3.1.
const defaultDockerHost = "unix:///var/run/docker.sock"

// dockerHostResult is the resolved --docker-host value plus where it came
// from, so callers can print a truthful banner/doctor line instead of
// silently guessing.
type dockerHostResult struct {
	Host   string
	Source string // "flag", "env DOCKER_HOST", "auto-detected", or "default"
}

// resolveDockerHost picks the Docker host to use. explicit is whatever
// --docker-host/VESSEL_DOCKER_HOST/the config file already resolved to (via
// Viper) — empty means the user did not specify one. Precedence:
//  1. explicit (flag/env/file) always wins — never override a user's choice.
//  2. the plain DOCKER_HOST env var, the same one `docker` itself honours,
//     so vessel matches whatever Docker context the user already has active.
//  3. the first candidate socket path that actually exists as a socket.
//  4. the hardcoded default.
//
// candidates and exists are injected so this stays a pure, table-testable
// function; detectDockerHost below supplies the real ones.
func resolveDockerHost(explicit, dockerHostEnv string, candidates []string, exists func(string) bool) dockerHostResult {
	if explicit != "" {
		return dockerHostResult{Host: explicit, Source: "flag"}
	}
	if dockerHostEnv != "" {
		return dockerHostResult{Host: dockerHostEnv, Source: "env DOCKER_HOST"}
	}
	for _, candidate := range candidates {
		if exists(candidate) {
			return dockerHostResult{Host: "unix://" + candidate, Source: "auto-detected"}
		}
	}
	return dockerHostResult{Host: defaultDockerHost, Source: "default"}
}

// detectDockerHost wraps resolveDockerHost with the real environment, the
// real candidate socket paths for this OS, and a real socket-file check.
func detectDockerHost(explicit string) dockerHostResult {
	return resolveDockerHost(explicit, os.Getenv("DOCKER_HOST"), candidateDockerSockets(), isSocketFile)
}

// candidateDockerSockets lists plausible Docker socket locations to probe,
// most-likely-first. Windows has no Unix-domain Docker socket by default
// (Docker Desktop there uses a named pipe, which vessel does not yet speak —
// see internal/dockerapi), so there is nothing useful to probe there.
func candidateDockerSockets() []string {
	if runtime.GOOS == "windows" {
		return nil
	}

	var candidates []string
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		// Docker Desktop (Mac and Linux), current versions: no longer
		// symlinked to /var/run/docker.sock by default.
		candidates = append(candidates, filepath.Join(home, ".docker", "run", "docker.sock"))
	}
	// Classic Linux daemon, and older Docker Desktop's compatibility symlink.
	candidates = append(candidates, "/var/run/docker.sock")
	if xdgRuntime := os.Getenv("XDG_RUNTIME_DIR"); xdgRuntime != "" {
		// Rootless Docker on Linux.
		candidates = append(candidates, filepath.Join(xdgRuntime, "docker.sock"))
	}
	return candidates
}

// isSocketFile reports whether path exists and is a Unix domain socket.
func isSocketFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSocket != 0
}

// dockerHostBannerNote returns the short parenthetical to append after the
// resolved host in a banner/doctor line, or "" when the value came from an
// explicit flag/env/file or the unchanged hardcoded default (nothing
// surprising to call out).
func dockerHostBannerNote(source string) string {
	switch source {
	case "env DOCKER_HOST":
		return " (from $DOCKER_HOST)"
	case "auto-detected":
		return " (auto-detected)"
	default:
		return ""
	}
}
