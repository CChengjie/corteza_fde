#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "$repo_root/tools/c311-browser/process-tree.sh"
# Keep FE-03 isolated from the FE-01/FE-02 browser helpers, which use
# 18081/18082 and may leave a development-server child process behind.
compose_port="${C311_COMPOSE_PORT:-18084}"
admin_port="${C311_ADMIN_PORT:-18085}"
artifact_dir="${C311_ARTIFACT_DIR:-$(mktemp -d \
  "${TMPDIR:-/tmp}/c311-fe03.XXXXXX")}"

if [[ -L "$artifact_dir" ]]; then
  echo "artifact directory must not be a symbolic link" >&2
  exit 1
fi
mkdir -p "$artifact_dir"
chmod 700 "$artifact_dir"
cleanup () {
  terminate_process_tree "${compose_pid:-}"
  terminate_process_tree "${admin_pid:-}"
}
trap cleanup EXIT INT TERM

corepack yarn --cwd "$repo_root/client/web/compose" serve --port "$compose_port" >"$artifact_dir/compose-fe03.log" 2>&1 & compose_pid=$!
corepack yarn --cwd "$repo_root/client/web/admin" serve --port "$admin_port" >"$artifact_dir/admin-fe03.log" 2>&1 & admin_pid=$!
deadline=$((SECONDS + 180))
until curl --fail --silent --max-time 2 "http://127.0.0.1:$compose_port" >/dev/null && curl --fail --silent --max-time 2 "http://127.0.0.1:$admin_port" >/dev/null; do
  if (( SECONDS >= deadline )); then echo "C311 servers did not start on $compose_port and $admin_port" >&2; exit 1; fi
  sleep 1
done

C311_COMPOSE_URL="http://127.0.0.1:$compose_port" C311_ADMIN_URL="http://127.0.0.1:$admin_port" C311_ARTIFACT_DIR="$artifact_dir" python3 "$repo_root/tools/c311-browser/fe03_matrix.py"
