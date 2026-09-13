// Package compose parses a deliberately small subset of the Compose file
// format and deploys it with Vessel's own Engine API client. It is not a
// reimplementation of `docker compose`: only the keys Vessel's UI can round
// trip are decoded, everything else is preserved verbatim in Service.Raw so a
// stored stack never silently loses the user's YAML.
package compose

// Service is one entry under the compose file's top-level `services` key.
type Service struct {
	Image      string            `json:"image"`
	Command    string            `json:"command,omitempty"`
	Entrypoint string            `json:"entrypoint,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
	Ports      []string          `json:"ports,omitempty"`
	Volumes    []string          `json:"volumes,omitempty"`
	Networks   []string          `json:"networks,omitempty"`
	Restart    string            `json:"restart,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
	DependsOn  []string          `json:"depends_on,omitempty"`

	// Raw is the service's decoded YAML mapping, including keys Vessel does
	// not model. It is round tripped, never interpreted.
	Raw map[string]any `json:"-"`
}

// NetworkDef is one entry under the top-level `networks` key.
type NetworkDef struct {
	Driver   string            `json:"driver,omitempty"`
	External bool              `json:"external,omitempty"`
	Name     string            `json:"name,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
}

// VolumeDef is one entry under the top-level `volumes` key.
type VolumeDef struct {
	Driver   string            `json:"driver,omitempty"`
	External bool              `json:"external,omitempty"`
	Name     string            `json:"name,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
}

// Compose is the parsed compose file.
type Compose struct {
	Services map[string]Service    `json:"services"`
	Networks map[string]NetworkDef `json:"networks,omitempty"`
	Volumes  map[string]VolumeDef  `json:"volumes,omitempty"`

	// Order is the service order as written, so deployment is deterministic
	// even though Services is a map.
	Order []string `json:"order,omitempty"`
}

// Warning is a non-fatal parse finding surfaced to the user. A compose file
// with warnings still deploys; the UI shows them next to the editor.
type Warning struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

// Warning kinds.
const (
	WarnUnresolvedVar   = "unresolved_variable"
	WarnDependsOn       = "unresolvable_depends_on"
	WarnObsoleteVersion = "obsolete_version"
	WarnUnknownKey      = "unknown_key"
)
