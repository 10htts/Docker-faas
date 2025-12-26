# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.0.0] - 2024-01-15

### Added
- Initial release of Docker FaaS
- OpenFaaS-compatible gateway API
- Docker provider for container management
- Function router with round-robin load balancing
- SQLite-based state store for function metadata
- Basic authentication middleware
- Prometheus metrics integration
- Comprehensive logging
- Docker Compose setup for development
- Complete API documentation
- Deployment guide
- Example functions
- Unit and integration tests
- CI/CD with GitHub Actions
- Health check endpoint
- Function scaling support
- Container log retrieval
- Resource limits support (CPU, memory)
- Read-only root filesystem support
- Environment variable injection
- Custom labels and annotations

### Security
- Basic authentication for all endpoints
- Constant-time password comparison
- TLS support via reverse proxy
- No authentication bypass vulnerabilities

### Documentation
- Complete README with quick start guide
- API documentation
- Deployment guide
- Contributing guidelines
- Example functions and stack files
- Integration test documentation

[Unreleased]: https://github.com/docker-faas/docker-faas/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/docker-faas/docker-faas/releases/tag/v1.0.0
