#!/usr/bin/env bash

set -Eeuo pipefail

readonly script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly task_root="$(cd "${script_dir}/.." && pwd)"
readonly candidate_root="$(cd "${1:?usage: run.sh CANDIDATE_SOURCE_DIRECTORY}" && pwd)"
readonly project_name="c311-rq-g0-${RANDOM}${RANDOM}"
readonly app_port="${C311_G0_APP_PORT:-18090}"
readonly base_url="http://127.0.0.1:${app_port}/api/v1"
readonly compose_file="${task_root}/B/runtime/docker-compose.yml"
compose=(docker compose --project-name "${project_name}" --file "${compose_file}")

cleanup() {
  "${compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

require() { command -v "$1" >/dev/null || { echo "missing required command: $1" >&2; exit 2; }; }
for command in docker curl jq; do require "${command}"; done

# The unit layer covers atomic attachment, scope, and privacy edge cases. The
# runtime layer below proves the candidate actually crosses the fixture boundary.
(cd "${candidate_root}/server" && go test ./compose/service/city311 ./compose/rest/city311 \
  -run 'Test(StagedAttachmentsAreAtomicSingleUseAndReplayable|AttachmentConsumeRollsBackOnLaterFailure|PublicStatusUsesSubmittedEmailAndMinimalProjection|PortalSubmit)')

CANDIDATE_ROOT="${candidate_root}" APP_PORT="${app_port}" "${compose[@]}" up --build --detach --wait

deadline=$((SECONDS + 120))
until curl --fail --silent "http://127.0.0.1:${app_port}/healthz" | jq -e '.status == "ok" and .database == "ok"' >/dev/null; do
  (( SECONDS < deadline )) || { echo "application did not become healthy" >&2; exit 1; }
  sleep 2
done

request_body='{"summary":"Pothole blocks traffic","description":"A pothole blocks the eastbound lane near the curb.","service_type":"POTHOLE","requester":{"display_name":"Alex Resident","email":"alex@example.invalid","phone":"+17165550101"},"location":{"address":"100 Example Street, Buffalo, NY 14201","latitude":42.88645,"longitude":-78.87837}}'
created="$(curl --fail --silent --header 'Content-Type: application/json' --header 'Idempotency-Key: c311-g0-runtime' --data "${request_body}" "${base_url}/portal/service-requests")"
request_number="$(jq -er '.request_number' <<<"${created}")"

curl --fail --silent "http://127.0.0.1:${app_port}/calls" >/dev/null 2>&1 && exit 1
calls="$("${compose[@]}" exec -T mapping wget -qO- http://127.0.0.1:8081/calls | jq -er '.geocode_calls')"
[[ "${calls}" == "1" ]] || { echo "mapping fixture was not called exactly once" >&2; exit 1; }

lookup_body="$(jq -n --arg request_number "${request_number}" '{request_number:$request_number,email:"alex@example.invalid"}')"
lookup="$(curl --fail --silent --header 'Content-Type: application/json' --data "${lookup_body}" "${base_url}/public/service-request-status")"
jq -e '.request_detail.status == "SUBMITTED" and (. | tostring | contains("description") | not) and (. | tostring | contains("attachments") | not)' <<<"${lookup}" >/dev/null

"${compose[@]}" restart app >/dev/null
deadline=$((SECONDS + 120))
until curl --fail --silent "http://127.0.0.1:${app_port}/healthz" | jq -e '.status == "ok"' >/dev/null; do
  (( SECONDS < deadline )) || { echo "application did not recover after restart" >&2; exit 1; }
  sleep 2
done
curl --fail --silent --header 'Content-Type: application/json' --data "${lookup_body}" "${base_url}/public/service-request-status" | jq -e --arg request_number "${request_number}" '.request_detail.request_number == $request_number' >/dev/null

echo 'C311-RQ-G0 evaluation passed: 100/100'
