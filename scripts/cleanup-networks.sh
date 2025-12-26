#!/usr/bin/env bash
set -euo pipefail

LABEL_KEY="com.docker-faas.network.type=function"
GATEWAY_CONTAINER_NAME="${GATEWAY_CONTAINER_NAME:-docker-faas-gateway}"
DRY_RUN="${DRY_RUN:-false}"

networks="$(docker network ls --filter "label=${LABEL_KEY}" --format "{{.Name}}")"
if [ -z "${networks}" ]; then
  echo "No managed function networks found."
  exit 0
fi

for network in ${networks}; do
  container_names="$(docker network inspect "${network}" --format '{{range $id, $c := .Containers}}{{println $c.Name}}{{end}}')"
  container_count="$(echo "${container_names}" | sed '/^$/d' | wc -l | tr -d ' ')"
  first_container="$(echo "${container_names}" | sed '/^$/d' | head -n 1)"

  if [ "${container_count}" -eq 0 ]; then
    echo "Removing unused network: ${network}"
    if [ "${DRY_RUN}" != "true" ]; then
      docker network rm "${network}" >/dev/null
    fi
    continue
  fi

  if [ "${container_count}" -eq 1 ] && [ "${first_container}" = "${GATEWAY_CONTAINER_NAME}" ]; then
    echo "Disconnecting gateway and removing network: ${network}"
    if [ "${DRY_RUN}" != "true" ]; then
      docker network disconnect -f "${network}" "${GATEWAY_CONTAINER_NAME}" >/dev/null 2>&1 || true
      docker network rm "${network}" >/dev/null
    fi
    continue
  fi

  echo "Skipping network in use: ${network} (containers: ${container_count})"
done
