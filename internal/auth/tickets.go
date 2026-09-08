package auth

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

type Tickets struct {
	mu     sync.Mutex
	values map[string]ticket
	Now    func() time.Time
}
type ticket struct {
	claims  Claims
	expires time.Time
}

func NewTickets() *Tickets { return &Tickets{values: map[string]ticket{}, Now: time.Now} }
func (t *Tickets) Issue(c Claims) (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	v := base64.RawURLEncoding.EncodeToString(b)
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.Now()
	for k, x := range t.values {
		if !x.expires.After(now) {
			delete(t.values, k)
		}
	}
	t.values[v] = ticket{c, now.Add(30 * time.Second)}
	return v, nil
}
func (t *Tickets) Consume(v string) (Claims, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	x, ok := t.values[v]
	delete(t.values, v)
	return x.claims, ok && x.expires.After(t.Now())
}
