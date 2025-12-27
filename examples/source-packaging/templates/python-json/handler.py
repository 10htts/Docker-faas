import json
import sys


def main():
    payload = sys.stdin.read().strip()
    if not payload:
        print("python-json: {"status": "empty"}")
        return

    try:
        data = json.loads(payload)
    except json.JSONDecodeError as exc:
        print(f"python-json: invalid JSON ({exc})")
        return

    keys = list(data.keys()) if isinstance(data, dict) else []
    print(json.dumps({"status": "ok", "keys": keys}))


if __name__ == "__main__":
    main()
