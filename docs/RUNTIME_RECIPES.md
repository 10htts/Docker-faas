# Runtime Build Recipes

This guide covers efficient source-build patterns for Docker FaaS by runtime.

The key boundary is simple:

- Docker FaaS owns the generic source-build flow and the built-in manifest runtimes.
- Your function repository owns language-specific tooling choices such as `uv`, `ruff`, `pnpm`, custom build caches, or release-only binaries.

If you need language-specific optimization, use a custom `Dockerfile` in the function repo. Docker FaaS will use it as-is.

## Choose the Right Build Path

| Use case | Recommended path |
| --- | --- |
| Quick start, simple handler, minimal dependencies | `docker-faas.yaml` manifest |
| Standard Python, Go, Node, or Bash example | Built-in manifest runtime |
| Runtime-specific tooling (`uv`, `ruff`, `pnpm`, distroless, Rust/Cargo) | Custom `Dockerfile` |
| Tight image-size or cold-start tuning | Custom `Dockerfile` |
| A runtime not built into Docker FaaS | Custom `Dockerfile` |

## General Build Rules

Regardless of runtime:

1. Keep the build context small. Add a `.dockerignore` and exclude local caches, test output, VCS metadata, and virtual environments.
2. Pin base images and dependencies in the function repo. Reproducible builds matter more than shaving a few seconds off one local build.
3. Put dependency installation before application copies where possible so Docker can reuse layers.
4. Use lockfiles when the ecosystem supports them.
5. Prefer multi-stage builds when the runtime compiler or package manager does not belong in production.
6. Keep linting and formatting in the function repo CI, not in Docker FaaS itself.

## Built-In Runtime Behavior

Current manifest-generated builds behave like this:

| Runtime | Built-in manifest support | Current generated behavior | Move to custom `Dockerfile` when... |
| --- | --- | --- | --- |
| Python | Yes | `python:3.11-slim`; installs `requirements.txt` with `pip` when present | You want `uv`, bytecode pre-compilation, wheel caching, OS packages, or repo-specific lint gates |
| Go | Yes | `golang:1.22` build stage with default `go mod download` and `go build -o app ./` | You want a smaller runtime image, a static binary, or stricter release flags |
| Node | Yes | `node:20-slim`; uses `npm ci` when `package-lock.json` exists, otherwise `npm install` | You want `pnpm`, Yarn, native build packages, or aggressive dependency pruning |
| Bash | Yes | `debian:bookworm-slim` with Bash and `of-watchdog` | You need a different shell toolchain or a smaller custom base image |
| Rust | No | Not built in today | Always use a custom `Dockerfile` |

## Python

Use the manifest runtime for simple handlers and examples. It is intentionally generic and easy to understand.

Move to a custom `Dockerfile` when your function repo needs:

- `uv` instead of `pip`
- `ruff` checks or formatting in repo CI
- Pre-built wheels or bytecode compilation
- Native system packages
- A tighter cold-start or image-size budget

Recommended repo-level process:

1. Keep Python dependency and lint configuration in the function repo.
2. Run lint and format checks before publishing a zip or Git source build.
3. Use a custom `Dockerfile` if the build must use `uv` or a non-default Python packaging flow.

Practical guidance:

- Put `pyproject.toml`, `requirements.txt`, and any lockfile in the function repo.
- Treat `ruff` as a repo policy, not a Docker FaaS policy.
- Keep generated caches and virtual environments out of the build context.
- If you need `uv`, make that part of the function repo's `Dockerfile` and CI, not the platform default for every user.

Bundled example:

- `examples/source-packaging/python-uv/` shows a custom-Dockerfile Python source build that keeps `uv` and `ruff` ownership in the function repo.

## Go

The built-in Go manifest runtime is already a good default for small handlers.

Use the manifest runtime when:

- `go mod download` plus `go build -o app ./` is enough
- The handler is small and you want a low-friction source-build path

Move to a custom `Dockerfile` when you need:

- A distroless or scratch runtime image
- Static linking or stricter `CGO_ENABLED` control
- Custom compiler flags, private modules, or OS-level build dependencies

Practical guidance:

- Keep `go.mod` and `go.sum` stable so the module-download layer stays cacheable.
- Copy module files before application source in custom Dockerfiles.
- Strip the runtime image down to the final binary when image size matters.

## Node

The built-in Node manifest runtime is suitable for many simple functions.

Use the manifest runtime when:

- `package-lock.json` is present and `npm ci` is enough
- You do not need a custom package manager or native build toolchain

Move to a custom `Dockerfile` when you need:

- `pnpm` or Yarn
- Native dependencies that require OS packages at build time
- More aggressive pruning between build and runtime stages

Practical guidance:

- Commit the lockfile so the generated build uses `npm ci`.
- Keep dev-only tooling out of the runtime image when using multi-stage builds.
- Avoid copying `node_modules` from the host into the build context.

## Bash

The built-in Bash runtime is intentionally simple. It is usually good enough as-is.

Move to a custom `Dockerfile` only if:

- The script depends on extra CLI tools
- You want a smaller or more specialized base image
- You need a shell other than the default setup

## Rust

Rust is a strong candidate for Docker FaaS, but it is not a built-in manifest runtime today.

For Rust functions:

1. Put a `Dockerfile` at the repo root.
2. Use a multi-stage build so Cargo stays out of the runtime image.
3. Cache dependency resolution separately from application source where possible.
4. Copy only the final release binary into the runtime stage.

If you want Rust to become a first-class `docker-faas.yaml` runtime later, treat that as a separate platform feature with its own template, tests, and documentation.

## What Belongs in This Repo

Good candidates for Docker FaaS itself:

- Documentation for runtime-specific build strategies
- Example manifests and `Dockerfile` recipes
- Additional built-in runtimes only when the project is ready to maintain them

Better owned by downstream function repositories:

- Language-specific lint and formatting rules
- Custom package manager choices
- Repo-level CI quality gates
- App-specific build flags and dependency policies
