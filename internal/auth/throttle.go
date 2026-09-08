package auth

import (
	"sync"
	"time"
)

// Throttle tracks failed credentials per username. Its clock is injectable for tests.
type Throttle struct {
	mu       sync.Mutex
	failures map[string]int
	until    map[string]time.Time
	Now      func() time.Time
}

func NewThrottle() *Throttle {
	return &Throttle{failures: map[string]int{}, until: map[string]time.Time{}, Now: time.Now}
}
func (t *Throttle) Delay(username string) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.Now()
	if until := t.until[username]; until.After(now) {
		return until.Sub(now)
	}
	return 0
}
func (t *Throttle) Failure(username string) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.failures[username]++
	n := t.failures[username]
	if n <= 3 {
		return 0
	}
	d := time.Second << min(n-4, 4)
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	t.until[username] = t.Now().Add(d)
	return d
}
func (t *Throttle) Success(username string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.failures, username)
	delete(t.until, username)
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
