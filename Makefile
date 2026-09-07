.PHONY: dev build test lint fmt clean

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -X github.com/0funct0ry/vessel/internal/version.Version=$(VERSION) \
           -X github.com/0funct0ry/vessel/internal/version.Commit=$(COMMIT) \
           -X github.com/0funct0ry/vessel/internal/version.Date=$(DATE)

dev:
	go run . serve

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/vessel .

test:
	go test ./...

lint:
	golangci-lint run ./...

fmt:
	gofmt -l -w .

clean:
	rm -rf bin
