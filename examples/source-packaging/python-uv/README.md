# python-uv

This example shows a Python function repo that uses a custom `Dockerfile` for runtime-specific optimization.

Use it when you want the function repo to own Python tooling choices such as:

- `uv` for dependency installation
- `ruff` configuration in `pyproject.toml`
- a custom Docker build sequence instead of the generic manifest runtime

## Files

- `Dockerfile` - custom source-build path used by Docker FaaS as-is
- `requirements.txt` - pinned Python dependencies
- `pyproject.toml` - repo-level Python tooling config
- `handler.py` - simple stdin handler

## Zip Packaging

Zip the contents of this directory. The zip root should contain the `Dockerfile`.

## Local Lint Example

If `ruff` is installed in your repo CI or development environment:

```bash
ruff check .
ruff format --check .
```

