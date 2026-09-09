#!/usr/bin/env bash
set -euo pipefail
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
artifact_dir="${C311_ARTIFACT_DIR:-$(mktemp -d "${TMPDIR:-/tmp}/c311-fe10.XXXXXX")}"
if [[ -L "$artifact_dir" ]]; then echo "artifact directory must not be a symbolic link" >&2; exit 1; fi
mkdir -p "$artifact_dir"; chmod 700 "$artifact_dir"
admin_port="${C311_ADMIN_PORT:-18092}"
compose_port="${C311_COMPOSE_PORT:-18093}"
corepack yarn --cwd "$repo_root/client/web/admin" serve --port "$admin_port" >"$artifact_dir/admin-fe10.log" 2>&1 & admin_pid=$!
corepack yarn --cwd "$repo_root/client/web/compose" serve --port "$compose_port" >"$artifact_dir/compose-fe10.log" 2>&1 & compose_pid=$!
cleanup () { kill "$admin_pid" "$compose_pid" 2>/dev/null || true; }
trap cleanup EXIT INT TERM
for _ in $(seq 1 180); do
  if curl --fail --silent "http://127.0.0.1:$admin_port" >/dev/null && curl --fail --silent "http://127.0.0.1:$compose_port" >/dev/null; then break; fi
  sleep 1
done
C311_ADMIN_URL="http://127.0.0.1:$admin_port" C311_COMPOSE_URL="http://127.0.0.1:$compose_port" C311_ARTIFACT_DIR="$artifact_dir" PYTHONPATH="$repo_root/tools/c311-browser" python3 "$repo_root/tools/c311-browser/fe10_matrix.py"
