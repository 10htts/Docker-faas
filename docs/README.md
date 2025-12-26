# Docker FaaS Documentation

Welcome to the Docker FaaS documentation!

## Documentation Index

### Getting Started
- [**Getting Started Guide**](GETTING_STARTED.md) - Start here! Quick setup and first function deployment
- [**README**](../README.md) - Project overview and quick reference

### Core Documentation
- [**API Reference**](API.md) - Complete API endpoint documentation
- [**Architecture**](ARCHITECTURE.md) - Internal architecture and design decisions
- [**Deployment Guide**](DEPLOYMENT.md) - Production deployment and configuration

### Contributing
- [**Contributing Guide**](../CONTRIBUTING.md) - How to contribute to the project
- [**Changelog**](../CHANGELOG.md) - Version history and changes

## Quick Links

### For Users
1. **First time?** → [Getting Started Guide](GETTING_STARTED.md)
2. **Deploying to production?** → [Deployment Guide](DEPLOYMENT.md)
3. **Need API details?** → [API Reference](API.md)
4. **Want to understand how it works?** → [Architecture](ARCHITECTURE.md)

### For Developers
1. **Want to contribute?** → [Contributing Guide](../CONTRIBUTING.md)
2. **Understanding the codebase?** → [Architecture](ARCHITECTURE.md)
3. **Running tests?** → [README - Development](../README.md#development)

### For Operators
1. **Production setup?** → [Deployment Guide](DEPLOYMENT.md)
2. **Monitoring?** → [Deployment Guide - Monitoring](DEPLOYMENT.md#monitoring-setup)
3. **Troubleshooting?** → [Getting Started - Troubleshooting](GETTING_STARTED.md#troubleshooting)

## Examples

Example functions and stack files are available in the [`examples/`](../examples/) directory:

- `hello-world/` - Simple Python function
- `stack.yml` - Example stack file with multiple functions
- `README.md` - Example usage instructions

## API Endpoints Summary

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/system/info` | GET | System information |
| `/system/functions` | GET | List functions |
| `/system/functions` | POST | Deploy function |
| `/system/functions` | PUT | Update function |
| `/system/functions` | DELETE | Remove function |
| `/system/scale-function/{name}` | POST | Scale function |
| `/system/logs` | GET | Get function logs |
| `/function/{name}` | POST/GET/etc | Invoke function |
| `/healthz` | GET | Health check |
| `/metrics` | GET | Prometheus metrics |

Full details in [API Reference](API.md).

## Configuration

All configuration is done via environment variables. See [README - Configuration](../README.md#configuration) for the complete list.

Key variables:
- `GATEWAY_PORT` - API port (default: 8080)
- `AUTH_ENABLED` - Enable authentication (default: true)
- `AUTH_USER` - Username (default: admin)
- `AUTH_PASSWORD` - Password (default: admin)
- `FUNCTIONS_NETWORK` - Base network name/prefix for per-function networks (default: docker-faas-net)
- `GATEWAY_CONTAINER_NAME` - Optional container name/ID for connecting the gateway to function networks
- `DEBUG_BIND_ADDRESS` - Bind address for debug ports (default: 127.0.0.1)

## Architecture Overview

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
                           │
                    Function Containers
```

See [Architecture](ARCHITECTURE.md) for details.

## Support

### Community
- 💬 [GitHub Discussions](https://github.com/docker-faas/docker-faas/discussions)
- 🐛 [Issue Tracker](https://github.com/docker-faas/docker-faas/issues)

### Resources
- 📚 [OpenFaaS Docs](https://docs.openfaas.com/)
- 🐳 [Docker Docs](https://docs.docker.com/)
- 📊 [Prometheus Docs](https://prometheus.io/docs/)

## License

Docker FaaS is licensed under the MIT License. See [LICENSE](../LICENSE) for details.
