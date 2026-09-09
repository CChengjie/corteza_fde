#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
project_name="${CITY311_RUNTIME_PROJECT:-city311-runtime-${GITHUB_RUN_ID:-local}-${GITHUB_RUN_ATTEMPT:-1}}"
app_port="${CITY311_RUNTIME_PORT:-18042}"
compose=(docker compose --project-name "$project_name" --env-file /dev/null --project-directory "$repo_root")

cleanup() {
  "${compose[@]}" down --volumes --remove-orphans
}
trap cleanup EXIT

"${compose[@]}" config --quiet
APP_PORT="$app_port" "${compose[@]}" up --detach --build

APP_PORT="$app_port" "${compose[@]}" exec --no-TTY app sh -c \
  'test "$ENVIRONMENT" = production && test -z "${LOCALE_DEVELOPMENT_MODE:-}"'

health_url="http://127.0.0.1:${app_port}/healthz"
for attempt in $(seq 1 60); do
  if response="$(curl --fail --silent --show-error "$health_url" 2>/dev/null)" && \
    grep --quiet '"status":"ok"' <<<"$response" && \
    grep --quiet '"database":"ok"' <<<"$response"; then
    printf '%s\n' "$response"
    exit 0
  fi
  sleep 2
done

"${compose[@]}" ps
"${compose[@]}" logs app
echo "City 311 runtime did not report healthy within 120 seconds." >&2
exit 1
