# Releasing Docker FaaS

This guide describes the manual release process for tagged versions.

## Prerequisites

- All tests passing (`scripts/run-tests.sh` or `scripts/run-tests.ps1`)
- Updated documentation and configuration references
- Version and changelog ready

## Release Steps

1) Update version strings:
   - `pkg/gateway/handlers.go` (`HandleSystemInfo` returns release/version info)
   - Any docs that reference the version number

2) Update `CHANGELOG.md`:
   - Move items from `[Unreleased]` into a new version section.
   - Add a new compare link at the bottom.

3) Build and verify artifacts:
   - Run `scripts/run-tests.sh` or `scripts/run-tests.ps1`.
   - Build the gateway binary (`go build ./cmd/gateway`).

4) Tag and publish:
   - `git tag -a vX.Y.Z -m "Release vX.Y.Z"`
   - `git push origin vX.Y.Z`

5) Create a GitHub release:
   - Include changelog notes and any upgrade steps.

## Post-Release

- Update any deployment manifests that pin versions.
- Announce the release in your preferred channels.
