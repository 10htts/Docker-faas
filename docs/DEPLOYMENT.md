# Deployment Guide

This guide covers various deployment scenarios for Docker FaaS.

## Table of Contents

- [Development Setup](#development-setup)
- [Production Deployment](#production-deployment)
- [Reverse Proxy Setup](#reverse-proxy-setup)
- [Security Hardening](#security-hardening)
- [Monitoring Setup](#monitoring-setup)
- [Backup and Recovery](#backup-and-recovery)

## Development Setup

### Quick Start with Docker Compose

1. Clone the repository:
```bash
git clone https://github.com/docker-faas/docker-faas.git
cd docker-faas
```

2. Start the gateway:
```bash
docker-compose up -d
```

3. Verify it's running:
```bash
curl http://localhost:8080/healthz
```

4. Login with faas-cli:
```bash
faas-cli login --gateway http://localhost:8080 -u admin -p admin
```

### Building from Source

1. Prerequisites:
   - Go 1.21 or later
   - Docker
   - Make

2. Build:
```bash
make install-deps
make build
```

3. Run:
```bash
./bin/docker-faas-gateway
```

## Production Deployment

### Using Docker (Single Host)

1. Create gateway network (functions get per-function networks automatically):
```bash
docker network create docker-faas-net
```

2. Run gateway:
```bash
docker run -d \
  --name docker-faas-gateway \
  --restart unless-stopped \
  -p 8080:8080 \
  -p 9090:9090 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v /data/docker-faas:/data \
  --network docker-faas-net \
  -e AUTH_ENABLED=true \
  -e AUTH_USER=admin \
  -e AUTH_PASSWORD=$(openssl rand -base64 32) \
  -e LOG_LEVEL=info \
  ghcr.io/docker-faas/docker-faas:latest
```

### Using Docker Compose (Production)

Create `docker-compose.prod.yml`:

```yaml
version: '3.8'

services:
  gateway:
    image: ghcr.io/docker-faas/docker-faas:latest
    container_name: docker-faas-gateway
    restart: unless-stopped
    ports:
      - "127.0.0.1:8080:8080"  # Only expose to localhost
      - "127.0.0.1:9090:9090"
    environment:
      - GATEWAY_PORT=8080
      - METRICS_PORT=9090
      - FUNCTIONS_NETWORK=docker-faas-net
      - GATEWAY_CONTAINER_NAME=docker-faas-gateway
      - AUTH_ENABLED=true
      - AUTH_USER=${FAAS_USER}
      - AUTH_PASSWORD=${FAAS_PASSWORD}
      - LOG_LEVEL=warn
      - STATE_DB_PATH=/data/docker-faas.db
      - READ_TIMEOUT=120s
      - WRITE_TIMEOUT=120s
      - EXEC_TIMEOUT=300s
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - faas-data:/data
    networks:
      - docker-faas-net
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"

networks:
  docker-faas-net:
    name: docker-faas-net
    driver: bridge

volumes:
  faas-data:
    name: docker-faas-data
```

Create `.env` file:
```bash
FAAS_USER=admin
FAAS_PASSWORD=$(openssl rand -base64 32)
```

Start:
```bash
docker-compose -f docker-compose.prod.yml up -d
```

## Reverse Proxy Setup

### Nginx

Create `/etc/nginx/sites-available/docker-faas`:

```nginx
upstream docker_faas {
    server 127.0.0.1:8080;
}

server {
    listen 80;
    server_name faas.example.com;

    # Redirect to HTTPS
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name faas.example.com;

    # SSL certificates
    ssl_certificate /etc/letsencrypt/live/faas.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/faas.example.com/privkey.pem;

    # SSL configuration
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers on;

    # Rate limiting
    limit_req_zone $binary_remote_addr zone=faas_limit:10m rate=10r/s;
    limit_req zone=faas_limit burst=20 nodelay;

    # Timeouts
    proxy_connect_timeout 300s;
    proxy_send_timeout 300s;
    proxy_read_timeout 300s;

    location / {
        proxy_pass http://docker_faas;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # WebSocket support (if needed)
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

Enable and restart:
```bash
sudo ln -s /etc/nginx/sites-available/docker-faas /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl restart nginx
```

### Caddy

Create `Caddyfile`:

```caddy
faas.example.com {
    reverse_proxy localhost:8080 {
        transport http {
            dial_timeout 300s
            read_timeout 300s
            write_timeout 300s
        }
    }

    # Rate limiting
    rate_limit {
        zone faas {
            rate 10r/s
        }
    }

    # Logging
    log {
        output file /var/log/caddy/faas.log
        format json
    }
}
```

### Traefik

Create `docker-compose.traefik.yml`:

```yaml
version: '3.8'

services:
  traefik:
    image: traefik:v2.10
    command:
      - "--providers.docker=true"
      - "--entrypoints.web.address=:80"
      - "--entrypoints.websecure.address=:443"
      - "--certificatesresolvers.letsencrypt.acme.email=admin@example.com"
      - "--certificatesresolvers.letsencrypt.acme.storage=/letsencrypt/acme.json"
      - "--certificatesresolvers.letsencrypt.acme.httpchallenge.entrypoint=web"
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - traefik-certs:/letsencrypt
    networks:
      - docker-faas-net

  gateway:
    image: ghcr.io/docker-faas/docker-faas:latest
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.faas.rule=Host(`faas.example.com`)"
      - "traefik.http.routers.faas.entrypoints=websecure"
      - "traefik.http.routers.faas.tls.certresolver=letsencrypt"
      - "traefik.http.services.faas.loadbalancer.server.port=8080"
    # ... rest of gateway config

volumes:
  traefik-certs:

networks:
  docker-faas-net:
    external: true
```

## Security Hardening

### 1. Change Default Credentials

```bash
# Generate strong password
export FAAS_PASSWORD=$(openssl rand -base64 32)

# Update in docker-compose or environment
docker-compose down
# Update docker-compose.yml with new password
docker-compose up -d
```

### 2. Enable TLS

Use a reverse proxy (Nginx/Caddy/Traefik) with Let's Encrypt certificates.

### 3. Restrict Docker Socket Access

Instead of mounting the Docker socket directly, use a proxy:

```bash
# Install docker-socket-proxy
docker run -d \
  --name docker-socket-proxy \
  --privileged \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -e CONTAINERS=1 \
  -e IMAGES=1 \
  -e NETWORKS=1 \
  tecnativa/docker-socket-proxy

# Update gateway to use proxy
docker run -d \
  --name docker-faas-gateway \
  -e DOCKER_HOST=tcp://docker-socket-proxy:2375 \
  ...
```

### 4. Network Isolation

```bash
# Functions run on per-function networks derived from FUNCTIONS_NETWORK
# Gateway is connected to each function network automatically
# Networks are removed on function delete when unused
```

#### Cleanup orphaned networks

Per-function networks are labeled and can be cleaned up safely when unused.

```bash
# Dry-run cleanup (Bash)
DRY_RUN=true ./scripts/cleanup-networks.sh

# Cleanup (Bash)
./scripts/cleanup-networks.sh
```

```powershell
# Dry-run cleanup (PowerShell)
.\scripts\cleanup-networks.ps1 -DryRun

# Cleanup (PowerShell)
.\scripts\cleanup-networks.ps1
```

### 5. Resource Limits

Add resource limits to gateway:

```yaml
services:
  gateway:
    # ...
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 2G
        reservations:
          cpus: '0.5'
          memory: 512M
```

## Monitoring Setup

### Prometheus

Create `prometheus.yml`:

```yaml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: 'docker-faas'
    static_configs:
      - targets: ['localhost:9090']
    basic_auth:
      username: 'admin'
      password: 'your-password'
```

### Alerting

Sample Prometheus alert rules are provided in `docs/monitoring/prometheus-alerts.yml`.

### Grafana Dashboard

Import the Docker FaaS dashboard (ID: coming soon) or create custom:

```json
{
  "dashboard": {
    "title": "Docker FaaS",
    "panels": [
      {
        "title": "Function Invocations",
        "targets": [
          {
            "expr": "rate(function_invocations_total[5m])"
          }
        ]
      }
    ]
  }
}
```

### Logging with ELK Stack

Configure JSON logging:

```yaml
services:
  gateway:
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "5"
        labels: "service=docker-faas"
```

## Backup and Recovery

### Database Backup

```bash
# Backup SQLite database
docker exec docker-faas-gateway \
  sqlite3 /data/docker-faas.db ".backup '/data/backup.db'"

# Copy to host
docker cp docker-faas-gateway:/data/backup.db ./backup-$(date +%Y%m%d).db
```

### Automated Backups

Use the bundled backup scripts:

```bash
./scripts/backup-db.sh
```

```powershell
.\scripts\backup-db.ps1
```

You can set `BACKUP_DIR` and `RETENTION_DAYS` to control backup location and retention.

Legacy example script:

```bash
#!/bin/bash
BACKUP_DIR="/backups/docker-faas"
DATE=$(date +%Y%m%d-%H%M%S)

mkdir -p $BACKUP_DIR

# Backup database
docker exec docker-faas-gateway \
  sqlite3 /data/docker-faas.db ".backup '/data/backup-$DATE.db'"

docker cp docker-faas-gateway:/data/backup-$DATE.db $BACKUP_DIR/

# Cleanup old backups (keep 7 days)
find $BACKUP_DIR -name "backup-*.db" -mtime +7 -delete
```

Add to crontab:
```
0 2 * * * /path/to/backup-db.sh
```

### Recovery

Preferred restore scripts:

```bash
./scripts/restore-db.sh ./backups/backup-<timestamp>.db
```

```powershell
.\scripts\restore-db.ps1 -BackupFile .\backups\backup-<timestamp>.db
```

```bash
# Stop gateway
docker-compose down

# Restore database
docker run --rm -v faas-data:/data -v $(pwd):/backup alpine \
  cp /backup/backup.db /data/docker-faas.db

# Start gateway
docker-compose up -d
```

## Health Checks

### Docker Health Check

Add to Dockerfile:
```dockerfile
HEALTHCHECK --interval=30s --timeout=3s --retries=3 \
  CMD curl -f http://localhost:8080/healthz || exit 1
```

### External Monitoring

Use tools like:
- UptimeRobot
- Pingdom
- StatusCake

Monitor endpoint: `https://faas.example.com/healthz`

## Troubleshooting

### Gateway won't start

```bash
# Check logs
docker-compose logs gateway

# Check Docker socket permissions
ls -l /var/run/docker.sock

# Verify network
docker network inspect docker-faas-net
```

### High memory usage

```bash
# Check container stats
docker stats docker-faas-gateway

# Check for zombie containers
docker ps -a | grep -E "Exited|Dead"

# Cleanup
docker system prune -a
```

### Database locked

```bash
# Stop gateway
docker-compose down

# Check database
docker run --rm -v faas-data:/data alpine \
  sqlite3 /data/docker-faas.db "PRAGMA integrity_check;"

# If corrupted, restore from backup
```
