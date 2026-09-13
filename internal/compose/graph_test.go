package compose

import (
	"reflect"
	"testing"
)

const graphFixture = `
services:
  api:
    image: alpine:3
    environment:
      PORT: "8080"
    volumes:
      - data:/var/lib/data
      - ./cfg:/etc/cfg
    networks:
      - front
  worker:
    image: alpine:3
    depends_on:
      - api
    networks:
      - front
      - back
networks:
  front:
    driver: bridge
  back:
    driver: bridge
volumes:
  data:
    driver: local
`

func TestParseComposeStructureLeavesVariablesIntact(t *testing.T) {
	yamlBytes := []byte(`
services:
  api:
    image: alpine:${TAG:-3.20}
    environment:
      SET: ${GIVEN:?required}
`)
	structural, _, err := ParseComposeStructure(yamlBytes)
	if err != nil {
		t.Fatal(err)
	}
	if got := structural.Services["api"].Image; got != "alpine:${TAG:-3.20}" {
		t.Fatalf("image = %q, want variable left intact", got)
	}
	if got := structural.Services["api"].Env["SET"]; got != "${GIVEN:?required}" {
		t.Fatalf("env SET = %q, want variable left intact", got)
	}

	resolved, warnings, err := ParseCompose(yamlBytes, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := resolved.Services["api"].Image; got != "alpine:3.20" {
		t.Fatalf("ParseCompose image = %q, want interpolated", got)
	}
	if len(warningKinds(warnings)) == 0 {
		t.Fatal("expected a warning for the unresolved required variable")
	}
}

func TestMarshalRoundTripsUnmodelledServiceKeys(t *testing.T) {
	file, _, err := ParseComposeStructure([]byte(`
services:
  api:
    image: alpine:3
    healthcheck:
      test: ["CMD", "true"]
`))
	if err != nil {
		t.Fatal(err)
	}

	out, err := file.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	reparsed, _, err := ParseComposeStructure(out)
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}
	if _, ok := reparsed.Services["api"].Raw["healthcheck"]; !ok {
		t.Fatalf("healthcheck key lost after Marshal round trip: %#v", reparsed.Services["api"].Raw)
	}
	if reparsed.Services["api"].Image != "alpine:3" {
		t.Fatalf("image = %q after round trip", reparsed.Services["api"].Image)
	}
}

func TestNetworkVolumeExtendedFieldsRoundTrip(t *testing.T) {
	file, _, err := ParseComposeStructure([]byte(`
networks:
  front:
    driver: bridge
    internal: true
    attachable: true
    driver_opts:
      com.docker.network.bridge.name: br0
    ipam:
      config:
        - subnet: 10.0.0.0/24
          gateway: 10.0.0.1
volumes:
  data:
    driver: local
    driver_opts:
      type: nfs
services:
  api:
    image: alpine:3
`))
	if err != nil {
		t.Fatal(err)
	}
	net := file.Networks["front"]
	if net.IPAMSubnet != "10.0.0.0/24" || net.IPAMGateway != "10.0.0.1" || !net.Internal || !net.Attachable {
		t.Fatalf("network fields = %#v", net)
	}
	if net.DriverOpts["com.docker.network.bridge.name"] != "br0" {
		t.Fatalf("network driver_opts = %#v", net.DriverOpts)
	}
	if file.Volumes["data"].DriverOpts["type"] != "nfs" {
		t.Fatalf("volume driver_opts = %#v", file.Volumes["data"].DriverOpts)
	}

	out, err := file.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	reparsed, _, err := ParseComposeStructure(out)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reparsed.Networks["front"], net) {
		t.Fatalf("network round trip = %#v, want %#v", reparsed.Networks["front"], net)
	}
}

func TestGraphRoundTrip(t *testing.T) {
	file, _, err := ParseComposeStructure([]byte(graphFixture))
	if err != nil {
		t.Fatal(err)
	}

	nodes, edges := ToGraph(file)

	rebuilt, err := FromGraph(nodes, edges)
	if err != nil {
		t.Fatal(err)
	}
	out, err := rebuilt.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	reparsed, _, err := ParseComposeStructure(out)
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}

	for name := range file.Services {
		want, got := file.Services[name], reparsed.Services[name]
		if want.Image != got.Image {
			t.Fatalf("service %q image = %q, want %q", name, got.Image, want.Image)
		}
		if !sameSet(want.Networks, got.Networks) {
			t.Fatalf("service %q networks = %v, want %v", name, got.Networks, want.Networks)
		}
		if !sameSet(want.DependsOn, got.DependsOn) {
			t.Fatalf("service %q depends_on = %v, want %v", name, got.DependsOn, want.DependsOn)
		}
	}
	if !sameSet(mapKeys(file.Networks), mapKeys(reparsed.Networks)) {
		t.Fatalf("networks = %v, want %v", mapKeys(reparsed.Networks), mapKeys(file.Networks))
	}
	if !sameSet(mapKeys(file.Volumes), mapKeys(reparsed.Volumes)) {
		t.Fatalf("volumes = %v, want %v", mapKeys(reparsed.Volumes), mapKeys(file.Volumes))
	}
	// api's bind mount must survive even though it has no graph edge.
	if !contains(reparsed.Services["api"].Volumes, "./cfg:/etc/cfg") {
		t.Fatalf("bind mount lost: %v", reparsed.Services["api"].Volumes)
	}
	if !contains(reparsed.Services["api"].Volumes, "data:/var/lib/data") {
		t.Fatalf("named volume mount lost: %v", reparsed.Services["api"].Volumes)
	}
}

func TestFromGraphEdgeEditsChangeNetworkMembership(t *testing.T) {
	file, _, err := ParseComposeStructure([]byte(graphFixture))
	if err != nil {
		t.Fatal(err)
	}
	nodes, edges := ToGraph(file)

	// Remove the api<->front network edge.
	filtered := edges[:0]
	for _, e := range edges {
		if e.Kind == EdgeNetwork && e.From == serviceNodeID("api") && e.To == networkNodeID("front") {
			continue
		}
		filtered = append(filtered, e)
	}

	rebuilt, err := FromGraph(nodes, filtered)
	if err != nil {
		t.Fatal(err)
	}
	if contains(rebuilt.Services["api"].Networks, "front") {
		t.Fatalf("expected front removed from api's networks, got %v", rebuilt.Services["api"].Networks)
	}

	// Add it back plus a new membership.
	filtered = append(filtered, Edge{ID: "network:api:front", Kind: EdgeNetwork, From: serviceNodeID("api"), To: networkNodeID("front")})
	filtered = append(filtered, Edge{ID: "network:api:back", Kind: EdgeNetwork, From: serviceNodeID("api"), To: networkNodeID("back")})
	rebuilt, err = FromGraph(nodes, filtered)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(rebuilt.Services["api"].Networks, "front") || !contains(rebuilt.Services["api"].Networks, "back") {
		t.Fatalf("expected api on front and back, got %v", rebuilt.Services["api"].Networks)
	}
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	am := map[string]bool{}
	for _, v := range a {
		am[v] = true
	}
	for _, v := range b {
		if !am[v] {
			return false
		}
	}
	return true
}

func mapKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func contains(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}
