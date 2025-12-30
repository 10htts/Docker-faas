# Web UI

The Web UI provides a lightweight interface to deploy, monitor, and debug functions.

## Access

- URL: `http://localhost:8080/ui/`
- Login uses the gateway Basic Auth credentials.

## Main Areas

- Overview: health, counts, and quick links
- Functions: list, details, scale, invoke
- Source Build: upload zip or Git, review `docker-faas.yaml`, edit files, deploy
- Secrets: create/update/delete (values are never shown)
- Logs: fetch logs with tail length
- Builds: build history with status and output
- Metrics: gateway metrics and health checks
- Settings: read-only gateway configuration snapshot
- Debug: shows debug status and mapped ports
- Network: shows per-function network name
- Backup: export function definitions, import to restore

## Common Flows

Build from zip or Git:
1) Load Source
2) Review/edit `docker-faas.yaml`
3) Edit files if needed
4) Build and deploy

Scale and invoke:
- Scale from the function detail view
- Invoke with custom headers/body and view response

## Backup and Import

Export creates a JSON file of current function definitions (no secret values).
Import replays those definitions; ensure referenced secrets exist beforehand.

## Logs

Use the Logs panel to fetch recent output for a selected function and tail length.

## Security Notes

- Passwords are not stored; the UI uses short-lived auth tokens.
- Sessions expire after 30 minutes of inactivity.
- Secrets are not exposed in the UI.
- Debug mode surfaces a warning when enabled.
- Metrics are fetched from `/system/metrics` and require gateway auth.
- Build output is capped server-side; entries may be marked truncated.
