# API Documentation

Complete API reference for Docker FaaS Gateway.

## Authentication

When `AUTH_ENABLED=true`, API requests require either Basic Auth or a UI token.

```bash
Authorization: Basic <base64(username:password)>
Authorization: Bearer <token>
```

Default credentials: `admin:admin`

## Endpoints

### GET /system/info

Get system information.

**Response:**
```json
{
  "provider": {
    "name": "docker-faas",
    "version": "2.0.0",
    "orchestration": "docker"
  },
  "version": {
    "release": "2.0.0",
    "sha": "dev"
  },
  "arch": "x86_64"
}
```

### GET /system/functions

List all deployed functions.

**Response:**
```json
[
  {
    "name": "hello-world",
    "image": "docker-faas/hello-world:latest",
    "replicas": 1,
    "availableReplicas": 1,
    "invocationCount": 0,
    "envProcess": "python3 handler.py",
    "labels": {
      "com.docker-faas.example": "true"
    },
    "annotations": {},
    "createdAt": "2024-01-15T10:30:00Z"
  }
]
```

### POST /system/builds

Build a function image from source (zip or Git) and optionally deploy it.

Security limits:
- Git URLs must use http/https/git/ssh and may not resolve to localhost or private IPs.
- Zip uploads are capped at 2000 entries, 100MB per file, and 500MB total uncompressed size.

**JSON (Git) Example:**
```json
{
  "name": "hello-python",
  "deploy": true,
  "source": {
    "type": "git",
    "runtime": "python",
    "git": {
      "url": "https://github.com/org/repo.git",
      "ref": "main",
      "path": "."
    },
    "manifest": "name: hello-python\nruntime: python\ncommand: \"python handler.py\""
  }
}
```

**Multipart (Zip) Example:**
- `name`: function name
- `runtime`: runtime name
- `sourceType`: `zip`
- `deploy`: `true`
- `manifest`: optional docker-faas.yaml contents
- `files`: optional JSON array of inline files (supports `remove`)
- `file`: zip file upload

**Response:**
```json
{
  "name": "hello-python",
  "image": "docker-faas/hello-python:1700000000",
  "deployed": true,
  "updated": false
}
```

### POST /system/builds/inspect

Inspect a source bundle (zip or Git) and return the detected `docker-faas.yaml` contents.

**Request:** Same payload format as `/system/builds` (JSON or multipart).

**Response:**
```json
{
  "name": "hello-python",
  "runtime": "python",
  "command": "python handler.py",
  "manifest": "name: hello-python\nruntime: python\ncommand: \"python handler.py\"",
  "files": [
    {
      "path": "handler.py",
      "content": "print(\"hello\")\n",
      "editable": true
    },
    {
      "path": "data/sample.png",
      "editable": false
    }
  ]
}
```
Notes: `files` includes text files up to 200KB. Binary or larger files return `editable: false`.

### GET /system/builds

List recent source build activity.

**Query Parameters:**
- `status` (optional) - Filter by status (comma-separated values).
- `name` (optional) - Filter by function name (substring match).
- `sourceType` (optional) - Filter by source type (comma-separated values, e.g. `git`, `zip`, `inline`).
- `since` (optional) - RFC3339 timestamp; include builds started at or after this time.
- `before` (optional) - RFC3339 timestamp; include builds started before this time.
- `limit` (optional) - Limit the number of results.
- `includeOutput` (optional) - Include build output in the response (`true` or `false`, default `true`).

**Response:** JSON array of build entries.

Notes:
- Output is capped at `BUILD_OUTPUT_LIMIT` bytes per entry. When truncated, `truncated` is `true`.

### DELETE /system/builds

Clear build history.

**Response:** `204 No Content`

### GET /system/builds/{id}

Get a build entry by ID.

**Query Parameters:**
- `includeOutput` (optional) - Include build output in the response (`true` or `false`, default `true`).

**Response:** JSON build entry.

### GET /system/builds/stream

Stream build updates as server-sent events.

**Response:** `text/event-stream` with build entry payloads.

### POST /system/functions

Deploy a new function.

**Request:**
```json
{
  "service": "my-function",
  "image": "my-org/my-function:latest",
  "network": "docker-faas-net-my-function",
  "envProcess": "node index.js",
  "envVars": {
    "NODE_ENV": "production",
    "DEBUG": "false"
  },
  "labels": {
    "tier": "backend"
  },
  "secrets": ["api-key", "db-password"],
  "limits": {
    "memory": "512m",
    "cpu": "1"
  },
  "requests": {
    "memory": "256m",
    "cpu": "0.5"
  },
  "readOnlyRootFilesystem": true
}
```

If `network` is omitted, the gateway creates a per-function network using `<FUNCTIONS_NETWORK>-<service>`.

**Response:** `202 Accepted`

### PUT /system/functions

Update an existing function.

**Request:** Same as POST

**Response:** `202 Accepted`

### DELETE /system/functions

Delete a function.

**Query Parameters:**
- `functionName` (required) - Name of the function to delete

**Example:**
```bash
DELETE /system/functions+functionName=my-function
```

**Response:** `202 Accepted`

### POST /system/scale-function/{name}

Scale a function to a specific replica count.

**Request:**
```json
{
  "serviceName": "my-function",
  "replicas": 5
}
```

**Response:** `202 Accepted`

### GET /system/logs

Get logs from a function.

**Query Parameters:**
- `name` (required) - Function name
- `tail` (optional) - Number of lines to return (default: 100)

**Example:**
```bash
GET /system/logs+name=my-function&tail=50
```

**Response:** Plain text logs

### POST /function/{name}

Invoke a function.

Authentication: `/function/*` requires Basic Auth by default. Set `REQUIRE_AUTH_FOR_FUNCTIONS=false` to allow unauthenticated invocation for OpenFaaS compatibility.

**Request Body:** Function input (any content type)

**Response:** Function output

**Headers:**
- `Content-Type` - Passed to function
- `X-Forwarded-For` - Original client IP
- `X-Forwarded-Host` - Original host
- `X-Forwarded-Proto` - Original protocol

**Example:**
```bash
curl -X POST http://localhost:8080/function/my-function \
  -H "Content-Type: application/json" \
  -d '{"key": "value"}' \
  -u admin:admin
```

### POST /async-function/{name}

Invoke a function asynchronously (fire-and-forget).

**Response:** `202 Accepted`

**Headers:**
- `X-Call-Id` - Call identifier for tracing

**Example:**
```bash
curl -X POST http://localhost:8080/async-function/my-function \
  -u admin:admin \
  -d "Hello World"
```

### POST /system/function-async/{name}

Alias for async invocation.

**Response:** `202 Accepted`

### GET /healthz

Health check endpoint. This endpoint is always unauthenticated so Docker and load balancers can probe it.

Checks:
- Docker connectivity
- Database connectivity
- Base network existence

**Response:** `200 OK` with body "OK" when healthy, `503` when unhealthy.
If `Accept: application/json` is provided, returns a JSON payload with check details.

### GET /metrics

Prometheus metrics endpoint (port 9090, no authentication).

**Response:** Prometheus format metrics

### GET /system/metrics

Prometheus metrics endpoint on the gateway port (authenticated).

**Response:** Prometheus format metrics

### GET /system/config

Returns a safe configuration snapshot for the UI.

**Response:** JSON config values

### POST /auth/login

Issue a short-lived UI token.

**Request:**
```json
{
  "username": "admin",
  "password": "admin"
}
```

**Response:**
```json
{
  "token": "...",
  "expiresAt": "2025-01-01T00:00:00Z"
}
```

### POST /auth/logout

Revoke the current token.

**Response:** `204 No Content`

## Error Responses

### 400 Bad Request
```json
{
  "error": "Invalid request body"
}
```

### 401 Unauthorized
```
Unauthorized
```

### 404 Not Found
```json
{
  "error": "Function not found"
}
```

### 409 Conflict
```json
{
  "error": "Function already exists, use PUT to update"
}
```

### 500 Internal Server Error
```json
{
  "error": "Failed to deploy function: <details>"
}
```

## Rate Limiting

Currently no rate limiting is implemented. Consider using a reverse proxy like Nginx or Traefik for rate limiting in production.

## Timeouts

- Read Timeout: 60s (configurable via `READ_TIMEOUT`)
- Write Timeout: 60s (configurable via `WRITE_TIMEOUT`)
- Execution Timeout: 60s (configurable via `EXEC_TIMEOUT`)

## OpenFaaS Compatibility

This API implements the core OpenFaaS Gateway API endpoints. The following differences exist:

### Supported:
- Function deployment (POST)
- Function updates (PUT)
- Function deletion (DELETE)
- Function listing (GET)
- Function scaling (POST)
- Function invocation (POST/GET/etc.)
- Function logs (GET)
- Async invocations
- System info (GET)
- Health check (GET)

### Not Yet Supported:
- Function namespaces

## Examples

### Deploy Function with Environment Variables

```bash
curl -X POST http://localhost:8080/system/functions \
  -u admin:admin \
  -H "Content-Type: application/json" \
  -d '{
    "service": "env-demo",
    "image": "ghcr.io/openfaas/alpine:latest",
    "envVars": {
      "fprocess": "env",
      "MY_VAR": "hello",
      "ANOTHER_VAR": "world"
    }
  }'
```

### Deploy Function with Resource Limits

```bash
curl -X POST http://localhost:8080/system/functions \
  -u admin:admin \
  -H "Content-Type: application/json" \
  -d '{
    "service": "resource-demo",
    "image": "my-function:latest",
    "limits": {
      "memory": "256m",
      "cpu": "0.5"
    }
  }'
```

### Invoke Function with JSON

```bash
curl -X POST http://localhost:8080/function/my-function \
  -u admin:admin \
  -H "Content-Type: application/json" \
  -d '{"name": "John", "age": 30}'
```

### Scale Function

```bash
curl -X POST http://localhost:8080/system/scale-function/my-function \
  -u admin:admin \
  -H "Content-Type: application/json" \
  -d '{"serviceName": "my-function", "replicas": 3}'
```

### Get Function Logs

```bash
curl http://localhost:8080/system/logs+name=my-function&tail=20 \
  -u admin:admin
```
