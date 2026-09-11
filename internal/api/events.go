package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/0funct0ry/vessel/internal/dockerapi"
	"github.com/0funct0ry/vessel/internal/store"
)

// eventView is the stable browser-facing shape for Docker's event stream.
// Docker's capitalized wire fields stay inside dockerapi.
type eventView struct {
	EventID   string            `json:"event_id"`
	Type      string            `json:"type"`
	Action    string            `json:"action"`
	ID        string            `json:"id"`
	Name      string            `json:"name,omitempty"`
	Attrs     map[string]string `json:"attrs"`
	Timestamp string            `json:"timestamp"`
}

func storedEventToView(event store.Event) eventView {
	attrs := map[string]string{}
	_ = json.Unmarshal(event.Attrs, &attrs)
	return eventView{EventID: event.ID, Type: event.Type, Action: event.Action, ID: event.SubjectID, Name: event.Name, Attrs: attrs, Timestamp: event.CreatedAt.UTC().Format(time.RFC3339Nano)}
}

func parseEventTypes(c *gin.Context) ([]string, error) {
	valid := map[string]bool{"container": true, "image": true, "volume": true, "network": true}
	seen := make(map[string]bool)
	var types []string
	for _, raw := range c.QueryArray("type") {
		for _, value := range strings.Split(raw, ",") {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			if !valid[value] {
				return nil, invalidQuery("type must be container, image, volume, or network")
			}
			if !seen[value] {
				seen[value] = true
				types = append(types, value)
			}
		}
	}
	return types, nil
}

func parseEventSince(c *gin.Context) (string, error) {
	since := strings.TrimSpace(c.Query("since"))
	if since == "" {
		return "", nil
	}
	if _, err := strconv.ParseInt(since, 10, 64); err == nil {
		return since, nil
	}
	if _, err := time.Parse(time.RFC3339, since); err == nil {
		return since, nil
	}
	return "", invalidQuery("since must be a Unix timestamp or RFC3339 time")
}

func eventToView(event dockerapi.Event) eventView {
	attrs := event.Actor.Attributes
	if attrs == nil {
		attrs = map[string]string{}
	}
	name := attrs["name"]
	if name == "" {
		name = attrs["container"]
	}
	return eventView{EventID: dockerapi.EventID(event), Type: event.Type, Action: event.Action, ID: event.Actor.ID, Name: name, Attrs: attrs, Timestamp: time.Unix(event.Time, 0).UTC().Format(time.RFC3339Nano)}
}

func persistEvent(ctx context.Context, persistence store.Store, event dockerapi.Event) error {
	if persistence == nil {
		return nil
	}
	attrs, err := json.Marshal(event.Actor.Attributes)
	if err != nil {
		return err
	}
	name := event.Actor.Attributes["name"]
	if name == "" {
		name = event.Actor.Attributes["container"]
	}
	created := time.Unix(event.Time, 0).UTC()
	if event.TimeNano > 0 {
		created = time.Unix(0, event.TimeNano).UTC()
	}
	_, err = persistence.CreateEvent(ctx, store.Event{ID: dockerapi.EventID(event), Type: event.Type, Action: event.Action, SubjectID: event.Actor.ID, Name: name, Attrs: attrs, CreatedAt: created})
	if errors.Is(err, store.ErrConflict) {
		return nil
	}
	return err
}

func parseEventLimit(c *gin.Context) (int, error) {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "5000"))
	if err != nil || limit < 1 || limit > 5000 {
		return 0, invalidQuery("limit must be between 1 and 5000")
	}
	return limit, nil
}

func (s *server) handleEvents(c *gin.Context) {
	types, err := parseEventTypes(c)
	if err != nil {
		Fail(c, err)
		return
	}
	since, err := parseEventSince(c)
	if err != nil {
		Fail(c, err)
		return
	}
	limit, err := parseEventLimit(c)
	if err != nil {
		Fail(c, err)
		return
	}
	filters := map[string][]string(nil)
	if len(types) > 0 {
		filters = map[string][]string{"type": types}
	}
	stored := []store.Event{}
	if s.store != nil {
		stored, err = s.store.ListEvents(c.Request.Context(), store.EventQuery{Limit: limit, Types: types})
		if err != nil {
			Fail(c, err)
			return
		}
	}
	reader, err := s.docker.Events(c.Request.Context(), dockerapi.EventsOptions{Since: since, Filters: filters})
	if err != nil {
		Fail(c, err)
		return
	}
	defer reader.Close()
	Stream(c, func(send func(string, any) error) error {
		for _, event := range stored {
			if err := send("docker", storedEventToView(event)); err != nil {
				return err
			}
		}
		for {
			event, err := reader.Next()
			if err != nil {
				if errors.Is(err, io.EOF) || errors.Is(err, c.Request.Context().Err()) {
					return nil
				}
				return err
			}
			if err := persistEvent(c.Request.Context(), s.store, event); err != nil {
				return err
			}
			if err := send("docker", eventToView(event)); err != nil {
				return err
			}
		}
	})
}

func (s *server) handleEventDelete(c *gin.Context) {
	if s.store == nil {
		Fail(c, store.ErrNotFound)
		return
	}
	if err := s.store.DeleteEvent(c, c.Param("id")); err != nil {
		Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *server) handleEventsClear(c *gin.Context) {
	if s.store == nil {
		Fail(c, store.ErrNotFound)
		return
	}
	n, err := s.store.ClearEvents(c)
	if err != nil {
		Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": n})
}
