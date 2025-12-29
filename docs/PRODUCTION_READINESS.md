# Production Readiness Improvements

This document tracks the critical production readiness improvements implemented for Docker FaaS v2.0.

## Overview

Docker FaaS v2.0 addresses key production blockers identified during the transition from development/staging to production deployment.

---

## 1. Database Migration System ✅ COMPLETE

### Problem
Existing v1.0 deployments would break when upgrading to v2.0 because the database schema doesn't include the `debug` column added in v2.0.

### Solution
Implemented a complete migration management system in `pkg/store/migrations.go`:

```go
type Migration struct {
    Version     int
    Description string
    Up          string    // SQL to apply migration
    Down        string    // SQL to rollback migration
}
```

### Features
- **Version tracking** via `schema_migrations` table
- **Ordered application** of migrations
- **Rollback support** for safe downgrades
- **Transaction-based** for atomicity
- **Automatic detection** and application on startup

### Migrations

**Migration 1: Initial Schema (v1.0)**
- Creates `functions` table with core columns
- Applied for fresh installations

**Migration 2: Debug Column (v2.0)**
- Adds `debug BOOLEAN NOT NULL DEFAULT 0` column
- Applied automatically on upgrade from v1.0

### Usage

Migrations are automatically applied when the gateway starts:

```go
migrationManager := NewMigrationManager(s.db, logger)
if err := migrationManager.ApplyMigrations(); err != nil {
    return fmt.Errorf("failed to apply migrations: %w", err)
}
```

### Verification

```bash
# Check current schema version
sqlite3 /data/docker-faas.db "SELECT * FROM schema_migrations ORDER BY version;"

# Should show:
# 1|Initial schema|<timestamp>
# 2|Add debug column for v2.0|<timestamp>
```

### Impact
✅ **Safe upgrades** from v1.0 to v2.0
✅ **Zero downtime** migration
✅ **Rollback capability** if needed
✅ **Future-proof** for v3.0+ schema changes

---

## 2. Debug Port Security ✅ COMPLETE

### Problem
Debug ports were bound to `0.0.0.0` (all network interfaces), exposing debuggers to the network. This allows potential remote code execution through debugger access.

### Solution
Changed default debug port binding to `127.0.0.1` (localhost only) with configurable override.

### Implementation

**Configuration** (`pkg/config/config.go`):
```go
type Config struct {
    // ...
    DebugBindAddress string  // Default: "127.0.0.1"
}

cfg.DebugBindAddress = getEnv("DEBUG_BIND_ADDRESS", "127.0.0.1")
```

**Provider** (`pkg/provider/docker_provider.go`):
```go
if deployment.Debug {
    hostConfig.PortBindings = nat.PortMap{
        "40000/tcp": []nat.PortBinding{{HostIP: p.debugBindAddress, HostPort: "0"}},
        "5678/tcp":  []nat.PortBinding{{HostIP: p.debugBindAddress, HostPort: "0"}},
    }

    // Security warning if exposed on all interfaces
    if p.debugBindAddress == "0.0.0.0" {
        p.logger.Warnf("⚠️  DEBUG MODE: Function %s has debug ports exposed on ALL interfaces (0.0.0.0)", deployment.Service)
        p.logger.Warn("⚠️  This is a security risk in production. Set DEBUG_BIND_ADDRESS=127.0.0.1")
    }
}
```

### Security Benefits

**Default Behavior (Secure)**:
- Debug ports only accessible from Docker host machine
- No remote debugger access possible
- Safe for local development

**Remote Debugging (Opt-in)**:
```bash
# Only use in trusted, isolated development networks
DEBUG_BIND_ADDRESS=0.0.0.0
```

### Configuration

**docker-compose.yml**:
```yaml
environment:
  # Secure default - localhost only
  - DEBUG_BIND_ADDRESS=127.0.0.1
```

**Override for remote debugging** (development only):
```bash
export DEBUG_BIND_ADDRESS=0.0.0.0
docker-compose up -d
```

### Verification

```bash
# Deploy function with debug mode
curl -X POST http://localhost:8080/system/functions \
  -u admin:admin \
  -H "Content-Type: application/json" \
  -d '{
    "service": "debug-test",
    "image": "golang:latest",
    "debug": true
  }'

# Check port bindings
docker port debug-test-0
# Should show: 40000/tcp -> 127.0.0.1:32771 (localhost only)
# NOT:         40000/tcp -> 0.0.0.0:32771 (all interfaces)
```

### Impact
✅ **Secure by default** - no remote debugger access
✅ **Clear warnings** when configured insecurely
✅ **Flexible** - can be overridden for specific use cases
✅ **Documented** - security implications clearly explained

---

## 3. Network Lifecycle Management - COMPLETE

### Implementation
- Per-function networks created on first deployment and labeled for cleanup.
- Gateway auto-connects to each function network for routing.
- Networks cleaned up on function delete when unused.
- Orphaned network cleanup scripts added:
  - `scripts/cleanup-networks.sh`
  - `scripts/cleanup-networks.ps1`

### Usage

```bash
DRY_RUN=true ./scripts/cleanup-networks.sh
./scripts/cleanup-networks.sh
```

```powershell
.\scripts\cleanup-networks.ps1 -DryRun
.\scripts\cleanup-networks.ps1
```

## 4. Operational Hardening - IN PROGRESS

### Completed
- Backup and restore scripts added (`scripts/backup-db.sh`, `scripts/restore-db.sh`, PowerShell equivalents).
- Backup/restore documentation updated in `docs/DEPLOYMENT.md`.
- Sample Prometheus alert rules added (`docs/monitoring/prometheus-alerts.yml`).
- Reverse proxy and rate limiting guidance available in deployment docs.

### Remaining
- Health check metrics enhancements.
- Example Grafana dashboard file.

---

## Testing Requirements

### 1. Smoke/E2E Test ✅ COMPLETE
**Location**: `tests/e2e/openfaas-compatibility-test.sh`

Tests:
- Deploy hello-world function
- Invoke function
- Update function
- Scale function
- Remove function

**Status**: 25 tests passing

### 2. Security Test - COMPLETE
**Location**: `tests/e2e/test-security.sh`

Tests:
- Verify authentication required on /system endpoints
- Verify no-new-privileges on running containers
- Verify CapDrop: ALL on running containers
- Verify secret mounts are read-only

### 3. Network Isolation Test - COMPLETE
**Location**: `tests/e2e/test-network-isolation.sh`

Tests:
- Deploy two functions (A and B)
- Verify function networks contain only gateway + own container
- Verify functions are isolated from each other at network level

### 4. Secrets Test ✅ COMPLETE
**Location**: `tests/e2e/test-secrets.sh`

Tests:
- Create secret via API
- Deploy function with secret
- Verify secret accessible in function
- Verify secret mounted read-only
- Update and delete secrets

**Status**: 9 tests passing

### 5. Debug Test - COMPLETE
**Location**: `tests/e2e/test-debug-mode.sh`

Tests:
- Enable debug mode on function
- Verify debug ports are mapped
- Verify ports bound to DEBUG_BIND_ADDRESS (default 127.0.0.1)

### 6. Upgrade Test - COMPLETE
**Location**: `tests/e2e/test-upgrade.sh`

Tests:
- Start with v1.0 database schema (no debug column)
- Start gateway and apply migrations
- Verify schema_migrations version >= 2

---

## Production Deployment Checklist

### Database
- [x] Migration system implemented
- [x] Automatic migration on startup
- [x] Backup automation script
- [x] Restore procedure documented
- [x] Backup retention policy defined

### Security
- [x] Debug ports bound to localhost by default
- [x] Security warnings on insecure configuration
- [x] Capability dropping (CapDrop: ALL)
- [x] No-new-privileges enforced
- [x] Secrets management implemented
- [x] TLS/HTTPS configuration documented
- [x] Rate limiting strategy documented

### Networking
- [x] Per-function network isolation
- [x] Gateway auto-connect to function networks
- [x] Network lifecycle documentation
- [x] Orphaned network cleanup script
- [x] Network troubleshooting guide

### Operations
- [x] Prometheus metrics endpoint
- [ ] Health check metrics enhanced
- [ ] Example Grafana dashboard
- [x] Example Prometheus alert rules
- [x] Deployment guide with reverse proxy
- [x] Backup/restore documentation

### Testing
- [x] Unit tests (30 tests, >80% coverage)
- [x] E2E compatibility tests (25 tests)
- [x] Secrets workflow tests (9 tests)
- [x] Security hardening tests
- [x] Network isolation tests
- [x] Debug mode tests
- [x] Upgrade migration tests

---

## Summary of Completed Work

### ✅ Database Migration System
- Full migration framework with versioning
- Safe upgrades from v1.0 to v2.0
- Rollback support for safety
- Transaction-based for atomicity

### ✅ Debug Port Security
- Secure default: localhost only (127.0.0.1)
- Configurable via DEBUG_BIND_ADDRESS
- Clear security warnings when misconfigured
- Documentation updated with security guidance

---

## Next Steps

1. **Add health check metrics** for deeper operational visibility.
2. **Publish a Grafana dashboard** for quick monitoring setup.
3. **Run performance testing** under production-like loads.

---

## References

- [Database Migrations](pkg/store/migrations.go)
- [Configuration](pkg/config/config.go)
- [Docker Provider](pkg/provider/docker_provider.go)
- [V2 Enhancements](docs/V2_ENHANCEMENTS.md)
- [Release Notes](RELEASE_NOTES_V2.md)
- [Network Cleanup Script](scripts/cleanup-networks.sh)
- [Backup Script](scripts/backup-db.sh)
- [Restore Script](scripts/restore-db.sh)
- [Alert Rules](docs/monitoring/prometheus-alerts.yml)
