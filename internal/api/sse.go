package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Stream writes a named SSE stream with cache-safe headers and periodic
// keepalives. The callback receives a sender tied to the request context.
func Stream(c *gin.Context, fn func(send func(string, any) error) error) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	c.Writer.Flush()

	type event struct {
		name string
		data []byte
	}
	events := make(chan event, 16)
	send := func(name string, value any) error {
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		select {
		case events <- event{name, data}:
			return nil
		case <-c.Request.Context().Done():
			return c.Request.Context().Err()
		}
	}

	done := make(chan error, 1)
	go func() { done <- fn(send); close(events) }()
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case event, ok := <-events:
			if !ok {
				err := <-done
				if err != nil {
					_, _ = fmt.Fprintf(c.Writer, "event: error\ndata: {\"code\":\"stream_closed\",\"message\":%q}\n\n", err.Error())
					c.Writer.Flush()
				}
				return
			}
			if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event.name, event.data); err != nil {
				return
			}
			c.Writer.Flush()
		case <-ticker.C:
			_, _ = fmt.Fprint(c.Writer, ": keepalive\n\n")
			c.Writer.Flush()
		case <-c.Request.Context().Done():
			return
		}
	}
}
