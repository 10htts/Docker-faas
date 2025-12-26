# Integration Tests

This directory contains integration tests for the Docker FaaS platform.

## Prerequisites

1. Docker daemon running
2. Docker FaaS gateway running (via docker-compose or manually)
3. Gateway accessible at `http://localhost:8080`

## Running Integration Tests

### Start the Gateway

```bash
# Using docker-compose
docker-compose up -d

# Wait for gateway to be ready
curl http://localhost:8080/healthz
```

### Run Tests

```bash
# Run integration tests
make integration-test

# Or directly with go
go test -v -tags=integration ./tests/integration/...
```

## Test Coverage

The integration tests cover:

- System info endpoint
- Health check endpoint
- Function deployment
- Function listing
- Function invocation
- Function scaling
- Function logs
- Function deletion

## Cleanup

After tests complete, you can clean up:

```bash
docker-compose down
docker volume rm docker-faas-data
```

## Troubleshooting

If tests fail:

1. Check gateway logs: `docker-compose logs gateway`
2. Verify Docker daemon is running: `docker ps`
3. Ensure network exists: `docker network ls | grep docker-faas-net`
4. Check authentication credentials match (admin/admin)
