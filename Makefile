.PHONY: all frontend build docker test test-backend test-frontend coverage print-version

COMMIT  := $(shell git rev-parse --short=7 HEAD 2>/dev/null || echo "unknown")
TAG     := $(shell git describe --exact-match HEAD 2>/dev/null)

ifneq ($(TAG),)
  VERSION ?= $(TAG:v%=%)
else
  _RAW_BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null)
  ifeq ($(_RAW_BRANCH),HEAD)
    _RAW_BRANCH := $(or $(GITHUB_REF_NAME),unknown)
  endif
  _BRANCH     := $(or $(shell echo "$(_RAW_BRANCH)" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9]/-/g;s/-\{2,\}/-/g;s/^-//;s/-$$//'),unknown)
  _DATE       := $(shell date +%Y%m%d)
  VERSION     ?= $(_BRANCH)-$(_DATE)-$(COMMIT)
endif

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

print-version:
	@echo $(VERSION)
