.PHONY: build test clean run docker-build docker-push install-deps lint fmt vet coverage integration-test e2e-test test-all help

# Variables
BINARY_NAME=docker-faas-gateway
DOCKER_IMAGE=docker-faas/gateway
VERSION?=latest
GOBASE=$(shell pwd)
GOBIN=$(GOBASE)/bin

help: ## Display this help screen
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

install-deps: ## Install dependencies
	go mod download
	go mod verify

build: ## Build the binary
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(GOBIN)
	go build -o $(GOBIN)/$(BINARY_NAME) ./cmd/gateway

run: ## Run the application
	go run ./cmd/gateway

test: ## Run unit tests
	go test -v -race -coverprofile=coverage.out ./...

coverage: test ## Generate coverage report
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

integration-test: ## Run integration tests
	go test -v -tags=integration ./tests/integration/...

e2e-test: ## Run end-to-end compatibility tests
	@echo "Running OpenFaaS compatibility tests..."
	@chmod +x tests/e2e/openfaas-compatibility-test.sh
	@./tests/e2e/openfaas-compatibility-test.sh

test-all: test integration-test ## Run all tests (unit + integration)

lint: ## Run linter
	golangci-lint run ./...

fmt: ## Format code
	go fmt ./...

vet: ## Run go vet
	go vet ./...

clean: ## Clean build artifacts
	@echo "Cleaning..."
	@rm -rf $(GOBIN)
	@rm -f coverage.out coverage.html
	@rm -f *.db *.sqlite *.sqlite3

docker-build: ## Build Docker image
	docker build -t $(DOCKER_IMAGE):$(VERSION) .

docker-push: ## Push Docker image
	docker push $(DOCKER_IMAGE):$(VERSION)

docker-compose-up: ## Start with docker-compose
	docker-compose up -d

docker-compose-down: ## Stop docker-compose
	docker-compose down

all: clean install-deps fmt vet test build ## Run all checks and build
