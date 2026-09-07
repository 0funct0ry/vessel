package api

import (
	"net/url"
	"sort"
	"strconv"
	"strings"
)

type collectionQuery struct {
	q      string
	status string
	sort   string
	all    bool
}

func parseCollectionQuery(values url.Values, collection string) (collectionQuery, error) {
	query := collectionQuery{q: strings.ToLower(strings.TrimSpace(values.Get("q")))}

	if raw, ok := singleValue(values, "all"); ok {
		if collection != "containers" {
			return query, invalidQuery("all is not supported for %s", collection)
		}
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return query, invalidQuery("all must be a boolean")
		}
		query.all = parsed
	}

	if raw, ok := singleValue(values, "status"); ok {
		if collection != "containers" {
			return query, invalidQuery("status is not supported for %s", collection)
		}
		query.status = strings.ToLower(strings.TrimSpace(raw))
		if query.status == "" {
			return query, invalidQuery("status must not be empty")
		}
	}

	if raw, ok := singleValue(values, "sort"); ok {
		query.sort = strings.ToLower(strings.TrimSpace(raw))
		if !validSort(collection, query.sort) {
			return query, invalidQuery("sort %q is not supported for %s", raw, collection)
		}
	}

	return query, nil
}

func singleValue(values url.Values, key string) (string, bool) {
	items, ok := values[key]
	if !ok {
		return "", false
	}
	if len(items) != 1 {
		return "", true
	}
	return items[0], true
}

func validSort(collection, key string) bool {
	switch collection {
	case "containers":
		return key == "name" || key == "created" || key == "state" || key == "cpu"
	case "images", "volumes":
		return key == "name" || key == "created"
	case "networks":
		return key == "name"
	default:
		return false
	}
}

func filterSortContainers(items []containerView, query collectionQuery) []containerView {
	filtered := make([]containerView, 0, len(items))
	for _, item := range items {
		if query.status != "" && strings.ToLower(item.State) != query.status {
			continue
		}
		haystack := strings.ToLower(strings.Join([]string{item.ID, item.Name, item.Image, item.ImageID, item.Status}, "\x00"))
		if query.q == "" || strings.Contains(haystack, query.q) {
			filtered = append(filtered, item)
		}
	}
	if query.sort == "cpu" || query.sort == "" {
		return filtered
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		a, b := filtered[i], filtered[j]
		switch query.sort {
		case "created":
			if a.Created != b.Created {
				return a.Created > b.Created
			}
		case "state":
			if a.State != b.State {
				return a.State < b.State
			}
		case "name":
			if a.Name != b.Name {
				return a.Name < b.Name
			}
		}
		return a.ID < b.ID
	})
	return filtered
}

func filterSortImages(items []imageView, query collectionQuery) []imageView {
	filtered := make([]imageView, 0, len(items))
	for _, item := range items {
		haystack := strings.ToLower(strings.Join(append([]string{item.ID}, append(item.RepoTags, item.RepoDigests...)...), "\x00"))
		if query.q == "" || strings.Contains(haystack, query.q) {
			filtered = append(filtered, item)
		}
	}
	if query.sort == "" {
		return filtered
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		a, b := filtered[i], filtered[j]
		if query.sort == "created" && a.Created != b.Created {
			return a.Created > b.Created
		}
		if query.sort == "name" {
			an, bn := firstOrEmpty(a.RepoTags), firstOrEmpty(b.RepoTags)
			if an != bn {
				return an < bn
			}
		}
		return a.ID < b.ID
	})
	return filtered
}

func filterSortVolumes(items []volumeView, query collectionQuery) []volumeView {
	filtered := make([]volumeView, 0, len(items))
	for _, item := range items {
		haystack := strings.ToLower(strings.Join([]string{item.Name, item.Driver, item.Mountpoint, item.Scope}, "\x00"))
		if query.q == "" || strings.Contains(haystack, query.q) {
			filtered = append(filtered, item)
		}
	}
	if query.sort == "" {
		return filtered
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		a, b := filtered[i], filtered[j]
		if query.sort == "created" && a.CreatedAt != b.CreatedAt {
			return a.CreatedAt > b.CreatedAt
		}
		if query.sort == "name" && a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Name < b.Name
	})
	return filtered
}

func filterSortNetworks(items []networkView, query collectionQuery) []networkView {
	filtered := make([]networkView, 0, len(items))
	for _, item := range items {
		haystack := strings.ToLower(strings.Join([]string{item.ID, item.Name, item.Driver, item.Scope}, "\x00"))
		if query.q == "" || strings.Contains(haystack, query.q) {
			filtered = append(filtered, item)
		}
	}
	if query.sort == "name" {
		sort.SliceStable(filtered, func(i, j int) bool {
			if filtered[i].Name == filtered[j].Name {
				return filtered[i].ID < filtered[j].ID
			}
			return filtered[i].Name < filtered[j].Name
		})
	}
	return filtered
}

func firstOrEmpty(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
