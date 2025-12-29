# Docker FaaS Documentation

Complete documentation for Docker FaaS - a lightweight, production-ready FaaS platform with 100% faas-cli compatibility.

## Quick Navigation

### Getting Started
- **[Getting Started Guide](GETTING_STARTED.md)** - Installation, setup, and your first function deployment
- **[Project Overview](../README.md)** - Main README with project description and quick start

### Core Documentation

#### Reference
- **[API Reference](API.md)** - Complete REST API documentation
- **[Configuration Reference](CONFIGURATION.md)** - All environment variables and configuration options
- **[Architecture](ARCHITECTURE.md)** - System design, components, and data flow

#### Guides
- **[Deployment Guide](DEPLOYMENT.md)** - Production deployment scenarios (Docker Compose, Swarm, standalone)
- **[Source Packaging](SOURCE_PACKAGING.md)** - Build functions from source (zip upload, Git repositories)
- **[Web UI Guide](WEB_UI.md)** - Using the web interface for function management
- **[Secrets Management](SECRETS.md)** - Secure secret storage and injection
- **[Source Examples](SOURCE_EXAMPLES.md)** - Example functions and language templates

#### Operations
- **[Production Readiness](PRODUCTION_READINESS.md)** - Production deployment checklist and best practices
- **[V2 Enhancements](V2_ENHANCEMENTS.md)** - What's new in version 2.0

### Governance & Community
- **[Contributing Guide](../CONTRIBUTING.md)** - How to contribute to the project
- **[Code of Conduct](../CODE_OF_CONDUCT.md)** - Community guidelines
- **[Security Policy](../SECURITY.md)** - Vulnerability reporting and security practices
- **[Changelog](../CHANGELOG.md)** - Version history and release notes

### Design & Planning
- **[UI Requirements](design/UI_REQUIREMENTS.md)** - Web UI design specifications

### Archived Documents
- **[archived/](archived/)** - Historical documents and old specifications
  - [Original Specification](archived/ORIGINAL_SPECIFICATION.md)
  - [Enhancement Reports](archived/ENHANCEMENTS_REPORT.md)
  - [Release Notes v2](archived/RELEASE_NOTES_V2.md)
  - [Validation Summary](archived/VALIDATION_SUMMARY.md)

## Documentation by Role

### For Users
1. Start with [Getting Started](GETTING_STARTED.md)
2. Learn about [function deployment](SOURCE_PACKAGING.md)
3. Explore the [Web UI](WEB_UI.md)
4. Reference the [API docs](API.md) for CLI/automation

### For Operators
1. Review [Production Readiness](PRODUCTION_READINESS.md)
2. Study [Deployment scenarios](DEPLOYMENT.md)
3. Configure with [Configuration Reference](CONFIGURATION.md)
4. Understand [Architecture](ARCHITECTURE.md) for troubleshooting

### For Developers
1. Understand the [Architecture](ARCHITECTURE.md)
2. Read [Contributing Guide](../CONTRIBUTING.md)
3. Check [Source Examples](SOURCE_EXAMPLES.md) for templates
4. Review [API Reference](API.md) for integration

## Quick Reference

### Common Tasks
- **Deploy a function**: [Getting Started](GETTING_STARTED.md)
- **Use secrets**: [Secrets Management](SECRETS.md)
- **Build from source**: [Source Packaging](SOURCE_PACKAGING.md)
- **Scale functions**: [API Reference](API.md#scaling)
- **View logs**: [Web UI Guide](WEB_UI.md#logs) or [API Reference](API.md#logs)

### Configuration
- **Environment variables**: [Configuration Reference](CONFIGURATION.md)
- **Authentication setup**: [Configuration Reference](CONFIGURATION.md#authentication)
- **Network settings**: [Architecture](ARCHITECTURE.md#networking)

### Troubleshooting
- **Deployment issues**: [Deployment Guide](DEPLOYMENT.md)
- **Build failures**: [Source Packaging](SOURCE_PACKAGING.md)
- **Performance tuning**: [Production Readiness](PRODUCTION_READINESS.md)

## Examples

Example functions and templates are available in the [examples/](../examples/) directory:
- Python templates (Flask, basic)
- Node.js templates
- Go templates
- Multi-stage build examples

## External Resources

- **OpenFaaS Compatibility**: This project implements the OpenFaaS API and works with [faas-cli](https://github.com/openfaas/faas-cli)
- **Docker Documentation**: [docs.docker.com](https://docs.docker.com)
- **Function Watchdog**: Compatible with OpenFaaS watchdog patterns

## Contributing to Documentation

Documentation improvements are always welcome! Please:
1. Follow the existing style and structure
2. Update cross-references when moving files
3. Test all code examples
4. Run link checks: `python scripts/check-doc-links.py`
5. Submit PRs with clear descriptions

See [CONTRIBUTING.md](../CONTRIBUTING.md) for details.
