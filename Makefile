.PHONY: dev build test lint fmt clean web-build check-bundle-size check-standalone site

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -X github.com/0funct0ry/vessel/internal/version.Version=$(VERSION) \
           -X github.com/0funct0ry/vessel/internal/version.Commit=$(COMMIT) \
           -X github.com/0funct0ry/vessel/internal/version.Date=$(DATE)

dev:
	go run . serve

web-build:
	cd web && npm ci && npm run build

build: web-build
	./scripts/check-bundle-size.sh
	CGO_ENABLED=0 go build -tags embed -ldflags "$(LDFLAGS)" -o bin/vessel .
	./scripts/check-standalone.sh

check-bundle-size:
	./scripts/check-bundle-size.sh

check-standalone:
	./scripts/check-standalone.sh

test:
	go test ./...

lint:
	golangci-lint run ./...

fmt:
	gofmt -l -w .

site:
	cd web-site && npm ci && npm run build

clean:
	rm -rf bin web/dist web-site/dist
