.PHONY: all frontend build test

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS  = -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT)"

all: frontend build

frontend:
	cd ui/frontend && npm install && npm run build

build:
	go build $(LDFLAGS) -o nexorious ./cmd/nexorious

test:
	go test ./...
