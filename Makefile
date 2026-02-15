.PHONY: build test install

BINARY ?= gh-auto-switch
PREFIX ?= $(HOME)/bin

build:
	go build -o $(BINARY) ./cmd/gh-auto-switch

test:
	go test ./...

install:
	mkdir -p "$(PREFIX)"
	go build -o "$(PREFIX)/$(BINARY)" ./cmd/gh-auto-switch

