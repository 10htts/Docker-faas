#!/usr/bin/env bash
set -euo pipefail

payload=$(cat)
if [[ -z "${payload}" ]]; then
  echo "bash-basic: hello"
  exit 0
fi

echo "bash-basic: ${payload}"
