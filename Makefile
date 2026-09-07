.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -o bin/ubereats-pp-cli$(BIN_EXT) ./cmd/ubereats-pp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/ubereats-pp-cli

clean:
	rm -rf bin/

build-mcp:
	go build -o bin/ubereats-pp-mcp$(BIN_EXT) ./cmd/ubereats-pp-mcp

install-mcp:
	go install ./cmd/ubereats-pp-mcp

build-all: build build-mcp
