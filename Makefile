.PHONY: build run clean test deps docker-build docker-run

# Binary name
BINARY=ticker-service

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Build the binary
build:
	$(GOBUILD) -o $(BINARY) -v cmd/ticker/main.go

# Run the application
run:
	$(GOBUILD) -o $(BINARY) -v cmd/ticker/main.go
	./$(BINARY) -config config.yaml

# Run with live reload (requires air)
dev:
	air -c .air.toml

# Clean build artifacts
clean:
	$(GOCLEAN)
	rm -f $(BINARY)

# Run tests
test:
	$(GOTEST) -v ./...

# Run tests with coverage
test-coverage:
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out

# Download dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

# Build Docker image
docker-build:
	docker build -t ticker-service-v2:latest .

# Run Docker container
docker-run:
	docker run -d \
		--name ticker-service \
		-p 8080:8080 \
		-e TICKER_REDIS_ADDR=host.docker.internal:6379 \
		ticker-service-v2:latest

# Format code
fmt:
	$(GOCMD) fmt ./...
	goimports -w .

# Lint code (requires golangci-lint)
lint:
	golangci-lint run

# Install development tools
install-tools:
	$(GOGET) -u github.com/cosmtrek/air
	$(GOGET) -u golang.org/x/tools/cmd/goimports
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin