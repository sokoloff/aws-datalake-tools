VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X github.com/sokoloff/aws-datalake-tools/internal/cli.Version=$(VERSION)"

.PHONY: all build build-lambda test lint clean tidy

all: lint test build

build:
	go build $(LDFLAGS) -o bin/datalake ./cmd/datalake

build-lambda:
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o bootstrap ./cmd/stream-processor

test:
	go test ./... -v -race

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/ bootstrap

tidy:
	go mod tidy
