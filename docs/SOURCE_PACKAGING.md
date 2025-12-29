# Source Packaging Guide (Option 1)

This guide defines how to package a function so it can be built from a zip upload or a GitHub repo. The builder runs on the same machine as the gateway and produces a Docker image automatically. Users only provide source code.

Status: Proposed (Option 1: source-to-image build).

## Single Source of Truth

The source package is the single source of truth. The builder always uses the repo or zip contents as the build context. The resulting image is an internal artifact and does not need to be authored by the user.

## Package Rules (Zip or GitHub)

1. The repository root (or zip root) is the build context.
2. Provide either:
   - A Dockerfile at the root (no guesswork), or
   - A `docker-faas.yaml` manifest that specifies the runtime and command.
3. Keep build artifacts out of the package. Use `.dockerignore` when possible.
4. The function must serve HTTP on port 8080.

## Recommended: Dockerfile (Zero Guesswork)

If `Dockerfile` exists at the root, the builder uses it as-is. This is the most reliable way to avoid auto-detection surprises.

Minimum requirements for the Dockerfile:
- The container listens on port 8080.
- The HTTP server handles POST/GET and returns a response body.
- Optional but recommended: use `of-watchdog` for OpenFaaS compatibility.

### Python Example (of-watchdog)

Project layout:
```
my-python-func/
  handler.py
  requirements.txt
  Dockerfile
```

Dockerfile:
```dockerfile
FROM python:3.11-slim

WORKDIR /home/app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY handler.py /home/app/handler.py

ENV fprocess="python /home/app/handler.py"
ENV mode="http"
ENV upstream_url="http://127.0.0.1:8080"

RUN wget -qO /usr/local/bin/fwatchdog https://github.com/openfaas/of-watchdog/releases/download/0.9.11/fwatchdog \
  && chmod +x /usr/local/bin/fwatchdog

EXPOSE 8080
CMD ["fwatchdog"]
```

### Go Example (HTTP server)

Project layout:
```
my-go-func/
  main.go
  go.mod
  Dockerfile
```

Dockerfile:
```dockerfile
FROM golang:1.22 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o app ./main.go

FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=builder /src/app /app/app
EXPOSE 8080
CMD ["/app/app"]
```

## Manifest-Based Builds (docker-faas.yaml)

If you do not want to include a Dockerfile, include a manifest so the builder knows exactly how to build the function. This avoids guessing by file detection and keeps a single source of truth in the repo.

Manifest name: `docker-faas.yaml`

### Minimal Schema

```yaml
name: hello-python
runtime: python
command: "python handler.py"
dependencies:
  - requirements.txt
env:
  LOG_LEVEL: info
labels:
  team: platform
secrets:
  - api-key
limits:
  memory: "256m"
  cpu: "0.5"
```

### Field Reference

| Field | Required | Type | Description |
| --- | --- | --- | --- |
| `name` | yes | string | Function name. |
| `runtime` | yes | string | Runtime template (`python`, `go`, `node`). |
| `command` | yes | string | Command to run the handler (wired to `fprocess`). |
| `dependencies` | no | list | Dependency files to install (e.g., `requirements.txt`). |
| `env` | no | map | Environment variables passed at runtime. |
| `labels` | no | map | Labels applied to the container. |
| `secrets` | no | list | Secret names mounted into the function. |
| `limits` | no | map | Resource limits (`memory`, `cpu`). |
| `requests` | no | map | Resource requests (`memory`, `cpu`). |
| `readOnlyRootFilesystem` | no | bool | Enable read-only filesystem. |
| `debug` | no | bool | Enable debug mode for the function. |
| `network` | no | string | Override network name. |
| `build` | no | list | Optional build commands executed before packaging. |

### Example: Go

```yaml
name: hello-go
runtime: go
command: "./app"
build:
  - "go mod download"
  - "go build -o app ./"
```

### Example: Full Manifest

```yaml
name: demo-full
runtime: python
command: "python handler.py"
dependencies:
  - requirements.txt
env:
  LOG_LEVEL: debug
labels:
  owner: platform
  tier: demo
secrets:
  - api-key
limits:
  memory: "256m"
  cpu: "0.5"
requests:
  memory: "128m"
  cpu: "0.25"
readOnlyRootFilesystem: true
debug: false
network: "docker-faas-net-demo-full"
```

### Example: Node

```yaml
name: hello-node
runtime: node
command: "node index.js"
dependencies:
  - package.json
  - package-lock.json
```

The builder uses the manifest to select a runtime template, run build steps, and generate the final image automatically.

## Zip Upload Checklist

- Zip root contains your project (no extra folder wrapper).
- Include `Dockerfile` or `docker-faas.yaml`.
- Keep sensitive files out of the zip.

## GitHub Repo Checklist

- Repo root contains the build context.
- Use a `.dockerignore` to avoid large files.
- Tag a release or use a specific commit for reproducible builds.

## Builder Requirements (Same Machine)

The build worker runs on the same host as the gateway and needs:
- Docker daemon access.
- Disk space for intermediate layers.
- Network access for dependency download.

## FAQ

Q: Do users need Docker installed+
A: No. The builder runs on the gateway host and produces the image internally.

Q: What if no Dockerfile and no manifest+
A: The builder may attempt auto-detection, but this is not recommended for production. Provide a Dockerfile or manifest to avoid guesswork.

## Examples and Templates

See `docs/SOURCE_EXAMPLES.md` for ready-to-zip examples and starter templates.

## Build API Payload (Proposed)

The UI emits a proposed payload for a future `/system/builds` endpoint. This is a draft format to standardize source builds.

### Zip Upload
```json
{
  "name": "hello-python",
  "source": {
    "type": "zip",
    "runtime": "python",
    "zip": {
      "filename": "hello-python.zip"
    },
    "manifest": "name: hello-python\nruntime: python\ncommand: \"python handler.py\""
  }
}
```

### Git Repository
```json
{
  "name": "hello-python",
  "source": {
    "type": "git",
    "runtime": "python",
    "git": {
      "url": "https://github.com/org/repo.git",
      "ref": "main",
      "path": "."
    }
  }
}
```

### Security Limits

- Git URLs must use http/https/git/ssh and may not resolve to localhost or private IPs.
- Zip uploads are capped at 2000 entries, 100MB per file, and 500MB total uncompressed size.
- Zip entries containing symlinks or path traversal are rejected.

### Inline Files (Editor)
```json
{
  "name": "hello-python",
  "source": {
    "type": "git",
    "runtime": "python",
    "git": {
      "url": "https://github.com/org/repo.git",
      "ref": "main",
      "path": "."
    },
    "files": [
      { "path": "handler.py", "content": "print('hello')\n" },
      { "path": "docker-faas.yaml", "content": "name: hello-python\nruntime: python\ncommand: \"python handler.py\"\n" }
    ]
  }
}
```
