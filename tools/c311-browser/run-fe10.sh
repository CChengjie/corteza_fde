#!/usr/bin/env bash
set -euo pipefail
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
artifact_dir="${C311_ARTIFACT_DIR:-$(mktemp -d "${TMPDIR:-/tmp}/c311-fe10.XXXXXX")}"
if [[ -L "$artifact_dir" ]]; then echo "artifact directory must not be a symbolic link" >&2; exit 1; fi
mkdir -p "$artifact_dir"; chmod 700 "$artifact_dir"
admin_port="${C311_ADMIN_PORT:-18120}"
compose_port="${C311_COMPOSE_PORT:-18121}"
admin_log="$artifact_dir/admin-fe10.log"
compose_log="$artifact_dir/compose-fe10.log"
admin_pid=""
compose_pid=""

cleanup () {
  [[ -z "$admin_pid" ]] || kill "$admin_pid" 2>/dev/null || true
  [[ -z "$compose_pid" ]] || kill "$compose_pid" 2>/dev/null || true
}

wait_for_server () {
  local pid="$1" port="$2" log="$3"
  for _ in $(seq 1 180); do
    if ! kill -0 "$pid" 2>/dev/null; then
      tail -80 "$log" >&2
      return 1
    fi
    if curl --fail --silent "http://127.0.0.1:$port" >/dev/null && grep -F "http://127.0.0.1:$port/" "$log" >/dev/null; then
      return 0
    fi
    sleep 1
  done
  echo "server did not start on $port" >&2
  tail -80 "$log" >&2
  return 1
}

trap cleanup EXIT INT TERM
PORT="$admin_port" corepack yarn --cwd "$repo_root/client/web/admin" serve --host 127.0.0.1 --port "$admin_port" >"$admin_log" 2>&1 & admin_pid=$!
wait_for_server "$admin_pid" "$admin_port" "$admin_log"
PORT="$compose_port" corepack yarn --cwd "$repo_root/client/web/compose" serve --host 127.0.0.1 --port "$compose_port" >"$compose_log" 2>&1 & compose_pid=$!
wait_for_server "$compose_pid" "$compose_port" "$compose_log"
C311_ADMIN_URL="http://127.0.0.1:$admin_port" C311_COMPOSE_URL="http://127.0.0.1:$compose_port" C311_ARTIFACT_DIR="$artifact_dir" PYTHONPATH="$repo_root/tools/c311-browser" python3 "$repo_root/tools/c311-browser/fe10_matrix.py"
