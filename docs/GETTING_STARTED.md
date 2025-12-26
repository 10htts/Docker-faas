# Getting Started with Docker FaaS

This guide will help you get up and running with Docker FaaS in minutes.

## Prerequisites

Before you begin, ensure you have:

- ✅ Docker 20.10 or later installed
- ✅ Docker Compose (optional but recommended)
- ✅ `faas-cli` installed (see below)
- ✅ Basic understanding of Docker and serverless concepts

### Installing faas-cli

**macOS/Linux:**
```bash
curl -sL https://cli.openfaas.com | sudo sh
```

**Windows (PowerShell):**
```powershell
$version = (Invoke-WebRequest "https://api.github.com/repos/openfaas/faas-cli/releases/latest" | ConvertFrom-Json)[0].tag_name
(New-Object System.Net.WebClient).DownloadFile("https://github.com/openfaas/faas-cli/releases/download/$version/faas-cli.exe", "faas-cli.exe")
```

**Verify installation:**
```bash
faas-cli version
```

## Quick Start (5 minutes)

### Step 1: Start Docker FaaS

**Option A: Using Docker Compose (Recommended)**

```bash
# Clone the repository
git clone https://github.com/docker-faas/docker-faas.git
cd docker-faas

# Start the gateway
docker-compose up -d

# Check status
docker-compose ps
```

**Option B: Using Docker directly**

```bash
# Create gateway network (functions get per-function networks automatically)
docker network create docker-faas-net

# Pull and run gateway
docker run -d \
  --name docker-faas-gateway \
  -p 8080:8080 -p 9090:9090 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v faas-data:/data \
  --network docker-faas-net \
  ghcr.io/docker-faas/docker-faas:latest
```

### Step 2: Verify Installation

```bash
# Check health
curl http://localhost:8080/healthz
# Expected output: OK

# Get system info
curl http://localhost:8080/system/info -u admin:admin
```

### Step 3: Login with faas-cli

```bash
# Login (use default credentials)
faas-cli login \
  --gateway http://localhost:8080 \
  --username admin \
  --password admin

# You should see: "credentials saved"
```

### Step 4: Deploy Your First Function

```bash
# Deploy a simple echo function
faas-cli deploy \
  --image ghcr.io/openfaas/alpine:latest \
  --name my-echo \
  --gateway http://localhost:8080 \
  --env fprocess="cat"

# Wait a few seconds for the image to pull and container to start
```

### Step 5: Test Your Function

```bash
# Invoke the function
echo "Hello, Docker FaaS!" | faas-cli invoke my-echo

# Expected output: Hello, Docker FaaS!
```

**🎉 Congratulations!** You've successfully deployed and invoked your first function!

## Next Steps

### List Your Functions

```bash
# List all deployed functions
faas-cli list

# Expected output:
# Function    Invocations    Replicas
# my-echo     1              1
```

### Invoke via HTTP

```bash
# Using curl
curl -X POST \
  http://localhost:8080/function/my-echo \
  -u admin:admin \
  -d "Testing via HTTP"

# Using wget
echo "Testing" | wget -O- \
  --http-user=admin \
  --http-password=admin \
  --post-data=@- \
  http://localhost:8080/function/my-echo
```

### Scale Your Function

```bash
# Scale to 3 replicas
faas-cli scale my-echo --replicas 3

# Verify scaling
faas-cli list
# You should see: Replicas: 3
```

### View Function Logs

```bash
# Get logs
faas-cli logs my-echo

# Or with curl
curl "http://localhost:8080/system/logs?name=my-echo&tail=50" \
  -u admin:admin
```

### Remove the Function

```bash
# Remove function
faas-cli remove my-echo

# Verify removal
faas-cli list
# Function should be gone
```

## Deploy More Complex Functions

### Using Python

Create a directory structure:
```
my-python-func/
├── handler.py
└── Dockerfile
```

**handler.py:**
```python
#!/usr/bin/env python3
import sys
import json

def handle(req):
    data = json.loads(req) if req else {}
    name = data.get('name', 'World')
    return json.dumps({"message": f"Hello, {name}!"})

if __name__ == "__main__":
    req = sys.stdin.read()
    response = handle(req)
    print(response)
```

**Dockerfile:**
```dockerfile
FROM ghcr.io/openfaas/of-watchdog:0.9.11 as watchdog
FROM python:3.11-alpine

COPY --from=watchdog /fwatchdog /usr/bin/fwatchdog
RUN chmod +x /usr/bin/fwatchdog

WORKDIR /home/app
COPY handler.py .

ENV fprocess="python3 handler.py"
ENV mode="streaming"

CMD ["fwatchdog"]
```

**Build and deploy:**
```bash
# Build image
cd my-python-func
docker build -t my-python-func:latest .

# Deploy
faas-cli deploy \
  --image my-python-func:latest \
  --name python-func \
  --gateway http://localhost:8080

# Test
echo '{"name": "Docker FaaS"}' | faas-cli invoke python-func
```

### Using Node.js

**handler.js:**
```javascript
#!/usr/bin/env node
const getStdin = require('get-stdin');

getStdin().then(val => {
  const data = JSON.parse(val || '{}');
  const name = data.name || 'World';
  console.log(JSON.stringify({ message: `Hello, ${name}!` }));
}).catch(e => {
  console.error(e);
});
```

### Using Go

**handler.go:**
```go
package main

import (
    "encoding/json"
    "fmt"
    "io"
    "os"
)

type Request struct {
    Name string `json:"name"`
}

type Response struct {
    Message string `json:"message"`
}

func main() {
    input, _ := io.ReadAll(os.Stdin)

    var req Request
    json.Unmarshal(input, &req)

    if req.Name == "" {
        req.Name = "World"
    }

    resp := Response{
        Message: fmt.Sprintf("Hello, %s!", req.Name),
    }

    output, _ := json.Marshal(resp)
    fmt.Println(string(output))
}
```

## Using Stack Files

Create `stack.yml`:
```yaml
version: 1.0
provider:
  name: openfaas
  gateway: http://localhost:8080

functions:
  echo:
    image: ghcr.io/openfaas/alpine:latest
    environment:
      fprocess: "cat"
    labels:
      example: "true"

  uppercase:
    image: ghcr.io/openfaas/alpine:latest
    environment:
      fprocess: "tr '[:lower:]' '[:upper:]'"
    labels:
      example: "true"

  reverse:
    image: ghcr.io/openfaas/alpine:latest
    environment:
      fprocess: "rev"
    labels:
      example: "true"
```

**Deploy all functions:**
```bash
# Deploy entire stack
faas-cli deploy -f stack.yml

# List functions
faas-cli list

# Test each function
echo "hello world" | faas-cli invoke echo
echo "hello world" | faas-cli invoke uppercase
echo "hello world" | faas-cli invoke reverse

# Remove entire stack
faas-cli remove -f stack.yml
```

## Monitoring and Metrics

### View Metrics

```bash
# Open in browser
open http://localhost:9090/metrics

# Or with curl
curl http://localhost:9090/metrics
```

### Key Metrics

- `gateway_http_requests_total` - Total requests
- `function_invocations_total` - Function calls
- `function_duration_seconds` - Execution time
- `function_errors_total` - Error count
- `functions_deployed` - Deployed count

### Prometheus Integration

Create `prometheus.yml`:
```yaml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: 'docker-faas'
    static_configs:
      - targets: ['localhost:9090']
```

**Run Prometheus:**
```bash
docker run -d \
  --name prometheus \
  -p 9091:9090 \
  -v $(pwd)/prometheus.yml:/etc/prometheus/prometheus.yml \
  prom/prometheus
```

## Common Workflows

### Development Workflow

1. **Write function code**
2. **Create Dockerfile** with of-watchdog
3. **Build image locally**
4. **Deploy to Docker FaaS**
5. **Test and iterate**
6. **Scale as needed**

### CI/CD Workflow

1. **Code pushed to Git**
2. **CI builds Docker image**
3. **CI pushes to registry**
4. **CD deploys to Docker FaaS**
5. **Health checks verify**

## Troubleshooting

### Function Not Starting

```bash
# Check Docker containers
docker ps -a | grep my-func

# Check logs
docker logs $(docker ps -q --filter "label=com.docker-faas.function=my-func")

# Check function logs via API
curl "http://localhost:8080/system/logs?name=my-func" -u admin:admin
```

### Function Timing Out

Increase timeouts in environment:
```bash
docker-compose down
# Edit docker-compose.yml and set:
# - EXEC_TIMEOUT=300s
docker-compose up -d
```

### Authentication Issues

```bash
# Re-login
faas-cli login \
  --gateway http://localhost:8080 \
  --username admin \
  --password admin

# Or disable auth temporarily
docker-compose down
# Edit docker-compose.yml and set:
# - AUTH_ENABLED=false
docker-compose up -d
```

### Network Issues

```bash
# Verify base network exists
docker network ls | grep docker-faas-net

# Inspect a function network
docker network ls | grep docker-faas-net-
docker network inspect docker-faas-net-<function-name>

# Recreate base network
docker network rm docker-faas-net
docker network create docker-faas-net

# Cleanup orphaned function networks
./scripts/cleanup-networks.sh
```

## Best Practices

### 1. Use Specific Image Tags
```bash
# Good
faas-cli deploy --image my-func:v1.2.3

# Bad
faas-cli deploy --image my-func:latest
```

### 2. Set Resource Limits
```yaml
functions:
  my-func:
    image: my-func:latest
    limits:
      memory: 256m
      cpu: "0.5"
```

### 3. Use Environment Variables
```yaml
functions:
  my-func:
    image: my-func:latest
    environment:
      DB_HOST: "postgres.example.com"
      LOG_LEVEL: "info"
```

### 4. Add Labels for Organization
```yaml
functions:
  my-func:
    image: my-func:latest
    labels:
      team: "backend"
      tier: "production"
      version: "1.0.0"
```

### 5. Monitor Your Functions
- Watch metrics regularly
- Set up alerting
- Review logs
- Track error rates

## Next Steps

- Read the [API Documentation](API.md)
- Check out [Deployment Guide](DEPLOYMENT.md)
- Explore [Architecture](ARCHITECTURE.md)
- Join the community discussions

## Getting Help

- 📚 [Documentation](../README.md)
- 🐛 [Report Issues](https://github.com/docker-faas/docker-faas/issues)
- 💬 [Discussions](https://github.com/docker-faas/docker-faas/discussions)
- 📧 Email: support@docker-faas.io

Happy function building! 🚀
