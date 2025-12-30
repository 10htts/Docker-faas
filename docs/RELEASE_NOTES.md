# Release Notes

## v2.0.0 (2025-12-30)

### Highlights
- Build functions from source (zip or Git) with manifest inspection and file editing.
- Web UI with token-based auth, build history, metrics, settings, backup/import, and logs.
- Stronger security posture (capabilities dropped, no-new-privileges, SSRF/zip protection).
- Async invocation endpoints compatible with OpenFaaS flows.

### Security
- Git URL validation blocks localhost/private address cloning.
- Zip extraction guards against bombs, symlinks, and path traversal.
- Debug ports bind to localhost by default.
- Auth rate limiting and UI tokens prevent credential leakage.

### Operations
- Database migrations run automatically on startup.
- Health checks validate Docker, DB, and network connectivity.
- Build history retention and output limits are configurable.

### Testing
- E2E tests for security, network isolation, debug mode, upgrades, and faas-cli.
- UI E2E coverage with Playwright.

### Upgrade Notes
- Rebuild gateway containers to pick up the new watchdog download logic.
- Review `docs/PRODUCTION_READINESS.md` for production defaults.
