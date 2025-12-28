## Secrets Management

Docker FaaS provides comprehensive secrets management compatible with OpenFaaS, allowing you to securely store and use sensitive data in your functions.

## Overview

Secrets are:
- Stored on the host at `/var/openfaas/secrets`
- Mounted read-only into function containers at `/var/openfaas/secrets/<secret-name>`
- Managed via REST API or faas-cli
- Validated before function deployment
- Accessible as files within functions

## Creating Secrets

### Via API

```bash
curl -X POST http://localhost:8080/system/secrets \
  -u admin:admin \
  -H "Content-Type: application/json" \
  -d '{
    "name": "api-key",
    "value": "my-secret-api-key"
  }'
```

### Via faas-cli

```bash
# Create a secret from a value
echo -n "my-secret-value" | faas-cli secret create my-secret

# Create a secret from a file
faas-cli secret create db-password --from-file=./password.txt
```

### Base64 Encoding

Secrets can be provided in plain text or base64 encoded:

```bash
# Plain text
curl -X POST http://localhost:8080/system/secrets \
  -u admin:admin \
  -H "Content-Type: application/json" \
  -d '{"name": "plain-secret", "value": "hello"}'

# Base64 encoded (automatically decoded)
curl -X POST http://localhost:8080/system/secrets \
  -u admin:admin \
  -H "Content-Type: application/json" \
  -d '{"name": "encoded-secret", "value": "aGVsbG8gd29ybGQ="}'
```

## Listing Secrets

### Via API

```bash
curl http://localhost:8080/system/secrets \
  -u admin:admin
```

Response:
```json
[
  {"name": "api-key"},
  {"name": "db-password"},
  {"name": "jwt-secret"}
]
```

### Via faas-cli

```bash
faas-cli secret list
```

## Updating Secrets

### Via API

```bash
curl -X PUT http://localhost:8080/system/secrets \
  -u admin:admin \
  -H "Content-Type: application/json" \
  -d '{
    "name": "api-key",
    "value": "new-api-key-value"
  }'
```

### Via faas-cli

```bash
# Update by recreating
faas-cli secret remove api-key
echo -n "new-value" | faas-cli secret create api-key
```

## Deleting Secrets

### Via API

```bash
curl -X DELETE "http://localhost:8080/system/secrets?name=api-key" \
  -u admin:admin
```

### Via faas-cli

```bash
faas-cli secret remove api-key
```

## Using Secrets in Functions

### In stack.yml

```yaml
version: 1.0
provider:
  name: openfaas
  gateway: http://localhost:8080

functions:
  secure-function:
    image: my-function:latest
    secrets:
      - api-key
      - db-password
      - jwt-secret
```

### Via faas-cli deploy

```bash
faas-cli deploy \
  --image my-function:latest \
  --name secure-function \
  --secret api-key \
  --secret db-password
```

### Reading Secrets in Functions

Secrets are mounted as files at `/var/openfaas/secrets/<secret-name>`:

**Python:**
```python
def read_secret(name):
    with open(f'/var/openfaas/secrets/{name}', 'r') as f:
        return f.read()

api_key = read_secret('api-key')
db_password = read_secret('db-password')
```

**Node.js:**
```javascript
const fs = require('fs');

function readSecret(name) {
    return fs.readFileSync(`/var/openfaas/secrets/${name}`, 'utf8');
}

const apiKey = readSecret('api-key');
const dbPassword = readSecret('db-password');
```

**Go:**
```go
package main

import (
    "io/ioutil"
)

func readSecret(name string) (string, error) {
    data, err := ioutil.ReadFile("/var/openfaas/secrets/" + name)
    if err != nil {
        return "", err
    }
    return string(data), nil
}

func main() {
    apiKey, _ := readSecret("api-key")
    dbPassword, _ := readSecret("db-password")
}
```

**Bash:**
```bash
#!/bin/bash

API_KEY=$(cat /var/openfaas/secrets/api-key)
DB_PASSWORD=$(cat /var/openfaas/secrets/db-password)

echo "Using API key: ${API_KEY}"
```

## Secret Validation

Docker FaaS validates that all required secrets exist before deploying a function. Missing secrets are auto-created as empty placeholders (with a warning), so the deployment can proceed:

```bash
# Create secrets first
echo -n "key123" | faas-cli secret create api-key

# Deploy function with secret
faas-cli deploy -f stack.yml

# If a secret is missing, the gateway auto-creates it as an empty file and logs a warning.
# Update the secret value after deployment if your function depends on it.
```

## Security Features

### File Permissions
- Secrets are stored with 0400 permissions (owner read-only)
- Only the gateway process can read secret files
- Function containers receive read-only bind mounts

### Container Access
- Secrets are mounted read-only into containers
- Containers cannot modify or delete secrets
- Each container only sees its declared secrets

### API Security
- All secret operations require authentication
- Secret values are never returned via GET requests
- Only secret names are exposed in list operations

## Example: Complete Workflow

### 1. Create Secrets

```bash
# Create database credentials
echo -n "mydb.example.com" | faas-cli secret create db-host
echo -n "admin" | faas-cli secret create db-user
echo -n "super-secret-password" | faas-cli secret create db-password

# Create API key
echo -n "sk-1234567890abcdef" | faas-cli secret create api-key
```

### 2. Create Function

**handler.py:**
```python
import os

def read_secret(name):
    path = f'/var/openfaas/secrets/{name}'
    if os.path.exists(path):
        with open(path, 'r') as f:
            return f.read().strip()
    return None

def handle(req):
    db_host = read_secret('db-host')
    db_user = read_secret('db-user')
    db_password = read_secret('db-password')
    api_key = read_secret('api-key')

    return f"Connected to {db_host} as {db_user} with API key {api_key[:10]}..."
```

### 3. Deploy with Secrets

**stack.yml:**
```yaml
version: 1.0
provider:
  name: openfaas
  gateway: http://localhost:8080

functions:
  db-connector:
    lang: python3
    handler: ./db-connector
    image: db-connector:latest
    secrets:
      - db-host
      - db-user
      - db-password
      - api-key
```

```bash
faas-cli deploy -f stack.yml
```

### 4. Test Function

```bash
curl -X POST http://localhost:8080/function/db-connector \
  -u admin:admin \
  -d "test"
```

## Best Practices

### 1. Never Commit Secrets
```bash
# DON'T do this
git add secrets.txt
git commit -m "Added secrets"

# DO this
echo "secrets.txt" >> .gitignore
```

### 2. Use Separate Secrets for Each Environment
```bash
# Development
faas-cli secret create api-key-dev --from-literal="dev-key"

# Production
faas-cli secret create api-key-prod --from-literal="prod-key"
```

### 3. Rotate Secrets Regularly
```bash
# Update secret with new value
faas-cli secret remove api-key
echo -n "new-rotated-key" | faas-cli secret create api-key

# Redeploy functions to pick up new secret
faas-cli deploy -f stack.yml
```

### 4. Validate Secrets Exist
```bash
# Check secrets before deployment
faas-cli secret list | grep -q "api-key" || echo "Missing api-key!"
```

### 5. Use Descriptive Names
```bash
# Good
faas-cli secret create stripe-api-key
faas-cli secret create postgres-password
faas-cli secret create jwt-signing-key

# Bad
faas-cli secret create secret1
faas-cli secret create password
faas-cli secret create key
```

## Troubleshooting

### Secret Not Found Error

**Error:** `secret validation failed: missing secrets: [api-key]`

**Solution:**
```bash
# Check if secret exists
faas-cli secret list

# Create missing secret
echo -n "value" | faas-cli secret create api-key
```

### Permission Denied Reading Secret

**Error:** `open /var/openfaas/secrets/api-key: permission denied`

**Solution:**
- Ensure the secret was created correctly
- Check function has the secret declared in stack.yml
- Verify the secret name matches exactly

### Secret Not Mounted in Container

**Issue:** Secret file doesn't exist in `/var/openfaas/secrets/`

**Solution:**
```bash
# Verify secret exists
faas-cli secret list

# Redeploy function
faas-cli deploy -f stack.yml --update=true
```

## API Reference

### POST /system/secrets
Create a new secret

**Request:**
```json
{
  "name": "my-secret",
  "value": "secret-value"
}
```

**Response:** `201 Created`

### GET /system/secrets
List all secrets

**Response:**
```json
[
  {"name": "secret1"},
  {"name": "secret2"}
]
```

### GET /system/secrets/{name}
Check if secret exists

**Response:** `200 OK` or `404 Not Found`

### PUT /system/secrets
Update an existing secret

**Request:**
```json
{
  "name": "my-secret",
  "value": "new-value"
}
```

**Response:** `200 OK`

### DELETE /system/secrets?name={name}
Delete a secret

**Response:** `204 No Content`

## Migration from Environment Variables

If you're currently using environment variables for secrets, migrate to the secrets system:

**Before (insecure):**
```yaml
functions:
  my-func:
    image: my-func:latest
    environment:
      API_KEY: "sk-1234567890"  # Visible in logs, process lists
      DB_PASSWORD: "secret123"   # Stored in container config
```

**After (secure):**
```yaml
functions:
  my-func:
    image: my-func:latest
    secrets:
      - api-key        # Mounted as file
      - db-password    # Not visible in logs
```

**Update function code:**
```python
# Before
import os
api_key = os.environ.get('API_KEY')

# After
def read_secret(name):
    with open(f'/var/openfaas/secrets/{name}', 'r') as f:
        return f.read().strip()

api_key = read_secret('api-key')
```

## See Also

- [OpenFaaS Secrets Documentation](https://docs.openfaas.com/reference/secrets/)
- [Security Best Practices](DEPLOYMENT.md#security-hardening)
- [Function Development Guide](GETTING_STARTED.md)
