# Docker FaaS

A lightweight, Docker-native Function-as-a-Service (FaaS) platform compatible with OpenFaaS and `faas-cli`. Run serverless functions on Docker without requiring Kubernetes.

[![CI](https://github.com/docker-faas/docker-faas/workflows/CI/badge.svg)](https://github.com/docker-faas/docker-faas/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/docker-faas/docker-faas)](https://goreportcard.com/report/github.com/docker-faas/docker-faas)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Version](https://img.shields.io/badge/version-2.0.0-blue)](RELEASE_NOTES_V2.md)

## 🎉 What's New in v2.0

- 🖥️ **Web UI** - Modern, production-focused control panel for managing functions
- 🔐 **Secrets Management** - OpenFaaS-compatible file-based secrets
- 🛡️ **Security Hardening** - Capability dropping, no-new-privileges, network isolation
- 🐛 **Debug Mode** - Automatic port mapping for popular debuggers

[See full release notes](RELEASE_NOTES_V2.md) | [v2 Enhancements Guide](docs/V2_ENHANCEMENTS.md) | [Web UI Guide](docs/WEB_UI.md)

## Features

- ✅ **OpenFaaS Compatible** - Works with standard `faas-cli` and OpenFaaS function contracts
- 🐳 **Docker Native** - Runs on Docker without Kubernetes
- 🖥️ **Web UI** - Modern control panel for visual function management *(NEW in v2.0)*
- 🔐 **Secrets Management** - Secure file-based secrets with validation *(NEW in v2.0)*
- 🛡️ **Enterprise Security** - Capability dropping, privilege controls, network isolation *(NEW in v2.0)*
- 🐛 **Debug Mode** - Built-in debugger support with auto port mapping *(NEW in v2.0)*
- 🔒 **Secure** - Built-in authentication and TLS support
- 📊 **Observable** - Prometheus metrics and comprehensive logging
- ⚡ **Fast** - Lightweight Go implementation with minimal overhead
- 🔄 **Dynamic** - Deploy, update, scale, and remove functions at runtime
- 🧪 **Well Tested** - Comprehensive unit and integration tests (>80% coverage)

## Quick Start

### Prerequisites

- Docker 20.10+
- Docker Compose (optional, for easier setup)
- `faas-cli` (for deploying functions)

### Installation

#### Using Docker Compose (Recommended)

```bash
# Clone the repository
git clone https://github.com/docker-faas/docker-faas.git
cd docker-faas

# Start the gateway
docker-compose up -d

# Verify installation
./verify-deployment.sh

# Or check health manually
curl http://localhost:8080/healthz

# Access Web UI
open http://localhost:8080/ui/
```

> **💡 Quick Test:** Run `./verify-deployment.sh` for automated verification, or see [QUICK_TEST.md](QUICK_TEST.md) for testing options.

### Access the Web UI

Open your browser and navigate to:

```
http://localhost:8080/ui/
```

Default credentials:
- **Username**: `admin`
- **Password**: `admin`

The Web UI provides:
- Visual function management
- Real-time system monitoring
- Function invocation testing
- Secrets management
- Logs viewer

See the [Web UI Guide](docs/WEB_UI.md) for full documentation.

#### Using Pre-built Image

```bash
# Pull the image
docker pull ghcr.io/docker-faas/docker-faas:latest

# Create gateway network (functions get per-function networks automatically)
docker network create docker-faas-net

# Run the gateway
docker run -d \
  --name docker-faas-gateway \
  -p 8080:8080 \
  -p 9090:9090 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v faas-data:/data \
  --network docker-faas-net \
  ghcr.io/docker-faas/docker-faas:latest
```

#### Building from Source

```bash
# Clone and build
git clone https://github.com/docker-faas/docker-faas.git
cd docker-faas

# Install dependencies
make install-deps

# Build
make build

# Run
./bin/docker-faas-gateway
```

### Deploy Your First Function

```bash
# Login to the gateway
faas-cli login --gateway http://localhost:8080 --username admin --password admin

# Deploy the example echo function
faas-cli deploy \
  --image ghcr.io/openfaas/alpine:latest \
  --name echo \
  --gateway http://localhost:8080 \
  --env fprocess="cat"

# Invoke the function
echo "Hello World" | faas-cli invoke echo --gateway http://localhost:8080
```

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    Docker FaaS Gateway                   │
│  ┌───────────────┐  ┌──────────┐  ┌─────────────────┐  │
│  │  HTTP API     │  │  Router  │  │  Prometheus     │  │
│  │  (OpenFaaS)   │  │  (LB)    │  │  Metrics        │  │
│  └───────────────┘  └──────────┘  └─────────────────┘  │
│  ┌───────────────┐  ┌──────────┐  ┌─────────────────┐  │
│  │  Auth         │  │  Store   │  │  Docker         │  │
│  │  Middleware   │  │  (SQLite)│  │  Provider       │  │
│  └───────────────┘  └──────────┘  └─────────────────┘  │
└──────────────────────────┬──────────────────────────────┘
                           │ Docker API
              ┌────────────┴────────────┐
              │    Docker Network       │
              │   (docker-faas-net)     │
              └────────────┬────────────┘
         ┌─────────────────┼─────────────────┐
         │                 │                 │
    ┌────▼────┐       ┌────▼────┐       ┌────▼────┐
    │Function │       │Function │       │Function │
    │   #1    │       │   #2    │       │   #3    │
    └─────────┘       └─────────┘       └─────────┘
```

### Components

1. **Gateway API** - Implements OpenFaaS-compatible REST API
2. **Router** - Routes requests to function containers with load balancing
3. **Docker Provider** - Manages Docker containers lifecycle
4. **Store** - SQLite database for function metadata
5. **Auth Middleware** - Basic authentication for API endpoints
6. **Metrics** - Prometheus metrics for observability

## API Endpoints

### System Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/system/info` | Get system information |
| GET | `/system/functions` | List all functions |
| POST | `/system/functions` | Deploy a new function |
| PUT | `/system/functions` | Update an existing function |
| DELETE | `/system/functions` | Delete a function |
| POST | `/system/scale-function/{name}` | Scale a function |
| GET | `/system/logs?name={function}` | Get function logs |
| GET | `/healthz` | Health check |
| GET | `/metrics` | Prometheus metrics (port 9090) |

### Function Invocation

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST/GET/PUT/DELETE | `/function/{name}` | Invoke a function |

## Configuration

Configure the gateway using environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `GATEWAY_PORT` | `8080` | Gateway HTTP port |
| `METRICS_PORT` | `9090` | Prometheus metrics port |
| `FUNCTIONS_NETWORK` | `docker-faas-net` | Base network name/prefix for per-function networks |
| `GATEWAY_CONTAINER_NAME` | `` | Optional container name/ID for connecting the gateway to function networks |
| `DEBUG_BIND_ADDRESS` | `127.0.0.1` | Bind address for debug ports |
| `AUTH_ENABLED` | `true` | Enable/disable authentication |
| `AUTH_USER` | `admin` | Basic auth username |
| `AUTH_PASSWORD` | `admin` | Basic auth password |
| `STATE_DB_PATH` | `docker-faas.db` | SQLite database path |
| `LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |
| `READ_TIMEOUT` | `60s` | HTTP read timeout |
| `WRITE_TIMEOUT` | `60s` | HTTP write timeout |
| `EXEC_TIMEOUT` | `60s` | Function execution timeout |
| `DEFAULT_REPLICAS` | `1` | Default replica count |
| `MAX_REPLICAS` | `10` | Maximum replica count |

## Usage Examples

### Using faas-cli

```bash
# Login
faas-cli login --gateway http://localhost:8080 -u admin -p admin

# Deploy from a stack file
faas-cli deploy -f examples/stack.yml

# List functions
faas-cli list

# Invoke a function
echo "test" | faas-cli invoke hello-world

# Scale a function
faas-cli scale hello-world --replicas 3

# Get logs
faas-cli logs hello-world

# Remove a function
faas-cli remove hello-world
```

### Using curl

```bash
# Deploy a function
curl -X POST http://localhost:8080/system/functions \
  -u admin:admin \
  -H "Content-Type: application/json" \
  -d '{
    "service": "my-function",
    "image": "ghcr.io/openfaas/alpine:latest",
    "envVars": {
      "fprocess": "cat"
    }
  }'

# Invoke a function
curl -X POST http://localhost:8080/function/my-function \
  -u admin:admin \
  -d "Hello World"

# Scale a function
curl -X POST http://localhost:8080/system/scale-function/my-function \
  -u admin:admin \
  -H "Content-Type: application/json" \
  -d '{"serviceName": "my-function", "replicas": 5}'

# Delete a function
curl -X DELETE http://localhost:8080/system/functions?functionName=my-function \
  -u admin:admin
```

## Metrics

Prometheus metrics are available at `http://localhost:9090/metrics`:

- `gateway_http_requests_total` - Total HTTP requests
- `function_invocations_total` - Total function invocations
- `function_duration_seconds` - Function execution duration
- `function_errors_total` - Function errors
- `functions_deployed` - Number of deployed functions
- `function_replicas` - Replica count per function

## Development

### Building

```bash
# Install dependencies
make install-deps

# Build
make build

# Run tests
make test

# Run with coverage
make coverage

# Run integration tests (requires running gateway)
make integration-test
```

### Project Structure

```
.
├── cmd/
│   └── gateway/          # Main application entry point
├── pkg/
│   ├── config/          # Configuration management
│   ├── gateway/         # HTTP handlers
│   ├── middleware/      # HTTP middleware (auth, logging)
│   ├── metrics/         # Prometheus metrics
│   ├── provider/        # Docker provider
│   ├── router/          # Request routing
│   ├── store/           # Database layer
│   └── types/           # Type definitions
├── tests/
│   └── integration/     # Integration tests
├── examples/            # Example functions
├── Dockerfile           # Gateway container image
├── docker-compose.yml   # Development setup
└── Makefile            # Build automation
```

## Migration to Kubernetes

To migrate to OpenFaaS on Kubernetes later:

1. Export your function definitions (already in `stack.yml`)
2. Install OpenFaaS on Kubernetes
3. Deploy functions using the same `faas-cli deploy` commands
4. Update the gateway URL to point to OpenFaaS gateway

No changes to function code or deployment files are required!

## Troubleshooting

### Gateway won't start

```bash
# Check Docker daemon
docker ps

# Check network
docker network ls | grep docker-faas-net

# Check logs
docker-compose logs gateway
```

### Function deployment fails

```bash
# Check if image exists
docker pull <image-name>

# Check gateway logs
curl http://localhost:8080/system/logs?name=<function-name> -u admin:admin

# Check Docker containers
docker ps -a | grep <function-name>
```

### Function invocation fails

```bash
# Check function status
faas-cli list

# Check function logs
faas-cli logs <function-name>

# Verify function is running
docker ps | grep <function-name>
```

## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for details.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- Inspired by [OpenFaaS](https://github.com/openfaas/faas)
- Uses [of-watchdog](https://github.com/openfaas/of-watchdog) for function runtime
- Built with [Docker Engine API](https://docs.docker.com/engine/api/)

## Support

- 📚 [Documentation](docs/)
- 🐛 [Issue Tracker](https://github.com/docker-faas/docker-faas/issues)
- 💬 [Discussions](https://github.com/docker-faas/docker-faas/discussions)

## Roadmap

- [ ] Async invocations with queue support
- [ ] Autoscaling based on metrics
- [ ] Multi-node support
- [ ] Function build API
- [ ] Advanced event sources
- [ ] WebUI dashboard
