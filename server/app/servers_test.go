package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	city311Contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store/adapters/rdbms/drivers/sqlite"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestHealthzReportsContractErrorUntilDatabaseIsAvailable(t *testing.T) {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	(&CortezaApp{}).healthz(response, request)
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.Equal(t, "application/json", response.Header().Get("Content-Type"))
	require.Contains(t, response.Body.String(), string(city311Contract.ErrorTemporarilyUnavailable))
	require.Contains(t, response.Body.String(), `"retryable":true`)
}

func TestHealthzReportsRuntimeConfigurationReadiness(t *testing.T) {
	ctx := context.Background()
	st, err := sqlite.Connect(ctx, fmt.Sprintf("sqlite3://file:%s-health?mode=memory&cache=shared", t.Name()))
	require.NoError(t, err)
	logCore, observedLogs := observer.New(zap.ErrorLevel)
	application := &CortezaApp{Store: st, Log: zap.New(logCore)}
	setRuntimeHealthEnvironment(t)
	t.Setenv("DATABASE_URL", "")

	response := httptest.NewRecorder()
	application.healthz(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.Contains(t, response.Body.String(), "A required City 311 runtime configuration is unavailable.")
	require.NotContains(t, response.Body.String(), "DATABASE_URL")
	require.Contains(t, observedLogs.All()[0].ContextMap()["error"], "DATABASE_URL is required")
}

func TestHealthzReportsIdentityConfigurationReadiness(t *testing.T) {
	ctx := context.Background()
	st, err := sqlite.Connect(ctx, fmt.Sprintf("sqlite3://file:%s-health?mode=memory&cache=shared", t.Name()))
	require.NoError(t, err)
	logCore, observedLogs := observer.New(zap.ErrorLevel)
	application := &CortezaApp{Store: st, Log: zap.New(logCore)}
	setRuntimeHealthEnvironment(t)
	t.Setenv("CITY311_SEED_CONSTITUENT_PASSWORD", "SeedConstituentPassword1!")
	t.Setenv("CITY311_SEED_CONSTITUENT_TWO_PASSWORD", "SeedConstituentPassword2!")
	t.Setenv("MAP_BASE_URL", "https://mapping.example.invalid")
	t.Setenv("MAP_API_TOKEN", "runtime-map-token")
	setFederatedIdentityHealthEnvironment(t)
	setCivicWorksHealthEnvironment(t)
	setWorkflowHealthEnvironment(t)

	t.Setenv("SESSION_SECRET", "")
	t.Setenv("APP_BASE_URL", "https://city311.example.invalid")
	response := httptest.NewRecorder()
	application.healthz(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.Contains(t, response.Body.String(), "A required City 311 identity configuration is unavailable.")
	require.NotContains(t, response.Body.String(), "SESSION_SECRET")
	require.NotContains(t, response.Body.String(), "APP_BASE_URL")
	require.Contains(t, observedLogs.All()[0].ContextMap()["error"], "SESSION_SECRET is required")

	t.Setenv("SESSION_SECRET", "runtime-identity-secret")
	for _, invalidBaseURL := range []string{"", "relative-url"} {
		t.Setenv("APP_BASE_URL", invalidBaseURL)
		response = httptest.NewRecorder()
		application.healthz(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		require.Equal(t, http.StatusServiceUnavailable, response.Code)
		require.Contains(t, response.Body.String(), "A required City 311 identity configuration is unavailable.")
		require.NotContains(t, response.Body.String(), "APP_BASE_URL")
		require.NotContains(t, response.Body.String(), "absolute HTTP or HTTPS URL")
	}

	t.Setenv("APP_BASE_URL", "https://city311.example.invalid")
	response = httptest.NewRecorder()
	application.healthz(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(t, `{"status":"ok","database":"ok"}`, response.Body.String())
}

func TestHealthzReportsMappingConfigurationReadiness(t *testing.T) {
	ctx := context.Background()
	st, err := sqlite.Connect(ctx, fmt.Sprintf("sqlite3://file:%s-health?mode=memory&cache=shared", t.Name()))
	require.NoError(t, err)
	logCore, observedLogs := observer.New(zap.ErrorLevel)
	application := &CortezaApp{Store: st, Log: zap.New(logCore)}
	setRuntimeHealthEnvironment(t)
	t.Setenv("SESSION_SECRET", "runtime-identity-secret")
	t.Setenv("APP_BASE_URL", "https://city311.example.invalid")
	t.Setenv("CITY311_SEED_CONSTITUENT_PASSWORD", "SeedConstituentPassword1!")
	t.Setenv("CITY311_SEED_CONSTITUENT_TWO_PASSWORD", "SeedConstituentPassword2!")
	setFederatedIdentityHealthEnvironment(t)
	setCivicWorksHealthEnvironment(t)
	setWorkflowHealthEnvironment(t)

	t.Setenv("MAP_BASE_URL", "")
	t.Setenv("MAP_API_TOKEN", "")
	response := httptest.NewRecorder()
	application.healthz(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.Contains(t, response.Body.String(), "A required City 311 mapping configuration is unavailable.")
	require.NotContains(t, response.Body.String(), "MAP_BASE_URL")
	require.NotContains(t, response.Body.String(), "MAP_API_TOKEN")
	require.Contains(t, observedLogs.All()[0].ContextMap()["error"], "MAP_BASE_URL is required")

	t.Setenv("MAP_BASE_URL", "relative-url")
	t.Setenv("MAP_API_TOKEN", "runtime-map-token")
	response = httptest.NewRecorder()
	application.healthz(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.Contains(t, response.Body.String(), "A required City 311 mapping configuration is unavailable.")
	require.NotContains(t, response.Body.String(), "MAP_BASE_URL")
	require.NotContains(t, response.Body.String(), "absolute HTTP or HTTPS URL")
	require.Contains(t, observedLogs.All()[1].ContextMap()["error"], "MAP_BASE_URL must be an absolute HTTP or HTTPS URL")

	t.Setenv("MAP_BASE_URL", "https://mapping.example.invalid")
	t.Setenv("MAP_API_TOKEN", "runtime-map-token")
	response = httptest.NewRecorder()
	application.healthz(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(t, `{"status":"ok","database":"ok"}`, response.Body.String())
}

func setCivicWorksHealthEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("CIVICWORKS_BASE_URL", "https://civicworks.example.invalid")
	t.Setenv("CIVICWORKS_API_TOKEN", "runtime-civicworks-token")
	t.Setenv("CIVICWORKS_WEBHOOK_SECRET", "runtime-civicworks-webhook-secret")
	t.Setenv("BENCHMARK_RUN_ID", "benchmark-run-health")
}

func setRuntimeHealthEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://corteza:secret@postgres:5432/corteza?sslmode=disable")
	t.Setenv("BENCHMARK_TIMEZONE", "America/New_York")
	t.Setenv("BENCHMARK_NOW", "2026-02-03T15:04:05Z")
	t.Setenv("BENCHMARK_SEED", "city311-public-v1")
	t.Setenv("CRM_API_CLIENT_ID", "city311-api-client")
	t.Setenv("CRM_API_CLIENT_SECRET", "runtime-api-secret")
	t.Setenv("MAIL_SMTP_HOST", "mail.example.test")
	t.Setenv("MAIL_SMTP_PORT", "587")
	t.Setenv("MAIL_SMTP_USERNAME", "city311")
	t.Setenv("MAIL_SMTP_PASSWORD", "runtime-mail-secret")
	t.Setenv("MAIL_API_BASE_URL", "https://mail.example.test")
	t.Setenv("MAIL_API_TOKEN", "runtime-mail-api-token")
}

func setFederatedIdentityHealthEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("OIDC_ISSUER_URL", "https://identity.example.invalid")
	t.Setenv("OIDC_STAFF_CLIENT_ID", "city311-staff")
	t.Setenv("OIDC_PUBLIC_CLIENT_ID", "city311-public")
	t.Setenv("OIDC_CLIENT_SECRET", "runtime-oidc-secret")
	t.Setenv("SAML_METADATA_URL", "https://identity.example.invalid/saml/metadata")
	t.Setenv("SAML_SP_ENTITY_ID", "https://city311.example.invalid/saml")
}

func setWorkflowHealthEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("WORKFLOW_OAUTH_TOKEN_URL", "https://workflow.example.invalid/oauth/token")
	t.Setenv("WORKFLOW_API_BASE_URL", "https://workflow.example.invalid")
	t.Setenv("WORKFLOW_CLIENT_ID", "runtime-workflow-client")
	t.Setenv("WORKFLOW_CLIENT_SECRET", "runtime-workflow-secret")
}

func TestHealthzReportsCivicWorksConfigurationReadiness(t *testing.T) {
	ctx := context.Background()
	st, err := sqlite.Connect(ctx, fmt.Sprintf("sqlite3://file:%s-health?mode=memory&cache=shared", t.Name()))
	require.NoError(t, err)
	logCore, observedLogs := observer.New(zap.ErrorLevel)
	application := &CortezaApp{Store: st, Log: zap.New(logCore)}
	setRuntimeHealthEnvironment(t)
	t.Setenv("SESSION_SECRET", "runtime-identity-secret")
	t.Setenv("APP_BASE_URL", "https://city311.example.invalid")
	t.Setenv("CITY311_SEED_CONSTITUENT_PASSWORD", "SeedConstituentPassword1!")
	t.Setenv("CITY311_SEED_CONSTITUENT_TWO_PASSWORD", "SeedConstituentPassword2!")
	t.Setenv("MAP_BASE_URL", "https://mapping.example.invalid")
	t.Setenv("MAP_API_TOKEN", "runtime-map-token")
	setFederatedIdentityHealthEnvironment(t)
	t.Setenv("CIVICWORKS_BASE_URL", "")
	t.Setenv("CIVICWORKS_API_TOKEN", "")
	t.Setenv("CIVICWORKS_WEBHOOK_SECRET", "")
	t.Setenv("BENCHMARK_RUN_ID", "")
	setWorkflowHealthEnvironment(t)

	response := httptest.NewRecorder()
	application.healthz(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.Contains(t, response.Body.String(), "A required City 311 CivicWorks configuration is unavailable.")
	require.NotContains(t, response.Body.String(), "CIVICWORKS_BASE_URL")
	require.Contains(t, observedLogs.All()[0].ContextMap()["error"], "CIVICWORKS_BASE_URL is required")

	setCivicWorksHealthEnvironment(t)
	response = httptest.NewRecorder()
	application.healthz(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	require.Equal(t, http.StatusOK, response.Code)
}

func TestHealthzReportsWorkflowConfigurationReadiness(t *testing.T) {
	ctx := context.Background()
	st, err := sqlite.Connect(ctx, fmt.Sprintf("sqlite3://file:%s-health?mode=memory&cache=shared", t.Name()))
	require.NoError(t, err)
	logCore, observedLogs := observer.New(zap.ErrorLevel)
	application := &CortezaApp{Store: st, Log: zap.New(logCore)}
	setRuntimeHealthEnvironment(t)
	t.Setenv("SESSION_SECRET", "runtime-identity-secret")
	t.Setenv("APP_BASE_URL", "https://city311.example.invalid")
	t.Setenv("CITY311_SEED_CONSTITUENT_PASSWORD", "SeedConstituentPassword1!")
	t.Setenv("CITY311_SEED_CONSTITUENT_TWO_PASSWORD", "SeedConstituentPassword2!")
	t.Setenv("MAP_BASE_URL", "https://mapping.example.invalid")
	t.Setenv("MAP_API_TOKEN", "runtime-map-token")
	setFederatedIdentityHealthEnvironment(t)
	setCivicWorksHealthEnvironment(t)
	t.Setenv("WORKFLOW_OAUTH_TOKEN_URL", "")
	t.Setenv("WORKFLOW_API_BASE_URL", "")
	t.Setenv("WORKFLOW_CLIENT_ID", "")
	t.Setenv("WORKFLOW_CLIENT_SECRET", "")

	response := httptest.NewRecorder()
	application.healthz(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.Contains(t, response.Body.String(), "A required City 311 workflow configuration is unavailable.")
	require.NotContains(t, response.Body.String(), "WORKFLOW_OAUTH_TOKEN_URL")
	require.Contains(t, observedLogs.All()[0].ContextMap()["error"], "WORKFLOW_OAUTH_TOKEN_URL is required")

	setWorkflowHealthEnvironment(t)
	response = httptest.NewRecorder()
	application.healthz(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	require.Equal(t, http.StatusOK, response.Code)
}
