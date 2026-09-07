package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type sseEvent struct {
	name    string
	data    []byte
	warning bool
}

// StreamWithOverflow is Stream with a bounded, non-blocking producer queue.
// When the queue fills, it discards oldest events and keeps one updated
// overflow event supplied by overflow. It is intended for high-volume feeds.
func StreamWithOverflow(c *gin.Context, capacity int, overflow func(int) (string, any), fn func(send func(string, any) error) error) {
	if capacity < 2 {
		capacity = 2
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	c.Writer.Flush()

	var mu sync.Mutex
	queue := make([]sseEvent, 0, capacity)
	droppedTotal := 0
	finished := false
	var result error
	wake := make(chan struct{}, 1)
	signal := func() {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
	push := func(event sseEvent) {
		mu.Lock()
		defer mu.Unlock()
		if len(queue) >= capacity {
			dropped := 0
			for len(queue) >= capacity {
				index := 0
				for i := range queue {
					if !queue[i].warning {
						index = i
						break
					}
				}
				queue = append(queue[:index], queue[index+1:]...)
				dropped++
			}
			warningIndex := -1
			for i := range queue {
				if queue[i].warning {
					warningIndex = i
					break
				}
			}
			if warningIndex == -1 {
				if len(queue) >= capacity-1 && len(queue) > 0 {
					queue = queue[1:]
					dropped++
				}
				droppedTotal += dropped
				name, value := overflow(droppedTotal)
				data, _ := json.Marshal(value)
				queue = append(queue, sseEvent{name: name, data: data, warning: true})
			} else {
				droppedTotal += dropped
				name, value := overflow(droppedTotal)
				data, _ := json.Marshal(value)
				queue[warningIndex].name, queue[warningIndex].data = name, data
			}
		}
		queue = append(queue, event)
		signal()
	}
	send := func(name string, value any) error {
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		select {
		case <-c.Request.Context().Done():
			return c.Request.Context().Err()
		default:
		}
		push(sseEvent{name: name, data: data})
		return nil
	}
	go func() {
		err := fn(send)
		mu.Lock()
		finished, result = true, err
		mu.Unlock()
		signal()
	}()

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		mu.Lock()
		if len(queue) > 0 {
			event := queue[0]
			queue = queue[1:]
			if event.warning {
				droppedTotal = 0
			}
			mu.Unlock()
			if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event.name, event.data); err != nil {
				return
			}
			c.Writer.Flush()
			continue
		}
		done, err := finished, result
		mu.Unlock()
		if done {
			if err != nil {
				_, _ = fmt.Fprintf(c.Writer, "event: error\ndata: {\"code\":\"stream_closed\",\"message\":%q}\n\n", err.Error())
				c.Writer.Flush()
			}
			return
		}
		select {
		case <-wake:
		case <-ticker.C:
			_, _ = fmt.Fprint(c.Writer, ": keepalive\n\n")
			c.Writer.Flush()
		case <-c.Request.Context().Done():
			return
		}
	}
}

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
