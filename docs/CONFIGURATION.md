# Configuration

Docker FaaS is configured through environment variables. Defaults are shown below.

## Core

| Variable | Default | Description |
| --- | --- | --- |
| `GATEWAY_PORT` | `8080` | Gateway HTTP port |
| `READ_TIMEOUT` | `60s` | HTTP read timeout |
| `WRITE_TIMEOUT` | `60s` | HTTP write timeout |
| `EXEC_TIMEOUT` | `60s` | Function execution timeout |
| `CORS_ALLOWED_ORIGINS` | `` | Comma-separated list of allowed origins (empty disables CORS) |
| `LOG_LEVEL` | `info` | Log level (`debug`, `info`, `warn`, `error`) |

## Docker and Networking

| Variable | Default | Description |
| --- | --- | --- |
| `DOCKER_HOST` | `` | Docker host (empty uses environment defaults) |
| `FUNCTIONS_NETWORK` | `docker-faas-net` | Base network name for per-function networks |
| `GATEWAY_CONTAINER_NAME` | `` | Optional container name/ID to attach the gateway to function networks |

## Auth

| Variable | Default | Description |
| --- | --- | --- |
| `AUTH_ENABLED` | `true` | Enable Basic Auth for API endpoints |
| `AUTH_USER` | `admin` | Basic Auth username |
| `AUTH_PASSWORD` | `admin` | Basic Auth password |
| `REQUIRE_AUTH_FOR_FUNCTIONS` | `true` | Require auth on `/function/*` (set `false` for OpenFaaS compatibility) |
| `AUTH_RATE_LIMIT` | `10` | Failed auth attempts allowed per window |
| `AUTH_RATE_WINDOW` | `1m` | Rate limit window duration |

## Database

| Variable | Default | Description |
| --- | --- | --- |
| `STATE_DB_PATH` | `docker-faas.db` | SQLite database path |

## Metrics

| Variable | Default | Description |
| --- | --- | --- |
| `METRICS_ENABLED` | `true` | Enable Prometheus metrics |
| `METRICS_PORT` | `9090` | Prometheus metrics port |

## Scaling

| Variable | Default | Description |
| --- | --- | --- |
| `DEFAULT_REPLICAS` | `1` | Default replica count |
| `MAX_REPLICAS` | `10` | Maximum replica count |

## Debug

| Variable | Default | Description |
| --- | --- | --- |
| `DEBUG_BIND_ADDRESS` | `127.0.0.1` | Host bind address for debug ports |

## Tips

- For OpenFaaS compatibility with `faas-cli invoke`, set `REQUIRE_AUTH_FOR_FUNCTIONS=false`.
- For production, keep `AUTH_ENABLED=true` and rotate `AUTH_PASSWORD`.
- When `AUTH_ENABLED=false` and `CORS_ALLOWED_ORIGINS` is empty, CORS defaults to `*` for local development.
