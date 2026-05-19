.PHONY: all frontend build docker test test-backend test-frontend coverage

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS  = -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT)"

all: frontend build

frontend:
	cd ui/frontend && npm install && npm run build
	touch ui/frontend/dist/.gitkeep

build:
	go build $(LDFLAGS) -o nexorious ./cmd/nexorious

docker:
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) -t nexorious:local .

test: test-backend test-frontend

test-backend:
	go test ./...

test-frontend:
	cd ui/frontend && npm run test

coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
