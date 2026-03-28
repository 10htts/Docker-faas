# Source Build Examples and Templates

This page lists ready-to-zip examples and starter templates for Docker FaaS source builds. Use them as the build context for zip or GitHub source builds.

For runtime-specific optimization guidance, see [Runtime Build Recipes](RUNTIME_RECIPES.md). Most examples below use the portable, built-in manifest flow. Custom `Dockerfile` examples are called out separately.

## Manifest Examples (ready to test)

Located under `examples/source-packaging/`. These examples use `docker-faas.yaml`:

1. `python-hello` - Minimal Python stdin handler.
2. `python-polars` - CSV summary using Polars.
3. `python-opencv` - Simple OpenCV edge example.
4. `go-hello` - Go stdin handler with build steps.
5. `node-hello` - Node stdin handler.
6. `bash-uppercase` - Bash example that uppercases input.

## Custom Dockerfile Examples

Located under `examples/source-packaging/`. These examples rely on a root-level `Dockerfile` instead of `docker-faas.yaml`:

1. `python-uv` - Python example that keeps custom `uv` and `ruff` choices in the function repo.

## Templates (starter packs)

Located under `examples/source-packaging/templates/`:

1. `python-basic` - Minimal Python handler.
2. `python-deps` - Python handler with `requirements.txt`.
3. `python-json` - Python handler expecting JSON input.
4. `go-basic` - Go handler with build steps.
5. `node-basic` - Node handler with `package.json`.
6. `bash-basic` - Bash handler skeleton.

These templates are intentionally minimal. If you want repo-specific Python lint policy, lockfile handling, distroless Go binaries, or a Rust build pipeline, start with a custom `Dockerfile` in your function repo and use [Runtime Build Recipes](RUNTIME_RECIPES.md) as the reference.

Before packaging the bundled Python examples or templates from this repository, run `scripts/run-python-checks.sh` or `scripts/run-python-checks.ps1` to apply the shared Ruff validation policy.

## Zip Packaging

Zip the contents of a single example or template directory (not the parent folder). The root of the zip should contain either `docker-faas.yaml` or `Dockerfile`, depending on the example.

Example:
```bash
cd examples/source-packaging/python-hello
zip -r hello-python.zip .
```
