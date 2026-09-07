package cmd

import (
	"fmt"
	"net"
	"strings"
)

// GuardBindError is returned by GuardBind when a non-loopback bind is refused.
// It carries ExitCode 3 per SPEC §3.3.
type GuardBindError struct {
	msg string
}

func (e *GuardBindError) Error() string { return e.msg }

// ExitCode is the process exit code that should be used when this error
// reaches the top of the command tree.
func (e *GuardBindError) ExitCode() int { return 3 }

// GuardBind refuses to bind a non-loopback address without auth enabled,
// unless override is set. It is a pure function so it can be unit tested
// without starting a server. See SPEC §3.3.
func GuardBind(addr string, authOn bool, override bool) error {
	if authOn || override || isLoopback(addr) {
		return nil
	}

	return &GuardBindError{msg: fmt.Sprintf(
		`refusing to bind %s without --auth
Vessel proxies the Docker socket, which is equivalent to root on this host.
Either bind loopback and use an SSH tunnel:
  ssh -L 7373:127.0.0.1:7373 user@host
or create a user and enable auth:
  vessel user add admin --role admin --db vessel.db
  vessel serve --db vessel.db --auth --addr 0.0.0.0
Override at your own risk with --i-know-what-im-doing.`, addr)}
}

// isLoopback reports whether addr refers to a loopback address, localhost, or
// a unix socket path. addr may be a bare host, a host:port pair, or a
// unix:// path.
func isLoopback(addr string) bool {
	if addr == "" {
		return true
	}

	if strings.HasPrefix(addr, "unix://") || strings.HasPrefix(addr, "/") {
		return true
	}

	host := addr
	if h, _, err := net.SplitHostPort(addr); err == nil {
		host = h
	}
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")

	if host == "" {
		// e.g. "0.0.0.0" split from ":7373" would leave "" only if addr was
		// literally ":7373" with no host — that means "all interfaces".
		return false
	}

	if strings.EqualFold(host, "localhost") {
		return true
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}

	return ip.IsLoopback()
}
