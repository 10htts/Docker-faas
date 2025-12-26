# Docker FaaS v2 Enhancements

This document covers the v2 enhancements implemented to make Docker FaaS production-ready with enhanced security and debugging capabilities.

## Overview

Three major enhancements have been implemented:

1. **✅ Secrets Management** - Secure secret handling compatible with OpenFaaS
2. **✅ Security Hardening** - Strong container isolation and privilege controls
3. **✅ Debug Mode** - Developer-friendly debugging with automatic port mapping

---

## 1. Secrets Management

### Features

- **File-based storage** at `/var/openfaas/secrets`
- **Read-only bind mounts** into function containers
- **Pre-deployment validation** prevents runtime errors
- **Base64 auto-detection** for encoded values
- **Thread-safe operations** with mutex protection
- **API management** via REST endpoints

### Usage

#### Create a Secret

```bash
curl -X POST http://localhost:8080/system/secrets \
  -u admin:admin \
  -H "Content-Type: application/json" \
  -d '{
    "name": "api-key",
    "value": "sk-123456789"
  }'
```

#### Deploy Function with Secrets

```yaml
functions:
  secure-app:
    image: my-app:latest
    secrets:
      - api-key
      - db-password
      - jwt-secret
```

#### Access Secrets in Function

```python
def read_secret(name):
    with open(f'/var/openfaas/secrets/{name}', 'r') as f:
        return f.read().strip()

api_key = read_secret('api-key')
```

### API Endpoints

- `POST /system/secrets` - Create secret
- `GET /system/secrets` - List all secrets
- `GET /system/secrets/{name}` - Check if secret exists
- `PUT /system/secrets` - Update secret
- `DELETE /system/secrets?name={name}` - Delete secret

### Security

- Secrets stored with **0400 permissions** (owner read-only)
- Mounted **read-only** into containers at `/var/openfaas/secrets/<name>`
- **No value exposure** via API (only names returned)
- **Validation before deployment** ensures all secrets exist

### Documentation

- Full guide: [docs/SECRETS.md](SECRETS.md)
- Implementation: [SECRETS_IMPLEMENTATION.md](../SECRETS_IMPLEMENTATION.md)

---

## 2. Security Hardening

### Features Implemented

#### Capability Dropping

All function containers now drop **all kernel capabilities** by default:

```go
SecurityOpt: []string{"no-new-privileges:true"}
CapDrop: []string{"ALL"}
```

This prevents:
- Privilege escalation attacks
- Container breakout attempts
- Kernel exploitation
- Unauthorized system calls

#### No New Privileges

The `no-new-privileges:true` flag ensures:
- Processes cannot gain additional privileges
- SetUID/SetGID bits are ignored
- Prevents privilege escalation via execve()
- Compatible with Docker and containerd

#### Network Isolation

Docker FaaS network configured with:
- **Per-function networks** derived from `FUNCTIONS_NETWORK`
- Functions cannot communicate directly with each other
- Gateway connects to each function network for routing
- Enhanced network segmentation

### Security Configuration

```yaml
# docker-compose.yml
networks:
  docker-faas-net:
    name: docker-faas-net
    driver: bridge
```

### Container Security Profile

Every function container runs with:

```json
{
  "CapDrop": ["ALL"],
  "SecurityOpt": ["no-new-privileges:true"],
  "ReadonlyRootfs": true,  // When specified
  "NetworkMode": "docker-faas-net-<service>"
}
```

### Verification

Check container security settings:

```bash
# Inspect a function container
docker inspect <container-id> | jq '.[0].HostConfig.CapDrop'
# Output: ["ALL"]

docker inspect <container-id> | jq '.[0].HostConfig.SecurityOpt'
# Output: ["no-new-privileges:true"]
```

### Security Benefits

| Feature | Before | After | Impact |
|---------|--------|-------|--------|
| Capabilities | All granted | All dropped | ✅ Prevents privilege escalation |
| New privileges | Allowed | Blocked | ✅ Prevents SetUID attacks |
| Network isolation | Shared network | Per-function networks | ✅ Container isolation |
| Root filesystem | Read-write | Read-only (optional) | ✅ Immutable containers |

---

## 3. Debug Mode

### Features

- **Automatic port mapping** for debug ports
- **Secure localhost binding** by default (127.0.0.1)
- **No timeout enforcement** in debug mode
- **Enhanced logging** for debugging
- **Persistent debug state** in database
- **Per-function debug control**
- **Configurable bind address** via DEBUG_BIND_ADDRESS environment variable

### Port Mappings

When `debug: true` is set, the following ports are automatically mapped:

| Language/Tool | Internal Port | Description |
|---------------|---------------|-------------|
| Go (Delve) | 40000 | Go debugger |
| Python (debugpy) | 5678 | Python debugger |
| Node.js (inspector) | 9229 | Node.js inspector |
| Java (JDWP) | 5005 | Java debug |

Ports are mapped to random available host ports to avoid conflicts.

**Security**: By default, debug ports are bound to `127.0.0.1` (localhost only) for security. To allow remote debugging (NOT recommended in production), set `DEBUG_BIND_ADDRESS=0.0.0.0`.

### Usage

#### Enable Debug Mode via API

```bash
curl -X POST http://localhost:8080/system/functions \
  -u admin:admin \
  -H "Content-Type: application/json" \
  -d '{
    "service": "my-debug-func",
    "image": "my-func:debug",
    "debug": true,
    "envVars": {
      "fprocess": "python handler.py"
    }
  }'
```

#### Enable Debug Mode via stack.yml

```yaml
functions:
  debug-app:
    image: my-app:debug
    debug: true
    environment:
      fprocess: "dlv debug --headless --listen=:40000 --api-version=2"
```

#### Find Debug Port Mapping

```bash
# List function status
curl http://localhost:8080/system/functions -u admin:admin | jq

# Or inspect container
docker port <container-name>
# Output: 40000/tcp -> 0.0.0.0:32771
```

### Connect Debugger

#### Go (Delve)

```bash
# Start function in debug mode
faas-cli deploy \
  --image my-go-func:debug \
  --name go-debug \
  --env fprocess="dlv debug --headless --listen=:40000 --api-version=2" \
  --annotation debug=true

# Find mapped port
docker port go-debug-0
# Output: 40000/tcp -> 0.0.0.0:32771

# Connect debugger
dlv connect localhost:32771
```

#### Python (debugpy)

```bash
# Python function with debugpy
faas-cli deploy \
  --image my-python-func:debug \
  --name python-debug \
  --env fprocess="python -m debugpy --listen 0.0.0.0:5678 --wait-for-client handler.py" \
  --annotation debug=true

# Find port
docker port python-debug-0

# Connect VS Code
# launch.json:
{
  "type": "python",
  "request": "attach",
  "connect": {
    "host": "localhost",
    "port": 5678
  }
}
```

#### Node.js (Inspector)

```bash
# Node function with inspector
faas-cli deploy \
  --image my-node-func:debug \
  --name node-debug \
  --env fprocess="node --inspect=0.0.0.0:9229 index.js" \
  --annotation debug=true

# Connect Chrome DevTools
# Navigate to: chrome://inspect
# Add localhost:<mapped-port>
```

### Debug Mode Behavior

When `debug: true`:

1. **Port Mapping**: Debug ports automatically mapped to host
2. **No Timeouts**: Execution timeouts disabled
3. **Persistent**: Debug flag stored in database
4. **Per-Function**: Each function can be debugged independently
5. **Secure Binding**: Ports bound to localhost by default

### Security Considerations

**Default Behavior (Secure)**:
- Debug ports bound to `127.0.0.1` (localhost only)
- Only accessible from the Docker host machine
- Debuggers cannot be accessed remotely
- Safe for development environments

**Remote Debugging (Insecure)**:
```bash
# NOT RECOMMENDED - Only use in isolated development environments
DEBUG_BIND_ADDRESS=0.0.0.0
```

**Warning**: Setting `DEBUG_BIND_ADDRESS=0.0.0.0` exposes debug ports on all network interfaces. This is a security risk as it allows remote code execution through debugger access. Only use in trusted, isolated networks.

### Container Configuration

Debug-enabled containers get:

```go
// Port bindings for debug ports (secure default: localhost only)
PortBindings: map[nat.Port][]nat.PortBinding{
    "40000/tcp": {{HostIP: "127.0.0.1", HostPort: ""}},  // Random port, localhost only
    "5678/tcp": {{HostIP: "127.0.0.1", HostPort: ""}},
    "9229/tcp": {{HostIP: "127.0.0.1", HostPort: ""}},
}

// Exposed ports
ExposedPorts: nat.PortSet{
    "40000/tcp": struct{}{},
    "5678/tcp": struct{}{},
    "9229/tcp": struct{}{},
}
```

### Verification

```bash
# Deploy debug function
curl -X POST http://localhost:8080/system/functions \
  -u admin:admin \
  -H "Content-Type: application/json" \
  -d '{
    "service": "debug-test",
    "image": "golang:latest",
    "debug": true,
    "envVars": {"fprocess": "sleep 3600"}
  }'

# Check port mappings
docker port debug-test-0
# Output:
# 40000/tcp -> 0.0.0.0:32771
# 5678/tcp -> 0.0.0.0:32772
# 9229/tcp -> 0.0.0.0:32773
```

---

## Testing All Enhancements

### Test Secrets

```bash
chmod +x tests/e2e/test-secrets.sh
./tests/e2e/test-secrets.sh
```

### Test Security Hardening

```bash
# Deploy function
faas-cli deploy --image alpine:latest --name security-test --env fprocess="cat"

# Check capabilities
docker inspect security-test-0 | jq '.[0].HostConfig.CapDrop'
# Should show: ["ALL"]

docker inspect security-test-0 | jq '.[0].HostConfig.SecurityOpt'
# Should show: ["no-new-privileges:true"]

# Try privilege escalation (should fail)
docker exec security-test-0 su - || echo "Privilege escalation blocked ✓"
```

### Test Debug Mode

```bash
# Deploy with debug
curl -X POST http://localhost:8080/system/functions \
  -u admin:admin \
  -H "Content-Type: application/json" \
  -d '{
    "service": "debug-test",
    "image": "golang:latest",
    "debug": true,
    "envVars": {"fprocess": "sleep 3600"}
  }'

# Verify ports
docker port debug-test-0 | grep 40000
# Should show: 40000/tcp -> 0.0.0.0:<random-port>
```

---

## Complete Feature Matrix

| Feature | v1.0 | v2.0 | OpenFaaS Compatible |
|---------|------|------|---------------------|
| **Core** |
| Function deployment | ✅ | ✅ | ✅ |
| Function invocation | ✅ | ✅ | ✅ |
| Scaling | ✅ | ✅ | ✅ |
| Load balancing | ✅ | ✅ | ✅ |
| Metrics | ✅ | ✅ | ✅ |
| **Security** |
| Basic auth | ✅ | ✅ | ✅ |
| Secrets management | ❌ | ✅ | ✅ |
| Capability dropping | ❌ | ✅ | ⚠️ Enhanced |
| No-new-privileges | ❌ | ✅ | ⚠️ Enhanced |
| Network isolation | Partial | ✅ | ⚠️ Enhanced |
| Read-only root | Optional | ✅ | ✅ |
| **Development** |
| Debug mode | ❌ | ✅ | ⚠️ Enhanced |
| Port mapping | ❌ | ✅ | ⚠️ Enhanced |
| Hot reload | ❌ | ❌ | Planned |
| **Operations** |
| Health checks | ✅ | ✅ | ✅ |
| Logging | ✅ | ✅ | ✅ |
| Resource limits | ✅ | ✅ | ✅ |

Legend:
- ✅ Fully implemented
- ⚠️ Enhanced beyond OpenFaaS
- ❌ Not implemented

---

## Production Deployment Checklist

### Security Hardening

- [x] Capability dropping enabled (CapDrop: ALL)
- [x] No-new-privileges enforced
- [x] Inter-container communication disabled
- [x] Secrets mounted read-only
- [x] TLS configured via reverse proxy
- [x] Authentication enabled
- [ ] Audit logging configured
- [ ] Rate limiting configured (via reverse proxy)

### Secrets Management

- [x] Secret storage directory created (`/var/openfaas/secrets`)
- [x] Proper file permissions (0700 directory, 0400 files)
- [x] Secrets validated before deployment
- [x] No secrets in environment variables
- [ ] Secret rotation policy defined
- [ ] Backup strategy for secrets

### Debug Mode

- [x] Debug mode disabled in production
- [x] Debug ports not exposed externally
- [ ] Separate debug environment configured
- [ ] Debug access restricted to authorized users

---

## Migration Guide

### From v1.0 to v2.0

1. **Update Docker Compose**:
   ```bash
   docker-compose down
   docker-compose pull
   docker-compose up -d
   ```

2. **Migrate Secrets**:
   ```bash
   # Create secrets from environment variables
   for func in $(faas-cli list --quiet); do
     # Extract env vars from old deployment
     # Create secrets
     # Redeploy with secrets
   done
   ```

3. **Verify Security**:
   ```bash
   # Check all containers have hardening
   docker ps --filter "label=com.docker-faas.type=function" -q | \
     xargs -I {} docker inspect {} | \
     jq '.[].HostConfig | {CapDrop, SecurityOpt}'
   ```

---

## Best Practices

### Secrets

1. **Never commit secrets** to version control
2. **Rotate secrets regularly** (at least quarterly)
3. **Use separate secrets** per environment (dev/staging/prod)
4. **Validate secrets exist** before deployment
5. **Audit secret access** via logs

### Security

1. **Run containers as non-root** user when possible
2. **Enable read-only root** filesystem for immutable functions
3. **Limit resource usage** with CPU/memory limits
4. **Monitor security events** in logs
5. **Regular security audits** of deployed functions

### Debug Mode

1. **Never enable debug in production**
2. **Use separate debug environment**
3. **Restrict debug port access** to VPN/internal network
4. **Disable debug** before promoting to production
5. **Remove debug images** from production registries

---

## Troubleshooting

### Secrets Not Mounting

**Symptom**: Function can't read `/var/openfaas/secrets/secret-name`

**Solution**:
```bash
# Verify secret exists
curl http://localhost:8080/system/secrets -u admin:admin

# Check function has secret declared
curl http://localhost:8080/system/functions -u admin:admin | jq '.[] | select(.name=="my-func")'

# Redeploy function
faas-cli deploy -f stack.yml --update=true
```

### Debug Port Not Accessible

**Symptom**: Cannot connect debugger to function

**Solution**:
```bash
# Verify debug enabled
curl http://localhost:8080/system/functions -u admin:admin | \
  jq '.[] | select(.name=="my-func") | .annotations.debug'

# Find mapped port
docker port my-func-0

# Test connectivity
nc -zv localhost <mapped-port>
```

### Container Fails to Start (Capability Issues)

**Symptom**: Function container exits immediately

**Solution**:
```bash
# Check logs
docker logs <container-id>

# If capability-related, may need specific capability
# Add to deployment (use sparingly):
# "capabilities": ["NET_BIND_SERVICE"]
```

---

## See Also

- [Secrets Management Guide](SECRETS.md)
- [Security Best Practices](DEPLOYMENT.md#security-hardening)
- [Debug Mode Tutorial](GETTING_STARTED.md#debugging-functions)
- [API Reference](API.md)
