package dockerapi

import (
	"encoding/json"
	"sort"
	"strings"
)

const dockerfileHeader = "# FROM unknown — base image cannot be recovered from history"

// ReconstructDockerfile produces an explicitly approximate Dockerfile from
// Docker's newest-first history and the final image configuration.
func ReconstructDockerfile(history []HistoryLayer, cfg ImageConfig) string {
	lines := []string{dockerfileHeader}
	for i := len(history) - 1; i >= 0; i-- {
		layer := history[i]
		if layer.ID == "<missing>" {
			lines = append(lines, "# <missing> layer has no local metadata")
		}
		createdBy := strings.TrimSpace(layer.CreatedBy)
		if createdBy == "" {
			continue
		}
		if command, ok := strings.CutPrefix(createdBy, "/bin/sh -c "); ok {
			lines = append(lines, command)
		} else {
			lines = append(lines, "# "+createdBy)
		}
	}
	for _, env := range cfg.Env {
		if env != "" {
			lines = append(lines, "ENV "+env)
		}
	}
	ports := make([]string, 0, len(cfg.ExposedPorts))
	for port := range cfg.ExposedPorts {
		ports = append(ports, port)
	}
	sort.Strings(ports)
	for _, port := range ports {
		lines = append(lines, "EXPOSE "+port)
	}
	if cfg.WorkingDir != "" {
		lines = append(lines, "WORKDIR "+cfg.WorkingDir)
	}
	if cfg.User != "" {
		lines = append(lines, "USER "+cfg.User)
	}
	labelKeys := make([]string, 0, len(cfg.Labels))
	for key := range cfg.Labels {
		labelKeys = append(labelKeys, key)
	}
	sort.Strings(labelKeys)
	for _, key := range labelKeys {
		lines = append(lines, "LABEL "+key+"="+dockerfileQuote(cfg.Labels[key]))
	}
	if len(cfg.Entrypoint) > 0 {
		lines = append(lines, "ENTRYPOINT "+dockerfileJSON(cfg.Entrypoint))
	}
	if len(cfg.Cmd) > 0 {
		lines = append(lines, "CMD "+dockerfileJSON(cfg.Cmd))
	}
	return strings.Join(lines, "\n") + "\n"
}

func dockerfileJSON(values []string) string {
	encoded, _ := json.Marshal(values)
	return string(encoded)
}
func dockerfileQuote(value string) string { encoded, _ := json.Marshal(value); return string(encoded) }
