.PHONY: all build test

all: build

build:
	go build ./cmd/nexorious

test:
	go test ./...
