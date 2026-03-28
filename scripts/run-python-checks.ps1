param(
  [switch]$Fix
)

$ErrorActionPreference = "Stop"

$python = Get-Command python -ErrorAction SilentlyContinue
if (-not $python) {
  $python = Get-Command py -ErrorAction SilentlyContinue
}

if (-not $python) {
  Write-Error "python or py is required to run Python example checks."
}

$args = @((Join-Path $PSScriptRoot "run-python-checks.py"))
if ($Fix) {
  $args += "--fix"
}

if ($python.Name -eq "py.exe" -or $python.Name -eq "py") {
  & $python.Source -3 @args
} else {
  & $python.Source @args
}
if ($LASTEXITCODE -ne 0) {
  throw "Python example checks failed."
}
