SHELL := /bin/sh

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BIN     := bin/nurtur

.PHONY: dev build test race vet staticcheck fmt clean e2e docker-up

# Local development uses the same Turso engine, without cloud sync.
dev:
	./scripts/dev.sh

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o $(BIN) ./cmd/nurtur

# Keep test packages serialized: each binary's first tursogo call extracts the
# embedded native library into a shared cache, and concurrent extraction races.
test:
	go test -p 1 ./...

race:
	go test -race ./...

vet:
	go vet ./...

staticcheck:
	go run honnef.co/go/tools/cmd/staticcheck@latest ./...

e2e:
	cd e2e && npm test

# Rebuild the image and start the local container in the background.
docker-up:
	docker compose up -d --build

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './.git/*')

clean:
	rm -rf bin data
