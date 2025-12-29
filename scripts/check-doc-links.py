#!/usr/bin/env python3
import argparse
import pathlib
import re
import sys
from urllib.parse import urlparse


SKIP_DIRS = {".git", ".claude", "tmp", "bin", "backups", "node_modules"}
INLINE_LINK_RE = re.compile(r"!?\[[^\]]*\]\(([^)]+)\)")
REF_LINK_RE = re.compile(r"^\s*\[[^\]]+\]:\s*(\S+)")


def is_external(target: str) -> bool:
    if target.startswith("#"):
        return True
    if target.startswith("//"):
        return True
    if target.startswith("/"):
        return True
    parsed = urlparse(target)
    if parsed.scheme in {"http", "https", "mailto", "tel", "ftp", "ws", "wss"}:
        return True
    return False


def iter_markdown_files(root: pathlib.Path, include_archived: bool):
    for path in root.rglob("*.md"):
        if any(part in SKIP_DIRS for part in path.parts):
            continue
        if not include_archived and "docs" in path.parts and "archived" in path.parts:
            continue
        yield path


def extract_links(text: str):
    links = []
    in_code_block = False
    for line in text.splitlines():
        stripped = line.strip()
        if stripped.startswith("```"):
            in_code_block = not in_code_block
            continue
        if in_code_block:
            continue
        for match in INLINE_LINK_RE.finditer(line):
            links.append(match.group(1))
        ref_match = REF_LINK_RE.match(line)
        if ref_match:
            links.append(ref_match.group(1))
    return links


def check_links(root: pathlib.Path, include_archived: bool):
    missing = []
    for path in iter_markdown_files(root, include_archived):
        text = path.read_text(encoding="utf-8")
        for target in extract_links(text):
            target = target.strip().strip("<>")
            if is_external(target):
                continue
            path_part = target.split("#", 1)[0]
            if not path_part:
                continue
            if re.match(r"^[A-Za-z]:\\\\", path_part):
                continue
            candidate = (path.parent / path_part).resolve()
            if not candidate.exists():
                missing.append((path, target))
    return missing


def main():
    parser = argparse.ArgumentParser(description="Check markdown links for local files.")
    parser.add_argument(
        "--include-archived",
        action="store_true",
        help="Include docs/archived in link checks.",
    )
    args = parser.parse_args()

    root = pathlib.Path(".").resolve()
    missing = check_links(root, include_archived=args.include_archived)
    if missing:
        print("Missing links:")
        for source, target in missing:
            print(f"- {source}: {target}")
        return 1
    print("All checked links are valid.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
