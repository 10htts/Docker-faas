# Docker FaaS UI - Requirements and Mock Prompt

## UI should accomplish

1) **Auth and session**
- Basic auth login for the gateway.
- Show current gateway URL and auth status.

2) **System overview**
- Health status and version info.
- Counts: deployed functions, replicas, errors, invocations.
- Quick links to docs and CLI commands.

3) **Functions list**
- Table of functions with name, image, replicas, status, debug flag, last updated.
- Filter/search by name, label, image.
- Sort by name, replicas, status, updated time.

4) **Function detail**
- Summary: image, network, replicas, limits/requests, read-only FS, debug flag.
- Containers: replica list, status, ports (incl. debug mappings).
- Invocations: last N results with status codes and latency (from metrics).

5) **Create function**
- Form compatible with OpenFaaS deploy payload.
- Fields: service, image, env vars, labels, secrets, limits, requests, read-only, debug.
- Validation: required fields, JSON map inputs.
- Submit to `/system/functions` with preview of request JSON.

6) **Update function**
- Edit existing function fields and apply update (PUT `/system/functions`).
- Clear warnings for disruptive changes (image, env, replicas).

7) **Scale function**
- Control replicas via slider/stepper (POST `/system/scale-function/{name}`).
- Show current replicas and status.

8) **Invoke function**
- Inline request builder (method, headers, body).
- Show response status, headers, body, and latency.

9) **Logs**
- Fetch logs (`/system/logs`) with tail selector.
- Optional auto-refresh or manual refresh.

10) **Debug view**
- Show debug status and port bindings.
- Display safety warning when debug ports are exposed broadly.
- Provide connection hints (host:port).

11) **Secrets**
- Create/update/delete secrets via `/system/secrets`.
- List secrets and show metadata (not values).

12) **Network view**
- Display function network name and gateway attachment info.
- Link to cleanup guidance.

13) **Status and errors**
- Clear error states for failed deploys/updates.
- Show raw API errors and suggested actions.

14) **Read-only mode**
- Optional toggle to disable write actions for safer use.

## Mock prompt for UI designer

Design a small, production-focused web UI for "Docker FaaS" (OpenFaaS-compatible gateway). The UI should be compact, utilitarian, and developer-centric with clear status indicators and minimal friction. Avoid default fonts and generic layouts. Use a bold, purposeful typographic style and an atmospheric background (gradient or subtle pattern). No purple on white.

Target screens:
- Login + Gateway configuration (basic auth)
- Overview dashboard (health, version, counts, recent activity)
- Functions list (search, filters, status chips)
- Function detail (summary, replicas, ports, debug info, invoke panel)
- Create/Update function form (OpenFaaS-compatible fields)
- Logs view (tail selector, refresh)
- Secrets management (create/update/delete)

Key interactions:
- Deploy, update, scale, delete functions
- Invoke function with custom headers/body and see response + latency
- Toggle debug mode and display port mappings
- Show warnings when debug ports are exposed beyond localhost

Data from API:
- `/system/info`, `/system/functions`, `/system/scale-function/{name}`, `/system/logs`, `/system/secrets`, `/function/{name}`

Style requirements:
- Distinct sectioning with strong hierarchy.
- Compact tables, clear status badges.
- Subtle motion for page-load and state changes.
- Mobile-friendly layout.

Deliverable:
- A single-page mockup with sections for the screens above, focused on readability and operational clarity.
