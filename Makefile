# Build into bin/ (gitignored) so the binary never collides with the github/
# source package at the repo root.
BINARY  := bin/github
PKG     := ./cmd/github
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X github.com/tamnd/github-cli/cli.Version=$(VERSION) \
	-X github.com/tamnd/github-cli/cli.Commit=$(COMMIT) \
	-X github.com/tamnd/github-cli/cli.Date=$(DATE)

.PHONY: build install test live vet fmt lint clean run

build:
	@mkdir -p $(dir $(BINARY))
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) $(PKG)

install:
	CGO_ENABLED=0 go install -trimpath -ldflags "$(LDFLAGS)" $(PKG)

# The default run is offline and deterministic.
test:
	go test ./...

# live talks to github.com. It answers the one question no offline test can:
# does the site still look the way the readers think it does.
live:
	GITHUB_LIVE=1 go test ./gh/ -run Live -count=1 -v

vet:
	go vet ./...

fmt:
	gofmt -w -s .

lint:
	golangci-lint run

clean:
	rm -rf bin dist

run: build
	./$(BINARY) $(ARGS)
