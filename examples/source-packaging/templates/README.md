# Source Packaging Templates

These templates are lightweight starting points for the `docker-faas.yaml` flow. Each directory is a complete build context.

They intentionally align with the built-in, generic manifest runtimes in Docker FaaS. If your function repo needs language-specific optimization such as Python `uv`, repo-level `ruff` checks, distroless Go builds, or Rust, switch to a custom `Dockerfile` and follow `docs/RUNTIME_RECIPES.md`.

## Templates

1. `python-basic` - Minimal Python handler.
2. `python-deps` - Python handler with `requirements.txt`.
3. `python-json` - Python handler that parses JSON input.
4. `go-basic` - Go handler with build steps.
5. `node-basic` - Node handler with `package.json`.
6. `bash-basic` - Bash handler skeleton.

## Zip Packaging

Zip the contents of a template directory (not the parent folder), then upload the zip file. The root of the zip should contain `docker-faas.yaml`.
