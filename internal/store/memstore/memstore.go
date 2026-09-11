package memstore

import (
	"context"
	"sort"
	"sync"

	"github.com/0funct0ry/vessel/internal/store"
)

const maxDeliveriesPerWebhook = 200

type Store struct {
	mu         sync.RWMutex
	settings   map[string]string
	users      map[int64]store.User
	usernames  map[string]int64
	webhooks   map[string]store.Webhook
	deliveries map[string]store.Delivery
	byWebhook  map[string][]string
	nextUserID int64
	events     map[string]store.Event
}

func New() *Store {
	return &Store{settings: map[string]string{}, users: map[int64]store.User{}, usernames: map[string]int64{}, webhooks: map[string]store.Webhook{}, deliveries: map[string]store.Delivery{}, byWebhook: map[string][]string{}, events: map[string]store.Event{}, nextUserID: 1}
}
func (s *Store) Close() error { return nil }
func (s *Store) GetSetting(_ context.Context, key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.settings[key]
	if !ok {
		return "", store.ErrNotFound
	}
	return v, nil
}
func (s *Store) SetSetting(_ context.Context, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings[key] = value
	return nil
}
func (s *Store) CreateUser(_ context.Context, u store.User) (store.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.usernames[u.Username]; ok {
		return store.User{}, store.ErrConflict
	}
	u.ID = s.nextUserID
	s.nextUserID++
	s.users[u.ID] = u
	s.usernames[u.Username] = u.ID
	return u, nil
}
func (s *Store) GetUser(_ context.Context, id int64) (store.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[id]
	if !ok {
		return store.User{}, store.ErrNotFound
	}
	return u, nil
}
func (s *Store) GetUserByUsername(_ context.Context, name string) (store.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.usernames[name]
	if !ok {
		return store.User{}, store.ErrNotFound
	}
	return s.users[id], nil
}
func (s *Store) ListUsers(_ context.Context) ([]store.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]store.User, 0, len(s.users))
	for _, u := range s.users {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func (s *Store) UpdateUser(_ context.Context, u store.User) (store.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.users[u.ID]
	if !ok {
		return store.User{}, store.ErrNotFound
	}
	if u.Username != old.Username {
		if id, exists := s.usernames[u.Username]; exists && id != u.ID {
			return store.User{}, store.ErrConflict
		}
		delete(s.usernames, old.Username)
		s.usernames[u.Username] = u.ID
	}
	s.users[u.ID] = u
	return u, nil
}
func (s *Store) DeleteUser(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	if !ok {
		return store.ErrNotFound
	}
	delete(s.users, id)
	delete(s.usernames, u.Username)
	return nil
}
func (s *Store) CreateWebhook(_ context.Context, w store.Webhook) (store.Webhook, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.webhooks[w.ID]; ok {
		return store.Webhook{}, store.ErrConflict
	}
	s.webhooks[w.ID] = w
	return w, nil
}
func (s *Store) GetWebhook(_ context.Context, id string) (store.Webhook, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	w, ok := s.webhooks[id]
	if !ok {
		return store.Webhook{}, store.ErrNotFound
	}
	return w, nil
}
func (s *Store) ListWebhooks(_ context.Context) ([]store.Webhook, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]store.Webhook, 0, len(s.webhooks))
	for _, w := range s.webhooks {
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func (s *Store) UpdateWebhook(_ context.Context, w store.Webhook) (store.Webhook, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.webhooks[w.ID]; !ok {
		return store.Webhook{}, store.ErrNotFound
	}
	s.webhooks[w.ID] = w
	return w, nil
}
func (s *Store) DeleteWebhook(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.webhooks[id]; !ok {
		return store.ErrNotFound
	}
	delete(s.webhooks, id)
	for _, did := range s.byWebhook[id] {
		delete(s.deliveries, did)
	}
	delete(s.byWebhook, id)
	return nil
}
func (s *Store) CreateDelivery(_ context.Context, d store.Delivery) (store.Delivery, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.webhooks[d.WebhookID]; !ok {
		return store.Delivery{}, store.ErrNotFound
	}
	if _, ok := s.deliveries[d.ID]; ok {
		return store.Delivery{}, store.ErrConflict
	}
	ids := append(s.byWebhook[d.WebhookID], d.ID)
	if len(ids) > maxDeliveriesPerWebhook {
		delete(s.deliveries, ids[0])
		ids = ids[1:]
	}
	s.byWebhook[d.WebhookID] = ids
	s.deliveries[d.ID] = d
	return d, nil
}
func (s *Store) GetDelivery(_ context.Context, id string) (store.Delivery, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.deliveries[id]
	if !ok {
		return store.Delivery{}, store.ErrNotFound
	}
	return d, nil
}
func (s *Store) ListDeliveries(_ context.Context, webhookID string, q store.DeliveryQuery) ([]store.Delivery, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.webhooks[webhookID]; !ok {
		return nil, store.ErrNotFound
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 50
	}
	out := make([]store.Delivery, 0)
	for i := len(s.byWebhook[webhookID]) - 1; i >= 0; i-- {
		d := s.deliveries[s.byWebhook[webhookID][i]]
		if q.Cursor != "" && d.ID >= q.Cursor {
			continue
		}
		out = append(out, d)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}
func (s *Store) UpdateDelivery(_ context.Context, d store.Delivery) (store.Delivery, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.deliveries[d.ID]; !ok {
		return store.Delivery{}, store.ErrNotFound
	}
	s.deliveries[d.ID] = d
	return d, nil
}

func (s *Store) CreateEvent(_ context.Context, e store.Event) (store.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.events[e.ID]; ok {
		return store.Event{}, store.ErrConflict
	}
	s.events[e.ID] = e
	return e, nil
}
func (s *Store) ListEvents(_ context.Context, q store.EventQuery) ([]store.Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	limit := q.Limit
	if limit <= 0 {
		limit = 50
	}
	allowed := map[string]bool{}
	for _, v := range q.Types {
		allowed[v] = true
	}
	out := []store.Event{}
	for _, e := range s.events {
		if len(allowed) == 0 || allowed[e.Type] {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
func (s *Store) DeleteEvent(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.events[id]; !ok {
		return store.ErrNotFound
	}
	delete(s.events, id)
	return nil
}
func (s *Store) ClearEvents(_ context.Context) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := int64(len(s.events))
	s.events = map[string]store.Event{}
	return n, nil
}
