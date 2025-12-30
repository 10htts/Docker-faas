# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [2.0.0] - 2025-12-30

### Added
- Source build API for zip and Git with inspect support
- Web UI with auth tokens, build history, settings, metrics, backup/import
- Async invocation endpoints (`/system/function-async` and `/async-function`)
- Database migrations with auto-apply on startup
- Build history tracking with output retention and filtering
- Network lifecycle cleanup and orphaned network tools
- Auth rate limiting and config snapshot endpoint
- E2E tests and Playwright UI tests

### Changed
- Gateway base image moved to Debian bookworm-slim for SQLite compatibility
- Debug ports bind to localhost by default with configuration override
- CORS configuration made environment-aware
- Documentation reorganized into a unified `docs/` structure

### Security
- Git URL validation to block SSRF and internal network access
- Zip extraction protection against bombs, symlinks, and path traversal
- Function name validation across API handlers
- Tokens replace password persistence in the UI

### Documentation
- New production readiness checklist and release process guide
- Source packaging and examples expanded

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

[Unreleased]: https://github.com/docker-faas/docker-faas/compare/v2.0.0...HEAD
[2.0.0]: https://github.com/docker-faas/docker-faas/releases/tag/v2.0.0
[1.0.0]: https://github.com/docker-faas/docker-faas/releases/tag/v1.0.0
