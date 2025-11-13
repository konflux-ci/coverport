.PHONY: build install clean test lint docker-build

# Binary name
BINARY_NAME=coverport

# Version info
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE = $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Build flags
LDFLAGS = -ldflags "-X github.com/konflux-ci/coverport/cli/cmd.version=$(VERSION) \
                     -X github.com/konflux-ci/coverport/cli/cmd.commit=$(COMMIT)"

# Docker
DOCKER_REGISTRY ?= quay.io
DOCKER_ORG ?= psturc
DOCKER_IMAGE = $(DOCKER_REGISTRY)/$(DOCKER_ORG)/coverport
DOCKER_TAG ?= $(VERSION)

all: build

build:
	@echo "Building $(BINARY_NAME)..."
	go build $(LDFLAGS) -o $(BINARY_NAME) main.go
	@echo "✅ Build complete: ./$(BINARY_NAME)"

install: build
	@echo "Installing $(BINARY_NAME)..."
	go install $(LDFLAGS)
	@echo "✅ Installed to $(shell go env GOPATH)/bin/$(BINARY_NAME)"

clean:
	@echo "Cleaning..."
	rm -f $(BINARY_NAME)
	rm -f coverage.out
	@echo "✅ Clean complete"

test:
	@echo "Running tests..."
	go test -v ./...

lint:
	@echo "Running linters..."
	golangci-lint run ./...

# Docker targets
docker-build:
	@echo "Building Docker image: $(DOCKER_IMAGE):$(DOCKER_TAG)"
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .
	@echo "✅ Docker image built"

docker-push: docker-build
	@echo "Pushing Docker image: $(DOCKER_IMAGE):$(DOCKER_TAG)"
	docker push $(DOCKER_IMAGE):$(DOCKER_TAG)
	@echo "✅ Docker image pushed"

# Development targets
dev-build:
	@echo "Building for development..."
	go build -race -o $(BINARY_NAME) main.go

run: build
	./$(BINARY_NAME)

# Help
help:
	@echo "Available targets:"
	@echo "  build         - Build the binary"
	@echo "  install       - Install the binary to GOPATH/bin"
	@echo "  clean         - Remove built artifacts"
	@echo "  test          - Run tests"
	@echo "  lint          - Run linters"
	@echo "  docker-build  - Build Docker image"
	@echo "  docker-push   - Build and push Docker image"
	@echo "  dev-build     - Build with race detector"
	@echo "  run           - Build and run"

