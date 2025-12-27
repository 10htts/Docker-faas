import sys


def main():
    payload = sys.stdin.read().strip()
    if payload:
        print("python-deps: " + payload)
    else:
        print("python-deps: ready")


if __name__ == "__main__":
    main()
