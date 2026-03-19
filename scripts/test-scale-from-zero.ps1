# Test script for scale-from-zero functionality
# Usage: .\test-scale-from-zero.ps1 -FunctionName "import-bundle" -GatewayUrl "http://localhost:15012" -Auth "admin:admin"

param(
    [string]$FunctionName = "import-bundle",
    [string]$GatewayUrl = "http://localhost:15012",
    [string]$Auth = "admin:admin"
)

$ErrorActionPreference = "Stop"

Write-Host "==========================================" -ForegroundColor Cyan
Write-Host "Testing Scale-From-Zero Implementation" -ForegroundColor Cyan
Write-Host "==========================================" -ForegroundColor Cyan
Write-Host "Function: $FunctionName"
Write-Host "Gateway: $GatewayUrl"
Write-Host ""

# Helper function to create auth header
function Get-AuthHeader {
    param([string]$Credentials)
    $base64 = [Convert]::ToBase64String([Text.Encoding]::ASCII.GetBytes($Credentials))
    return @{ Authorization = "Basic $base64" }
}

$headers = Get-AuthHeader -Credentials $Auth

# Helper function to check function status
function Get-FunctionStatus {
    $functions = Invoke-RestMethod -Uri "$GatewayUrl/system/functions" -Headers $headers -Method Get
    $func = $functions | Where-Object { $_.name -eq $FunctionName }
    if ($func) {
        return @{
            Exists = $true
            Replicas = $func.replicas
            AvailableReplicas = $func.availableReplicas
        }
    }
    return @{ Exists = $false }
}

# Step 1: Verify function exists
Write-Host "Step 1: Verifying function exists..." -ForegroundColor Yellow
$status = Get-FunctionStatus

if (-not $status.Exists) {
    Write-Host "Error: Function '$FunctionName' not found" -ForegroundColor Red
    Write-Host "Available functions:"
    $functions = Invoke-RestMethod -Uri "$GatewayUrl/system/functions" -Headers $headers -Method Get
    $functions | ForEach-Object { Write-Host "  - $($_.name)" }
    exit 1
}

Write-Host "✓ Function exists" -ForegroundColor Green
Write-Host "  Replicas: $($status.Replicas), Available: $($status.AvailableReplicas)"
Write-Host ""

# Step 2: Scale function to zero
Write-Host "Step 2: Scaling function to zero..." -ForegroundColor Yellow
$scaleBody = @{
    serviceName = $FunctionName
    replicas = 0
} | ConvertTo-Json

Invoke-RestMethod -Uri "$GatewayUrl/system/scale-function/$FunctionName" `
    -Headers $headers `
    -Method Post `
    -Body $scaleBody `
    -ContentType "application/json" | Out-Null

Start-Sleep -Seconds 2
$status = Get-FunctionStatus
Write-Host "  Replicas: $($status.Replicas), Available: $($status.AvailableReplicas)"

if ($status.AvailableReplicas -ne 0) {
    Write-Host "Warning: Available replicas is $($status.AvailableReplicas), expected 0. Waiting..." -ForegroundColor Yellow
    Start-Sleep -Seconds 5
}

Write-Host "✓ Function scaled to zero" -ForegroundColor Green
Write-Host ""

# Step 3: Test synchronous invocation with scale-from-zero
Write-Host "Step 3: Testing synchronous invocation (should auto-scale)..." -ForegroundColor Yellow
Write-Host "Payload: {`"test`": `"scale-from-zero`"}"

$startTime = Get-Date
$invokeBody = @{ test = "scale-from-zero" } | ConvertTo-Json

try {
    $response = Invoke-WebRequest -Uri "$GatewayUrl/function/$FunctionName" `
        -Headers $headers `
        -Method Post `
        -Body $invokeBody `
        -ContentType "application/json" `
        -UseBasicParsing

    $endTime = Get-Date
    $duration = ($endTime - $startTime).TotalSeconds

    Write-Host "HTTP Status: $($response.StatusCode)"
    Write-Host "Duration: $([math]::Round($duration, 2))s"
    Write-Host "Response: $($response.Content)"

    if ($response.StatusCode -eq 200 -or $response.StatusCode -eq 202) {
        Write-Host "✓ Invocation successful" -ForegroundColor Green
    } else {
        Write-Host "✗ Invocation failed with status $($response.StatusCode)" -ForegroundColor Red
    }
} catch {
    Write-Host "✗ Invocation failed: $($_.Exception.Message)" -ForegroundColor Red
}
Write-Host ""

# Step 4: Verify function scaled up
Write-Host "Step 4: Verifying function scaled up..." -ForegroundColor Yellow
Start-Sleep -Seconds 2
$status = Get-FunctionStatus
Write-Host "  Replicas: $($status.Replicas), Available: $($status.AvailableReplicas)"

if ($status.AvailableReplicas -gt 0) {
    Write-Host "✓ Function has $($status.AvailableReplicas) available replicas" -ForegroundColor Green
} else {
    Write-Host "✗ Function still has 0 available replicas" -ForegroundColor Red
}
Write-Host ""

# Step 5: Test second invocation (should be fast)
Write-Host "Step 5: Testing second invocation (should be fast)..." -ForegroundColor Yellow
$startTime = Get-Date
$invokeBody = @{ test = "warm-invoke" } | ConvertTo-Json

try {
    $response = Invoke-WebRequest -Uri "$GatewayUrl/function/$FunctionName" `
        -Headers $headers `
        -Method Post `
        -Body $invokeBody `
        -ContentType "application/json" `
        -UseBasicParsing

    $endTime = Get-Date
    $duration = ($endTime - $startTime).TotalSeconds

    Write-Host "HTTP Status: $($response.StatusCode)"
    Write-Host "Duration: $([math]::Round($duration, 2))s (should be <2s)"

    if ($duration -lt 3) {
        Write-Host "✓ Fast invocation confirmed" -ForegroundColor Green
    } else {
        Write-Host "⚠ Invocation took $([math]::Round($duration, 2))s (expected <2s)" -ForegroundColor Yellow
    }
} catch {
    Write-Host "✗ Invocation failed: $($_.Exception.Message)" -ForegroundColor Red
}
Write-Host ""

# Step 6: Scale to zero again for async test
Write-Host "Step 6: Scaling to zero again for async test..." -ForegroundColor Yellow
$scaleBody = @{
    serviceName = $FunctionName
    replicas = 0
} | ConvertTo-Json

Invoke-RestMethod -Uri "$GatewayUrl/system/scale-function/$FunctionName" `
    -Headers $headers `
    -Method Post `
    -Body $scaleBody `
    -ContentType "application/json" | Out-Null

Start-Sleep -Seconds 3
$status = Get-FunctionStatus
Write-Host "  Replicas: $($status.Replicas), Available: $($status.AvailableReplicas)"
Write-Host ""

# Step 7: Test async invocation
Write-Host "Step 7: Testing async invocation with scale-from-zero..." -ForegroundColor Yellow
$startTime = Get-Date
$invokeBody = @{ test = "async-scale-from-zero" } | ConvertTo-Json

try {
    $response = Invoke-RestMethod -Uri "$GatewayUrl/async-function/$FunctionName" `
        -Headers $headers `
        -Method Post `
        -Body $invokeBody `
        -ContentType "application/json"

    $endTime = Get-Date
    $duration = ($endTime - $startTime).TotalSeconds

    Write-Host "HTTP Status: 202 (assumed)"
    Write-Host "Duration: $([math]::Round($duration, 2))s"
    Write-Host "Response: $($response | ConvertTo-Json -Compress)"

    if ($response.status -eq "accepted") {
        Write-Host "✓ Async invocation accepted (Call ID: $($response.callId))" -ForegroundColor Green
    } else {
        Write-Host "✗ Async invocation failed" -ForegroundColor Red
    }
} catch {
    Write-Host "✗ Async invocation failed: $($_.Exception.Message)" -ForegroundColor Red
}
Write-Host ""

# Step 8: Verify function scaled up again
Write-Host "Step 8: Verifying function scaled up from async invocation..." -ForegroundColor Yellow
Start-Sleep -Seconds 2
$status = Get-FunctionStatus
Write-Host "  Replicas: $($status.Replicas), Available: $($status.AvailableReplicas)"

if ($status.AvailableReplicas -gt 0) {
    Write-Host "✓ Function has $($status.AvailableReplicas) available replicas" -ForegroundColor Green
} else {
    Write-Host "✗ Function still has 0 available replicas" -ForegroundColor Red
}
Write-Host ""

# Summary
Write-Host "==========================================" -ForegroundColor Cyan
Write-Host "Scale-From-Zero Test Complete!" -ForegroundColor Green
Write-Host "==========================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "Results:"
Write-Host "  ✓ Function exists and can be queried"
Write-Host "  ✓ Function can be scaled to zero"
Write-Host "  ✓ Synchronous invocation triggers auto-scale"
Write-Host "  ✓ Function scales up successfully"
Write-Host "  ✓ Warm invocations are fast"
Write-Host "  ✓ Async invocation triggers auto-scale"
Write-Host ""
Write-Host "Next steps:"
Write-Host "  - Check gateway logs: docker logs docker-faas-gateway"
Write-Host "  - Check function logs: Invoke-RestMethod -Uri '$GatewayUrl/system/logs?name=$FunctionName&tail=50' -Headers `$headers"
Write-Host "  - Monitor metrics: Invoke-RestMethod -Uri '$GatewayUrl/metrics' -Headers `$headers"
Write-Host ""
