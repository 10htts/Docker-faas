#!/usr/bin/env python3
import argparse
import os
import shutil
import subprocess
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
TARGETS = ["examples", "scripts"]


def resolve_ruff_command():
    candidates = []
    if os.environ.get("RUFF_BIN"):
        candidates.append([os.environ["RUFF_BIN"]])
    candidates.append(["ruff"])
    candidates.append([sys.executable, "-m", "ruff"])

    for candidate in candidates:
        binary = candidate[0]
        if binary != sys.executable and shutil.which(binary) is None:
            continue

        try:
            result = subprocess.run(
                candidate + ["--version"],
                cwd=REPO_ROOT,
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
                check=False,
            )
        except FileNotFoundError:
            continue

        if result.returncode == 0:
            return candidate

    return None


def run(command):
    print("+", " ".join(command))
    result = subprocess.run(command, cwd=REPO_ROOT, check=False)
    return result.returncode


def main():
    parser = argparse.ArgumentParser(
        description=("Run shared Ruff validation for the bundled Python examples and scripts.")
    )
    parser.add_argument(
        "--fix",
        action="store_true",
        help="Apply Ruff fixes and formatting in-place.",
    )
    args = parser.parse_args()

    ruff = resolve_ruff_command()
    if ruff is None:
        print(
            "ruff is required for Python example validation. Install it with "
            "`python -m pip install ruff` or set RUFF_BIN.",
            file=sys.stderr,
        )
        return 1

    check_cmd = ruff + ["check"]
    format_cmd = ruff + ["format"]
    if args.fix:
        check_cmd.append("--fix")
    else:
        format_cmd.append("--check")

    targets = [str(REPO_ROOT / target) for target in TARGETS]
    if run(check_cmd + targets) != 0:
        return 1
    if run(format_cmd + targets) != 0:
        return 1

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
