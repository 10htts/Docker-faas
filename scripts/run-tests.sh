#!/usr/bin/env bash
set -euo pipefail

GATEWAY="${GATEWAY:-http://localhost:8080}"
AUTH_USER="${AUTH_USER:-${DOCKER_FAAS_USER:-admin}}"
AUTH_PASSWORD="${AUTH_PASSWORD:-${DOCKER_FAAS_PASSWORD:-admin}}"
SKIP_E2E="${SKIP_E2E:-}"
SKIP_COMPAT="${SKIP_COMPAT:-}"
SKIP_FAAS_CLI="${SKIP_FAAS_CLI:-}"
SKIP_UPGRADE="${SKIP_UPGRADE:-}"
SKIP_UI_E2E="${SKIP_UI_E2E:-}"

printf "Running unit tests...\n"
go test ./...

if [[ -n "${SKIP_E2E}" ]]; then
  printf "Skipping E2E tests.\n"
  exit 0
fi

export GATEWAY AUTH_USER AUTH_PASSWORD

chmod +x tests/e2e/*.sh

run_e2e() {
  local script="$1"
  printf "Running %s...\n" "$script"
  ./tests/e2e/$script
}

run_e2e test-security.sh
run_e2e test-network-isolation.sh
run_e2e test-debug-mode.sh
run_e2e test-secrets.sh
run_e2e test-metrics.sh

if [[ -z "${SKIP_UPGRADE}" ]]; then
  if command -v sqlite3 >/dev/null 2>&1; then
    run_e2e test-upgrade.sh
  else
    printf "sqlite3 not found; skipping test-upgrade.sh\n"
  fi
fi

if [[ -z "${SKIP_COMPAT}" ]]; then
  if command -v faas-cli >/dev/null 2>&1; then
    run_e2e openfaas-compatibility-test.sh
  else
    printf "faas-cli not found; skipping openfaas-compatibility-test.sh\n"
  fi
fi

if [[ -z "${SKIP_FAAS_CLI}" ]]; then
  if command -v faas-cli >/dev/null 2>&1; then
    run_e2e test-faas-cli-workflow.sh
  else
    printf "faas-cli not found; skipping test-faas-cli-workflow.sh\n"
  fi
fi

if [[ -z "${SKIP_UI_E2E}" ]]; then
  if command -v node >/dev/null 2>&1; then
    if [[ -f tests/ui/package.json ]]; then
      printf "Running UI E2E tests...\n"
      export GATEWAY_URL="${GATEWAY}"
      (
        cd tests/ui
        if [[ ! -d node_modules ]]; then
          npm install
        fi
        npx playwright install
        npx playwright test
      )
    else
      printf "tests/ui/package.json not found; skipping UI E2E tests\n"
    fi
  else
    printf "node not found; skipping UI E2E tests\n"
  fi
fi

printf "All requested tests completed.\n"
