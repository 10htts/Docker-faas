#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if command -v python3 >/dev/null 2>&1; then
  PYTHON_BIN="python3"
elif command -v python >/dev/null 2>&1; then
  PYTHON_BIN="python"
else
  printf "python3 or python is required to run Python example checks\n" >&2
  exit 1
fi

"$PYTHON_BIN" "$SCRIPT_DIR/run-python-checks.py" "$@"
