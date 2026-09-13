package compose

import (
	"errors"
	"reflect"
	"sort"

	"gopkg.in/yaml.v3"
)

// Patch re-encodes a Compose by mutating the *yaml.Node document it was
// originally parsed from (via ParseComposeStructureNode), rather than
// rebuilding the file from scratch the way Marshal does. Only the keys whose
// decoded value actually changed are touched: everything else — sibling
// keys, key order, indentation — round-trips through the original Node
// objects untouched. A brand-new service/network/volume is appended at the
// end of its parent mapping, matching the observed behavior of every
// reference compose-graph editor this package's tests compare against.
//
// This is the patch path graph edits should use; Marshal remains for the
// one case Patch cannot serve — a stack with no prior parsed document to
// patch against (e.g. a stack being created for the first time).
func (c Compose) Patch(original *yaml.Node) ([]byte, error) {
	doc := original
	if doc != nil && doc.Kind == yaml.DocumentNode {
		if len(doc.Content) == 0 {
			return c.Marshal()
		}
		doc = doc.Content[0]
	}
	if doc == nil || doc.Kind != yaml.MappingNode {
		return nil, errors.New("compose: patch target is not a mapping document")
	}

	patchServices(doc, c)
	patchNetworks(doc, c)
	patchVolumes(doc, c)

	return yaml.Marshal(original)
}

// --- generic yaml.Node mapping helpers -------------------------------------

func mappingKeyIndex(m *yaml.Node, key string) int {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return i
		}
	}
	return -1
}

func mappingGet(m *yaml.Node, key string) *yaml.Node {
	i := mappingKeyIndex(m, key)
	if i < 0 {
		return nil
	}
	return m.Content[i+1]
}

// mappingSet assigns key to value, replacing the value node in place (so its
// position is preserved) if key already exists, or appending a new key/value
// pair at the end otherwise.
func mappingSet(m *yaml.Node, key string, value *yaml.Node) {
	i := mappingKeyIndex(m, key)
	if i >= 0 {
		m.Content[i+1] = value
		return
	}
	m.Content = append(m.Content, strNode(key), value)
}

func mappingDelete(m *yaml.Node, key string) {
	i := mappingKeyIndex(m, key)
	if i < 0 {
		return
	}
	m.Content = append(m.Content[:i], m.Content[i+2:]...)
}

// --- node builders (used only for brand-new keys/resources) ----------------

func strNode(s string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s} }

func boolNode(b bool) *yaml.Node {
	v := "false"
	if b {
		v = "true"
	}
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: v}
}

func seqNode(items []string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, item := range items {
		n.Content = append(n.Content, strNode(item))
	}
	return n
}

func mapNode(pairs map[string]string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, k := range sortedStringMapKeys(pairs) {
		n.Content = append(n.Content, strNode(k), strNode(pairs[k]))
	}
	return n
}

func sortedStringMapKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// setScalarOrDelete sets key to a plain string scalar, or removes it when
// value is empty (matching Marshal's omitempty convention).
func setScalarOrDelete(m *yaml.Node, key, value string) {
	if value == "" {
		mappingDelete(m, key)
		return
	}
	mappingSet(m, key, strNode(value))
}

func setSliceOrDelete(m *yaml.Node, key string, value []string) {
	if len(value) == 0 {
		mappingDelete(m, key)
		return
	}
	mappingSet(m, key, seqNode(value))
}

func setMapOrDelete(m *yaml.Node, key string, value map[string]string) {
	if len(value) == 0 {
		mappingDelete(m, key)
		return
	}
	mappingSet(m, key, mapNode(value))
}

func setBoolOrDelete(m *yaml.Node, key string, value bool) {
	if !value {
		mappingDelete(m, key)
		return
	}
	mappingSet(m, key, boolNode(true))
}

// nodeAsMap decodes a *yaml.Node into the generic map[string]any shape the
// rest of this package's decode helpers already operate on.
func nodeAsMap(n *yaml.Node) (map[string]any, bool) {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil, false
	}
	var out map[string]any
	if err := n.Decode(&out); err != nil {
		return nil, false
	}
	return out, true
}

// --- services ---------------------------------------------------------------

func patchServices(doc *yaml.Node, c Compose) {
	services := mappingGet(doc, "services")
	if services == nil || services.Kind != yaml.MappingNode {
		services = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		mappingSet(doc, "services", services)
	}

	// Drop services no longer present.
	for i := 0; i+1 < len(services.Content); {
		if _, ok := c.Services[services.Content[i].Value]; !ok {
			services.Content = append(services.Content[:i], services.Content[i+2:]...)
			continue
		}
		i += 2
	}

	for _, name := range c.serviceOrder() {
		svc := c.Services[name]
		existing := mappingGet(services, name)
		mappingSet(services, name, patchServiceNode(existing, svc))
	}
}

func patchServiceNode(existing *yaml.Node, svc Service) *yaml.Node {
	raw, ok := nodeAsMap(existing)
	if !ok {
		// No prior node to patch (brand new service, or a malformed one) —
		// encode fresh; there is nothing to preserve.
		n := &yaml.Node{}
		if err := n.Encode(svc.marshal()); err == nil {
			return n
		}
		return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	}
	old := decodeService(raw)
	node := existing

	if old.Image != svc.Image {
		setScalarOrDelete(node, "image", svc.Image)
	}
	if old.Command != svc.Command {
		setScalarOrDelete(node, "command", svc.Command)
	}
	if old.Entrypoint != svc.Entrypoint {
		setScalarOrDelete(node, "entrypoint", svc.Entrypoint)
	}
	if old.Restart != svc.Restart {
		setScalarOrDelete(node, "restart", svc.Restart)
	}
	if !sameStringMap(old.Env, svc.Env) {
		setMapOrDelete(node, "environment", svc.Env)
	}
	if !sameStrings(old.Ports, svc.Ports) {
		setSliceOrDelete(node, "ports", svc.Ports)
	}
	if !sameStrings(old.Volumes, svc.Volumes) {
		setSliceOrDelete(node, "volumes", svc.Volumes)
	}
	if !sameStrings(sortedCopy(old.Networks), sortedCopy(svc.Networks)) {
		setSliceOrDelete(node, "networks", sortedCopy(svc.Networks))
	}
	if !sameStringMap(old.Labels, svc.Labels) {
		setMapOrDelete(node, "labels", svc.Labels)
	}
	if !sameStrings(sortedCopy(old.DependsOn), sortedCopy(svc.DependsOn)) {
		setSliceOrDelete(node, "depends_on", sortedCopy(svc.DependsOn))
	}

	return node
}

// --- networks ----------------------------------------------------------------

func patchNetworks(doc *yaml.Node, c Compose) {
	networks := mappingGet(doc, "networks")
	if len(c.Networks) == 0 {
		if networks != nil {
			mappingDelete(doc, "networks")
		}
		return
	}
	if networks == nil || networks.Kind != yaml.MappingNode {
		networks = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		mappingSet(doc, "networks", networks)
	}
	for i := 0; i+1 < len(networks.Content); {
		if _, ok := c.Networks[networks.Content[i].Value]; !ok {
			networks.Content = append(networks.Content[:i], networks.Content[i+2:]...)
			continue
		}
		i += 2
	}
	for _, name := range sortedKeys(c.Networks) {
		def := c.Networks[name]
		existing := mappingGet(networks, name)
		mappingSet(networks, name, patchNetworkNode(existing, def))
	}
}

func patchNetworkNode(existing *yaml.Node, def NetworkDef) *yaml.Node {
	raw, ok := nodeAsMap(existing)
	if !ok {
		n := &yaml.Node{}
		if err := n.Encode(def.marshal()); err == nil {
			return n
		}
		return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	}
	subnet, gateway := ipamConfig(valueOf(raw, "ipam"))
	old := NetworkDef{
		Driver:      str(valueOf(raw, "driver")),
		External:    truthy(valueOf(raw, "external")),
		Name:        str(valueOf(raw, "name")),
		Labels:      stringMap(valueOf(raw, "labels")),
		IPAMSubnet:  subnet,
		IPAMGateway: gateway,
		Internal:    truthy(valueOf(raw, "internal")),
		Attachable:  truthy(valueOf(raw, "attachable")),
		DriverOpts:  stringMap(valueOf(raw, "driver_opts")),
	}
	node := existing

	if old.Driver != def.Driver {
		setScalarOrDelete(node, "driver", def.Driver)
	}
	if old.Name != def.Name {
		setScalarOrDelete(node, "name", def.Name)
	}
	if old.External != def.External {
		setBoolOrDelete(node, "external", def.External)
	}
	if old.Internal != def.Internal {
		setBoolOrDelete(node, "internal", def.Internal)
	}
	if old.Attachable != def.Attachable {
		setBoolOrDelete(node, "attachable", def.Attachable)
	}
	if !sameStringMap(old.Labels, def.Labels) {
		setMapOrDelete(node, "labels", def.Labels)
	}
	if !sameStringMap(old.DriverOpts, def.DriverOpts) {
		setMapOrDelete(node, "driver_opts", def.DriverOpts)
	}
	if old.IPAMSubnet != def.IPAMSubnet || old.IPAMGateway != def.IPAMGateway {
		if def.IPAMSubnet == "" && def.IPAMGateway == "" {
			mappingDelete(node, "ipam")
		} else {
			entry := map[string]any{}
			if def.IPAMSubnet != "" {
				entry["subnet"] = def.IPAMSubnet
			}
			if def.IPAMGateway != "" {
				entry["gateway"] = def.IPAMGateway
			}
			ipamNode := &yaml.Node{}
			_ = ipamNode.Encode(map[string]any{"config": []any{entry}})
			mappingSet(node, "ipam", ipamNode)
		}
	}

	return node
}

// --- volumes -------------------------------------------------------------

func patchVolumes(doc *yaml.Node, c Compose) {
	volumes := mappingGet(doc, "volumes")
	if len(c.Volumes) == 0 {
		if volumes != nil {
			mappingDelete(doc, "volumes")
		}
		return
	}
	if volumes == nil || volumes.Kind != yaml.MappingNode {
		volumes = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		mappingSet(doc, "volumes", volumes)
	}
	for i := 0; i+1 < len(volumes.Content); {
		if _, ok := c.Volumes[volumes.Content[i].Value]; !ok {
			volumes.Content = append(volumes.Content[:i], volumes.Content[i+2:]...)
			continue
		}
		i += 2
	}
	for _, name := range sortedKeys(c.Volumes) {
		def := c.Volumes[name]
		existing := mappingGet(volumes, name)
		mappingSet(volumes, name, patchVolumeNode(existing, def))
	}
}

func patchVolumeNode(existing *yaml.Node, def VolumeDef) *yaml.Node {
	raw, ok := nodeAsMap(existing)
	if !ok {
		n := &yaml.Node{}
		if err := n.Encode(def.marshal()); err == nil {
			return n
		}
		return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	}
	old := VolumeDef{
		Driver:     str(valueOf(raw, "driver")),
		External:   truthy(valueOf(raw, "external")),
		Name:       str(valueOf(raw, "name")),
		Labels:     stringMap(valueOf(raw, "labels")),
		DriverOpts: stringMap(valueOf(raw, "driver_opts")),
	}
	node := existing

	if old.Driver != def.Driver {
		setScalarOrDelete(node, "driver", def.Driver)
	}
	if old.Name != def.Name {
		setScalarOrDelete(node, "name", def.Name)
	}
	if old.External != def.External {
		setBoolOrDelete(node, "external", def.External)
	}
	if !sameStringMap(old.Labels, def.Labels) {
		setMapOrDelete(node, "labels", def.Labels)
	}
	if !sameStringMap(old.DriverOpts, def.DriverOpts) {
		setMapOrDelete(node, "driver_opts", def.DriverOpts)
	}

	return node
}

// --- comparison helpers ----------------------------------------------------

func sameStrings(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return reflect.DeepEqual(a, b)
}

func sameStringMap(a, b map[string]string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return reflect.DeepEqual(a, b)
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
