# Getting Started

This guide gets you running with Docker FaaS quickly. For deeper topics, see the links at the end.

## Prerequisites

- Docker 20.10+
- Docker Compose (recommended)
- Optional: `faas-cli` for CLI workflows

## Install faas-cli (optional)

macOS/Linux:
```bash
curl -sL https://cli.openfaas.com | sudo sh
```

Windows (PowerShell):
```powershell
$version = (Invoke-WebRequest "https://api.github.com/repos/openfaas/faas-cli/releases/latest" | ConvertFrom-Json)[0].tag_name
(New-Object System.Net.WebClient).DownloadFile("https://github.com/openfaas/faas-cli/releases/download/$version/faas-cli.exe", "faas-cli.exe")
```

## Quick Start

1) Start the gateway:
```bash
git clone https://github.com/docker-faas/docker-faas.git
cd docker-faas
docker-compose up -d
```

2) Verify health:
```bash
curl http://localhost:8080/healthz
```

3) Login (default creds):
```bash
faas-cli login --gateway http://localhost:8080 --username admin --password admin
```

4) Deploy a simple function:
```bash
faas-cli deploy   --gateway http://localhost:8080   --image ghcr.io/openfaas/alpine:latest   --name echo   --env fprocess="cat"
```

5) Invoke it:
```bash
echo "Hello" | faas-cli invoke echo --gateway http://localhost:8080
```

## Common Tasks

List functions:
```bash
faas-cli list --gateway http://localhost:8080
```

Scale (if `faas-cli scale` is unavailable, use API):
```bash
faas-cli scale echo --gateway http://localhost:8080 --replicas 3
```

```bash
curl -u admin:admin -H "Content-Type: application/json"   -d '{"serviceName":"echo","replicas":3}'   http://localhost:8080/system/scale-function/echo
```

Logs:
```bash
faas-cli logs echo --gateway http://localhost:8080
```

## Next Steps

- Build from source: [SOURCE_PACKAGING.md](SOURCE_PACKAGING.md)
- API reference: [API.md](API.md)
- Web UI guide: [WEB_UI.md](WEB_UI.md)
- Deployment options: [DEPLOYMENT.md](DEPLOYMENT.md)
