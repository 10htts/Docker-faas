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

## Production Baseline

Use this baseline as a starting point for a production deployment:

```
AUTH_ENABLED=true
AUTH_PASSWORD=change-this
REQUIRE_AUTH_FOR_FUNCTIONS=true
AUTH_RATE_LIMIT=10
AUTH_RATE_WINDOW=1m
AUTH_TOKEN_TTL=30m
CORS_ALLOWED_ORIGINS=https://your-ui.example.com
DEBUG_BIND_ADDRESS=127.0.0.1
BUILD_HISTORY_LIMIT=100
BUILD_HISTORY_RETENTION=24h
BUILD_OUTPUT_LIMIT=204800
```

Adjust the build history limits to meet your retention requirements and storage constraints.

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
