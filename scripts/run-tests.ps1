param(
  [switch]$SkipE2E,
  [switch]$SkipCompatibility,
  [switch]$SkipFaasCli,
  [switch]$SkipUpgrade,
  [switch]$SkipUiE2E,
  [string]$Gateway = "http://localhost:8080",
  [string]$AuthUser,
  [string]$AuthPassword
)

$ErrorActionPreference = "Stop"

Write-Host "Running unit tests..." -ForegroundColor Cyan
& go test ./...
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

if ($SkipE2E) {
  Write-Host "Skipping E2E tests." -ForegroundColor Yellow
  exit 0
}

$bashCandidates = @(
  $env:GIT_BASH,
  "C:\\Program Files\\Git\\bin\\bash.exe",
  "C:\\Program Files (x86)\\Git\\bin\\bash.exe",
  "C:\\Windows\\System32\\bash.exe"
) | Where-Object { $_ -and (Test-Path $_) }

$bash = $bashCandidates | Select-Object -First 1
if (-not $bash) {
  Write-Warning "Git Bash not found; cannot run E2E shell scripts."
  exit 0
}

$env:GATEWAY = $Gateway
if (-not $AuthUser) { $AuthUser = $env:AUTH_USER }
if (-not $AuthUser) { $AuthUser = $env:DOCKER_FAAS_USER }
if (-not $AuthUser) { $AuthUser = "admin" }
if (-not $AuthPassword) { $AuthPassword = $env:AUTH_PASSWORD }
if (-not $AuthPassword) { $AuthPassword = $env:DOCKER_FAAS_PASSWORD }
if (-not $AuthPassword) { $AuthPassword = "admin" }
$env:AUTH_USER = $AuthUser
$env:AUTH_PASSWORD = $AuthPassword

$extraPath = @()
$faasCliDir = Join-Path $env:LOCALAPPDATA "Programs\\faas-cli"
$sqliteDir = Join-Path $env:LOCALAPPDATA "Programs\\sqlite3"
if (Test-Path (Join-Path $faasCliDir "faas-cli.exe")) { $extraPath += $faasCliDir }
if (Test-Path (Join-Path $sqliteDir "sqlite3.exe")) { $extraPath += $sqliteDir }

$pathPrefix = ""
if ($extraPath.Count -gt 0) {
  $unixPaths = $extraPath | ForEach-Object { $_.Replace('\','/') -replace '^([A-Za-z]):', '/$1' }
  $pathPrefix = "PATH=$($unixPaths -join ':'):`$PATH "
}

& $bash -lc "$pathPrefix chmod +x tests/e2e/*.sh"
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

$tests = @(
  "test-security.sh",
  "test-network-isolation.sh",
  "test-debug-mode.sh",
  "test-secrets.sh",
  "test-metrics.sh",
  "test-upgrade.sh",
  "openfaas-compatibility-test.sh",
  "test-faas-cli-workflow.sh"
)

foreach ($test in $tests) {
  if ($test -eq "test-upgrade.sh" -and $SkipUpgrade) { continue }
  if ($test -eq "openfaas-compatibility-test.sh" -and $SkipCompatibility) { continue }
  if ($test -eq "test-faas-cli-workflow.sh" -and $SkipFaasCli) { continue }

  if ($test -eq "test-upgrade.sh") {
    & $bash -lc "$pathPrefix command -v sqlite3 >/dev/null 2>&1"
    if ($LASTEXITCODE -ne 0) {
      Write-Warning "sqlite3 not found; skipping test-upgrade.sh"
      continue
    }
  }

  if ($test -eq "openfaas-compatibility-test.sh" -or $test -eq "test-faas-cli-workflow.sh") {
    & $bash -lc "$pathPrefix command -v faas-cli >/dev/null 2>&1"
    if ($LASTEXITCODE -ne 0) {
      Write-Warning "faas-cli not found; skipping $test"
      continue
    }
  }

  Write-Host "Running $test..." -ForegroundColor Cyan
  & $bash -lc "$pathPrefix ./tests/e2e/$test"
  if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
}

if (-not $SkipUiE2E) {
  $node = Get-Command node -ErrorAction SilentlyContinue
  if ($node -and (Test-Path "tests/ui/package.json")) {
    Write-Host "Running UI E2E tests..." -ForegroundColor Cyan
    $env:GATEWAY_URL = $Gateway
    Push-Location "tests/ui"
    try {
      if (-not (Test-Path "node_modules")) {
        & npm install
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
      }
      & npx playwright install
      if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
      & npx playwright test
      if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    } finally {
      Pop-Location
    }
  } else {
    Write-Warning "node or tests/ui/package.json not found; skipping UI E2E tests."
  }
}

Write-Host "All requested tests completed." -ForegroundColor Green
