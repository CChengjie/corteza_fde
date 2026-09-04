#!/usr/bin/env bash

set -Eeuo pipefail

readonly script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly repository_root="$(cd "${script_dir}/.." && pwd)"
readonly project_name="${CITY311_ACCEPTANCE_PROJECT:-city311-acceptance}"
readonly app_port="${CITY311_ACCEPTANCE_PORT:-18080}"
readonly base_url="http://127.0.0.1:${app_port}"
readonly health_url="${base_url}/healthz"
readonly api_url="${base_url}/api/v1"
readonly database_name="corteza_acceptance"
readonly database_user="corteza_acceptance"
readonly database_password="corteza-acceptance-password"
readonly cookie_name="city311_session"
readonly login_identifier="runtime.acceptance"
readonly account_email="runtime.acceptance@example.invalid"
readonly account_password="RuntimeAcceptance1!"
readonly workflow_id="runtime-acceptance-workflow"
readonly volume_marker="runtime-acceptance-volume-marker"

compose=(docker compose --env-file /dev/null --project-name "${project_name}")
stack_started=0
temporary_dir=""
fixture_file=""
signin_headers=""

log() {
  printf '[runtime-acceptance] %s\n' "$*"
}

fail() {
  printf '[runtime-acceptance] ERROR: %s\n' "$*" >&2
  return 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "required command is unavailable: $1"
}

validate_project_name() {
  [[ "${project_name}" =~ ^city311-acceptance(-[a-z0-9][a-z0-9_-]*)?$ ]] ||
    fail "CITY311_ACCEPTANCE_PROJECT must start with city311-acceptance"
  [[ "${app_port}" =~ ^[0-9]+$ ]] && (( app_port > 0 && app_port <= 65535 )) ||
    fail "CITY311_ACCEPTANCE_PORT must be an integer between 1 and 65535"
}

configure_acceptance_environment() {
  export APP_PORT="${app_port}"
  export APP_BASE_URL="${base_url}"
  export AUTH_BASE_URL="${base_url}"
  export POSTGRES_DB="${database_name}"
  export POSTGRES_USER="${database_user}"
  export POSTGRES_PASSWORD="${database_password}"
  export DATABASE_URL="postgres://${database_user}:${database_password}@postgres:5432/${database_name}?sslmode=disable"
  export BENCHMARK_TIMEZONE="America/New_York"
  export BENCHMARK_NOW="2026-02-03T15:04:05Z"
  export BENCHMARK_SEED="city311-public-v1"
  export SESSION_SECRET="runtime-acceptance-session-secret"
  export CITY311_SEED_CONSTITUENT_PASSWORD="RuntimeSeedConstituent1!"
  export CITY311_SEED_CONSTITUENT_TWO_PASSWORD="RuntimeSeedConstituent2!"
  export MAP_BASE_URL="https://mapping.example.invalid"
  export MAP_API_TOKEN="runtime-acceptance-mapping-token"
  export CIVICWORKS_BASE_URL="https://civicworks.example.invalid"
  export CIVICWORKS_API_TOKEN="runtime-acceptance-civicworks-token"
  export CIVICWORKS_WEBHOOK_SECRET="runtime-acceptance-civicworks-webhook-secret"
  export BENCHMARK_RUN_ID="runtime-acceptance"
  export WORKFLOW_OAUTH_TOKEN_URL="https://workflow.example.invalid/oauth/token"
  export WORKFLOW_API_BASE_URL="https://workflow.example.invalid"
  export WORKFLOW_CLIENT_ID="runtime-acceptance"
  export WORKFLOW_CLIENT_SECRET="runtime-acceptance-workflow-secret"
  export OIDC_ISSUER_URL="https://identity.example.invalid"
  export OIDC_STAFF_CLIENT_ID="runtime-acceptance-staff"
  export OIDC_PUBLIC_CLIENT_ID="runtime-acceptance-public"
  export OIDC_CLIENT_SECRET="runtime-acceptance-oidc-secret"
  export SAML_METADATA_URL="https://identity.example.invalid/saml/metadata"
  export SAML_SP_ENTITY_ID="${base_url}/saml"
  export CRM_API_CLIENT_ID="runtime-acceptance-api-client"
  export CRM_API_CLIENT_SECRET="runtime-acceptance-api-secret"
  export MAIL_SMTP_HOST="mail.example.invalid"
  export MAIL_SMTP_PORT="587"
  export MAIL_SMTP_USERNAME="runtime-acceptance"
  export MAIL_SMTP_PASSWORD="${project_name}-${BENCHMARK_SEED}"
  export MAIL_API_BASE_URL="https://mail.example.invalid"
  export MAIL_API_TOKEN="runtime-acceptance-mail-api-token"
  export MAIL_FROM="noreply@city.example"
}

static_checks() {
  local rendered
  rendered="$(cd "${repository_root}" && "${compose[@]}" config --format json)"
  jq -e '
    .services.app.build.dockerfile == "Dockerfile" and
    .services.app.environment.DB_DSN == .services.app.environment.DATABASE_URL and
    .services.app.environment.UPGRADE_ALWAYS == "true" and
    .services.app.environment.PROVISION_ALWAYS == "true" and
    .services.app.environment.PROVISION_PATH == "/corteza/provision/*" and
    .services.app.environment.HTTP_API_BASE_URL == "/api" and
    .services.app.environment.BENCHMARK_TIMEZONE == "America/New_York" and
    .services.app.environment.BENCHMARK_NOW == "2026-02-03T15:04:05Z" and
    .services.app.environment.BENCHMARK_SEED == "city311-public-v1" and
    .services.app.environment.CRM_API_CLIENT_ID != "" and
    .services.app.environment.CRM_API_CLIENT_SECRET != "" and
    .services.app.environment.MAIL_SMTP_HOST != "" and
    .services.app.environment.MAIL_SMTP_PORT == "587" and
    .services.app.environment.MAIL_SMTP_USERNAME != "" and
    .services.app.environment.MAIL_SMTP_PASSWORD != "" and
    .services.app.environment.MAIL_API_BASE_URL != "" and
    .services.app.environment.MAIL_API_TOKEN != "" and
    .services.app.environment.SMTP_HOST == .services.app.environment.MAIL_SMTP_HOST and
    .services.app.environment.SMTP_PORT == .services.app.environment.MAIL_SMTP_PORT and
    .services.app.environment.SMTP_USER == .services.app.environment.MAIL_SMTP_USERNAME and
    .services.app.environment.SMTP_PASS == .services.app.environment.MAIL_SMTP_PASSWORD and
    .services.app.volumes[0].source == "attachment_data" and
    .services.postgres.volumes[0].source == "postgres_data" and
    .services.app.depends_on.postgres.condition == "service_healthy"
  ' <<<"${rendered}" >/dev/null
  grep -q 'COPY server/ ./' "${repository_root}/Dockerfile"
  grep -q '!server/webconsole/dist/.placeholder' "${repository_root}/.dockerignore"
  grep -q 'COPY --from=server-build /src/server/provision /corteza/provision' "${repository_root}/Dockerfile"
  grep -q 'go build -trimpath' "${repository_root}/Dockerfile"
  grep -q 'http://127.0.0.1:80/healthz' "${repository_root}/Dockerfile"
  log "static runtime contract passed"
}

show_failure_context() {
  local status=$?
  trap - EXIT
  if (( status != 0 && stack_started == 1 )); then
    log "capturing container state and logs after failure"
    (cd "${repository_root}" && "${compose[@]}" ps) >&2 || true
    (cd "${repository_root}" && "${compose[@]}" logs --no-color --tail=200) >&2 || true
  fi
  if [[ "${KEEP_RUNTIME:-0}" != "1" && ${stack_started} == 1 ]]; then
    (cd "${repository_root}" && "${compose[@]}" down --volumes --remove-orphans) >/dev/null 2>&1 || true
  elif (( stack_started == 1 )); then
    log "KEEP_RUNTIME=1; leaving Compose project ${project_name} running"
  fi
  if [[ -n "${temporary_dir}" ]]; then
    rm -rf "${temporary_dir}"
  fi
  exit "${status}"
}

wait_for_health() {
  local deadline=$((SECONDS + 120))
  local response
  while (( SECONDS < deadline )); do
    response="$(curl --fail --silent --show-error "${health_url}" 2>/dev/null || true)"
    if jq -e 'length == 2 and .status == "ok" and .database == "ok"' <<<"${response}" >/dev/null 2>&1; then
      log "health contract passed: ${response}"
      return 0
    fi
    sleep 2
  done
  fail "${health_url} did not return the exact ready contract within 120 seconds"
}

database_scalar() {
  local query=$1
  (cd "${repository_root}" && "${compose[@]}" exec -T postgres \
    psql --no-psqlrc --tuples-only --no-align --set ON_ERROR_STOP=1 \
    --username "${database_user}" --dbname "${database_name}" --command "${query}")
}

authenticated_get() {
  local path=$1
  curl --fail --silent --show-error \
    --header "Cookie: ${cookie_name}=${session_token}" \
    "${api_url}${path}"
}

assert_persisted_state() {
  local session request_list draft workflow integration audit attachment marker account_count
  session="$(authenticated_get '/session')"
  jq -e '
    .authenticated == true and
    (.actor.application_roles | index("constituent") != null) and
    (.actor.application_roles | index("platform_administrator") != null) and
    (.actor.application_roles | index("workflow_designer") != null)
  ' <<<"${session}" >/dev/null

  request_list="$(authenticated_get '/portal/service-requests?page_size=100')"
  jq -e --arg request_id "${request_id}" '.items | any(.request_id == $request_id)' <<<"${request_list}" >/dev/null

  draft="$(authenticated_get "/portal/service-request-drafts/${draft_id}")"
  jq -e --arg draft_id "${draft_id}" '.request_id == $draft_id and .status == "DRAFT"' <<<"${draft}" >/dev/null

  workflow="$(authenticated_get "/admin/workflows/${workflow_id}")"
  jq -e --arg workflow_id "${workflow_id}" '.workflow_id == $workflow_id and .version == 1' <<<"${workflow}" >/dev/null

  integration="$(authenticated_get '/admin/integrations/mapping')"
  jq -e '.integration_id == "mapping" and .active == true and .secret_configured == true and .version == 2' <<<"${integration}" >/dev/null

  audit="$(curl --fail --silent --show-error --get \
    --header "Cookie: ${cookie_name}=${session_token}" \
    --data-urlencode "filters={\"request_id\":[\"${request_id}\"]}" \
    --data 'page_size=100' \
    "${api_url}/staff/audit-events")"
  jq -e '.total_count > 0 and (.items | length > 0)' <<<"${audit}" >/dev/null

  attachment="$(authenticated_get "/attachments/${attachment_id}")"
  jq -e --arg expected "${attachment_base64}" '.body_encoding == "base64" and .body == $expected' <<<"${attachment}" >/dev/null

  marker="$(cd "${repository_root}" && "${compose[@]}" exec -T app sh -c 'cat /data/runtime-acceptance-volume.txt')"
  [[ "${marker}" == "${volume_marker}" ]] || fail "attachment volume marker changed after app restart"

  account_count="$(database_scalar "SELECT count(*) FROM compose_city311_local_account WHERE login_identifier = '${login_identifier}';")"
  [[ "${account_count}" == "1" ]] || fail "registered local account is missing after app restart"
  log "account, request, draft, workflow, audit, attachment, integration, and volume state persisted"
}

create_runtime_fixtures() {
  local registration user_id signin_body upload_response attachment_token request_body request_response draft_body draft_response workflow_body workflow_response integration_response

  registration="$(jq -n \
    --arg display_name 'Runtime Acceptance User' \
    --arg email "${account_email}" \
    --arg login_identifier "${login_identifier}" \
    --arg password "${account_password}" \
    '{display_name:$display_name,email:$email,login_identifier:$login_identifier,password:$password,preferred_language:"EN"}')"
  curl --fail --silent --show-error \
    --header 'Content-Type: application/json' \
    --data "${registration}" \
    "${api_url}/accounts" >/dev/null

  user_id="$(database_scalar "SELECT id FROM users WHERE email = '${account_email}';")"
  [[ "${user_id}" =~ ^[0-9]+$ ]] || fail "could not resolve the acceptance account ID"
  database_scalar "UPDATE compose_city311_actor_profile SET application_roles = '[\"constituent\",\"platform_administrator\",\"workflow_designer\"]' WHERE id = ${user_id};" >/dev/null

  signin_body="$(jq -n --arg login_identifier "${login_identifier}" --arg password "${account_password}" \
    '{login_identifier:$login_identifier,password:$password}')"
  curl --fail --silent --show-error \
    --dump-header "${signin_headers}" \
    --output /dev/null \
    --header 'Content-Type: application/json' \
    --data "${signin_body}" \
    "${api_url}/session"
  session_token="$(sed -n "s/^[Ss]et-[Cc]ookie: ${cookie_name}=\([^;]*\).*/\1/p" "${signin_headers}" | tr -d '\r' | head -n 1)"
  [[ -n "${session_token}" ]] || fail "sign-in did not return the City 311 session cookie"

  printf 'persistent City 311 attachment\n' >"${fixture_file}"
  attachment_base64="$(base64 <"${fixture_file}" | tr -d '\r\n')"
  upload_response="$(curl --fail --silent --show-error \
    --header "Cookie: ${cookie_name}=${session_token}" \
    --form "file=@${fixture_file};type=text/plain" \
    --form 'filename=runtime-attachment.txt' \
    --form 'media_type=text/plain' \
    "${api_url}/portal/attachments")"
  attachment_token="$(jq -er '.attachment_token' <<<"${upload_response}")"

  workflow_body="$(jq -n --arg workflow_id "${workflow_id}" '{
    workflow_id:$workflow_id,
    name:"Runtime acceptance workflow",
    trigger:"SERVICE_REQUEST_CREATED",
    active:false,
    conditions:[],
    actions:[{type:"FIELD_UPDATE",field:"custom_fields.runtime_acceptance",value:true}]
  }')"
  workflow_response="$(curl --fail --silent --show-error \
    --header "Cookie: ${cookie_name}=${session_token}" \
    --header 'Content-Type: application/json' \
    --data "${workflow_body}" \
    "${api_url}/admin/workflows")"
  jq -e --arg workflow_id "${workflow_id}" '.workflow_id == $workflow_id and .version == 1' <<<"${workflow_response}" >/dev/null

  request_body="$(jq -n --arg attachment_token "${attachment_token}" '{
    summary:"Runtime acceptance pothole",
    description:"This request verifies persistence across an application-only restart.",
    service_type:"POTHOLE",
    requester:{display_name:"Runtime Acceptance User",email:"runtime.acceptance@example.invalid"},
    location:{address:"100 Example Street, Buffalo, NY 14201",latitude:42.88645,longitude:-78.87837},
    attachment_tokens:[$attachment_token]
  }')"
  request_response="$(curl --fail --silent --show-error \
    --header "Cookie: ${cookie_name}=${session_token}" \
    --header 'Content-Type: application/json' \
    --header 'Idempotency-Key: runtime-acceptance-request' \
    --data "${request_body}" \
    "${api_url}/portal/service-requests")"
  request_id="$(jq -er '.request_id' <<<"${request_response}")"
  attachment_id="$(jq -er '.attachments[0].attachment_id' <<<"${request_response}")"

  draft_body='{"summary":"Runtime acceptance draft retained across application restarts"}'
  draft_response="$(curl --fail --silent --show-error \
    --header "Cookie: ${cookie_name}=${session_token}" \
    --header 'Content-Type: application/json' \
    --data "${draft_body}" \
    "${api_url}/portal/service-request-drafts")"
  draft_id="$(jq -er '.request_id' <<<"${draft_response}")"

  integration_response="$(curl --fail --silent --show-error \
    --request PATCH \
    --header "Cookie: ${cookie_name}=${session_token}" \
    --header 'Content-Type: application/json' \
    --header 'If-Match: "1"' \
    --data '{"active":true,"configuration":{"base_url":"https://runtime-acceptance.example.invalid"},"secret":"runtime-acceptance-secret"}' \
    "${api_url}/admin/integrations/mapping")"
  jq -e '.integration_id == "mapping" and .active == true and .secret_configured == true and .version == 2' <<<"${integration_response}" >/dev/null

  (cd "${repository_root}" && "${compose[@]}" exec -T app sh -c \
    'printf %s "$1" > /data/runtime-acceptance-volume.txt' sh "${volume_marker}")
  log "created runtime fixtures through the HTTP API"
}

restart_application_only() {
  local database_container_before=$1
  local database_started_before=$2
  local database_container_after database_started_after
  (cd "${repository_root}" && "${compose[@]}" restart app)
  wait_for_health
  database_container_after="$(cd "${repository_root}" && "${compose[@]}" ps --quiet postgres)"
  database_started_after="$(docker inspect --format '{{.State.StartedAt}}' "${database_container_after}")"
  [[ "${database_container_after}" == "${database_container_before}" ]] || fail "PostgreSQL container was replaced during app restart"
  [[ "${database_started_after}" == "${database_started_before}" ]] || fail "PostgreSQL restarted during app-only restart"
  assert_persisted_state
}

main() {
  local database_container database_started
  if (( $# > 1 )) || { (( $# == 1 )) && [[ "$1" != "--static" ]]; }; then
    fail "usage: $0 [--static]"
  fi
  require_command docker
  require_command curl
  require_command jq
  require_command base64
  validate_project_name
  configure_acceptance_environment
  static_checks
  if [[ "${1:-}" == "--static" ]]; then
    exit 0
  fi
  docker info >/dev/null 2>&1 || fail "Docker daemon is unavailable"

  temporary_dir="$(mktemp -d)"
  fixture_file="${temporary_dir}/runtime-attachment.txt"
  signin_headers="${temporary_dir}/signin.headers"

  trap show_failure_context EXIT
  cd "${repository_root}"
  "${compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
  stack_started=1
  log "starting a clean database from current source"
  "${compose[@]}" up --build --detach
  wait_for_health

  create_runtime_fixtures
  assert_persisted_state
  database_container="$("${compose[@]}" ps --quiet postgres)"
  database_started="$(docker inspect --format '{{.State.StartedAt}}' "${database_container}")"

  log "restarting only the application against the migrated database"
  restart_application_only "${database_container}" "${database_started}"
  log "repeating application startup to verify idempotent migrations"
  restart_application_only "${database_container}" "${database_started}"

  log "all clean/migrated database and persistence acceptance checks passed"
}

main "$@"
