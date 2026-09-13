package compose

import (
	"sort"

	"gopkg.in/yaml.v3"
)

// Marshal re-encodes a parsed Compose back into compose YAML. It is not
// byte-identical to any source file: comments and the original key order
// (beyond Order) are lost, since the generic-map decode ParseCompose and
// ParseComposeStructure both go through already discards them. Each
// Service.Raw's unmodelled keys are preserved; modeled fields win when a key
// appears in both.
func (c Compose) Marshal() ([]byte, error) {
	doc := map[string]any{}

	services := map[string]any{}
	for _, name := range c.serviceOrder() {
		services[name] = c.Services[name].marshal()
	}
	doc["services"] = services

	if len(c.Networks) > 0 {
		networks := map[string]any{}
		for name, def := range c.Networks {
			networks[name] = def.marshal()
		}
		doc["networks"] = networks
	}
	if len(c.Volumes) > 0 {
		volumes := map[string]any{}
		for name, def := range c.Volumes {
			volumes[name] = def.marshal()
		}
		doc["volumes"] = volumes
	}

	return yaml.Marshal(doc)
}

func (c Compose) serviceOrder() []string {
	seen := map[string]bool{}
	order := make([]string, 0, len(c.Services))
	for _, name := range c.Order {
		if _, ok := c.Services[name]; ok && !seen[name] {
			order = append(order, name)
			seen[name] = true
		}
	}
	for name := range c.Services {
		if !seen[name] {
			order = append(order, name)
			seen[name] = true
		}
	}
	return order
}

func (s Service) marshal() map[string]any {
	out := map[string]any{}
	for k, v := range s.Raw {
		out[k] = v
	}
	if s.Image != "" {
		out["image"] = s.Image
	} else {
		delete(out, "image")
	}
	setOrDelete(out, "command", s.Command)
	setOrDelete(out, "entrypoint", s.Entrypoint)
	setOrDelete(out, "restart", s.Restart)
	if len(s.Env) > 0 {
		out["environment"] = sortedMap(s.Env)
	} else {
		delete(out, "environment")
	}
	if len(s.Ports) > 0 {
		out["ports"] = s.Ports
	} else {
		delete(out, "ports")
	}
	if len(s.Volumes) > 0 {
		out["volumes"] = s.Volumes
	} else {
		delete(out, "volumes")
	}
	if len(s.Networks) > 0 {
		list := append([]string(nil), s.Networks...)
		sort.Strings(list)
		out["networks"] = list
	} else {
		delete(out, "networks")
	}
	if len(s.Labels) > 0 {
		out["labels"] = sortedMap(s.Labels)
	} else {
		delete(out, "labels")
	}
	if len(s.DependsOn) > 0 {
		list := append([]string(nil), s.DependsOn...)
		sort.Strings(list)
		out["depends_on"] = list
	} else {
		delete(out, "depends_on")
	}
	return out
}

func (n NetworkDef) marshal() map[string]any {
	out := map[string]any{}
	setOrDelete(out, "driver", n.Driver)
	setOrDelete(out, "name", n.Name)
	if n.External {
		out["external"] = true
	}
	if n.Internal {
		out["internal"] = true
	}
	if n.Attachable {
		out["attachable"] = true
	}
	if len(n.Labels) > 0 {
		out["labels"] = sortedMap(n.Labels)
	}
	if len(n.DriverOpts) > 0 {
		out["driver_opts"] = sortedMap(n.DriverOpts)
	}
	if n.IPAMSubnet != "" || n.IPAMGateway != "" {
		entry := map[string]any{}
		if n.IPAMSubnet != "" {
			entry["subnet"] = n.IPAMSubnet
		}
		if n.IPAMGateway != "" {
			entry["gateway"] = n.IPAMGateway
		}
		out["ipam"] = map[string]any{"config": []any{entry}}
	}
	return out
}

func (v VolumeDef) marshal() map[string]any {
	out := map[string]any{}
	setOrDelete(out, "driver", v.Driver)
	setOrDelete(out, "name", v.Name)
	if v.External {
		out["external"] = true
	}
	if len(v.Labels) > 0 {
		out["labels"] = sortedMap(v.Labels)
	}
	if len(v.DriverOpts) > 0 {
		out["driver_opts"] = sortedMap(v.DriverOpts)
	}
	return out
}

func setOrDelete(out map[string]any, key, value string) {
	if value != "" {
		out[key] = value
	} else {
		delete(out, key)
	}
}

// sortedMap exists only to name the intent at each call site; yaml.v3 already
// sorts map[string]string keys on encode.
func sortedMap(m map[string]string) map[string]string {
	return m
}
