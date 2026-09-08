package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/0funct0ry/vessel/internal/store"
	"github.com/0funct0ry/vessel/internal/version"
)

var retryDelays = []time.Duration{time.Second, 5 * time.Second, 25 * time.Second, 2 * time.Minute, 10 * time.Minute}

func (e *Engine) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case j := <-e.queue:
			e.send(ctx, j)
		}
	}
}
func (e *Engine) send(ctx context.Context, j job) {
	d, err := e.store.GetDelivery(ctx, j.deliveryID)
	if err != nil || d.Status != store.DeliveryPending {
		return
	}
	if d.NextRetryAt != nil && d.NextRetryAt.After(e.now()) {
		e.schedule(ctx, d.ID, *d.NextRetryAt)
		return
	}
	w, err := e.store.GetWebhook(ctx, d.WebhookID)
	if err != nil || !w.Enabled {
		return
	}
	start := e.now()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(d.Payload))
	if err == nil {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "vessel/"+version.Version)
		req.Header.Set("X-Vessel-Event", payloadType(d.Payload))
		req.Header.Set("X-Vessel-Delivery", d.ID)
		req.Header.Set("X-Vessel-Webhook", w.ID)
		if w.Secret != nil {
			req.Header.Set("X-Vessel-Signature", Signature(*w.Secret, start.Unix(), d.Payload))
		}
		for k, v := range headers(w.Headers) {
			req.Header.Set(k, v)
		}
	}
	var code *int
	var body *string
	var problem *string
	if err != nil {
		x := err.Error()
		problem = &x
	} else {
		resp, x := e.client.Do(req)
		if x != nil {
			y := x.Error()
			problem = &y
		} else {
			n := resp.StatusCode
			code = &n
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			resp.Body.Close()
			s := string(b)
			body = &s
			if n < 200 || n >= 300 {
				y := fmt.Sprintf("webhook returned HTTP %d", n)
				problem = &y
			}
		}
	}
	ms := int(e.now().Sub(start).Milliseconds())
	d.StatusCode = code
	d.ResponseBody = body
	d.ResponseMS = &ms
	d.Error = problem
	if problem == nil {
		d.Status = store.DeliverySuccess
		d.NextRetryAt = nil
		_, _ = e.store.UpdateDelivery(ctx, d)
		return
	}
	if d.Attempt >= maxAttempts(w) {
		d.Status = store.DeliveryDead
		d.NextRetryAt = nil
		_, _ = e.store.UpdateDelivery(ctx, d)
		return
	}
	d.Attempt++
	next := e.now().Add(e.jitterDelay(d.Attempt - 2))
	d.NextRetryAt = &next
	d.Status = store.DeliveryPending
	_, _ = e.store.UpdateDelivery(ctx, d)
	e.schedule(ctx, d.ID, next)
}
func maxAttempts(w store.Webhook) int {
	if w.MaxAttempts > 0 {
		return w.MaxAttempts
	}
	return 5
}
func retryDelay(index int) time.Duration {
	if index < 0 {
		index = 0
	}
	if index >= len(retryDelays) {
		index = len(retryDelays) - 1
	}
	return retryDelays[index]
}
func (e *Engine) jitterDelay(index int) time.Duration {
	base := retryDelay(index)
	return time.Duration(float64(base) * (0.8 + 0.4*e.jitter()))
}
func headers(raw []byte) map[string]string {
	out := map[string]string{}
	_ = json.Unmarshal(raw, &out)
	return out
}
func payloadType(body []byte) string {
	var x struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal(body, &x)
	return x.Type
}
