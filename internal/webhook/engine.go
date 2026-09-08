package webhook

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	mathrand "math/rand/v2"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0funct0ry/vessel/internal/dockerapi"
	"github.com/0funct0ry/vessel/internal/store"
)

type EventSource interface {
	Events(context.Context, dockerapi.EventsOptions) (*dockerapi.EventReader, error)
}
type Config struct {
	Store     store.Store
	Host      Host
	Client    *http.Client
	Now       func() time.Time
	QueueSize int
	// Jitter returns a number in [0,1), allowing deterministic retry tests.
	Jitter func() float64
}
type job struct{ deliveryID string }
type Engine struct {
	store   store.Store
	host    Host
	client  *http.Client
	now     func() time.Time
	jitter  func() float64
	queue   chan job
	dropped atomic.Uint64
	cancel  context.CancelFunc
	once    sync.Once
}

func New(cfg Config) *Engine {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 1024
	}
	if cfg.Client == nil {
		cfg.Client = &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	if cfg.Jitter == nil {
		cfg.Jitter = mathrand.Float64
	}
	return &Engine{store: cfg.Store, host: cfg.Host, client: cfg.Client, now: cfg.Now, jitter: cfg.Jitter, queue: make(chan job, cfg.QueueSize)}
}
func (e *Engine) Dropped() uint64 { return e.dropped.Load() }
func (e *Engine) Start(ctx context.Context, source EventSource) {
	ctx, e.cancel = context.WithCancel(ctx)
	for i := 0; i < 4; i++ {
		go e.worker(ctx)
	}
	go e.resume(ctx)
	if source != nil {
		go e.read(ctx, source)
	}
}
func (e *Engine) Close() {
	if e.cancel != nil {
		e.once.Do(e.cancel)
	}
}
func (e *Engine) read(ctx context.Context, source EventSource) {
	for ctx.Err() == nil {
		r, err := source.Events(ctx, dockerapi.EventsOptions{})
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
				continue
			}
		}
		for {
			ev, err := r.Next()
			if err != nil {
				r.Close()
				break
			}
			e.Dispatch(ctx, ev)
		}
	}
}
func (e *Engine) Dispatch(ctx context.Context, event dockerapi.Event) {
	webhooks, err := e.store.ListWebhooks(ctx)
	if err != nil {
		return
	}
	eid := newID("evt_", 8)
	for _, w := range webhooks {
		if !Matches(w, event) {
			continue
		}
		did := newID("dl_", 12)
		p, err := BuildPayload(eid, did, w.ID, event, e.host)
		if err != nil {
			continue
		}
		body, err := json.Marshal(p)
		if err != nil {
			continue
		}
		_, err = e.store.CreateDelivery(ctx, store.Delivery{ID: did, WebhookID: w.ID, EventID: eid, Payload: body, Attempt: 1, Status: store.DeliveryPending, CreatedAt: e.now().UTC()})
		if err == nil {
			e.enqueue(job{did})
		}
	}
}

// TestWebhook creates a synthetic container.start delivery for one configured
// webhook. It is intentionally routed through the normal sender and signer.
func (e *Engine) TestWebhook(ctx context.Context, webhookID string) error {
	w, err := e.store.GetWebhook(ctx, webhookID)
	if err != nil {
		return err
	}
	if !w.Enabled {
		return nil
	}
	evt := dockerapi.Event{Type: "container", Action: "start", Time: e.now().Unix(), Actor: dockerapi.EventActor{ID: "test", Attributes: map[string]string{"name": "vessel-test", "image": "vessel/test"}}, Raw: json.RawMessage(`{"synthetic":true}`)}
	did, eid := newID("dl_", 12), newID("evt_", 8)
	p, err := BuildPayload(eid, did, w.ID, evt, e.host)
	if err != nil {
		return err
	}
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if _, err = e.store.CreateDelivery(ctx, store.Delivery{ID: did, WebhookID: w.ID, EventID: eid, Payload: body, Attempt: 1, Status: store.DeliveryPending, CreatedAt: e.now().UTC()}); err == nil {
		e.enqueue(job{did})
	}
	return err
}

// Redeliver resets a terminal delivery and queues it as a new attempt.
func (e *Engine) Redeliver(ctx context.Context, deliveryID string) error {
	d, err := e.store.GetDelivery(ctx, deliveryID)
	if err != nil {
		return err
	}
	d.Attempt = 1
	d.Status = store.DeliveryPending
	d.StatusCode = nil
	d.ResponseMS = nil
	d.ResponseBody = nil
	d.Error = nil
	d.NextRetryAt = nil
	if _, err = e.store.UpdateDelivery(ctx, d); err == nil {
		e.enqueue(job{d.ID})
	}
	return err
}

// Enqueue never waits for a sender. At capacity it removes the oldest job.
func (e *Engine) enqueue(j job) {
	select {
	case e.queue <- j:
		return
	default:
	}
	select {
	case <-e.queue:
		e.dropped.Add(1)
	default:
	}
	select {
	case e.queue <- j:
	default:
		e.dropped.Add(1)
	}
}
func (e *Engine) resume(ctx context.Context) {
	ws, err := e.store.ListWebhooks(ctx)
	if err != nil {
		return
	}
	now := e.now()
	for _, w := range ws {
		ds, err := e.store.ListDeliveries(ctx, w.ID, store.DeliveryQuery{Limit: 5000})
		if err != nil {
			continue
		}
		for _, d := range ds {
			if d.Status == store.DeliveryPending {
				if d.NextRetryAt != nil && d.NextRetryAt.After(now) {
					e.schedule(ctx, d.ID, *d.NextRetryAt)
				} else {
					e.enqueue(job{d.ID})
				}
			}
		}
	}
}

func (e *Engine) schedule(ctx context.Context, deliveryID string, at time.Time) {
	go func() {
		t := time.NewTimer(time.Until(at))
		defer t.Stop()
		select {
		case <-ctx.Done():
		case <-t.C:
			e.enqueue(job{deliveryID})
		}
	}()
}
func newID(prefix string, n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return prefix + strconvID(time.Now().UnixNano())
	}
	return prefix + hex.EncodeToString(b)
}
func strconvID(v int64) string {
	return hex.EncodeToString([]byte(time.Unix(0, v).Format("20060102150405.000000000")))
}
