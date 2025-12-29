# Deployment

This guide covers common deployment options and operational notes.

## Docker Compose (recommended)

```bash
cp .env.example .env
# Edit .env for credentials and settings

docker-compose up -d
```

## Docker Run (single host)

```bash
docker network create docker-faas-net

docker run -d   --name docker-faas-gateway   -p 8080:8080 -p 9090:9090   -v /var/run/docker.sock:/var/run/docker.sock   -v faas-data:/data   --network docker-faas-net   ghcr.io/docker-faas/docker-faas:latest
```

## Configuration

All settings are environment variables. See [CONFIGURATION.md](CONFIGURATION.md).

Key production settings:
- Change `AUTH_PASSWORD`
- Set `REQUIRE_AUTH_FOR_FUNCTIONS` for OpenFaaS compatibility
- Set `DEBUG_BIND_ADDRESS=127.0.0.1` unless you need remote debugging
- Configure CORS via `CORS_ALLOWED_ORIGINS`

## TLS / Reverse Proxy

Deploy behind a reverse proxy (nginx/traefik/caddy) for TLS termination and rate limiting. Keep gateway on an internal network and expose only the proxy.

## Persistence and Backups

Database lives under `/data`. Use the scripts:
- `scripts/backup-db.sh` / `scripts/backup-db.ps1`
- `scripts/restore-db.sh` / `scripts/restore-db.ps1`

## Health and Metrics

- Health: `GET /healthz`
- Metrics: `GET /metrics` on the metrics port (default `9090`)
- Docker healthchecks are enabled in `docker-compose.yml` and `Dockerfile`

## Security Notes

- Docker socket mount gives full Docker control. Use a Docker API proxy if needed.
- Keep debug ports bound to localhost unless you explicitly need remote debugging.
