package compose

import (
	"fmt"
	"sort"
	"strings"
)

// Node/Edge kinds for the Graph tab's compose<->graph translation.
const (
	NodeService = "service"
	NodeNetwork = "network"
	NodeVolume  = "volume"

	EdgeDependency = "dependency"
	EdgeNetwork    = "network"
	EdgeMount      = "mount"
)

// Node is one graph vertex: exactly one of Service/Network/Volume is set,
// matching Kind.
type Node struct {
	ID      string      `json:"id"`
	Kind    string      `json:"kind"`
	Name    string      `json:"name"`
	Service *Service    `json:"service,omitempty"`
	Network *NetworkDef `json:"network,omitempty"`
	Volume  *VolumeDef  `json:"volume,omitempty"`
}

// Edge is one graph edge. MountPath/ReadOnly are only meaningful for
// EdgeMount; dependency and network edges carry no extra data.
type Edge struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	From      string `json:"from"`
	To        string `json:"to"`
	MountPath string `json:"mount_path,omitempty"`
	ReadOnly  bool   `json:"read_only,omitempty"`
}

// ToGraph converts a parsed Compose into the node/edge shape the Graph tab
// renders. Bind mounts (a Service.Volumes entry with no matching volume node)
// have no edge to attach to and stay only on the service node's own Volumes
// field, edited from its property panel as plain text.
func ToGraph(c Compose) ([]Node, []Edge) {
	nodes := make([]Node, 0, len(c.Services)+len(c.Networks)+len(c.Volumes))
	edges := []Edge{}

	for _, name := range serviceNames(c) {
		svc := c.Services[name]
		nodes = append(nodes, Node{ID: serviceNodeID(name), Kind: NodeService, Name: name, Service: &svc})
	}
	for _, name := range sortedKeys(c.Networks) {
		def := c.Networks[name]
		nodes = append(nodes, Node{ID: networkNodeID(name), Kind: NodeNetwork, Name: name, Network: &def})
	}
	for _, name := range sortedKeys(c.Volumes) {
		def := c.Volumes[name]
		nodes = append(nodes, Node{ID: volumeNodeID(name), Kind: NodeVolume, Name: name, Volume: &def})
	}

	for _, name := range serviceNames(c) {
		svc := c.Services[name]
		for _, net := range svc.Networks {
			if _, ok := c.Networks[net]; !ok {
				continue
			}
			edges = append(edges, Edge{
				ID:   fmt.Sprintf("network:%s:%s", name, net),
				Kind: EdgeNetwork,
				From: serviceNodeID(name),
				To:   networkNodeID(net),
			})
		}
		for _, entry := range svc.Volumes {
			volName, target, ro, ok := namedVolumeMount(entry, c.Volumes)
			if !ok {
				continue
			}
			edges = append(edges, Edge{
				ID:        fmt.Sprintf("mount:%s:%s:%s", name, volName, target),
				Kind:      EdgeMount,
				From:      volumeNodeID(volName),
				To:        serviceNodeID(name),
				MountPath: target,
				ReadOnly:  ro,
			})
		}
		for _, dep := range svc.DependsOn {
			if _, ok := c.Services[dep]; !ok {
				continue
			}
			edges = append(edges, Edge{
				ID:   fmt.Sprintf("dependency:%s:%s", dep, name),
				Kind: EdgeDependency,
				From: serviceNodeID(dep),
				To:   serviceNodeID(name),
			})
		}
	}

	return nodes, edges
}

// FromGraph rebuilds a Compose from a (possibly user-edited) node/edge set.
// Network and mount edges are the sole source of a service's
// networks/named-volume entries; dependency edges are the sole source of
// depends_on. Bind-mount entries in a service's Volumes (which have no
// corresponding edge) are preserved as-is from the node's own Service field.
func FromGraph(nodes []Node, edges []Edge) (Compose, error) {
	out := Compose{Services: map[string]Service{}, Networks: map[string]NetworkDef{}, Volumes: map[string]VolumeDef{}}

	nodeByID := map[string]Node{}
	for _, n := range nodes {
		nodeByID[n.ID] = n
		switch n.Kind {
		case NodeService:
			if n.Service == nil {
				return Compose{}, fmt.Errorf("compose: service node %q has no service data", n.ID)
			}
			svc := *n.Service
			svc.Networks = nil
			svc.DependsOn = nil
			out.Services[n.Name] = svc
			out.Order = append(out.Order, n.Name)
		case NodeNetwork:
			if n.Network == nil {
				return Compose{}, fmt.Errorf("compose: network node %q has no network data", n.ID)
			}
			out.Networks[n.Name] = *n.Network
		case NodeVolume:
			if n.Volume == nil {
				return Compose{}, fmt.Errorf("compose: volume node %q has no volume data", n.ID)
			}
			out.Volumes[n.Name] = *n.Volume
		default:
			return Compose{}, fmt.Errorf("compose: node %q has unknown kind %q", n.ID, n.Kind)
		}
	}

	for name, svc := range out.Services {
		svc.Volumes = bindMountsOnly(svc.Volumes, out.Volumes)
		out.Services[name] = svc
	}

	for _, e := range edges {
		from, fromOK := nodeByID[e.From]
		to, toOK := nodeByID[e.To]
		if !fromOK || !toOK {
			return Compose{}, fmt.Errorf("compose: edge %q references a missing node", e.ID)
		}
		switch e.Kind {
		case EdgeNetwork:
			svc := out.Services[from.Name]
			svc.Networks = appendUnique(svc.Networks, to.Name)
			out.Services[from.Name] = svc
		case EdgeMount:
			svc := out.Services[to.Name]
			entry := from.Name + ":" + e.MountPath
			if e.ReadOnly {
				entry += ":ro"
			}
			svc.Volumes = append(svc.Volumes, entry)
			out.Services[to.Name] = svc
		case EdgeDependency:
			svc := out.Services[to.Name]
			svc.DependsOn = appendUnique(svc.DependsOn, from.Name)
			out.Services[to.Name] = svc
		default:
			return Compose{}, fmt.Errorf("compose: edge %q has unknown kind %q", e.ID, e.Kind)
		}
	}

	for name, svc := range out.Services {
		sort.Strings(svc.Networks)
		sort.Strings(svc.DependsOn)
		out.Services[name] = svc
	}

	return out, nil
}

func serviceNodeID(name string) string { return "service:" + name }
func networkNodeID(name string) string { return "network:" + name }
func volumeNodeID(name string) string  { return "volume:" + name }

func serviceNames(c Compose) []string {
	if len(c.Order) == len(c.Services) {
		return c.Order
	}
	return sortedKeys(c.Services)
}

// namedVolumeMount parses a short-form `source:target[:ro]` volume entry and
// reports whether source names a volume defined in volumes (as opposed to a
// bind-mount path).
func namedVolumeMount(entry string, volumes map[string]VolumeDef) (name, target string, ro bool, ok bool) {
	parts := strings.Split(entry, ":")
	if len(parts) < 2 {
		return "", "", false, false
	}
	name, target = parts[0], parts[1]
	if len(parts) >= 3 && parts[2] == "ro" {
		ro = true
	}
	if strings.HasPrefix(name, "/") || strings.HasPrefix(name, ".") {
		return "", "", false, false
	}
	if _, defined := volumes[name]; !defined {
		return "", "", false, false
	}
	return name, target, ro, true
}

// bindMountsOnly drops volume entries that name a volume defined in volumes
// (those are rebuilt from mount edges instead), keeping bind mounts and any
// reference to an undefined volume name as plain text so it isn't silently
// lost.
func bindMountsOnly(entries []string, volumes map[string]VolumeDef) []string {
	out := entries[:0:0]
	for _, entry := range entries {
		parts := strings.Split(entry, ":")
		if len(parts) < 2 {
			out = append(out, entry)
			continue
		}
		if _, defined := volumes[parts[0]]; defined {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func appendUnique(list []string, value string) []string {
	for _, existing := range list {
		if existing == value {
			return list
		}
	}
	return append(list, value)
}
