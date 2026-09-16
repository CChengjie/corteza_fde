#!/usr/bin/env bash
set -euo pipefail
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
project_name="${C311_REAL_HTTP_PROJECT:-c311-real-http-${GITHUB_RUN_ID:-local}-${GITHUB_RUN_ATTEMPT:-1}}"
frontend_port="${C311_REAL_FRONTEND_PORT:-18092}"
app_port="${C311_REAL_APP_PORT:-18090}"
compose=(docker compose --project-name "$project_name" --env-file /dev/null --project-directory "$repo_root")
artifact_dir="${C311_ARTIFACT_DIR:-$(mktemp -d "${TMPDIR:-/tmp}/c311-real-http.XXXXXX")}"
mkdir -p "$artifact_dir"
cleanup() {
  "${compose[@]}" logs --no-color >"$artifact_dir/compose.log" 2>&1 || true
  "${compose[@]}" ps --all >"$artifact_dir/compose-ps.txt" 2>&1 || true
  "${compose[@]}" down --volumes --remove-orphans || true
}
trap cleanup EXIT
trap 'exit 143' INT TERM
# The fixture clock must be ahead of the host clock so a freshly issued
# City311 session is not reported as expired by the browser during CI.
benchmark_now="${C311_BENCHMARK_NOW:-2099-01-01T00:00:00Z}"
APP_PORT="$app_port" FRONTEND_PORT="$frontend_port" ADMIN_PORT="${C311_REAL_ADMIN_PORT:-18093}" BENCHMARK_NOW="$benchmark_now" BENCHMARK_RUN_ID="$project_name" "${compose[@]}" up --detach --build --wait --wait-timeout 180
for attempt in $(seq 1 90); do
  if curl --fail --silent "http://127.0.0.1:${app_port}/healthz" | grep --quiet '"database":"ok"'; then break; fi
  if [[ "$attempt" == 90 ]]; then "${compose[@]}" ps; exit 1; fi
  sleep 2
done
for attempt in $(seq 1 60); do
  if curl --fail --silent "http://127.0.0.1:${frontend_port}/config.js" >/dev/null; then break; fi
  if [[ "$attempt" == 60 ]]; then "${compose[@]}" ps; exit 1; fi
  sleep 2
done
C311_REAL_FRONTEND_URL="http://127.0.0.1:${frontend_port}" \
C311_REAL_ADMIN_URL="http://127.0.0.1:${C311_REAL_ADMIN_PORT:-18093}" \
C311_ARTIFACT_DIR="$artifact_dir" python3 "$repo_root/tools/c311-browser/real_http_smoke.py"
