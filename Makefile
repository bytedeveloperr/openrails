.PHONY: help build docker-build docker-build-no-cache run test clean vendor dev migrate-up migrate-down docker-up docker-down

# Variables
BINARY_NAME=billing
DOCKER_IMAGE=billing:latest
DOCKER_REGISTRY?=
BUILDKIT_PROGRESS?=plain

# Go variables
GOBASE=$(shell pwd)
GOBIN=$(GOBASE)/bin
GOFILES=$(wildcard *.go)

# Git variables
GIT_COMMIT=$(shell git rev-parse HEAD)
GIT_TAG=$(shell git describe --tags --always --dirty)

# Build variables
LDFLAGS=-ldflags "-X main.Version=$(GIT_TAG) -X main.Commit=$(GIT_COMMIT)"

# Default target
help: ## Show this help
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@awk 'BEGIN {FS = ":.*##"; } /^[a-zA-Z_-]+:.*?##/ { printf "  %-20s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

build: ## Build the binary locally
	@echo "Building $(BINARY_NAME)..."
	@go build $(LDFLAGS) -o $(BINARY_NAME) ./cmd/billing/

build-vendor: vendor ## Build using vendor directory (offline)
	@echo "Building $(BINARY_NAME) with vendor..."
	@go build -mod=vendor $(LDFLAGS) -o $(BINARY_NAME) ./cmd/billing/

vendor: ## Download dependencies to vendor directory
	@echo "Downloading dependencies to vendor..."
	@go mod vendor

docker-build: ## Build Docker image with BuildKit cache
	@echo "Building Docker image with cache..."
	@DOCKER_BUILDKIT=1 docker build \
		--progress=$(BUILDKIT_PROGRESS) \
		--tag $(DOCKER_IMAGE) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg GIT_TAG=$(GIT_TAG) \
		.

docker-build-no-cache: ## Build Docker image without cache
	@echo "Building Docker image without cache..."
	@DOCKER_BUILDKIT=1 docker build \
		--progress=$(BUILDKIT_PROGRESS) \
		--no-cache \
		--tag $(DOCKER_IMAGE) \
		.

docker-build-multi: ## Build multi-platform Docker image
	@echo "Building multi-platform Docker image..."
	@docker buildx build \
		--platform linux/amd64,linux/arm64 \
		--tag $(DOCKER_IMAGE) \
		.

run: build ## Build and run the application
	@echo "Running $(BINARY_NAME)..."
	@./$(BINARY_NAME) server

dev: ## Run the application in development mode with hot reload
	@echo "Running in development mode..."
	@go run ./cmd/billing/ server

test: ## Run tests
	@echo "Running tests..."
	@go test -v -race ./...

test-coverage: ## Run tests with coverage
	@echo "Running tests with coverage..."
	@go test -v -race -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html

lint: ## Run linter
	@echo "Running linter..."
	@golangci-lint run ./...

fmt: ## Format code
	@echo "Formatting code..."
	@go fmt ./...
	@goimports -w .

clean: ## Clean build artifacts
	@echo "Cleaning..."
	@rm -f $(BINARY_NAME)
	@rm -f coverage.out coverage.html
	@rm -rf vendor/
	@go clean -cache

mod-tidy: ## Tidy go.mod
	@echo "Tidying dependencies..."
	@go mod tidy

mod-update: ## Update dependencies
	@echo "Updating dependencies..."
	@go get -u ./...
	@go mod tidy

docker-up: ## Start services with docker-compose
	@echo "Starting services..."
	@docker-compose up -d

docker-down: ## Stop services with docker-compose
	@echo "Stopping services..."
	@docker-compose down

docker-logs: ## View docker-compose logs
	@docker-compose logs -f

migrate-up: ## Run database migrations up
	@echo "Running migrations up..."
	@go run ./cmd/billing/ migrate up

migrate-down: ## Run database migrations down
	@echo "Running migrations down..."
	@go run ./cmd/billing/ migrate down

migrate-status: ## Check migration status
	@echo "Checking migration status..."
	@go run ./cmd/billing/ migrate status

# Quick commands for development
.PHONY: quick-build
quick-build: ## Quick build without vendoring
	@go build -o $(BINARY_NAME) ./cmd/billing/

.PHONY: watch
watch: ## Watch for changes and rebuild
	@echo "Watching for changes..."
	@fswatch -o . | xargs -n1 -I{} make build