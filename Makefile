.PHONY: build test clean

# Build variables
BINARY_NAME=pgboundary
GO=go

# Build the application
build:
	$(GO) build -o $(BINARY_NAME)

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
	$(GO) clean
	$(GO) fmt ./... && $(GO) vet ./... && $(GO) mod tidy

# Build and run tests
all: clean build test