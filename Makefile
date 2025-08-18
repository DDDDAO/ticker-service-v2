# Ticker Service Makefile

# Variables
BINARY_NAME=ticker-service
DOCKER_IMAGE=ticker-service-v2
DOCKER_REGISTRY=ghcr.io
DOCKER_ORG=your-org
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GO=go
GOFLAGS=-v
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(shell date -u +%Y%m%d-%H%M%S)"

# Colors for output
RED=\033[0;31m
GREEN=\033[0;32m
YELLOW=\033[1;33m
NC=\033[0m # No Color

.PHONY: help
help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*##"; printf "\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  ${GREEN}%-20s${NC} %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

.PHONY: deps
deps: ## Download dependencies
	@echo "${YELLOW}Downloading dependencies...${NC}"
	$(GO) mod download
	$(GO) mod verify

.PHONY: tidy
tidy: ## Tidy go modules
	@echo "${YELLOW}Tidying go modules...${NC}"
	$(GO) mod tidy

.PHONY: build
build: ## Build the binary
	@echo "${GREEN}Building $(BINARY_NAME)...${NC}"
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BINARY_NAME) cmd/ticker/main.go

.PHONY: run
run: ## Run the application locally
	@echo "${GREEN}Running $(BINARY_NAME)...${NC}"
	$(GO) run cmd/ticker/main.go -config config.yaml

.PHONY: dev
dev: ## Run with hot reload (requires air)
	@echo "${GREEN}Running in development mode with hot reload...${NC}"
	@if ! which air > /dev/null; then \
		echo "${RED}air not installed. Please run: make install-tools${NC}"; \
		exit 1; \
	fi
	air -c .air.toml

.PHONY: test
test: ## Run tests
	@echo "${YELLOW}Running tests...${NC}"
	$(GO) test -v -race -cover ./...

.PHONY: test-coverage
test-coverage: ## Run tests with coverage report
	@echo "${YELLOW}Running tests with coverage...${NC}"
	$(GO) test -v -race -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "${GREEN}Coverage report generated: coverage.html${NC}"

.PHONY: lint
lint: ## Run linter
	@echo "${YELLOW}Running linter...${NC}"
	@if ! which golangci-lint > /dev/null; then \
		echo "${RED}golangci-lint not installed. Installing...${NC}"; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
	fi
	golangci-lint run

.PHONY: fmt
fmt: ## Format code
	@echo "${YELLOW}Formatting code...${NC}"
	$(GO) fmt ./...
	@if which goimports > /dev/null; then \
		goimports -w .; \
	else \
		echo "${YELLOW}goimports not installed. Skipping...${NC}"; \
	fi

.PHONY: clean
clean: ## Clean build artifacts
	@echo "${YELLOW}Cleaning...${NC}"
	rm -f $(BINARY_NAME) $(BINARY_NAME)-linux-amd64
	rm -f coverage.out coverage.html

.PHONY: docker-build
docker-build: ## Build Docker image
	@echo "${GREEN}Building Docker image $(DOCKER_IMAGE):$(VERSION)...${NC}"
	docker build -t $(DOCKER_IMAGE):$(VERSION) -t $(DOCKER_IMAGE):latest .

.PHONY: docker-run
docker-run: ## Run Docker container locally
	@echo "${GREEN}Running Docker container...${NC}"
	docker run -d \
		--name $(BINARY_NAME) \
		-p 8080:8080 \
		-v $(PWD)/config.yaml:/app/config.yaml:ro \
		-e TICKER_REDIS_ADDR=host.docker.internal:6379 \
		$(DOCKER_IMAGE):latest

.PHONY: docker-stop
docker-stop: ## Stop and remove Docker container
	@echo "${YELLOW}Stopping Docker container...${NC}"
	docker stop $(BINARY_NAME) || true
	docker rm $(BINARY_NAME) || true

.PHONY: compose-up
compose-up: ## Start services with docker-compose
	@echo "${GREEN}Starting services with docker-compose...${NC}"
	docker-compose up -d

.PHONY: compose-down
compose-down: ## Stop services with docker-compose
	@echo "${YELLOW}Stopping services with docker-compose...${NC}"
	docker-compose down

.PHONY: compose-logs
compose-logs: ## Show docker-compose logs
	docker-compose logs -f

.PHONY: k8s-deploy
k8s-deploy: ## Deploy to Kubernetes
	@echo "${GREEN}Deploying to Kubernetes...${NC}"
	kubectl apply -f k8s/

.PHONY: k8s-delete
k8s-delete: ## Delete from Kubernetes
	@echo "${YELLOW}Deleting from Kubernetes...${NC}"
	kubectl delete -f k8s/

.PHONY: install-tools
install-tools: ## Install development tools
	@echo "${YELLOW}Installing development tools...${NC}"
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install golang.org/x/tools/cmd/goimports@latest
	go install github.com/cosmtrek/air@latest
	@echo "${GREEN}Tools installed successfully${NC}"

.PHONY: health
health: ## Check service health
	@echo "${GREEN}Checking service health...${NC}"
	@curl -s http://localhost:8080/health | jq . || echo "${RED}Service not responding${NC}"

.PHONY: all
all: deps fmt lint test build ## Run all checks and build

.DEFAULT_GOAL := help