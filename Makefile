.PHONY: build test clean

# Build variables
BINARY_NAME=pgboundary
GO=go
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_DATE=$(shell date -u +'%Y-%m-%d_%H:%M:%S')

# Build the application
build:
	$(GO) build -ldflags "-X pgboundary/cmd.version=$(VERSION) -X pgboundary/cmd.commit=$(COMMIT) -X pgboundary/cmd.buildDate=$(BUILD_DATE)" -o $(BINARY_NAME)

# Run tests
test:
	$(GO) test -v ./...

# Run tests with race detection
test-race:
	$(GO) test -v -race ./...

# run goreleaser
release:
	goreleaser release --snapshot --clean

# Clean build artifacts
clean:
	rm -rf dist
	rm -f $(BINARY_NAME)
	$(GO) mod tidy
	$(GO) clean
	$(GO) fmt ./... && $(GO) vet ./... && $(GO) mod tidy
	if command -v golangci-lint >/dev/null 2>&1; then golangci-lint run; fi

# Build and run tests
all: clean build test