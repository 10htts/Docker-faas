# Docker FaaS

Docker-native Function-as-a-Service (FaaS) gateway compatible with OpenFaaS and `faas-cli`. Run functions on Docker without Kubernetes.

## Highlights

- OpenFaaS-compatible API and `faas-cli` workflows
- Web UI for deploy, monitor, logs, secrets, and debug
- Source packaging (zip/Git) and builder templates
- Security hardening: cap drop, no-new-privileges, per-function networks
- Prometheus metrics and structured logging

## Quick Start

### Prerequisites

- Docker 20.10+
- Docker Compose (optional)
- `faas-cli` (optional, for CLI deployments)

### Docker Compose

```bash
git clone https://github.com/10htts/Docker-faas.git
cd docker-faas

docker-compose up -d

curl http://localhost:8080/healthz
```

Web UI: `http://localhost:8080/ui/`

Default credentials:
- Username: `admin`
- Password: `admin`

### Deploy a Function

```bash
faas-cli login --gateway http://localhost:8080 --username admin --password admin

faas-cli deploy \
  --image ghcr.io/openfaas/alpine:latest \
  --name echo \
  --gateway http://localhost:8080 \
  --env fprocess="cat"

echo "Hello" | faas-cli invoke echo --gateway http://localhost:8080
```

Auth note: `/function/*` requires Basic Auth by default. Set `REQUIRE_AUTH_FOR_FUNCTIONS=false` for OpenFaaS compatibility.

## Documentation

**[Complete Documentation Index](docs/README.md)**

### Quick Links
- **[Getting Started Guide](docs/GETTING_STARTED.md)** - Installation, setup, and first deployment
- **[API Reference](docs/API.md)** - Complete REST API documentation
- **[Architecture](docs/ARCHITECTURE.md)** - System design and components
- **[Deployment Guide](docs/DEPLOYMENT.md)** - Production deployment scenarios
- **[Configuration](docs/CONFIGURATION.md)** - Environment variables and settings
- **[Web UI Guide](docs/WEB_UI.md)** - Using the web interface
- **[Source Packaging](docs/SOURCE_PACKAGING.md)** - Build from zip/Git sources
- **[Production Readiness](docs/PRODUCTION_READINESS.md)** - Production deployment checklist
- **[Secrets Management](docs/SECRETS.md)** - Secure secret storage and injection

## Configuration

All configuration is via environment variables. See `docs/CONFIGURATION.md` for the full list and defaults.

## Development

```bash
make build
make test
```

E2E tests (requires Docker):

```bash
./scripts/run-tests.sh
```

Windows PowerShell:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\run-tests.ps1
```

## Contributing

Contributions are welcome! Please see:
- **[Contributing Guide](CONTRIBUTING.md)** - Development setup and guidelines
- **[Code of Conduct](CODE_OF_CONDUCT.md)** - Community standards
- **[Security Policy](SECURITY.md)** - Vulnerability reporting
- **[Changelog](CHANGELOG.md)** - Version history and releases

## License

MIT License. See `LICENSE`.
