// Package webhook dispatches Docker events to configured webhook endpoints.
package webhook

import (
	"encoding/json"
	"path"
	"strings"

	"github.com/0funct0ry/vessel/internal/dockerapi"
	"github.com/0funct0ry/vessel/internal/store"
)

// EventType returns Vessel's stable type.action spelling for an Engine event.
func EventType(e dockerapi.Event) string { return e.Type + "." + e.Action }

// Matches reports whether webhook w accepts e. Event patterns and all configured
// filters must match; malformed filter JSON deliberately matches nothing.
func Matches(w store.Webhook, e dockerapi.Event) bool {
	if !w.Enabled || !matchesEvent(w.EventTypes, EventType(e)) {
		return false
	}
	var f struct {
		Name  string            `json:"name"`
		Image string            `json:"image"`
		Label map[string]string `json:"label"`
	}
	if len(w.Filters) != 0 && json.Unmarshal(w.Filters, &f) != nil {
		return false
	}
	a := e.Actor.Attributes
	if f.Name != "" && !glob(f.Name, a["name"]) {
		return false
	}
	if f.Image != "" && !glob(f.Image, a["image"]) {
		return false
	}
	for k, v := range f.Label {
		if a["label."+k] != v && a[k] != v {
			return false
		}
	}
	return true
}

func matchesEvent(patterns []string, event string) bool {
	for _, p := range patterns {
		if p == "*" || p == event || strings.HasSuffix(p, ".*") && strings.HasPrefix(event, strings.TrimSuffix(p, "*")) {
			return true
		}
	}
	return false
}
func glob(pattern, value string) bool { ok, err := path.Match(pattern, value); return err == nil && ok }
