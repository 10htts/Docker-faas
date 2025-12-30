package builder

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const fwatchdogVersion = "0.11.0"

// GenerateDockerfile builds a Dockerfile from a manifest and runtime.
func GenerateDockerfile(manifest *Manifest, contextDir string) (string, error) {
	if manifest == nil {
		return "", fmt.Errorf("manifest is required to generate Dockerfile")
	}

	runtime := strings.ToLower(manifest.Runtime)
	if runtime == "" {
		return "", fmt.Errorf("runtime is required in docker-faas.yaml")
	}

	switch runtime {
	case "python":
		return pythonDockerfile(manifest, contextDir), nil
	case "node":
		return nodeDockerfile(manifest, contextDir), nil
	case "go":
		return goDockerfile(manifest), nil
	case "bash":
		return bashDockerfile(manifest), nil
	default:
		return "", fmt.Errorf("unsupported runtime: %s", runtime)
	}
}

func pythonDockerfile(manifest *Manifest, contextDir string) string {
	installDeps := ""
	if hasFile(contextDir, "requirements.txt") || includesDependency(manifest.Dependencies, "requirements.txt") {
		installDeps = "RUN pip install --no-cache-dir -r requirements.txt\n"
	}

	return fmt.Sprintf(`FROM python:3.11-slim

WORKDIR /home/app
COPY . .
%s
RUN apt-get update && apt-get install -y curl ca-certificates && rm -rf /var/lib/apt/lists/*
RUN ARCH="$(uname -m)" && \
  case "$ARCH" in \
    x86_64|amd64) WATCHDOG="fwatchdog-amd64" ;; \
    aarch64|arm64) WATCHDOG="fwatchdog-arm64" ;; \
    armv7l|armv7|armhf) WATCHDOG="fwatchdog-arm" ;; \
    *) echo "Unsupported arch: $ARCH" >&2; exit 1 ;; \
  esac && \
  curl -sSL -o /usr/local/bin/fwatchdog "https://github.com/openfaas/of-watchdog/releases/download/%s/${WATCHDOG}" && \
  chmod +x /usr/local/bin/fwatchdog

ENV fprocess="%s"
ENV mode="streaming"

EXPOSE 8080
CMD ["fwatchdog"]
`, installDeps, fwatchdogVersion, manifest.Command)
}

func nodeDockerfile(manifest *Manifest, contextDir string) string {
	installCmd := ""
	if hasFile(contextDir, "package-lock.json") {
		installCmd = "RUN npm ci\n"
	} else if hasFile(contextDir, "package.json") || includesDependency(manifest.Dependencies, "package.json") {
		installCmd = "RUN npm install\n"
	}

	return fmt.Sprintf(`FROM node:20-slim

WORKDIR /home/app
COPY . .
%s
RUN apt-get update && apt-get install -y curl ca-certificates && rm -rf /var/lib/apt/lists/*
RUN ARCH="$(uname -m)" && \
  case "$ARCH" in \
    x86_64|amd64) WATCHDOG="fwatchdog-amd64" ;; \
    aarch64|arm64) WATCHDOG="fwatchdog-arm64" ;; \
    armv7l|armv7|armhf) WATCHDOG="fwatchdog-arm" ;; \
    *) echo "Unsupported arch: $ARCH" >&2; exit 1 ;; \
  esac && \
  curl -sSL -o /usr/local/bin/fwatchdog "https://github.com/openfaas/of-watchdog/releases/download/%s/${WATCHDOG}" && \
  chmod +x /usr/local/bin/fwatchdog

ENV fprocess="%s"
ENV mode="streaming"

EXPOSE 8080
CMD ["fwatchdog"]
`, installCmd, fwatchdogVersion, manifest.Command)
}

func goDockerfile(manifest *Manifest) string {
	buildSteps := manifest.Build
	if len(buildSteps) == 0 {
		buildSteps = []string{"go mod download", "go build -o app ./"}
	}

	buildLines := ""
	for _, step := range buildSteps {
		buildLines += fmt.Sprintf("RUN %s\n", step)
	}

	return fmt.Sprintf(`FROM golang:1.22 AS builder

WORKDIR /src
COPY . .
%s

FROM debian:bookworm-slim
WORKDIR /home/app
RUN apt-get update && apt-get install -y curl ca-certificates && rm -rf /var/lib/apt/lists/*
RUN ARCH="$(uname -m)" && \
  case "$ARCH" in \
    x86_64|amd64) WATCHDOG="fwatchdog-amd64" ;; \
    aarch64|arm64) WATCHDOG="fwatchdog-arm64" ;; \
    armv7l|armv7|armhf) WATCHDOG="fwatchdog-arm" ;; \
    *) echo "Unsupported arch: $ARCH" >&2; exit 1 ;; \
  esac && \
  curl -sSL -o /usr/local/bin/fwatchdog "https://github.com/openfaas/of-watchdog/releases/download/%s/${WATCHDOG}" && \
  chmod +x /usr/local/bin/fwatchdog

COPY --from=builder /src /home/app

ENV fprocess="%s"
ENV mode="streaming"

EXPOSE 8080
CMD ["fwatchdog"]
`, buildLines, fwatchdogVersion, manifest.Command)
}

func bashDockerfile(manifest *Manifest) string {
	return fmt.Sprintf(`FROM debian:bookworm-slim

WORKDIR /home/app
COPY . .
RUN apt-get update && apt-get install -y bash curl ca-certificates && rm -rf /var/lib/apt/lists/*
RUN ARCH="$(uname -m)" && \
  case "$ARCH" in \
    x86_64|amd64) WATCHDOG="fwatchdog-amd64" ;; \
    aarch64|arm64) WATCHDOG="fwatchdog-arm64" ;; \
    armv7l|armv7|armhf) WATCHDOG="fwatchdog-arm" ;; \
    *) echo "Unsupported arch: $ARCH" >&2; exit 1 ;; \
  esac && \
  curl -sSL -o /usr/local/bin/fwatchdog "https://github.com/openfaas/of-watchdog/releases/download/%s/${WATCHDOG}" && \
  chmod +x /usr/local/bin/fwatchdog

ENV fprocess="%s"
ENV mode="streaming"

EXPOSE 8080
CMD ["fwatchdog"]
`, fwatchdogVersion, manifest.Command)
}

func includesDependency(deps []string, name string) bool {
	for _, dep := range deps {
		if dep == name {
			return true
		}
	}
	return false
}

func hasFile(contextDir, name string) bool {
	_, err := os.Stat(filepath.Join(contextDir, name))
	return err == nil
}
