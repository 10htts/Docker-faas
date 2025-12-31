# Architecture Documentation

This document describes the internal architecture of Docker FaaS.

## System Overview

Docker FaaS is a lightweight FaaS platform built on Docker, designed to be compatible with OpenFaaS while being simpler to deploy and operate.

## Components

### 1. Gateway API (`pkg/gateway`)

The gateway provides the HTTP API interface compatible with OpenFaaS.

**Responsibilities:**
- Handle incoming HTTP requests
- Validate request payloads
- Coordinate between provider, router, and store
- Return appropriate responses
- Apply middleware (auth, logging)

**Key Handlers:**
- `HandleSystemInfo` - System information
- `HandleListFunctions` - List deployed functions
- `HandleDeployFunction` - Deploy new function
- `HandleUpdateFunction` - Update existing function
- `HandleDeleteFunction` - Remove function
- `HandleScaleFunction` - Scale function replicas
- `HandleGetLogs` - Retrieve function logs
- `HandleInvokeFunction` - Invoke function
- `HandleHealthz` - Health check

### 2. Docker Provider (`pkg/provider`)

Manages the lifecycle of Docker containers for functions.

**Responsibilities:**
- Pull Docker images
- Create and start containers
- Scale containers up/down
- Remove containers
- Retrieve container information
- Get container logs
- Manage Docker network

**Key Operations:**
- `DeployFunction` - Create containers for a function
- `UpdateFunction` - Update function containers
- `RemoveFunction` - Remove all function containers
- `ScaleFunction` - Adjust replica count
- `GetFunctionContainers` - Get container details
- `GetContainerLogs` - Retrieve logs

**Docker Integration:**
- Uses official Docker Engine API client
- Creates containers with proper labels for identification
- Creates per-function networks and connects the gateway to each
- Applies resource limits (CPU, memory)
- Supports environment variables and secrets

### 3. Router (`pkg/router`)

Routes incoming function invocations to appropriate containers.

**Responsibilities:**
- Select target container using load balancing
- Forward HTTP requests to containers
- Handle timeouts and errors
- Maintain connection pools

**Load Balancing:**
- Round-robin algorithm
- Per-function counter tracking
- Filters for running containers only

**Request Forwarding:**
- Preserves original headers
- Adds X-Forwarded-* headers
- Respects timeout configurations
- Streams response back to client

### 4. State Store (`pkg/store`)

Persists function metadata using SQLite.

**Responsibilities:**
- Store function configurations
- Retrieve function metadata
- Update function settings
- Track replica counts
- Maintain creation/update timestamps

**Database Schema:**
```sql
CREATE TABLE functions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,
    image TEXT NOT NULL,
    env_process TEXT,
    env_vars TEXT,       -- JSON
    labels TEXT,         -- JSON
    secrets TEXT,        -- JSON
    network TEXT NOT NULL,
    replicas INTEGER NOT NULL DEFAULT 1,
    limits TEXT,         -- JSON
    requests TEXT,       -- JSON
    read_only BOOLEAN NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);
```

### 5. Middleware (`pkg/middleware`)

Provides cross-cutting concerns via HTTP middleware.

**Authentication Middleware:**
- Basic HTTP authentication
- Constant-time comparison
- Bypass for health checks
- Configurable enable/disable

**Logging Middleware:**
- Request/response logging
- Duration tracking
- Structured logging (logrus)
- Metrics recording

### 6. Metrics (`pkg/metrics`)

Prometheus metrics for observability.

**Metrics Exposed:**
- `gateway_http_requests_total` - Total gateway requests
- `function_invocations_total` - Function invocations
- `function_duration_seconds` - Invocation duration
- `function_errors_total` - Function errors
- `functions_deployed` - Deployed function count
- `function_replicas` - Replicas per function

### 7. Configuration (`pkg/config`)

Centralized configuration management.

**Configuration Sources:**
1. Environment variables (primary)
2. Default values (fallback)

**Type Safety:**
- Strongly typed configuration struct
- Helper functions for parsing
- Duration parsing
- Boolean parsing

## Data Flow

### Function Deployment

```
Client Request
    v
Gateway (HandleDeployFunction)
    v
Validate Request
    v
Provider (DeployFunction)
    v
Pull Image -> Create Containers -> Start Containers
    v
Store (CreateFunction)
    v
Persist Metadata
    v
Update Metrics
    v
Response to Client
```

### Function Invocation

```
Client Request
    v
Gateway (HandleInvokeFunction)
    v
Router (RouteRequest)
    v
Select Container (Round-Robin)
    v
Forward Request to Container
    v
Container (of-watchdog -> function)
    v
Stream Response Back
    v
Record Metrics
    v
Response to Client
```

### Function Scaling

```
Client Request
    v
Gateway (HandleScaleFunction)
    v
Validate Request
    v
Store (GetFunction)
    v
Provider (ScaleFunction)
    v
Create/Remove Containers
    v
Store (UpdateReplicas)
    v
Update Metrics
    v
Response to Client
```

## Network Architecture

```
+-------------------------------------+
|         Host Network                |
|                                     |
|  +------------------------------+  |
|  |   Docker FaaS Gateway        |  |
|  |   - Port 8080 (API)          |  |
|  |   - Port 9090 (Metrics)      |  |
|  +----------+-------------------+  |
|             |                       |
+-------------+-----------------------+
              |
    +---------v----------+
    |  docker-faas-net   |
    |  (Bridge Network)  |
    +---------+----------+
              |
    +---------+----------------------+
    |                                |
+---v----+  +--------+  +--------+
|Func-1-0|  |Func-1-1|  |Func-2-0|
|:8080   |  |:8080   |  |:8080   |
+--------+  +--------+  +--------+
```

**Network Details:**
- Gateway bridges host and function network
- Functions isolated on private network
- Functions expose port 8080 internally
- Gateway routes to containers by IP
- DNS resolution within network

## Security Model

### Authentication Flow

```
Request
    v
Auth Middleware
    v
Extract Basic Auth Header
    v
Constant-Time Compare
    v
[Valid] -> Next Handler
    v
[Invalid] -> 401 Unauthorized
```

### Container Isolation

- Functions run on isolated network
- No direct host access
- Resource limits enforced
- Read-only filesystem option
- No privileged containers

### Data Protection

- Credentials stored as environment variables
- Database on persistent volume
- No secrets in logs
- TLS via reverse proxy

## Scalability Considerations

### Vertical Scaling
- Gateway is single-threaded Go HTTP server
- Can handle thousands of concurrent requests
- Limited by CPU and memory

### Horizontal Scaling
- Functions scale horizontally via replicas
- Round-robin load balancing
- Stateless gateway design

### Limitations
- Single gateway instance
- SQLite not suitable for high concurrency
- No distributed state

### Future Improvements
- Multi-gateway support
- Redis/PostgreSQL backend
- Distributed lock manager
- Message queue for async

## Error Handling

### Gateway Errors
- Validation errors -> 400 Bad Request
- Authentication errors -> 401 Unauthorized
- Not found -> 404 Not Found
- Conflicts -> 409 Conflict
- Internal errors -> 500 Internal Server Error

### Provider Errors
- Image pull failures
- Container creation failures
- Network errors
- Timeout errors

**Error Recovery:**
- Failed deployments rolled back
- Orphaned containers cleaned up
- Database transactions for consistency

## Observability

### Logging Levels
- **DEBUG**: Detailed execution flow
- **INFO**: Normal operations
- **WARN**: Potential issues
- **ERROR**: Actual errors

### Metrics Collection
- Request counters
- Duration histograms
- Error rates
- Function statistics

### Health Checks
- `/healthz` endpoint
- No dependencies checked
- Always returns 200 OK

## Performance Characteristics

### Latency
- Gateway overhead: <5ms
- Container routing: <10ms
- Function execution: Variable
- Total: Depends on function

### Throughput
- Gateway: 10k+ req/s
- Limited by function performance
- Network bandwidth constraints

### Resource Usage
- Gateway: ~50MB RAM idle
- SQLite: Minimal overhead
- Containers: Per function

## Comparison with OpenFaaS

### Similarities
- Compatible API
- Function contracts
- faas-cli support
- Metrics format

### Differences
- Docker instead of Kubernetes
- SQLite instead of etcd
- No distributed components
- Simpler deployment

### Migration Path
1. Export function definitions (use the Web UI export or script against `/system/functions`).
2. Migrate secrets to your K8s secret store.
3. Deploy OpenFaaS on Kubernetes.
4. Deploy functions with `faas-cli` using the exported configs (images, env vars, labels, secrets, limits).
5. Validate routing and scaling, then decommission the Docker host.
3. Import functions
4. Update gateway URL
5. No code changes needed
