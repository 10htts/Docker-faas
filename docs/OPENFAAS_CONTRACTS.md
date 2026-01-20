# OpenFaaS Function Contracts and Custom Envelopes

**Document Version**: 1.0
**Last Updated**: 2026-01-20

---

## Overview

This document explains the difference between **standard OpenFaaS function contracts** and **custom application-level envelopes** that may be used on top of OpenFaaS/docker-faas.

**Key Point**: Custom request/response formats are **application-level abstractions** that work with **both** OpenFaaS and docker-faas. They are **not** compatibility issues between the platforms.

---

## Standard OpenFaaS Function Contract

### Request Format

OpenFaaS functions receive raw HTTP requests. The function receives:
- HTTP method (GET, POST, PUT, DELETE, etc.)
- HTTP headers
- HTTP body (raw bytes)
- Query parameters

### Example: Standard OpenFaaS Function

**Python Function (handler.py)**:
```python
import sys

def main():
    # Read raw input from stdin
    input_data = sys.stdin.read()

    # Process and return
    print(f"Received: {input_data}")

if __name__ == "__main__":
    main()
```

**Invocation**:
```bash
# Using faas-cli
echo '{"user_id": 123}' | faas-cli invoke my-function

# Using curl
curl -X POST http://localhost:8080/function/my-function \
  -d '{"user_id": 123}'
```

**What the function receives**:
```
{"user_id": 123}
```

**What the function returns**:
```
Received: {"user_id": 123}
```

---

## Custom Application-Level Envelopes

Some applications build **additional abstractions** on top of OpenFaaS. These envelopes:
- Wrap the raw HTTP request/response
- Add metadata (context, state, tokens, etc.)
- Enable complex workflows (e.g., Temporal, state machines)

### Example: Workflow Engine Envelope

**Custom Request Format**:
```json
{
  "node_id": "node-123",
  "function": "http-request",
  "parameters": {
    "user_id": 123,
    "action": "create_order"
  },
  "state": {
    "workflow_id": "wf-456",
    "step": 3
  },
  "context": {
    "trace_id": "abc-def-ghi",
    "user_session": "sess-789"
  },
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

**Custom Response Format**:
```json
{
  "status": "success",
  "result": {
    "order_id": "ord-999"
  },
  "state": {
    "workflow_id": "wf-456",
    "step": 4
  },
  "metadata": {
    "execution_time_ms": 123
  }
}
```

### How It Works

1. **Workflow Orchestrator** (e.g., Temporal, custom Go service):
   - Creates the custom envelope
   - Calls OpenFaaS/docker-faas HTTP API with envelope as body

2. **OpenFaaS/docker-faas Gateway**:
   - Receives raw HTTP POST with envelope JSON
   - Routes to function container
   - Returns raw HTTP response

3. **Function Handler**:
   - Parses the custom envelope
   - Extracts `parameters`
   - Performs business logic
   - Returns custom response envelope

4. **Workflow Orchestrator**:
   - Receives raw HTTP response
   - Parses custom response envelope
   - Continues workflow

---

## Compatibility Implications

### ✅ Compatible with Both Platforms

Custom envelopes work identically on:
- **OpenFaaS** (Kubernetes-based)
- **docker-faas** (Docker-based)

**Why?** Both platforms:
1. Accept raw HTTP requests
2. Pass body unchanged to function
3. Return function response unchanged

### ❌ Not Compatible with Standard Functions

Functions written for custom envelopes **cannot** be invoked with standard tools:

```bash
# This won't work with envelope-based functions
echo '{"user_id": 123}' | faas-cli invoke my-workflow-function
```

The function expects:
```json
{
  "node_id": "...",
  "function": "...",
  "parameters": {"user_id": 123},
  ...
}
```

But receives:
```json
{"user_id": 123}
```

---

## Design Patterns

### Pattern 1: Pure OpenFaaS Functions (Recommended for General Use)

**Use Case**: Generic, reusable functions
**Invocation**: `faas-cli`, `curl`, any HTTP client

```python
# handler.py
import sys
import json

def main():
    input_data = json.loads(sys.stdin.read())
    user_id = input_data.get("user_id")

    # Business logic
    result = {"message": f"Processed user {user_id}"}

    print(json.dumps(result))

if __name__ == "__main__":
    main()
```

### Pattern 2: Envelope-Based Functions (For Workflow Systems)

**Use Case**: Complex workflows, state management
**Invocation**: Custom orchestrator only

```python
# handler.py
import sys
import json

def main():
    envelope = json.loads(sys.stdin.read())

    # Extract parameters from envelope
    params = envelope.get("parameters", {})
    state = envelope.get("state", {})

    # Business logic
    user_id = params.get("user_id")
    workflow_id = state.get("workflow_id")

    # Build response envelope
    response = {
        "status": "success",
        "result": {"message": f"Processed user {user_id}"},
        "state": {
            "workflow_id": workflow_id,
            "step": state.get("step", 0) + 1
        }
    }

    print(json.dumps(response))

if __name__ == "__main__":
    main()
```

### Pattern 3: Hybrid Functions (Flexible)

**Use Case**: Functions that work both standalone and in workflows

```python
# handler.py
import sys
import json

def is_envelope(data):
    """Check if input is an envelope or raw data"""
    return isinstance(data, dict) and "node_id" in data and "parameters" in data

def main():
    input_data = json.loads(sys.stdin.read())

    if is_envelope(input_data):
        # Envelope mode
        params = input_data["parameters"]
        state = input_data.get("state", {})

        # Business logic
        result = process(params)

        # Return envelope
        response = {
            "status": "success",
            "result": result,
            "state": state
        }
    else:
        # Standard mode
        result = process(input_data)
        response = result

    print(json.dumps(response))

def process(params):
    user_id = params.get("user_id")
    return {"message": f"Processed user {user_id}"}

if __name__ == "__main__":
    main()
```

---

## Migration Between Formats

### Converting Standard → Envelope

If you have standard functions and want to use them in a workflow system, create a wrapper:

```python
# wrapper.py
import sys
import json
import subprocess

def main():
    envelope = json.loads(sys.stdin.read())
    params = envelope["parameters"]

    # Call original function with raw params
    result = subprocess.run(
        ["python", "original_handler.py"],
        input=json.dumps(params),
        capture_output=True,
        text=True
    )

    # Wrap result in envelope
    response = {
        "status": "success",
        "result": json.loads(result.stdout),
        "state": envelope.get("state", {})
    }

    print(json.dumps(response))

if __name__ == "__main__":
    main()
```

### Converting Envelope → Standard

Extract parameters and call function:

```bash
# Invoke envelope-based function with standard tools
curl -X POST http://localhost:8080/function/my-workflow-function \
  -d '{
    "node_id": "manual-invoke",
    "function": "process",
    "parameters": {"user_id": 123},
    "state": {},
    "context": {}
  }'
```

---

## Best Practices

### 1. Document Your Contract

Clearly document whether functions expect:
- Standard OpenFaaS format
- Custom envelope format
- Hybrid (both)

### 2. Use Type Hints (Python)

```python
from typing import TypedDict, Optional

class Envelope(TypedDict):
    node_id: str
    function: str
    parameters: dict
    state: Optional[dict]
    context: Optional[dict]

def handle_envelope(envelope: Envelope) -> dict:
    ...
```

### 3. Validate Input

```python
def validate_envelope(data):
    required_fields = ["node_id", "function", "parameters"]
    for field in required_fields:
        if field not in data:
            raise ValueError(f"Missing required field: {field}")
```

### 4. Error Handling

Return errors in the same format:

```python
try:
    result = process(params)
    response = {"status": "success", "result": result}
except Exception as e:
    response = {
        "status": "error",
        "error": str(e),
        "result": None
    }
```

---

## Testing

### Testing Standard Functions

```bash
# Direct invocation
echo '{"user_id": 123}' | faas-cli invoke my-function

# Integration test
curl -X POST http://localhost:8080/function/my-function \
  -d '{"user_id": 123}' \
  --fail
```

### Testing Envelope Functions

```bash
# Create test envelope
cat > test_envelope.json <<EOF
{
  "node_id": "test",
  "function": "process",
  "parameters": {"user_id": 123},
  "state": {},
  "context": {"test": true}
}
EOF

# Invoke with envelope
curl -X POST http://localhost:8080/function/my-workflow-function \
  -d @test_envelope.json \
  --fail
```

---

## Conclusion

**Custom envelopes are application-level abstractions, not compatibility issues.**

- ✅ docker-faas and OpenFaaS are 100% compatible for raw HTTP function invocations
- ✅ Custom envelopes work identically on both platforms
- ❌ Envelope-based functions are intentionally not compatible with standard tools (by design)
- ✅ Hybrid functions can support both formats

Choose the pattern that best fits your use case:
- **Standard format**: For general-purpose, reusable functions
- **Envelope format**: For complex workflow systems
- **Hybrid format**: For maximum flexibility

---

**Related Documents**:
- [API Reference](API.md)
- [Migration Guide](OPENFAAS_MIGRATION.md)
- [Architecture](ARCHITECTURE.md)
