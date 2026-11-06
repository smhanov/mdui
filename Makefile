.PHONY: run test lint build

VERSION = $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT = $(shell git rev-parse --short HEAD 2>/dev/null)
DATE = $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

run:
	go run . $(ARGS)

test:
	go test ./...

lint:
	go vet ./...

build:
	go build -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)" -o mdui .
