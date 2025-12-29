# Secrets Management

Docker FaaS stores secrets as files on the host and mounts them read-only into function containers.

## Storage

- Host path: `/var/openfaas/secrets`
- Container path: `/var/openfaas/secrets`
- File permissions: `0400`
- API never returns secret values

## API Endpoints

- `POST /system/secrets` (create)
- `PUT /system/secrets` (update)
- `DELETE /system/secrets?name=...` (delete)
- `GET /system/secrets` (list)
- `GET /system/secrets/{name}` (exists)

## Examples

Create a secret:
```bash
curl -X POST http://localhost:8080/system/secrets   -u admin:admin   -H "Content-Type: application/json"   -d '{"name":"api-key","value":"secret-value"}'
```

Use in a deployment:
```yaml
functions:
  my-func:
    image: my-func:latest
    secrets:
      - api-key
```

Read inside a function:
```python
import os
print(open("/var/openfaas/secrets/api-key").read().strip())
```

## Missing Secrets

If a function references missing secrets, Docker FaaS creates empty files by default to keep deployment moving. For production, create secrets ahead of time and treat empty secrets as a configuration error.
