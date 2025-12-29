# Docker FaaS v2 Enhancements

This is a summary of the major v2 features. Details live in the linked docs.

## Highlights

- Secrets management: [SECRETS.md](SECRETS.md)
- Security hardening (cap drop, no-new-privileges): [ARCHITECTURE.md](ARCHITECTURE.md)
- Per-function network isolation and lifecycle: [ARCHITECTURE.md](ARCHITECTURE.md)
- Debug mode with port mapping and localhost bind default: [CONFIGURATION.md](CONFIGURATION.md)
- Source packaging (zip/Git) and `docker-faas.yaml`: [SOURCE_PACKAGING.md](SOURCE_PACKAGING.md)
- Database migrations and upgrade safety: [PRODUCTION_READINESS.md](PRODUCTION_READINESS.md)
- Web UI: [WEB_UI.md](WEB_UI.md)

## Compatibility

- OpenFaaS API compatible
- Works with `faas-cli`
