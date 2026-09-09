#!/usr/bin/env bash
set -euo pipefail
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
artifact_dir="${C311_ARTIFACT_DIR:-$(mktemp -d "${TMPDIR:-/tmp}/c311-fe09.XXXXXX")}"
if [[ -L "$artifact_dir" ]]; then echo "artifact directory must not be a symbolic link" >&2; exit 1; fi
mkdir -p "$artifact_dir"; chmod 700 "$artifact_dir"
package_artifact_dir="$(mktemp -d "${TMPDIR:-/tmp}/c311-fe09-packages.XXXXXX")"
cleanup_packages () { rm -rf "$package_artifact_dir"; }
trap cleanup_packages EXIT INT TERM
C311_ARTIFACT_DIR="$package_artifact_dir" "$repo_root/tools/ci/install-local-c311-packages.sh" >"$artifact_dir/package-install.log" 2>&1
admin_port="${C311_ADMIN_PORT:-18091}"
corepack yarn --cwd "$repo_root/client/web/admin" serve --port "$admin_port" >"$artifact_dir/admin-fe09.log" 2>&1 & pid=$!
cleanup () { kill "$pid" 2>/dev/null || true; cleanup_packages; }
trap cleanup EXIT INT TERM
for _ in $(seq 1 180); do curl --fail --silent "http://127.0.0.1:$admin_port" >/dev/null && break; sleep 1; done
C311_ADMIN_URL="http://127.0.0.1:$admin_port" C311_ARTIFACT_DIR="$artifact_dir" PYTHONPATH="$repo_root/tools/c311-browser" python3 "$repo_root/tools/c311-browser/fe09_matrix.py"
