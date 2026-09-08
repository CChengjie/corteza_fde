#!/usr/bin/env bash

set -Eeuo pipefail

readonly script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly output_root="${1:?usage: materialize-first-three.sh OUTPUT_DIRECTORY}"

[[ ! -e "${output_root}" ]] || { printf 'refusing to overwrite %s\n' "${output_root}" >&2; exit 1; }

materialize() {
  local task_id=$1 base=$2 answer=$3
  local source_project="${script_dir}/projects/${task_id}"
  local output_project="${output_root}/${task_id}"

  install -d "${output_project}/B/customer-system" "${output_project}/B/runtime" "${output_project}/B/fixtures" "${output_project}/R" "${output_project}/ref-answer/customer-system"
  tar -C "${script_dir}/bootstrap/${base}" -czf "${output_project}/B/customer-system/source.tar.gz" .
  tar -C "${script_dir}/bootstrap/${answer}" -czf "${output_project}/ref-answer/customer-system/source.tar.gz" .
  cp "${source_project}/task.yaml" "${output_project}/task.yaml"
  cp "${source_project}/B/constraints.md" "${output_project}/B/constraints.md"
  cp "${source_project}/R/requirement.md" "${output_project}/R/requirement.md"
  cp "${script_dir}/runtime/docker-compose.yml" "${output_project}/B/runtime/docker-compose.yml"
  cp "${script_dir}/runtime/mapping-fixture.py" "${output_project}/B/fixtures/mapping-fixture.py"
  shasum -a 256 "${output_project}/B/customer-system/source.tar.gz" >"${output_project}/B/customer-system/source.tar.gz.sha256"
  shasum -a 256 "${output_project}/ref-answer/customer-system/source.tar.gz" >"${output_project}/ref-answer/customer-system/source.tar.gz.sha256"
}

materialize C311-SYS-P1 b0 p1
materialize C311-SYS-D1 p1 d1
materialize C311-SYS-I1 d1 i1

# Brownfield B snapshots are byte-identical predecessor answers, not fresh
# archives of equivalent source trees. This preserves immutable provenance.
cp "${output_root}/C311-SYS-P1/ref-answer/customer-system/source.tar.gz" "${output_root}/C311-SYS-D1/B/customer-system/source.tar.gz"
cp "${output_root}/C311-SYS-D1/ref-answer/customer-system/source.tar.gz" "${output_root}/C311-SYS-I1/B/customer-system/source.tar.gz"
shasum -a 256 "${output_root}/C311-SYS-D1/B/customer-system/source.tar.gz" >"${output_root}/C311-SYS-D1/B/customer-system/source.tar.gz.sha256"
shasum -a 256 "${output_root}/C311-SYS-I1/B/customer-system/source.tar.gz" >"${output_root}/C311-SYS-I1/B/customer-system/source.tar.gz.sha256"
printf 'materialized %s\n' "${output_root}"
