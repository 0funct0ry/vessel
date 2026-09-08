package webhook

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/0funct0ry/vessel/internal/dockerapi"
)

type Host struct {
	Name   string `json:"name"`
	Engine string `json:"engine"`
}
type Container struct {
	ID       string            `json:"id"`
	Name     string            `json:"name,omitempty"`
	Image    string            `json:"image,omitempty"`
	ExitCode *int              `json:"exit_code,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
}
type Payload struct {
	ID         string          `json:"id"`
	DeliveryID string          `json:"delivery_id"`
	WebhookID  string          `json:"webhook_id"`
	Type       string          `json:"type"`
	CreatedAt  time.Time       `json:"created_at"`
	Host       Host            `json:"host"`
	Container  *Container      `json:"container,omitempty"`
	Raw        json.RawMessage `json:"raw"`
}

// BuildPayload converts event attributes only; it never inspects Docker again.
func BuildPayload(eventID, deliveryID, webhookID string, e dockerapi.Event, host Host) (Payload, error) {
	created := time.Unix(e.Time, 0).UTC()
	if e.TimeNano != 0 {
		created = time.Unix(0, e.TimeNano).UTC()
	}
	p := Payload{ID: eventID, DeliveryID: deliveryID, WebhookID: webhookID, Type: EventType(e), CreatedAt: created, Host: host, Raw: e.Raw}
	if e.Type == "container" {
		a := e.Actor.Attributes
		labels := map[string]string{}
		for k, v := range a {
			if strings.HasPrefix(k, "label.") {
				labels[strings.TrimPrefix(k, "label.")] = v
			}
		}
		c := &Container{ID: e.Actor.ID, Name: a["name"], Image: a["image"], Labels: labels}
		if raw, ok := a["exitCode"]; ok {
			if n, err := strconv.Atoi(raw); err == nil {
				c.ExitCode = &n
			}
		}
		p.Container = c
	}
	return p, nil
}
