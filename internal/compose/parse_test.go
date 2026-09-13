package compose

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func warningKinds(warnings []Warning) []string {
	out := make([]string, 0, len(warnings))
	for _, w := range warnings {
		out = append(out, w.Kind)
	}
	return out
}

func TestParseComposeEnvironmentListAndMapForms(t *testing.T) {
	file, _, err := ParseCompose([]byte(`
services:
  api:
    image: alpine:3
    environment:
      - PORT=8080
      - DEBUG
  web:
    image: nginx:1
    environment:
      PORT: 80
      NAME: web
`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := file.Services["api"].Env; !reflect.DeepEqual(got, map[string]string{"PORT": "8080", "DEBUG": ""}) {
		t.Fatalf("list-form env = %#v", got)
	}
	if got := file.Services["web"].Env; !reflect.DeepEqual(got, map[string]string{"PORT": "80", "NAME": "web"}) {
		t.Fatalf("map-form env = %#v", got)
	}
	if !reflect.DeepEqual(file.Order, []string{"api", "web"}) {
		t.Fatalf("order = %v", file.Order)
	}
}

func TestParseComposeInterpolatesDefaults(t *testing.T) {
	file, warnings, err := ParseCompose([]byte(`
services:
  api:
    image: alpine:${TAG:-3.20}
    environment:
      SET: ${GIVEN}
      LITERAL: $$NOT_A_VAR
`), map[string]string{"GIVEN": "yes"})
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if got := file.Services["api"].Image; got != "alpine:3.20" {
		t.Fatalf("image = %q", got)
	}
	if got := file.Services["api"].Env; got["SET"] != "yes" || got["LITERAL"] != "$NOT_A_VAR" {
		t.Fatalf("env = %#v", got)
	}
}

func TestParseComposeWarnsOnRequiredVariable(t *testing.T) {
	file, warnings, err := ParseCompose([]byte(`
services:
  api:
    image: alpine:3
    environment:
      TOKEN: ${API_TOKEN:?set API_TOKEN in the env panel}
`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(warningKinds(warnings), []string{WarnUnresolvedVar}) {
		t.Fatalf("warnings = %#v", warnings)
	}
	if !strings.Contains(warnings[0].Message, "API_TOKEN") || !strings.Contains(warnings[0].Message, "env panel") {
		t.Fatalf("message = %q", warnings[0].Message)
	}
	if got := file.Services["api"].Env["TOKEN"]; got != "" {
		t.Fatalf("unresolved value = %q", got)
	}
}

func TestParseComposeWarnsOnObsoleteVersionAndDanglingDependsOn(t *testing.T) {
	_, warnings, err := ParseCompose([]byte(`
version: "3.9"
services:
  api:
    image: alpine:3
    depends_on:
      - db
`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(warningKinds(warnings), []string{WarnObsoleteVersion, WarnDependsOn}) {
		t.Fatalf("warnings = %#v", warnings)
	}
}

func TestParseComposeKeepsUnknownKeysInRaw(t *testing.T) {
	file, _, err := ParseCompose([]byte(`
services:
  api:
    image: alpine:3
    healthcheck:
      test: ["CMD", "true"]
      interval: 10s
    deploy:
      replicas: 2
`), nil)
	if err != nil {
		t.Fatal(err)
	}
	raw := file.Services["api"].Raw
	health, ok := raw["healthcheck"].(map[string]any)
	if !ok || health["interval"] != "10s" {
		t.Fatalf("healthcheck not round tripped: %#v", raw["healthcheck"])
	}
	if _, ok := raw["deploy"]; !ok {
		t.Fatalf("deploy not round tripped: %#v", raw)
	}
}

func TestParseComposeDecodesTopLevelNetworksAndVolumes(t *testing.T) {
	file, _, err := ParseCompose([]byte(`
services:
  api:
    image: alpine:3
    networks: [back]
    volumes:
      - data:/var/lib/data
      - /etc/hosts:/etc/hosts:ro
    ports:
      - "8080:80"
      - "53:53/udp"
networks:
  back:
    driver: bridge
volumes:
  data:
  shared:
    external: true
`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if file.Networks["back"].Driver != "bridge" {
		t.Fatalf("networks = %#v", file.Networks)
	}
	if !file.Volumes["shared"].External || file.Volumes["data"].External {
		t.Fatalf("volumes = %#v", file.Volumes)
	}
	service := file.Services["api"]
	if !reflect.DeepEqual(service.Volumes, []string{"data:/var/lib/data", "/etc/hosts:/etc/hosts:ro"}) {
		t.Fatalf("volumes = %#v", service.Volumes)
	}
	if !reflect.DeepEqual(service.Ports, []string{"8080:80", "53:53/udp"}) {
		t.Fatalf("ports = %#v", service.Ports)
	}
}

func TestParseComposeRejectsEmptyAndInvalidFiles(t *testing.T) {
	if _, _, err := ParseCompose([]byte("services: {}\n"), nil); !errors.Is(err, ErrNoServices) {
		t.Fatalf("empty services = %v", err)
	}
	if _, _, err := ParseCompose([]byte("\t not: [yaml\n"), nil); err == nil {
		t.Fatal("expected a YAML error")
	}
}

func TestParseEnvFile(t *testing.T) {
	got := ParseEnvFile("# comment\nexport TAG=3.20\nQUOTED=\"a b\"\nBLANK=\n\nnope\n")
	want := map[string]string{"TAG": "3.20", "QUOTED": "a b", "BLANK": ""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("env = %#v", got)
	}
}
