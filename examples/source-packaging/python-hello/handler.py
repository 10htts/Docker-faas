import sys


def main():
    payload = sys.stdin.read().strip()
    if payload:
        print("Hello from docker-faas (python). Input: " + payload)
    else:
        print("Hello from docker-faas (python). No input provided.")


if __name__ == "__main__":
    main()
