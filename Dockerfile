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

RUN apt-get update && apt-get install -y ca-certificates sqlite3 libsqlite3-0 git curl && rm -rf /var/lib/apt/lists/*

WORKDIR /root/

# Create data directories for database
RUN mkdir -p /data && chmod 777 /data && mkdir -p /root/data && chmod 777 /root/data

# Copy the binary from builder
COPY --from=builder /app/docker-faas-gateway .

# Copy web UI files
COPY --from=builder /app/web /root/web
# Copy documentation files for UI links
COPY --from=builder /app/docs /root/docs

# Expose ports
EXPOSE 8080 9090

HEALTHCHECK --interval=10s --timeout=3s --retries=3 CMD curl -fsS http://localhost:8080/healthz || exit 1

# Run the application
CMD ["./docker-faas-gateway"]
