# Production Readiness

This checklist summarizes the production-readiness items implemented and the operational tasks you should verify before launch.

## Minimum Checklist

- [x] Database migrations on startup
- [x] Secrets management and validation
- [x] Debug ports bound to localhost by default
- [x] CapDrop: ALL and no-new-privileges
- [x] Per-function networks and cleanup
- [x] Auth rate limiting
- [x] Health checks: DB, Docker, network
- [x] Metrics exposed on `/metrics`
- [x] Backups and restore scripts

## Operational Tasks

- Set strong `AUTH_PASSWORD`
- Configure TLS via reverse proxy
- Confirm `DEBUG_BIND_ADDRESS` is not `0.0.0.0`
- Set `CORS_ALLOWED_ORIGINS` for your UI origin
- Run E2E tests before production

## Tests

Run all tests:
```bash
./scripts/run-tests.sh
```

Key scripts:
- `tests/e2e/openfaas-compatibility-test.sh`
- `tests/e2e/test-faas-cli-workflow.sh`
- `tests/e2e/test-security.sh`
- `tests/e2e/test-network-isolation.sh`
- `tests/e2e/test-debug-mode.sh`
- `tests/e2e/test-secrets.sh`
- `tests/e2e/test-upgrade.sh`

## References

- [Configuration](CONFIGURATION.md)
- [Deployment](DEPLOYMENT.md)
- [Architecture](ARCHITECTURE.md)
- [Monitoring alerts](monitoring/prometheus-alerts.yml)
