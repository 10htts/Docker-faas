# Scale-From-Zero Implementation

## Overview

This document describes the scale-from-zero functionality that has been implemented in docker-faas to provide full OpenFaaS compatibility. The implementation ensures that functions with 0 replicas are automatically started when invoked, eliminating the need for manual intervention.

## What Was Changed

### 1. Modified Files

#### `pkg/gateway/handlers.go`
Added three new methods to the `Gateway` struct:

- **`scaleFromZero(ctx, fn)`** - Scales a function from 0 to 1 replica
  - Builds deployment spec from stored metadata
  - Calls `provider.ScaleFunction()` with target of 1 replica
  - Updates replica count in database
  - Updates metrics

- **`waitForFunctionReady(ctx, functionName, timeout)`** - Waits for function to be ready
  - Polls every 500ms to check container health
  - Uses configurable timeout (default: 30 seconds)
  - Returns error if function doesn't become ready within timeout
  - Respects context cancellation

- **`isContainerHealthy(ctx, functionName)`** - Checks if container is running
  - Retrieves container list for function
  - Verifies at least one container has status "running" or "Up"
  - Returns boolean indicating health status

Modified **`HandleInvokeFunction`** to:
- Get function metadata before routing
- Check available replicas count
- Trigger scale-from-zero if `availableReplicas == 0`
- Wait for function to be ready before routing request
- Return appropriate HTTP errors (404 for not found, 504 for timeout, 500 for scale errors)

#### `pkg/gateway/async_handlers.go`
Added imports for `strings` and `time` packages.

Modified **`HandleInvokeFunctionAsync`** to:
- Get function metadata before async invocation
- Check available replicas count
- Trigger scale-from-zero if needed (synchronously before spawning goroutine)
- Wait for function to be ready
- Only spawn async goroutine after function is confirmed running

### 2. Key Implementation Details

#### Container Health Check
```go
// Checks if status contains "running" or "Up" (case-sensitive)
for _, c := range containers {
    if strings.Contains(c.Status, "running") || strings.Contains(c.Status, "Up") {
        return true
    }
}
```

#### Polling with Timeout
```go
deadline := time.Now().Add(timeout)
ticker := time.NewTicker(500 * time.Millisecond)
defer ticker.Stop()

for {
    select {
    case <-ctx.Done():
        return ctx.Err()
    case <-ticker.C:
        if g.isContainerHealthy(ctx, functionName) {
            return nil
        }
        if time.Now().After(deadline) {
            return fmt.Errorf("timeout waiting for function to be ready")
        }
    }
}
```

#### Error Handling
- **404 Not Found** - Function doesn't exist in database
- **500 Internal Server Error** - Failed to get containers or scale function
- **504 Gateway Timeout** - Function failed to start within 30 seconds
- **200+ (original response)** - Successful invocation after scaling

## Testing the Implementation

### Test Scenario 1: Scale from Zero on Synchronous Invocation

```bash
# 1. Deploy a function
faas-cli deploy -f functions/stack.yml --filter import-bundle

# 2. Verify it's running
curl -u admin:admin http://localhost:15012/system/functions | jq '.[] | select(.name=="import-bundle")'

# 3. Scale it down to zero
curl -u admin:admin -X POST http://localhost:15012/system/scale-function/import-bundle \
  -H "Content-Type: application/json" \
  -d '{"serviceName": "import-bundle", "replicas": 0}'

# 4. Verify it's scaled down (availableReplicas should be 0)
curl -u admin:admin http://localhost:15012/system/functions | jq '.[] | select(.name=="import-bundle")'

# 5. Invoke the function - should auto-scale from zero
time curl -u admin:admin -X POST http://localhost:15012/function/import-bundle \
  -H "Content-Type: application/json" \
  -d '{"test": "hello"}'

# Expected: First invocation takes longer (container startup time)
# Expected: Function scales to 1 replica and processes request
# Expected: Subsequent invocations are fast

# 6. Verify it's scaled back up
curl -u admin:admin http://localhost:15012/system/functions | jq '.[] | select(.name=="import-bundle")'
```

### Test Scenario 2: Scale from Zero on Async Invocation

```bash
# 1. Scale function to zero
curl -u admin:admin -X POST http://localhost:15012/system/scale-function/import-bundle \
  -H "Content-Type: application/json" \
  -d '{"serviceName": "import-bundle", "replicas": 0}'

# 2. Invoke async - should scale and return 202 Accepted
curl -u admin:admin -X POST http://localhost:15012/async-function/import-bundle \
  -H "Content-Type: application/json" \
  -d '{"test": "async"}'

# Expected: Returns 202 Accepted with callId
# Expected: Function scales to 1 replica
# Expected: Async invocation proceeds in background

# 3. Check logs to verify async execution
curl -u admin:admin "http://localhost:15012/system/logs?name=import-bundle&tail=50"
```

### Test Scenario 3: Container Startup Timeout

```bash
# 1. Deploy a function with a slow startup (e.g., initialization code)
# 2. Scale to zero
# 3. Invoke function
# Expected: If startup takes > 30 seconds, returns 504 Gateway Timeout
# Expected: If startup < 30 seconds, returns successful response
```

### Test Scenario 4: Health Checks During Scale-Up

```bash
# Monitor gateway logs during scale-from-zero to see polling behavior:
docker logs -f docker-faas-gateway

# You should see:
# - "Scaling function X from zero..."
# - Container health check debug logs every 500ms
# - "Function X scaled from zero and ready"
```

### Test Scenario 5: Function Not Found

```bash
# Invoke non-existent function
curl -u admin:admin -X POST http://localhost:15012/function/does-not-exist \
  -d '{"test": "data"}'

# Expected: 404 Not Found
# Expected: Error message: "Function not found"
```

## Performance Considerations

### Cold Start Latency
- First invocation after scaling from zero includes:
  - Container creation time (~1-3 seconds for small images)
  - Container startup time (depends on application initialization)
  - Health check polling (up to 500ms additional latency)

- Subsequent invocations have normal latency (< 50ms typically)

### Polling Interval
- Current implementation polls every 500ms
- Trade-off between responsiveness and system load
- Can be adjusted by modifying `time.NewTicker(500 * time.Millisecond)` in `waitForFunctionReady`

### Timeout Configuration
- Default timeout: 30 seconds
- Configured in `HandleInvokeFunction` and `HandleInvokeFunctionAsync`
- Can be adjusted per deployment requirements
- Consider increasing for functions with long initialization times

## OpenFaaS Compatibility

This implementation provides compatibility with OpenFaaS scale-from-zero behavior:

✅ **Automatic scale-up on invocation** - Functions with 0 replicas auto-scale
✅ **Synchronous invocations wait** - Caller receives response after startup
✅ **Async invocations defer** - Returns 202 Accepted only after function is ready
✅ **Health checking** - Verifies container is running before routing
✅ **Timeout handling** - Returns 504 if startup takes too long
✅ **Metrics tracking** - Updates replica count metrics after scaling

### Differences from OpenFaaS

1. **No auto-scale-down** - This implementation only handles scale-from-zero UP, not automatic scale-down after idle period
2. **Simpler health check** - Checks container status only (no HTTP health endpoints yet)
3. **Fixed polling interval** - OpenFaaS may use more sophisticated readiness probes

## Future Enhancements

### Potential Improvements

1. **HTTP Health Checks**
   - Add configurable health check endpoint (e.g., `/_/health`)
   - Verify application is ready, not just container running
   - Support custom health check paths per function

2. **Auto-Scale-Down**
   - Implement idle detection (no invocations for X minutes)
   - Automatic scale to zero after idle period
   - Configurable via labels: `com.openfaas.scale.zero: "true"`

3. **Metrics & Observability**
   - Track cold start duration
   - Count scale-from-zero events
   - Expose Prometheus metrics for monitoring

4. **Configuration Options**
   - Per-function timeout configuration
   - Per-function polling interval
   - Disable scale-from-zero for specific functions

5. **Optimizations**
   - Container image pre-pulling
   - Keep warm pools of pre-started containers
   - Progressive timeout (shorter initial, longer if needed)

## Troubleshooting

### Function doesn't start (504 Gateway Timeout)

**Check container logs:**
```bash
docker logs import-bundle-0
```

**Common causes:**
- Container image not available locally
- Network connectivity issues
- Application crashes on startup
- Initialization takes > 30 seconds

**Solutions:**
- Increase timeout in code
- Fix application startup issues
- Pre-pull images: `docker pull <image>`

### Function starts but invocation fails

**Check container status:**
```bash
docker ps -a | grep import-bundle
```

**Verify container is listening on port 8080:**
```bash
docker exec import-bundle-0 netstat -tuln | grep 8080
```

**Test direct container access:**
```bash
CONTAINER_IP=$(docker inspect import-bundle-0 -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}')
curl http://$CONTAINER_IP:8080
```

### Scaling triggered every invocation

**Symptom:** Every request triggers scale-from-zero
**Cause:** Containers not transitioning to "running" status
**Check:**
```bash
curl -u admin:admin http://localhost:15012/system/functions | \
  jq '.[] | select(.name=="import-bundle") | {replicas, availableReplicas}'
```

**Debug:** Check `isContainerHealthy` logs for health check failures

## Migration Guide

### Upgrading from Previous Versions

No database migrations required. The implementation uses existing schema.

**Steps:**
1. Stop gateway: `docker stop docker-faas-gateway`
2. Rebuild: `go build -o gateway cmd/gateway/main.go`
3. Restart gateway with new binary
4. Test scale-from-zero with a sample function

### Disabling Scale-From-Zero (if needed)

Currently, scale-from-zero cannot be disabled. If you need functions to always run:

**Option 1:** Set minimum replicas in deployment
```yaml
labels:
  com.openfaas.scale.min: "1"
```

**Option 2:** Never scale functions to 0
```bash
# Always maintain at least 1 replica
curl -X POST http://localhost:15012/system/scale-function/import-bundle \
  -d '{"serviceName": "import-bundle", "replicas": 1}'
```

## Code References

- Function invocation handler: `pkg/gateway/handlers.go:569` (HandleInvokeFunction)
- Async invocation handler: `pkg/gateway/async_handlers.go:12` (HandleInvokeFunctionAsync)
- Scale-from-zero logic: `pkg/gateway/handlers.go:705` (scaleFromZero)
- Health checking: `pkg/gateway/handlers.go:772` (isContainerHealthy)
- Container polling: `pkg/gateway/handlers.go:753` (waitForFunctionReady)

## Summary

The scale-from-zero implementation provides automatic function startup when invoked with 0 replicas, ensuring compatibility with OpenFaaS workflows while maintaining the simplicity of the docker-faas architecture.

Key benefits:
- ✅ Reduces resource usage (scale to 0 when idle)
- ✅ Automatic startup on demand
- ✅ Compatible with OpenFaaS tooling
- ✅ No manual intervention required
- ✅ Proper error handling and timeouts
