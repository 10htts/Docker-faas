# End-to-End Testing

This directory contains end-to-end tests for Docker FaaS, including OpenFaaS compatibility testing.

## OpenFaaS Compatibility Test

The `openfaas-compatibility-test.sh` script performs comprehensive testing to ensure Docker FaaS behaves identically to OpenFaaS.

### Prerequisites

1. Docker FaaS running (via docker-compose)
2. `faas-cli` installed
3. `curl` installed
4. `jq` installed (optional but recommended)

### Running the Test

```bash
# Start Docker FaaS
docker-compose up -d

# Wait for it to be ready
curl http://localhost:8080/healthz

# Run the compatibility test
chmod +x tests/e2e/openfaas-compatibility-test.sh
./tests/e2e/openfaas-compatibility-test.sh
```

### What Gets Tested

The test suite covers **25 comprehensive tests**:

#### Core Functionality
1. ✅ Gateway health check
2. ✅ System info endpoint
3. ✅ faas-cli login compatibility

#### Function Deployment
4. ✅ Deploy function using faas-cli
5. ✅ Deploy with environment variables
6. ✅ Deploy with labels
7. ✅ Deploy from stack.yml file
8. ✅ Update existing function

#### Function Invocation
9. ✅ Invoke via faas-cli
10. ✅ Invoke via HTTP POST
11. ✅ Invoke with JSON payload
12. ✅ Multiple invocations (load balancing test)
13. ✅ Concurrent invocations (stress test)
14. ✅ Different HTTP methods (GET, POST, PUT, DELETE)
15. ✅ Large payload handling (1MB+)

#### Function Management
16. ✅ List functions
17. ✅ Scale function up
18. ✅ Scale function down
19. ✅ Function logs retrieval
20. ✅ Remove function
21. ✅ Remove from stack file

#### Features & Configuration
22. ✅ Environment variables in functions
23. ✅ Invoke function from stack deployment
24. ✅ Authentication enforcement
25. ✅ Prometheus metrics endpoint

### Expected Output

```
========================================
Docker FaaS - OpenFaaS Compatibility Test
========================================

Checking prerequisites...
✓ faas-cli found (0.16.18)
✓ curl found
✓ jq found

TEST 1: Gateway health check (/healthz)
✓ PASS

TEST 2: System info endpoint (/system/info)
✓ PASS

...

========================================
Test Summary
========================================
Total Tests:  25
Passed:       25
Failed:       0

Success Rate: 100%

========================================
🎉 ALL TESTS PASSED!
Docker FaaS is fully compatible with OpenFaaS!
========================================
```

### Test Details

#### Deployment Tests
- Verifies faas-cli can deploy functions
- Tests stack.yml file deployment
- Validates environment variable injection
- Confirms label support

#### Invocation Tests
- Tests synchronous invocations
- Validates payload handling
- Verifies HTTP method support
- Stress tests concurrent requests

#### Scaling Tests
- Scales up to multiple replicas
- Validates load balancing across replicas
- Tests scaling down

#### Management Tests
- List functions via API and faas-cli
- Retrieve function logs
- Update function configurations
- Remove functions cleanly

### Troubleshooting

If tests fail:

1. **Check gateway is running**:
   ```bash
   curl http://localhost:8080/healthz
   ```

2. **Verify authentication**:
   ```bash
   faas-cli login --gateway http://localhost:8080 -u admin -p admin
   ```

3. **Check Docker containers**:
   ```bash
   docker ps | grep docker-faas
   ```

4. **View gateway logs**:
   ```bash
   docker-compose logs gateway
   ```

5. **Clean up and retry**:
   ```bash
   ./scripts/cleanup.sh
   docker-compose up -d
   sleep 5
   ./tests/e2e/openfaas-compatibility-test.sh
   ```

### CI Integration

This test can be run in CI/CD pipelines:

```yaml
- name: Run E2E Tests
  run: |
    docker-compose up -d
    sleep 10
    ./tests/e2e/openfaas-compatibility-test.sh
```

### Manual Verification

You can also manually verify compatibility:

```bash
# Deploy a function
faas-cli deploy --image functions/alpine:latest --name test --fprocess cat

# Invoke it
echo "Hello" | faas-cli invoke test

# Scale it
faas-cli scale test --replicas 3

# Check it
faas-cli list

# Remove it
faas-cli remove test
```

All commands should work identically to OpenFaaS.

## Additional E2E Tests

These scripts cover production readiness scenarios.

### Available Tests
- `test-secrets.sh` - secrets create/update/delete and mount verification
- `test-security.sh` - auth enforcement, CapDrop, no-new-privileges, read-only secrets
- `test-network-isolation.sh` - per-function network isolation and gateway attachment
- `test-debug-mode.sh` - debug port binding and mapping
- `test-faas-cli-workflow.sh` - faas-cli login, deploy, invoke, scale, remove
- `test-upgrade.sh` - database migration upgrade path (requires sqlite3 and gateway image)

### Running the Tests

```bash
chmod +x tests/e2e/test-*.sh
./tests/e2e/test-security.sh
./tests/e2e/test-network-isolation.sh
./tests/e2e/test-debug-mode.sh
./tests/e2e/test-upgrade.sh
```

### Notes
- `test-upgrade.sh` requires `sqlite3` and a local gateway image (default: `docker-faas/gateway:latest`).
- Set `GATEWAY_IMAGE` to override the image used by the upgrade test.
