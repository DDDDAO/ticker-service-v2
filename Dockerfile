# syntax=docker/dockerfile:1.2

# Build stage
FROM golang:1.21-alpine AS builder
RUN apk add --no-cache git
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies with cache mount
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy source code
COPY . .

# Build the binary with cache mount
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o ticker-service cmd/ticker/main.go

# Final stage
FROM alpine:latest
RUN apk --no-cache add ca-certificates curl
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/ticker-service .

# Copy configuration files
COPY config.yaml .
COPY config.production.yaml .

# Set production config by default (can be overridden)
ENV CONFIG_FILE=config.production.yaml

EXPOSE 8080

CMD ["sh", "-c", "./ticker-service -config ${CONFIG_FILE:-config.yaml}"]