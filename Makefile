VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X github.com/sokoloff/aws-datalake-tools/internal/cli.Version=$(VERSION) -s -w"

.PHONY: all build build-lambda test test-integration test-all lint clean tidy coverage

all: lint test build

build:
	go build $(LDFLAGS) -o bin/datalake ./cmd/datalake

build-lambda:
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o bootstrap ./cmd/stream-processor

test:
	go test ./... -v -race

test-integration:  ## requires Docker
	go test -tags=integration ./test/integration/... -v -count=1 -timeout 10m

test-all: test test-integration

coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated at coverage.html"

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/ bootstrap dist/

tidy:
	go mod tidy
