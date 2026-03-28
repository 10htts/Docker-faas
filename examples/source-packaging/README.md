# Source Packaging Examples

These examples are designed for the Option 1 source build flow. Each example is a full build context that contains either a `docker-faas.yaml` manifest or a root-level `Dockerfile`.

## Manifest Examples

1. `python-hello` - Minimal Python stdin handler.
2. `python-polars` - Uses Polars for CSV summarization.
3. `python-opencv` - Uses OpenCV to generate edges.
4. `go-hello` - Go stdin handler with build steps.
5. `node-hello` - Node stdin handler.
6. `bash-uppercase` - Bash example that uppercases input.

## Custom Dockerfile Examples

1. `python-uv` - Python example that keeps custom `uv` and `ruff` choices in the function repo.

## Templates

Starter templates live in `examples/source-packaging/templates/`:

1. `python-basic`
2. `python-deps`
3. `python-json`
4. `go-basic`
5. `node-basic`
6. `bash-basic`

## Zip Packaging

Zip the contents of an example directory (not the parent folder), then upload the zip file. The root of the zip should contain either `docker-faas.yaml` or `Dockerfile`.

For the Python examples in this repository, run `scripts/run-python-checks.sh` or `scripts/run-python-checks.ps1` from the repo root before packaging to validate them against the shared Ruff policy.

Example:
```bash
cd examples/source-packaging/python-hello
zip -r hello-python.zip .
```
