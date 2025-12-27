import sys


def main():
    payload = sys.stdin.read().strip()
    if payload:
        print("python-basic: " + payload)
    else:
        print("python-basic: hello")


if __name__ == "__main__":
    main()
