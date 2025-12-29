# Docker-FaaS Enhancement Report

## 1. Project Status Overview
**Date:** 2025-12-23
**Status:** Functional Alpha
**Scope:** Single-node, Docker-backed serverless platform.

The current implementation provides a solid foundation with a working Gateway, SQLite-based persistence, and a Docker provider that handles basic lifecycle (Deploy, Invoke, Scale, Remove). API compatibility with OpenFaaS is partially achieved for synchronous operations.

## 2. Critical Functional Corrections (Parity Gaps)

### 2.1 Secrets Management (CRITICAL)
*   **Current State:** Secrets are accepted in the Deployment API and stored in the SQLite database (`pkg/store/store.go`), but they are **never injected** into the containers.
*   **Correction Needed:** 
    *   In `pkg/provider/docker_provider.go`, `createContainer` must inject secrets.
    *   **Standard:** OpenFaaS standard is to mount secrets as files in `/var/openfaas/secrets/<secret-name>`.
    *   **Action:** Retrieve secret values (needs a new Secrets Store or table) and mount them as temporary files or Docker Swarm secrets (if available) or tmpfs mounts.

### 2.2 Asynchronous Invocation
*   **Current State:** Only synchronous invocation (`/function/{name}`) is supported.
*   **Correction Needed:** 
    *   Implement `/async-function/{name}` endpoint.
    *   **Implementation:** Since this is a single-node alternative, a heavy queue like NATS might be overkill but is the standard. For a lightweight alternative, an in-memory buffered channel with a worker pool or a Redis-backed queue could be used to decouple the request from execution.
    *   **Callback:** Implement `X-Callback-Url` handling.

### 2.3 Health Checking
*   **Current State:** "Running" status is determined solely by Docker container state (`Up`).
*   **Correction Needed:** 
    *   Implement application-level health checks. The gateway should probe `http://<container_ip>:8080/_/health`.
    *   Do not route traffic to containers until they pass this health check (Readiness Probe).

## 3. Strong Isolation Enhancements (Security)

The user requirement for "strong isolation" is not met by standard Docker containers.

### 3.1 Kernel-Level Isolation (Runtime)
*   **Proposal:** Support **gVisor (`runsc`)** or **Kata Containers**.
*   **Action:** Add a `runtime` field to `FunctionDeployment`. If set to "hardened", `docker_provider` should set `HostConfig.Runtime = "runsc"`.
*   **Pre-requisite:** User must have `runsc` installed on the host.

### 3.2 Container Hardening (Default)
Even without gVisor, standard Docker containers should be hardened:
*   **Action:** Modify `createContainer` in `docker_provider.go`:
    *   **Capabilities:** Drop all capabilities by default (`CapDrop: []string{"ALL"}`). Only add back what is strictly necessary (e.g., `NET_BIND_SERVICE`).
    *   **Privileges:** Set `SecurityOpt: []string{"no-new-privileges:true"}`.
    *   **Seccomp:** Apply a strict Seccomp profile.
    *   **User Namespaces:** Enable `UserNSMode` if supported by the host configuration.

### 3.3 Network Isolation
*   **Current State:** All functions share a single bridge network `docker-faas`. This allows lateral movement (function-to-function attacks).
*   **Correction Needed:** 
    *   Use per-function networks derived from the base `FUNCTIONS_NETWORK` value.
    *   Connect the gateway to each function network so it can route traffic without allowing function-to-function communication.

## 4. Debugging Capabilities

To "enable debugging" as requested:

### 4.1 Debug Mode Deployment
*   **Proposal:** Add a `--debug` or `debug: true` flag to deployment.
*   **Effect:**
    *   **Port Mapping:** Automatically map the language-specific debugger port (e.g., 40000 for Delve/Go, 5678 for Python/PTVSD) from the container to a random host port.
    *   **Timeout:** Override the standard timeout (default 60s) to essentially infinite (e.g., 1 hour) to allow for breakpoints.
    *   **Readiness:** Disable strict readiness checks which might kill a paused container.

### 4.2 Log Streaming
*   **Current State:** Logs are fetched via polling (`/system/logs`).
*   **Correction:** Implement a WebSocket endpoint for real-time log streaming, similar to `docker logs -f`.

## 5. Architectural Improvements

### 5.1 Provider Interface
*   **Proposition:** Define a strict Go `interface` for the Provider.
*   **Reason:** Allows swapping `DockerProvider` for `ContainerdProvider` or `FirecrackerProvider` in the future without changing Gateway logic.

### 5.2 Hot/Cold Starts (Scale-to-Zero)
*   **Current State:** Manual scaling only.
*   **Proposition:** Implement an "Idler" routine.
    *   If a function hasn't been invoked in `X` minutes, scale to 0.
    *   On invoke, if replicas=0, hold the request, scale to 1, wait for readiness, then forward.

## 6. Implementation Plan (Prioritized)

1.  **Hardening:** Implement CapDrop and no-new-privileges immediately. (Low effort, high value).
2.  **Secrets:** Fix the secrets gap (Critical functionality).
3.  **Debug Mode:** Add the `debug` field to the struct and handle port mapping.
4.  **Network Isolation:** Use per-function networks and connect the gateway to each.
