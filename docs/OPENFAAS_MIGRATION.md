# Migrating from docker-faas to OpenFaaS

**Document Version**: 1.0
**Last Updated**: 2026-01-20

---

## Overview

This guide helps you migrate from docker-faas (local development) to OpenFaaS (production deployment on Kubernetes).

**Good News**: docker-faas is designed to be **100% compatible** with OpenFaaS. The same function definitions work on both platforms with minimal or no changes.

---

## Table of Contents

1. [Compatibility Status](#compatibility-status)
2. [Pre-Migration Checklist](#pre-migration-checklist)
3. [Migration Steps](#migration-steps)
4. [Configuration Changes](#configuration-changes)
5. [Authentication Changes](#authentication-changes)
6. [Resource Format Compatibility](#resource-format-compatibility)
7. [Testing the Migration](#testing-the-migration)
8. [Troubleshooting](#troubleshooting)

---

## Compatibility Status

### ✅ 100% Compatible

- **Function definitions** (`stack.yml`)
- **Docker images** (same images work on both platforms)
- **Environment variables**
- **Secrets** (both use `/var/openfaas/secrets/{name}`)
- **Resource limits** (both Docker and Kubernetes formats supported)
- **of-watchdog** (same version on both)
- **faas-cli** (same tool for both platforms)

### ⚠️ Different (By Design)

| Feature | docker-faas | OpenFaaS (K8s) |
|---------|-------------|----------------|
| **Orchestration** | Docker API | Kubernetes API |
| **State storage** | SQLite | etcd/K8s |
| **Scaling** | Manual containers | K8s HPA |
| **Service discovery** | Docker DNS | K8s services |
| **Use case** | Local dev | Production |

---

## Pre-Migration Checklist

Before migrating, verify:

- [ ] All functions are defined in `stack.yml` files
- [ ] Functions use standard OpenFaaS watchdog
- [ ] Resource limits use Kubernetes format (`256Mi`, `500m`) or Docker format (`256m`, `0.5`)
- [ ] Secrets are documented and exportable
- [ ] Environment variables are documented
- [ ] Functions have been tested locally with docker-faas
- [ ] You have access to a Kubernetes cluster with OpenFaaS installed
- [ ] You have `kubectl` and `faas-cli` installed

---

## Migration Steps

### Step 1: Export Function Definitions

Your `stack.yml` files should work as-is on OpenFaaS. Verify the format:

**Example `stack.yml`**:
```yaml
version: 1.0
provider:
  name: openfaas
  gateway: http://localhost:8080  # Will be changed for production

functions:
  my-function:
    image: my-org/my-function:latest
    environment:
      ENVIRONMENT: production
    labels:
      com.openfaas.scale.min: "1"
      com.openfaas.scale.max: "10"
    limits:
      memory: 256Mi  # Kubernetes format
      cpu: 500m      # Kubernetes format
    secrets:
      - db-password
```

**Compatibility Check**:
```bash
# Test locally with docker-faas
faas-cli deploy -f stack.yml --gateway http://localhost:8080

# Verify function works
faas-cli invoke my-function --gateway http://localhost:8080 < test-input.json
```

### Step 2: Export Secrets

#### From docker-faas

Secrets are stored in:
- **Default location**: `~/.docker-faas/secrets/`
- **Custom location**: Check `STATE_DB_PATH` in your `.env`

**Export script**:
```bash
#!/bin/bash
# export-secrets.sh

SECRETS_DIR="$HOME/.docker-faas/secrets"
EXPORT_DIR="./secrets-backup"

mkdir -p "$EXPORT_DIR"

for secret_file in "$SECRETS_DIR"/*; do
    secret_name=$(basename "$secret_file")
    echo "Exporting secret: $secret_name"
    cp "$secret_file" "$EXPORT_DIR/$secret_name"
done

echo "Secrets exported to: $EXPORT_DIR"
```

**Run export**:
```bash
chmod +x export-secrets.sh
./export-secrets.sh
```

#### To OpenFaaS

**Import to OpenFaaS** (requires faas-cli and kubectl):
```bash
#!/bin/bash
# import-secrets.sh

EXPORT_DIR="./secrets-backup"

for secret_file in "$EXPORT_DIR"/*; do
    secret_name=$(basename "$secret_file")
    echo "Creating secret: $secret_name"

    # Using faas-cli (recommended)
    cat "$secret_file" | faas-cli secret create "$secret_name"

    # Or using kubectl directly
    # kubectl create secret generic "$secret_name" \
    #   --from-file="$secret_name=$secret_file" \
    #   --namespace openfaas-fn
done

echo "Secrets imported to OpenFaaS"
```

**Run import**:
```bash
chmod +x import-secrets.sh
./import-secrets.sh
```

### Step 3: Update Gateway URL

Update your `stack.yml` for production:

```yaml
version: 1.0
provider:
  name: openfaas
  gateway: https://gateway.production.example.com  # Production gateway

functions:
  my-function:
    # ... rest of configuration
```

Or use environment variable (recommended):
```bash
export OPENFAAS_URL=https://gateway.production.example.com
```

### Step 4: Authenticate to OpenFaaS

#### Basic Auth

```bash
# Set credentials
export OPENFAAS_URL=https://gateway.production.example.com
export OPENFAAS_USERNAME=admin

# Get password from cluster
PASSWORD=$(kubectl get secret -n openfaas basic-auth -o jsonpath="{.data.basic-auth-password}" | base64 --decode)

# Login
echo -n $PASSWORD | faas-cli login --username admin --password-stdin
```

#### Token Auth (OpenFaaS Pro)

```bash
export OPENFAAS_URL=https://gateway.production.example.com
export OPENFAAS_TOKEN=<your-token>

faas-cli login --token $OPENFAAS_TOKEN
```

### Step 5: Deploy to OpenFaaS

Deploy your functions:

```bash
# Deploy all functions in stack.yml
faas-cli deploy -f stack.yml

# Or deploy specific function
faas-cli deploy -f stack.yml --filter my-function

# Verify deployment
faas-cli list

# Test function
echo '{"test": "data"}' | faas-cli invoke my-function
```

### Step 6: Update Application Configuration

Update your application to point to the new gateway:

```bash
# Before (docker-faas local)
export OPENFAAS_URL=http://localhost:8080
export OPENFAAS_USERNAME=admin
export OPENFAAS_PASSWORD=admin

# After (OpenFaaS production)
export OPENFAAS_URL=https://gateway.production.example.com
export OPENFAAS_USERNAME=admin
export OPENFAAS_PASSWORD=<from-k8s-secret>
```

---

## Configuration Changes

### Environment Variables

| Purpose | docker-faas (local) | OpenFaaS (production) |
|---------|---------------------|----------------------|
| **Gateway URL** | `OPENFAAS_URL=http://localhost:8080` | `OPENFAAS_URL=https://gateway.prod.com` |
| **Username** | `OPENFAAS_USERNAME=admin` | `OPENFAAS_USERNAME=admin` |
| **Password** | `OPENFAAS_PASSWORD=admin` | `OPENFAAS_PASSWORD=<from-secret>` |
| **Token** | Via `/auth/login` endpoint | `OPENFAAS_TOKEN=<token>` (Pro only) |

### Function Environment Variables

**No changes needed!** Function environment variables work the same way:

```yaml
functions:
  my-function:
    environment:
      DATABASE_URL: postgres://...
      API_KEY: ${API_KEY}  # Can use environment variable substitution
```

---

## Authentication Changes

### docker-faas (Local Development)

**Supports**:
- ✅ HTTP Basic Auth
- ✅ Bearer Token Auth (via `/auth/login`)

**Example**:
```bash
# Basic Auth
faas-cli login --gateway http://localhost:8080 \
  --username admin --password admin

# Token Auth
TOKEN=$(curl -X POST http://localhost:8080/auth/login \
  -d '{"username":"admin","password":"admin"}' | jq -r '.token')

curl -X POST http://localhost:8080/function/my-function \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"test": "data"}'
```

### OpenFaaS (Production)

**Community Edition**:
- ✅ HTTP Basic Auth
- ❌ Token Auth (not available)

**Pro Edition**:
- ✅ HTTP Basic Auth
- ✅ OAuth2/OIDC
- ✅ Token Auth

**Migration Path**:
1. If using Basic Auth: **No changes needed**
2. If using Token Auth: Upgrade to OpenFaaS Pro or switch to Basic Auth

---

## Resource Format Compatibility

### Memory Limits

**Both formats supported** (since docker-faas v2.1.0):

```yaml
functions:
  my-function:
    limits:
      memory: 256Mi  # Kubernetes format (preferred for migration)
      # memory: 256m  # Docker format (also works)
```

| Format | Example | Supported by docker-faas | Supported by OpenFaaS |
|--------|---------|-------------------------|----------------------|
| **Kubernetes** | `256Mi`, `1Gi` | ✅ Yes | ✅ Yes |
| **Docker** | `256m`, `1g` | ✅ Yes | ❌ No |

**Recommendation**: Use Kubernetes format (`Mi`, `Gi`) for seamless migration.

### CPU Limits

**Both formats supported** (since docker-faas v2.1.0):

```yaml
functions:
  my-function:
    limits:
      cpu: 500m  # Kubernetes millicores (preferred for migration)
      # cpu: 0.5  # Docker cores (also works)
```

| Format | Example | Supported by docker-faas | Supported by OpenFaaS |
|--------|---------|-------------------------|----------------------|
| **Kubernetes** | `500m`, `1000m` | ✅ Yes | ✅ Yes |
| **Docker** | `0.5`, `1` | ✅ Yes | ❌ No |

**Recommendation**: Use Kubernetes millicore format (`m`) for seamless migration.

---

## Testing the Migration

### 1. Pre-Migration Test (docker-faas)

```bash
# Test locally
export OPENFAAS_URL=http://localhost:8080
faas-cli login --username admin --password admin

# Deploy
faas-cli deploy -f stack.yml

# Test
echo '{"test": "data"}' | faas-cli invoke my-function

# Record expected output
echo '{"test": "data"}' | faas-cli invoke my-function > expected-output.txt
```

### 2. Post-Migration Test (OpenFaaS)

```bash
# Test on OpenFaaS
export OPENFAAS_URL=https://gateway.production.example.com
faas-cli login --username admin --password-stdin < prod-password.txt

# Deploy
faas-cli deploy -f stack.yml

# Test
echo '{"test": "data"}' | faas-cli invoke my-function > actual-output.txt

# Compare outputs
diff expected-output.txt actual-output.txt
```

### 3. Load Testing

```bash
# Test with hey or ab
hey -n 1000 -c 10 -m POST \
  -d '{"test": "data"}' \
  https://gateway.production.example.com/function/my-function
```

---

## Troubleshooting

### Issue: "Function not found" after deployment

**Cause**: Function may still be deploying.

**Solution**:
```bash
# Check deployment status
kubectl get pods -n openfaas-fn

# Check function status
faas-cli describe my-function

# Wait for ready status
kubectl wait --for=condition=ready pod -l faas_function=my-function -n openfaas-fn --timeout=60s
```

### Issue: "Secret not found"

**Cause**: Secrets not created in OpenFaaS namespace.

**Solution**:
```bash
# Check secrets in openfaas-fn namespace
kubectl get secrets -n openfaas-fn

# Create missing secret
echo -n "secret-value" | faas-cli secret create my-secret

# Or with kubectl
kubectl create secret generic my-secret \
  --from-literal=my-secret=secret-value \
  --namespace openfaas-fn
```

### Issue: "Memory limit exceeded"

**Cause**: Kubernetes enforces memory limits more strictly than Docker.

**Solution**:
```yaml
# Increase memory limits in stack.yml
functions:
  my-function:
    limits:
      memory: 512Mi  # Increased from 256Mi
```

### Issue: "Authentication failed"

**Cause**: Incorrect credentials or token.

**Solution**:
```bash
# Get password from Kubernetes
PASSWORD=$(kubectl get secret -n openfaas basic-auth \
  -o jsonpath="{.data.basic-auth-password}" | base64 --decode)

# Re-login
echo -n $PASSWORD | faas-cli login --username admin --password-stdin
```

---

## Best Practices

### 1. Use Kubernetes Resource Formats

Always use Kubernetes formats in `stack.yml`:
```yaml
limits:
  memory: 256Mi  # Not 256m
  cpu: 500m      # Not 0.5
```

### 2. Test Locally First

```bash
# Always test with docker-faas before deploying to OpenFaaS
faas-cli deploy -f stack.yml --gateway http://localhost:8080
# ... test thoroughly ...
# Then deploy to production
faas-cli deploy -f stack.yml --gateway https://prod.example.com
```

### 3. Use Environment Variables for Gateway URL

```bash
# Don't hardcode in stack.yml
export OPENFAAS_URL=https://gateway.production.example.com
faas-cli deploy -f stack.yml  # Uses OPENFAAS_URL automatically
```

### 4. Version Your Functions

```yaml
functions:
  my-function:
    image: my-org/my-function:v1.2.3  # Explicit version tag
    # NOT: my-org/my-function:latest
```

### 5. Document Secrets

Create a secrets manifest:
```yaml
# secrets.yml
secrets:
  - name: db-password
    description: PostgreSQL password
    required: true
  - name: api-key
    description: Third-party API key
    required: true
```

---

## Migration Checklist

- [ ] Export all function definitions
- [ ] Export all secrets
- [ ] Update resource limits to Kubernetes format
- [ ] Test functions locally with docker-faas
- [ ] Set up OpenFaaS credentials
- [ ] Import secrets to OpenFaaS
- [ ] Deploy functions to OpenFaaS
- [ ] Verify functions work correctly
- [ ] Update application configuration
- [ ] Perform load testing
- [ ] Set up monitoring and alerts
- [ ] Document any changes made
- [ ] Update deployment documentation

---

## Next Steps

After successful migration:

1. **Set up CI/CD**: Automate deployments to OpenFaaS
2. **Configure autoscaling**: Use Kubernetes HPA
3. **Set up monitoring**: Prometheus, Grafana
4. **Configure ingress**: TLS certificates, domain names
5. **Implement backup**: Function definitions, secrets
6. **Document runbooks**: Deployment, rollback procedures

---

## Related Documents

- [OpenFaaS Contracts](OPENFAAS_CONTRACTS.md) - Understanding request/response formats
- [API Reference](API.md) - REST API documentation
- [Configuration](CONFIGURATION.md) - Environment variables
- [Secrets Management](SECRETS.md) - Working with secrets

---

## Support

If you encounter issues during migration:

1. Check [OpenFaaS documentation](https://docs.openfaas.com/)
2. Review [docker-faas GitHub issues](https://github.com/10htts/Docker-faas/issues)
3. Join [OpenFaaS Slack community](https://slack.openfaas.io/)

---

**Document Version**: 1.0
**Last Updated**: 2026-01-20
