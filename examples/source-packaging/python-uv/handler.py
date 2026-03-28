import sys

import requests


def main():
    payload = sys.stdin.read().strip() or "hello"
    print(f"python-uv: {payload} (requests {requests.__version__})")


if __name__ == "__main__":
    main()
