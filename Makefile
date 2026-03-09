BINARY := bin/memsh
LOCAL_BIN ?= $(HOME)/.local/bin
LOCAL_SHARE ?= $(HOME)/.config/memsh

.PHONY: build test install-local fmt

build:
	go build -o $(BINARY) ./cmd/memsh

test:
	go test ./...

fmt:
	gofmt -w ./cmd ./internal

install-local: build
	bash ./install.sh