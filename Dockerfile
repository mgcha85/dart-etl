# Build Stage (Go)
FROM golang:1.22-bullseye AS builder

WORKDIR /app

# Copy dependency files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN go build -o dart-etl ./cmd/server/main.go

# Runtime Stage (Python + Go Binary)
FROM python:3.10-slim-bullseye

WORKDIR /app

# Install system dependencies
RUN apt-get update && apt-get install -y \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Install Python dependencies
COPY python/requirements.txt ./python/
RUN pip install --no-cache-dir -r python/requirements.txt

# Copy Go binary and source files
COPY --from=builder /app/dart-etl .
COPY --from=builder /app/python ./python
COPY --from=builder /app/.env .

# Create storage directory
RUN mkdir -p storage

# Use the Go binary as entrypoint
CMD ["./dart-etl"]
