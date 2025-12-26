# Build stage
FROM golang:1.24 AS builder

WORKDIR /app

# Install build dependencies
RUN apt-get update && apt-get install -y git gcc libc6-dev libsqlite3-dev

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o docker-faas-gateway ./cmd/gateway

# Final stage
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y ca-certificates sqlite3 libsqlite3-0 && rm -rf /var/lib/apt/lists/*

WORKDIR /root/

# Copy the binary from builder
COPY --from=builder /app/docker-faas-gateway .

# Expose ports
EXPOSE 8080 9090

# Run the application
CMD ["./docker-faas-gateway"]
