.PHONY: all frontend sqlc build test

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS  = -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT)"

all: frontend sqlc build

frontend:
	cd ui && npm install && npm run build

sqlc:
	sqlc generate

build:
	go build $(LDFLAGS) -o nexorious ./cmd/nexorious

test:
	go test ./...
