# Docker-FaaS Specification (OpenFaaS-Compatible, Docker-Based)

> Archived document. This snapshot is retained for historical context and may be outdated.
> For current documentation, see ../README.md.


This document defines the requirements for a new project that provides a Docker-native FaaS runtime compatible with `faas-cli`. The goal is a simple, reliable, debuggable system that runs on Docker (no Kubernetes required) but can migrate to Kubernetes later.

---

## 1. Goals

- Run a FaaS platform on Docker without Kubernetes.
- Be compatible with `faas-cli` and OpenFaaS function contracts.
- Support dynamic deploys (deploy/update/remove/list functions at runtime).
- Keep operations simple for IT (clear logs, few moving parts).
- Provide a migration path to OpenFaaS on Kubernetes later.

---

## 2. Non-Goals

- Full Kubernetes feature parity.
- Multi-tenant isolation across untrusted tenants.
- Serverless autoscaling to zero for very large scale.
- Advanced event sources (Kafka, NATS, etc.) in the first version.

---

## 3. Compatibility Requirements (OpenFaaS / faas-cli)

The platform must implement the OpenFaaS Gateway HTTP API used by `faas-cli`.

Minimum required endpoints:
- `GET /system/info`
- `GET /system/functions`
- `POST /system/functions` (deploy new)
- `PUT /system/functions` (update existing)
- `DELETE /system/functions` (remove)
- `POST /system/scale-function` (scale)
- `GET /system/logs+name=...` (logs)
- `POST /function/{name}` (invoke function)
- `GET /healthz` (gateway health)

Nice-to-have:
- `GET /system/metrics` (Prometheus)
- `GET /system/function/{name}` (details)
- `GET /system/providers` (provider info)

The gateway must accept OpenFaaS function specs (same JSON payloads `faas-cli` sends).

Example: `faas-cli deploy -f stack.yml` should work unchanged.

---

## 4. High-Level Architecture

### 4.1 Components

1. **Gateway API**
   - Implements OpenFaaS-compatible endpoints.
   - Validates requests, stores state, and orchestrates containers.

2. **Provider (Docker Scheduler)**
   - Talks to Docker API (socket or TCP).
   - Creates, updates, scales, and removes containers.
   - Manages networks, labels, and environment variables.

3. **Function Router**
   - Routes `POST /function/{name}` to the correct container.
   - Supports load balancing across replicas.
   - Handles timeouts and retries.

4. **State Store**
   - Stores function metadata, scale settings, and config.
   - SQLite is acceptable for MVP; Postgres optional.

5. **Log Collector**
   - Streams container logs via Docker API.
   - Exposes logs via `GET /system/logs`.

### 4.2 Process Layout (Single Node)

- `docker-faas-gateway` container
- `docker-faas-provider` container (or combined into gateway)
- Function containers (one per function, scaled horizontally)

---

## 5. Function Runtime Contract

The function runtime must be compatible with OpenFaaS function containers:
- `of-watchdog` is used as the entrypoint.
- Input is passed as HTTP body.
- Output is HTTP response body.
- Use the standard OpenFaaS function request/response pattern.

The router must forward:
- Method, headers, body.
- Support large bodies (configurable limit).
- Apply timeouts (`read_timeout`, `write_timeout`, `exec_timeout`).

---

## 6. Scaling Behavior

MVP scaling:
- `scale.min` and `scale.max` supported.
- `POST /system/scale-function` updates replicas.
- Round-robin across replicas.

Optional:
- Scale-to-zero based on idle time.
- Basic autoscaler based on CPU or QPS.

---

## 7. Security Requirements

Minimum:
- Gateway protected with Basic Auth (`OPENFAAS_USER/PASSWORD` style).
- TLS support via reverse proxy (Caddy/Nginx/Traefik).
- Function invocation must require auth (unless explicitly disabled).
- Secrets should be supported as env vars or mounted files.

Optional:
- JWT auth for function invocation.
- Signed deploy requests.

---

## 8. Observability

Logs:
- Gateway logs (requests, deploys, errors).
- Function logs (via Docker logs API).

Metrics (Prometheus):
- `gateway_http_requests_total`
- `function_invocations_total`
- `function_duration_seconds`
- `function_errors_total`

Health:
- `GET /healthz` for gateway
- `GET /system/info` for overall status

---

## 9. Configuration and Defaults

Environment variables:
- `DOCKER_HOST` (default: unix socket)
- `GATEWAY_PORT` (default: 8080)
- `FUNCTIONS_NETWORK` (default: docker-faas-net; used as a prefix for per-function networks)
- `AUTH_USER`, `AUTH_PASSWORD`
- `DEFAULT_TIMEOUTS` (read/write/exec)
- `STATE_DB_PATH`

Networking:
- Each function runs on its own Docker network derived from `FUNCTIONS_NETWORK`.
- Gateway connects to each function network for routing.

---

## 10. Deployment Model

### 10.1 Docker Compose (Dev)

Services:
- `docker-faas-gateway`
- `docker-faas-provider` (optional separate)
- `redis` (optional)

### 10.2 Production (Single Node)

Run gateway and provider on a Docker host.
Use a reverse proxy for TLS and auth hardening.

---

## 11. Migration Path to Kubernetes

To migrate later:
- Keep `stack.yml` and `faas-cli` usage identical.
- Replace this runtime with OpenFaaS on K8s.
- Deploy functions to K8s with the same `faas-cli` commands.

---

## 12. Example User Flows

### Deploy
```
faas-cli login --gateway http://localhost:15012
faas-cli deploy --gateway http://localhost:15012 -f stack.yml
```

### Invoke
```
curl -X POST http://localhost:15012/function/http-request -d '{"foo":"bar"}'
```

### Scale
```
faas-cli scale http-request --gateway http://localhost:15012 --replicas 3
```

---

## 13. MVP Scope Checklist

- [ ] Gateway implements required OpenFaaS endpoints
- [ ] Docker provider creates/removes function containers
- [ ] Router forwards requests to functions
- [ ] Logs are available via `/system/logs`
- [ ] `faas-cli deploy` works with `stack.yml`
- [ ] Basic auth for gateway
- [ ] Minimal metrics/health endpoints

---

## 14. Future Enhancements

- Async invocations / queue-worker pattern
- Autoscaling based on load
- Secrets integration (Vault)
- Function build API (build images from source)
- Multi-node support (swarm-like)

---

## 15. Open Questions

- Do we require function image builds in this platform, or only deploy prebuilt images+
- Should functions run with strict resource limits by default+
- Do we need scale-to-zero now, or later+
- What level of security is required for function invocation+
