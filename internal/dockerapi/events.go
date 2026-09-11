package dockerapi

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// EventID is stable across independent readers of the same Docker event.
func EventID(e Event) string {
	b := e.Raw
	if len(b) == 0 {
		b = []byte(fmt.Sprintf("%s\x00%s\x00%s\x00%d", e.Type, e.Action, e.Actor.ID, e.TimeNano))
	}
	sum := sha256.Sum256(b)
	return fmt.Sprintf("evt_%x", sum[:12])
}

// Event is one decoded entry from GET /events.
type Event struct {
	Type     string          `json:"Type"`
	Action   string          `json:"Action"`
	Actor    EventActor      `json:"Actor"`
	Time     int64           `json:"time"`
	TimeNano int64           `json:"timeNano"`
	Raw      json.RawMessage `json:"-"`
}

// EventActor mirrors the Actor field of an Event.
type EventActor struct {
	ID         string            `json:"ID"`
	Attributes map[string]string `json:"Attributes"`
}

// EventsOptions controls GET /events.
type EventsOptions struct {
	Since   string
	Until   string
	Filters map[string][]string
}

// EventReader streams decoded events. Call Next in a loop until it returns
// an error (io.EOF on a clean daemon-side close, ctx.Err() on cancellation);
// Close releases the underlying HTTP response.
type EventReader struct {
	body interface{ Close() error }
	dec  *json.Decoder
}

// NewEventReader decodes Docker event JSON from body. It is primarily useful
// to adapters and tests that need to provide an event stream without creating
// a full Engine client.
func NewEventReader(body interface {
	io.Reader
	Close() error
}) *EventReader {
	return &EventReader{body: body, dec: json.NewDecoder(bufio.NewReader(body))}
}

// Close releases the underlying HTTP connection.
func (r *EventReader) Close() error {
	return r.body.Close()
}

// Next decodes and returns the next event.
func (r *EventReader) Next() (Event, error) {
	var raw json.RawMessage
	if err := r.dec.Decode(&raw); err != nil {
		return Event{}, err
	}

	var e Event
	if err := json.Unmarshal(raw, &e); err != nil {
		return Event{}, fmt.Errorf("dockerapi: decoding event: %w", err)
	}
	e.Raw = raw
	return e, nil
}

// Events calls GET /events and returns an EventReader over the response
// body.
func (c *Client) Events(ctx context.Context, opts EventsOptions) (*EventReader, error) {
	q := url.Values{}
	if opts.Since != "" {
		q.Set("since", opts.Since)
	}
	if opts.Until != "" {
		q.Set("until", opts.Until)
	}
	if len(opts.Filters) > 0 {
		f, err := json.Marshal(opts.Filters)
		if err != nil {
			return nil, fmt.Errorf("dockerapi: encoding filters: %w", err)
		}
		q.Set("filters", string(f))
	}

	path := "/events"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url(path), nil)
	if err != nil {
		return nil, fmt.Errorf("dockerapi: building events request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, mapError(resp)
	}

	return NewEventReader(resp.Body), nil
}
