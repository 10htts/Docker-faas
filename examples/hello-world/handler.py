#!/usr/bin/env python3
import sys
import json

def handle(req):
    """
    Simple hello world function
    """
    return f"Hello! You said: {req}"

if __name__ == "__main__":
    req = sys.stdin.read()
    response = handle(req)
    print(response)
