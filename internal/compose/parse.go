package compose

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ErrNoServices is returned for a compose file with no usable `services` key.
var ErrNoServices = errors.New("compose: file declares no services")

// variableRE matches $$ (an escaped dollar), ${...} and $NAME.
var variableRE = regexp.MustCompile(`\$(\$|\{[^{}]*\}|[A-Za-z_][A-Za-z0-9_]*)`)

// ParseCompose decodes a compose file, interpolating ${VAR} references against
// env. It returns the decoded document, any non-fatal findings, and an error
// only when the YAML itself is unusable.
func ParseCompose(yamlBytes []byte, env map[string]string) (Compose, []Warning, error) {
	var doc map[string]any
	if err := yaml.Unmarshal(yamlBytes, &doc); err != nil {
		return Compose{}, nil, fmt.Errorf("compose: %w", err)
	}
	if len(doc) == 0 {
		return Compose{}, nil, ErrNoServices
	}

	warnings := []Warning{}
	if _, ok := doc["version"]; ok {
		warnings = append(warnings, Warning{WarnObsoleteVersion, "the top-level `version:` key is obsolete and is ignored"})
	}

	interpolated, varWarnings := interpolateValue(doc, env)
	warnings = append(warnings, varWarnings...)
	doc, _ = interpolated.(map[string]any)

	rawServices, ok := doc["services"].(map[string]any)
	if !ok || len(rawServices) == 0 {
		return Compose{}, warnings, ErrNoServices
	}

	out := Compose{Services: map[string]Service{}, Networks: map[string]NetworkDef{}, Volumes: map[string]VolumeDef{}, Order: serviceOrder(yamlBytes, rawServices)}
	for name, raw := range rawServices {
		mapping, ok := raw.(map[string]any)
		if !ok {
			warnings = append(warnings, Warning{WarnUnknownKey, fmt.Sprintf("service %q is not a mapping and was skipped", name)})
			continue
		}
		out.Services[name] = decodeService(mapping)
	}
	for name, raw := range mappingOf(doc["networks"]) {
		out.Networks[name] = NetworkDef{Driver: str(valueOf(raw, "driver")), External: truthy(valueOf(raw, "external")), Name: str(valueOf(raw, "name")), Labels: stringMap(valueOf(raw, "labels"))}
	}
	for name, raw := range mappingOf(doc["volumes"]) {
		out.Volumes[name] = VolumeDef{Driver: str(valueOf(raw, "driver")), External: truthy(valueOf(raw, "external")), Name: str(valueOf(raw, "name")), Labels: stringMap(valueOf(raw, "labels"))}
	}

	// depends_on targets must name a service in the same file.
	for _, name := range out.Order {
		for _, dep := range out.Services[name].DependsOn {
			if _, ok := out.Services[dep]; !ok {
				warnings = append(warnings, Warning{WarnDependsOn, fmt.Sprintf("service %q depends on %q, which this file does not define", name, dep)})
			}
		}
	}
	if len(out.Services) == 0 {
		return out, warnings, ErrNoServices
	}
	return out, warnings, nil
}

func decodeService(mapping map[string]any) Service {
	service := Service{
		Image:      str(mapping["image"]),
		Command:    commandString(mapping["command"]),
		Entrypoint: commandString(mapping["entrypoint"]),
		Env:        stringMap(mapping["environment"]),
		Ports:      stringSlice(mapping["ports"]),
		Volumes:    stringSlice(mapping["volumes"]),
		Networks:   networkNames(mapping["networks"]),
		Restart:    str(mapping["restart"]),
		Labels:     stringMap(mapping["labels"]),
		DependsOn:  dependsOn(mapping["depends_on"]),
		Raw:        mapping,
	}
	if service.Env == nil {
		service.Env = map[string]string{}
	}
	if service.Labels == nil {
		service.Labels = map[string]string{}
	}
	return service
}

// serviceOrder recovers the order the services were written in, which the
// generic map decode loses. It falls back to sorted names.
func serviceOrder(yamlBytes []byte, services map[string]any) []string {
	order := make([]string, 0, len(services))
	var root yaml.Node
	if err := yaml.Unmarshal(yamlBytes, &root); err == nil && len(root.Content) == 1 {
		doc := root.Content[0]
		for i := 0; i+1 < len(doc.Content); i += 2 {
			if doc.Content[i].Value != "services" {
				continue
			}
			node := doc.Content[i+1]
			for j := 0; j+1 < len(node.Content); j += 2 {
				if _, ok := services[node.Content[j].Value]; ok {
					order = append(order, node.Content[j].Value)
				}
			}
		}
	}
	if len(order) == len(services) {
		return order
	}
	order = order[:0]
	for name := range services {
		order = append(order, name)
	}
	sort.Strings(order)
	return order
}

// interpolateValue walks a decoded YAML tree substituting ${VAR} references in
// every string it finds.
func interpolateValue(value any, env map[string]string) (any, []Warning) {
	switch typed := value.(type) {
	case string:
		return interpolate(typed, env)
	case []any:
		warnings := []Warning{}
		out := make([]any, len(typed))
		for i, item := range typed {
			replaced, w := interpolateValue(item, env)
			out[i], warnings = replaced, append(warnings, w...)
		}
		return out, warnings
	case map[string]any:
		warnings := []Warning{}
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			replaced, w := interpolateValue(item, env)
			out[key], warnings = replaced, append(warnings, w...)
		}
		return out, warnings
	default:
		return value, nil
	}
}

// interpolate resolves ${VAR}, ${VAR:-default} and ${VAR:?message} against env.
// An unset ${VAR:?message} becomes an empty string plus a warning rather than a
// hard failure, so the editor can show every problem at once.
func interpolate(text string, env map[string]string) (string, []Warning) {
	warnings := []Warning{}
	out := variableRE.ReplaceAllStringFunc(text, func(match string) string {
		body := match[1:]
		if body == "$" {
			return "$"
		}
		if !strings.HasPrefix(body, "{") {
			return env[body]
		}
		body = body[1 : len(body)-1]
		name, operator, argument := splitVariable(body)
		value, ok := env[name]
		switch {
		case ok && value != "":
			return value
		case operator == ":-" || operator == "-":
			return argument
		case operator == ":?" || operator == "?":
			message := argument
			if message == "" {
				message = "is required"
			}
			warnings = append(warnings, Warning{WarnUnresolvedVar, fmt.Sprintf("%s %s", name, message)})
			return ""
		default:
			return value
		}
	})
	return out, warnings
}

func splitVariable(body string) (name, operator, argument string) {
	for _, candidate := range []string{":-", ":?", ":+", "-", "?", "+"} {
		if at := strings.Index(body, candidate); at > 0 {
			return body[:at], candidate, body[at+len(candidate):]
		}
	}
	return body, "", ""
}

// commandString flattens compose's two forms for command/entrypoint (a shell
// string, or an exec-form list) into the single string the UI edits.
func commandString(value any) string {
	if list, ok := value.([]any); ok {
		parts := make([]string, 0, len(list))
		for _, item := range list {
			parts = append(parts, str(item))
		}
		return strings.Join(parts, " ")
	}
	return str(value)
}

func str(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case bool:
		return strconv.FormatBool(typed)
	case int:
		return strconv.Itoa(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		return fmt.Sprint(typed)
	}
}

func truthy(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return typed == "true" || typed == "yes"
	case map[string]any:
		return true
	default:
		return false
	}
}

func valueOf(value any, key string) any {
	if mapping, ok := value.(map[string]any); ok {
		return mapping[key]
	}
	return nil
}

// mappingOf normalizes a top-level networks/volumes block, whose entries may be
// null (`mynet:`) as well as mappings.
func mappingOf(value any) map[string]any {
	out := map[string]any{}
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			out[key] = item
		}
	case []any:
		for _, item := range typed {
			if name := str(item); name != "" {
				out[name] = nil
			}
		}
	}
	return out
}

// stringMap normalizes both compose forms for environment and labels: a
// mapping (`{K: V}`) and a list of `K=V` (or bare `K`) entries.
func stringMap(value any) map[string]string {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]string, len(typed))
		for key, item := range typed {
			out[key] = str(item)
		}
		return out
	case []any:
		out := make(map[string]string, len(typed))
		for _, item := range typed {
			entry := str(item)
			if at := strings.Index(entry, "="); at >= 0 {
				out[entry[:at]] = entry[at+1:]
			} else if entry != "" {
				out[entry] = ""
			}
		}
		return out
	default:
		return nil
	}
}

func stringSlice(value any) []string {
	switch typed := value.(type) {
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if mapping, ok := item.(map[string]any); ok {
				out = append(out, longFormMount(mapping))
				continue
			}
			out = append(out, str(item))
		}
		return out
	case string:
		return []string{typed}
	default:
		return nil
	}
}

// longFormMount collapses the long-form `volumes:` entry back into the short
// `source:target[:ro]` string the engine layer already understands.
func longFormMount(mapping map[string]any) string {
	entry := str(mapping["source"]) + ":" + str(mapping["target"])
	if truthy(mapping["read_only"]) {
		entry += ":ro"
	}
	return entry
}

func networkNames(value any) []string {
	if mapping, ok := value.(map[string]any); ok {
		out := make([]string, 0, len(mapping))
		for name := range mapping {
			out = append(out, name)
		}
		sort.Strings(out)
		return out
	}
	return stringSlice(value)
}

func dependsOn(value any) []string {
	if mapping, ok := value.(map[string]any); ok {
		out := make([]string, 0, len(mapping))
		for name := range mapping {
			out = append(out, name)
		}
		sort.Strings(out)
		return out
	}
	return stringSlice(value)
}

// ParseEnvFile decodes a .env file into the map ParseCompose interpolates
// against. Blank lines and `#` comments are ignored; `export K=V` is accepted.
func ParseEnvFile(content string) map[string]string {
	env := map[string]string{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		at := strings.Index(line, "=")
		if at <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:at])
		value := strings.TrimSpace(line[at+1:])
		if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
			value = value[1 : len(value)-1]
		}
		env[key] = value
	}
	return env
}
