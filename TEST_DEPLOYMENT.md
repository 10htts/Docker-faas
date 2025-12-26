# Test Deployment Guide

This guide will help you perform a complete test deployment to verify Docker FaaS works exactly like OpenFaaS.

## Quick Test Deployment

### Step 1: Start Docker FaaS

```bash
# Start the platform
docker-compose up -d

# Wait for it to be ready (check health)
curl http://localhost:8080/healthz
# Should return: OK

# Check logs
docker-compose logs -f gateway
```

### Step 2: Run Automated Compatibility Tests

```bash
# Run the full E2E test suite (25 tests)
make e2e-test

# Or run directly
chmod +x tests/e2e/openfaas-compatibility-test.sh
./tests/e2e/openfaas-compatibility-test.sh
```

Expected output:
```
========================================
Docker FaaS - OpenFaaS Compatibility Test
========================================

Checking prerequisites...
✓ faas-cli found
✓ curl found
✓ jq found

TEST 1: Gateway health check (/healthz)
✓ PASS

TEST 2: System info endpoint (/system/info)
✓ PASS

... (23 more tests)

========================================
Test Summary
========================================
Total Tests:  25
Passed:       25
Failed:       0

Success Rate: 100%

🎉 ALL TESTS PASSED!
Docker FaaS is fully compatible with OpenFaaS!
========================================
```

## Manual Test Deployment

If you prefer to test manually, follow these steps:

### 1. Login to Gateway

```bash
faas-cli login --gateway http://localhost:8080 --username admin --password admin
```

Expected: `credentials saved for admin http://localhost:8080`

### 2. Deploy a Simple Function

```bash
# Deploy echo function
faas-cli deploy \
  --gateway http://localhost:8080 \
  --image ghcr.io/openfaas/alpine:latest \
  --name echo \
  --env fprocess="cat"
```

Expected: `Deployed. 202 Accepted.`

### 3. List Functions

```bash
faas-cli list --gateway http://localhost:8080
```

Expected output:
```
Function    Invocations    Replicas
echo        0              1
```

### 4. Invoke the Function

```bash
# Using faas-cli
echo "Hello Docker FaaS!" | faas-cli invoke echo --gateway http://localhost:8080
```

Expected: `Hello Docker FaaS!`

```bash
# Using curl
curl -X POST http://localhost:8080/function/echo \
  -u admin:admin \
  -d "Testing via HTTP"
```

Expected: `Testing via HTTP`

### 5. Scale the Function

```bash
# Scale to 3 replicas
faas-cli scale echo --replicas 3 --gateway http://localhost:8080

# Verify
faas-cli list --gateway http://localhost:8080
```

Expected:
```
Function    Invocations    Replicas
echo        X              3
```

### 6. Test Load Balancing

```bash
# Invoke multiple times
for i in {1..10}; do
  echo "Request $i" | faas-cli invoke echo --gateway http://localhost:8080
done
```

All requests should succeed and be distributed across replicas.

### 7. Check Logs

```bash
# View function logs
faas-cli logs echo --gateway http://localhost:8080
```

Should show container logs.

### 8. Deploy from Stack File

Create `test-stack.yml`:
```yaml
version: 1.0
provider:
  name: openfaas
  gateway: http://localhost:8080

functions:
  nodeinfo:
    image: ghcr.io/openfaas/alpine:latest
    environment:
      fprocess: "uname -a"
    labels:
      test: "true"

  env-test:
    image: ghcr.io/openfaas/alpine:latest
    environment:
      fprocess: "env"
      CUSTOM_VAR: "hello"
      ANOTHER_VAR: "world"
```

Deploy the stack:
```bash
faas-cli deploy -f test-stack.yml
```

Test the functions:
```bash
faas-cli invoke nodeinfo --gateway http://localhost:8080
faas-cli invoke env-test --gateway http://localhost:8080
```

### 9. Update a Function

```bash
faas-cli deploy \
  --gateway http://localhost:8080 \
  --image ghcr.io/openfaas/alpine:latest \
  --name echo \
  --env fprocess="cat" \
  --env UPDATED="true" \
  --update=true
```

### 10. Remove Functions

```bash
# Remove individual function
faas-cli remove echo --gateway http://localhost:8080

# Remove from stack
faas-cli remove -f test-stack.yml
```

## Advanced Tests

### Test with Python Function

1. **Build the example function**:
```bash
cd examples/hello-world
docker build -t docker-faas/hello-world:latest .
```

2. **Deploy it**:
```bash
faas-cli deploy \
  --gateway http://localhost:8080 \
  --image docker-faas/hello-world:latest \
  --name hello-world
```

3. **Test it**:
```bash
echo "Docker FaaS" | faas-cli invoke hello-world --gateway http://localhost:8080
```

Expected: `Hello! You said: Docker FaaS`

### Test with JSON Payloads

```bash
# Deploy echo function if not already deployed
faas-cli deploy \
  --gateway http://localhost:8080 \
  --image ghcr.io/openfaas/alpine:latest \
  --name echo \
  --env fprocess="cat"

# Test JSON
curl -X POST http://localhost:8080/function/echo \
  -u admin:admin \
  -H "Content-Type: application/json" \
  -d '{"name": "Docker FaaS", "version": "1.0.0", "status": "awesome"}'
```

Should echo back the JSON.

### Stress Test

```bash
# Install Apache Bench (if not already installed)
# Ubuntu/Debian: sudo apt-get install apache2-utils
# macOS: already included

# Run stress test (100 requests, 10 concurrent)
echo "test" > /tmp/payload.txt
ab -n 100 -c 10 -p /tmp/payload.txt \
  -A admin:admin \
  -T "text/plain" \
  http://localhost:8080/function/echo
```

Check results for:
- All requests successful
- No failed requests
- Reasonable response times

### Test Metrics

```bash
# Check Prometheus metrics
curl http://localhost:9090/metrics

# Look for key metrics
curl http://localhost:9090/metrics | grep function_invocations_total
curl http://localhost:9090/metrics | grep function_duration_seconds
curl http://localhost:9090/metrics | grep gateway_http_requests_total
```

## Verification Checklist

Use this checklist to verify OpenFaaS compatibility:

- [ ] Gateway health endpoint works
- [ ] System info returns correct data
- [ ] faas-cli login works
- [ ] Function deployment works
- [ ] Function listing works
- [ ] Function invocation works (faas-cli)
- [ ] Function invocation works (HTTP)
- [ ] Function scaling works (up and down)
- [ ] Load balancing works across replicas
- [ ] Function logs retrieval works
- [ ] Function update works
- [ ] Function removal works
- [ ] Stack file deployment works
- [ ] Stack file removal works
- [ ] Environment variables work
- [ ] Labels/annotations work
- [ ] Different HTTP methods work (GET, POST, PUT, DELETE)
- [ ] Large payloads work (>1MB)
- [ ] Concurrent requests work
- [ ] JSON payloads work
- [ ] Metrics are exposed
- [ ] Authentication is enforced

## Comparison with OpenFaaS

| Feature | OpenFaaS | Docker FaaS | Status |
|---------|----------|-------------|--------|
| faas-cli compatibility | ✅ | ✅ | 100% |
| Function deployment | ✅ | ✅ | 100% |
| Function invocation | ✅ | ✅ | 100% |
| Scaling | ✅ | ✅ | 100% |
| Stack files | ✅ | ✅ | 100% |
| Basic auth | ✅ | ✅ | 100% |
| Metrics | ✅ | ✅ | 100% |
| Logs | ✅ | ✅ | 100% |
| Environment vars | ✅ | ✅ | 100% |
| Labels | ✅ | ✅ | 100% |
| Resource limits | ✅ | ✅ | 100% |
| Async invocations | ✅ | ⚠️ | Planned for v2 |
| Secrets API | ✅ | ⚠️ | Env vars only |
| Build API | ✅ | ⚠️ | Planned for v2 |
| Namespaces | ✅ | ⚠️ | Planned for v2 |

## Troubleshooting

### Gateway Not Starting

```bash
# Check Docker containers
docker-compose ps

# Check logs
docker-compose logs gateway

# Restart
docker-compose restart gateway
```

### Function Not Deploying

```bash
# Check if image exists
docker pull ghcr.io/openfaas/alpine:latest

# Check gateway logs
docker-compose logs gateway

# Check function containers
docker ps -a | grep your-function-name
```

### Function Invocation Failing

```bash
# Check function status
faas-cli list

# Check function logs
faas-cli logs your-function-name

# Check function containers
docker ps | grep your-function-name

# Inspect container
docker inspect $(docker ps -q --filter "label=com.docker-faas.function=your-function-name")
```

### Authentication Issues

```bash
# Verify credentials
echo "admin" | faas-cli login --gateway http://localhost:8080 --username admin --password-stdin

# Check if auth is enabled
docker-compose exec gateway env | grep AUTH

# Temporarily disable auth for testing
# Edit docker-compose.yml and set AUTH_ENABLED=false
docker-compose down
docker-compose up -d
```

## Cleanup

After testing:

```bash
# Remove all test functions
faas-cli remove echo
faas-cli remove nodeinfo
faas-cli remove env-test
faas-cli remove hello-world

# Or use the cleanup script
./scripts/cleanup.sh

# Stop the gateway
docker-compose down

# Remove volumes (optional - will delete database)
docker-compose down -v
```

## Next Steps

If all tests pass:

1. ✅ Docker FaaS is working correctly
2. ✅ It's compatible with OpenFaaS
3. ✅ Ready for production use
4. ✅ Can be shared with others
5. ✅ Can be deployed to production

## Getting Help

If tests fail or you encounter issues:

1. Check the [troubleshooting section](#troubleshooting)
2. Review the logs: `docker-compose logs`
3. Check GitHub issues
4. Open a new issue with test output

## Success Criteria

Your deployment is successful if:

- ✅ All 25 E2E tests pass
- ✅ All manual tests work as expected
- ✅ No errors in logs
- ✅ Functions deploy and invoke correctly
- ✅ Metrics are being collected
- ✅ Authentication works

**If all criteria are met, congratulations! Your Docker FaaS deployment is production-ready!** 🎉
