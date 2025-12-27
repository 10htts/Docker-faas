#!/usr/bin/env bash
set -euo pipefail

payload=$(cat)
if [[ -z "${payload}" ]]; then
  echo "uppercase-bash: no input"
  exit 0
fi

echo "${payload}" | tr '[:lower:]' '[:upper:]'
