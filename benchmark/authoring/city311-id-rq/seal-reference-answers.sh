#!/usr/bin/env bash

set -Eeuo pipefail

readonly script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly repository_root="$(cd "${script_dir}/../../.." && pwd)"
readonly manifest="${script_dir}/tasks.yaml"
readonly expected_commit="0463635356477fd02cbfa92e170d80d13e9dec22"

usage() {
  printf 'usage: %s verify | seal OUTPUT_DIRECTORY\n' "${0##*/}" >&2
  exit 2
}

require_reference_tree() {
  git -C "${repository_root}" merge-base --is-ancestor "${expected_commit}" HEAD || {
    printf 'reference tree must descend from %s\n' "${expected_commit}" >&2
    exit 1
  }
  git -C "${repository_root}" diff --quiet || {
    printf 'reference tree has uncommitted changes\n' >&2
    exit 1
  }
}

verify() {
  require_reference_tree
  (cd "${repository_root}/server" && go test ./compose/service/city311 ./compose/rest/city311 ./store/tests)
  (cd "${repository_root}" && ./scripts/runtime-acceptance.sh)
}

seal() {
  local output_directory=$1 task_id
  require_reference_tree
  mkdir -p "${output_directory}"
  while IFS= read -r task_id; do
    git -C "${repository_root}" archive --format=tar.gz --prefix="${task_id}/" "${expected_commit}" >"${output_directory}/${task_id}-reference.tar.gz"
    shasum -a 256 "${output_directory}/${task_id}-reference.tar.gz" >"${output_directory}/${task_id}-reference.tar.gz.sha256"
  done < <(ruby -ryaml -e '
    YAML.load_file(ARGV.fetch(0)).fetch("tasks").each { |id, task|
      puts id if task.fetch("reference_status") == "ready"
    }
  ' "${manifest}")
}

case $# in
  1) [[ "$1" == verify ]] || usage; verify ;;
  2) [[ "$1" == seal ]] || usage; seal "$2" ;;
  *) usage ;;
esac
