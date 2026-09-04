#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
compose_port="${C311_COMPOSE_PORT:-18090}"
admin_port="${C311_ADMIN_PORT:-18091}"
if [[ -n "${C311_ARTIFACT_DIR:-}" ]]; then
  artifact_dir="$C311_ARTIFACT_DIR"
else
  artifact_dir="$(mktemp -d "${TMPDIR:-/tmp}/c311-fe06.XXXXXX")"
fi
if [[ -L "$artifact_dir" ]]; then
  echo "artifact directory must not be a symbolic link" >&2
  exit 1
fi
mkdir -p "$artifact_dir"
chmod 700 "$artifact_dir"
package_artifact_dir="$(mktemp -d "${TMPDIR:-/tmp}/c311-fe06-packages.XXXXXX")"
cleanup () {
  kill "${compose_pid:-}" "${admin_pid:-}" 2>/dev/null || true
  rm -rf "$package_artifact_dir"
}
trap cleanup EXIT INT TERM

C311_ARTIFACT_DIR="$package_artifact_dir" "$repo_root/tools/ci/install-local-c311-packages.sh" >"$artifact_dir/package-install.log" 2>&1

corepack yarn --cwd "$repo_root/client/web/compose" serve --port "$compose_port" >"$artifact_dir/compose-fe06.log" 2>&1 & compose_pid=$!
corepack yarn --cwd "$repo_root/client/web/admin" serve --port "$admin_port" >"$artifact_dir/admin-fe06.log" 2>&1 & admin_pid=$!
deadline=$((SECONDS + 180))
until curl --fail --silent --max-time 2 "http://127.0.0.1:$compose_port" >/dev/null && curl --fail --silent --max-time 2 "http://127.0.0.1:$admin_port" >/dev/null; do
  if (( SECONDS >= deadline )); then
    echo "C311 servers did not start on $compose_port and $admin_port" >&2
    exit 1
  fi
  sleep 1
done

C311_ADMIN_URL="http://127.0.0.1:$admin_port" C311_ARTIFACT_DIR="$artifact_dir" PYTHONPATH="$repo_root/tools/c311-browser" python3 "$repo_root/tools/c311-browser/fe06_matrix.py"
