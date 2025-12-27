# Source Build Examples and Templates

This page lists ready-to-zip examples and starter templates for the `docker-faas.yaml` flow. Use them as the build context for zip or GitHub source builds.

## Examples (ready to test)

Located under `examples/source-packaging/`:

1. `python-hello` - Minimal Python stdin handler.
2. `python-polars` - CSV summary using Polars.
3. `python-opencv` - Simple OpenCV edge example.
4. `go-hello` - Go stdin handler with build steps.
5. `node-hello` - Node stdin handler.
6. `bash-uppercase` - Bash example that uppercases input.

## Templates (starter packs)

Located under `examples/source-packaging/templates/`:

1. `python-basic` - Minimal Python handler.
2. `python-deps` - Python handler with `requirements.txt`.
3. `python-json` - Python handler expecting JSON input.
4. `go-basic` - Go handler with build steps.
5. `node-basic` - Node handler with `package.json`.
6. `bash-basic` - Bash handler skeleton.

## Zip Packaging

Zip the contents of a single example or template directory (not the parent folder). The root of the zip must contain `docker-faas.yaml`.

Example:
```bash
cd examples/source-packaging/python-hello
zip -r hello-python.zip .
```
