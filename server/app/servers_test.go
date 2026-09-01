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

func TestHealthzReportsIdentityConfigurationReadiness(t *testing.T) {
	ctx := context.Background()
	st, err := sqlite.Connect(ctx, fmt.Sprintf("sqlite3://file:%s-health?mode=memory&cache=shared", t.Name()))
	require.NoError(t, err)
	logCore, observedLogs := observer.New(zap.ErrorLevel)
	application := &CortezaApp{Store: st, Log: zap.New(logCore)}
	t.Setenv("CITY311_SEED_CONSTITUENT_PASSWORD", "SeedConstituentPassword1!")
	t.Setenv("CITY311_SEED_CONSTITUENT_TWO_PASSWORD", "SeedConstituentPassword2!")

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
