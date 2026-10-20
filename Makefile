SHELL := /bin/sh

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BIN     := bin/nurtur

.PHONY: dev build test race vet staticcheck fmt clean

# Local development uses the same Turso engine, without cloud sync.
dev:
	./scripts/dev.sh

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o $(BIN) ./cmd/nurtur

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

staticcheck:
	go run honnef.co/go/tools/cmd/staticcheck@latest ./...

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './.git/*')

clean:
	rm -rf bin data
