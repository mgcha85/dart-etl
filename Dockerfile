# Build Stage (Go)
FROM golang:1.23-bullseye AS builder

WORKDIR /app

# Copy dependency files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=1 go build -o dart-etl ./cmd/server/main.go

# Runtime Stage (Alpine with CGO support for SQLite)
FROM debian:bullseye-slim

WORKDIR /app

# Install system dependencies
RUN apt-get update && apt-get install -y \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Copy Go binary
COPY --from=builder /app/dart-etl .

# Create storage directory
RUN mkdir -p storage

# Expose API port
EXPOSE 8080

# Use the Go binary as entrypoint
CMD ["./dart-etl"]
