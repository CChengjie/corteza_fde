#!/usr/bin/env bash

set -Eeuo pipefail

readonly script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly repository_root="$(cd "${script_dir}/../../../../.." && pwd)"
readonly base_commit="ac8cbb33913b6ae2904bc6a512dba24466779012"
readonly answer_commit="5781c9db9a15a3f4e2f6ddccf596bfb553b9263f"

[[ $# == 1 ]] || { printf 'usage: %s OUTPUT_DIRECTORY\n' "${0##*/}" >&2; exit 2; }
readonly output_directory=$1
readonly project_directory="${output_directory}/C311-RQ-G0"

[[ ! -e "${project_directory}" ]] || { printf 'refusing to overwrite %s\n' "${project_directory}" >&2; exit 1; }
git -C "${repository_root}" cat-file -e "${base_commit}^{commit}"
git -C "${repository_root}" cat-file -e "${answer_commit}^{commit}"

mkdir -p "${project_directory}/B/customer-system" \
  "${project_directory}/B/contracts" \
  "${project_directory}/B/fixtures" \
  "${project_directory}/B/runtime" \
  "${project_directory}/R" \
  "${project_directory}/ref-answer/customer-system" \
  "${project_directory}/evaluation"

git -C "${repository_root}" archive --format=tar.gz --prefix=source/ "${base_commit}" >"${project_directory}/B/customer-system/source.tar.gz"
git -C "${repository_root}" archive --format=tar.gz --prefix=source/ "${answer_commit}" >"${project_directory}/ref-answer/customer-system/source.tar.gz"
git -C "${repository_root}" show "${answer_commit}:server/compose/types/city311/contract.json" >"${project_directory}/B/contracts/city311-contract.json"

cp "${script_dir}/task.yaml" "${project_directory}/task.yaml"
cp "${script_dir}/B/customer-context.md" "${project_directory}/B/customer-context.md"
cp "${script_dir}/B/constraints.md" "${project_directory}/B/constraints.md"
cp -R "${script_dir}/B/fixtures/." "${project_directory}/B/fixtures/"
cp -R "${script_dir}/B/runtime/." "${project_directory}/B/runtime/"
cp "${script_dir}/R/requirement.md" "${project_directory}/R/requirement.md"
cp "${script_dir}/ref-answer/README.md" "${project_directory}/ref-answer/README.md"
cp "${script_dir}/evaluation/scoring.yaml" "${project_directory}/evaluation/scoring.yaml"
cp "${script_dir}/evaluation/validator-dag.yaml" "${project_directory}/evaluation/validator-dag.yaml"
cp "${script_dir}/evaluation/run.sh" "${project_directory}/evaluation/run.sh"

shasum -a 256 "${project_directory}/B/customer-system/source.tar.gz" >"${project_directory}/B/customer-system/source.tar.gz.sha256"
shasum -a 256 "${project_directory}/ref-answer/customer-system/source.tar.gz" >"${project_directory}/ref-answer/customer-system/source.tar.gz.sha256"
shasum -a 256 "${project_directory}/B/contracts/city311-contract.json" >"${project_directory}/B/contracts/city311-contract.json.sha256"
printf 'materialized %s\n' "${project_directory}"
