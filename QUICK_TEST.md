# Quick Test Guide - Docker FaaS

Run this to verify Docker FaaS works exactly like OpenFaaS.

## Option 1: Automated Verification (Recommended)

```bash
# Make script executable
chmod +x verify-deployment.sh

# Run verification
./verify-deployment.sh
```

This will:
- ✅ Check all prerequisites
- ✅ Start Docker FaaS
- ✅ Test all endpoints
- ✅ Deploy and invoke a test function
- ✅ Verify authentication
- ✅ Check metrics
- ✅ Clean up after itself

**Expected output:** All green checkmarks ✓

## Option 2: Full OpenFaaS Compatibility Test

```bash
# Start Docker FaaS
docker-compose up -d

# Run 25 comprehensive tests
make e2e-test

# Or run directly
chmod +x tests/e2e/openfaas-compatibility-test.sh
./tests/e2e/openfaas-compatibility-test.sh
```

**Expected output:** 25/25 tests passed

## Option 3: Manual Quick Test

```bash
# 1. Start
docker-compose up -d

# 2. Check health
curl http://localhost:8080/healthz
# Expected: OK

# 3. Login
faas-cli login --gateway http://localhost:8080 -u admin -p admin

# 4. Deploy
faas-cli deploy \
  --image ghcr.io/openfaas/alpine:latest \
  --name test \
  --env fprocess="cat"

# 5. Invoke
echo "Hello!" | faas-cli invoke test
# Expected: Hello!

# 6. Scale
faas-cli scale test --replicas 3

# 7. Check
faas-cli list
# Expected: test function with 3 replicas

# 8. Remove
faas-cli remove test

# 9. Stop
docker-compose down
```

## What Gets Tested

### Automated Verification Script
- Docker and Docker Compose installed
- Gateway starts successfully
- Health endpoint works
- System info endpoint works
- Metrics endpoint works
- Authentication works
- Function deployment works
- Function invocation works
- Function removal works

### Full Compatibility Test (25 tests)
1. Gateway health check
2. System info endpoint
3. faas-cli login
4. Function deployment
5. Function listing
6. Invoke via faas-cli
7. Invoke via HTTP POST
8. Invoke with JSON
9. Scale up
10. Multiple invocations (load balancing)
11. Scale down
12. Environment variables
13. Verify env vars
14. Labels support
15. Log retrieval
16. Function update
17. Function removal
18. Stack file deployment
19. Invoke from stack
20. Remove from stack
21. Concurrent invocations
22. HTTP methods (GET, POST, PUT, DELETE)
23. Large payloads (1MB)
24. Authentication enforcement
25. Prometheus metrics

## Success Criteria

✅ **All tests pass** = Docker FaaS is production-ready and OpenFaaS-compatible!

## If Tests Fail

1. Check Docker is running: `docker ps`
2. Check logs: `docker-compose logs gateway`
3. Restart: `docker-compose restart`
4. Clean start: `./scripts/cleanup.sh && docker-compose up -d`

## Quick Comparison

| Test | OpenFaaS | Docker FaaS |
|------|----------|-------------|
| Deploy function | ✅ | ✅ |
| Invoke function | ✅ | ✅ |
| Scale function | ✅ | ✅ |
| faas-cli compatible | ✅ | ✅ |
| Metrics | ✅ | ✅ |
| Authentication | ✅ | ✅ |
| Stack files | ✅ | ✅ |
| Load balancing | ✅ | ✅ |

**Result: 100% Compatible!** 🎉

## Time Required

- Automated verification: ~2 minutes
- Full compatibility test: ~5 minutes
- Manual quick test: ~3 minutes

## What You'll See

### Successful Verification:
```
╔════════════════════════════════════════════════╗
║   Docker FaaS - Deployment Verification       ║
╚════════════════════════════════════════════════╝

Step 1: Checking prerequisites...
✓ Docker installed (24.0.7)
✓ Docker Compose installed (2.23.0)
✓ faas-cli installed (0.16.18)

Step 2: Starting Docker FaaS...
✓ Docker FaaS started successfully

Step 3: Waiting for gateway to be ready...
✓ Gateway is ready!

Step 4: Running basic health checks...
✓ Health endpoint responding (200 OK)
✓ System info endpoint working
✓ Metrics endpoint working
✓ Authentication enforced

Step 5: Testing function deployment...
✓ faas-cli login successful
✓ Function deployed successfully
✓ Function appears in list
✓ Function invocation successful
✓ Test function removed

Step 6: Checking Docker resources...
✓ Docker containers running: 1
✓ Docker network 'docker-faas-net' exists
✓ Docker volume 'docker-faas-data' exists

╔════════════════════════════════════════════════╗
║  ✓ Deployment Verification Complete!          ║
║  Your Docker FaaS is ready for use!           ║
╚════════════════════════════════════════════════╝
```

### Successful E2E Tests:
```
========================================
Docker FaaS - OpenFaaS Compatibility Test
========================================

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

## Ready to Deploy?

If all tests pass, your Docker FaaS is:
- ✅ Production-ready
- ✅ OpenFaaS-compatible
- ✅ Fully tested
- ✅ Ready to share on GitHub

## Next Steps

1. **Test passed?** → See [DEPLOYMENT.md](docs/DEPLOYMENT.md) for production setup
2. **Want to publish?** → Run `./scripts/init-git.sh` to initialize Git
3. **Need help?** → See [TEST_DEPLOYMENT.md](TEST_DEPLOYMENT.md) for detailed testing

**Happy function deployment!** 🚀
