package city311

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/pkg/auth"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/go-chi/jwtauth"
	"github.com/lestrrat-go/jwx/jwt"
	"github.com/stretchr/testify/require"
)

func TestDataExportHTTPContractScopePaginationAndErrors(t *testing.T) {
	router, st, _ := testRouter(t)
	manager, err := store.LookupUserByEmail(context.Background(), st, "department-manager@city311.example.invalid")
	require.NoError(t, err)
	path := "/api/v1/export/service-requests?page_size=1&filters=" + url.QueryEscape(`{"department":"STREETS"}`)

	unauthenticated := executeJSON(t, router, http.MethodGet, path, nil, nil, 0)
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)
	forbidden := executeOAuthExport(t, router, path, manager.ID, "profile.read")
	require.Equal(t, http.StatusForbidden, forbidden.Code)

	first := executeOAuthExport(t, router, path, manager.ID, contract.ScopeCRMExport)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	require.Contains(t, first.Body.String(), `"owning_department":"STREETS"`)
	require.Contains(t, first.Body.String(), `"next_page_token":"`)
	var firstBody contract.ExportResponse
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &firstBody))
	require.WithinDuration(t, time.Now().UTC(), firstBody.GeneratedAt, 2*time.Second)

	invalidFilter := executeOAuthExport(t, router, "/api/v1/export/service-requests?filters="+url.QueryEscape(`{"unknown":"value"}`), manager.ID, contract.ScopeCRMExport)
	require.Equal(t, http.StatusUnprocessableEntity, invalidFilter.Code, invalidFilter.Body.String())
	require.Contains(t, invalidFilter.Body.String(), string(contract.ErrorInvalidFilter))
	invalidToken := executeOAuthExport(t, router, "/api/v1/export/service-requests?page_token=invalid", manager.ID, contract.ScopeCRMExport)
	require.Equal(t, http.StatusBadRequest, invalidToken.Code, invalidToken.Body.String())
	require.Contains(t, invalidToken.Body.String(), string(contract.ErrorInvalidPageToken))
}

func TestDataExportHTTPRateLimitIsSixtyRequestsPerClient(t *testing.T) {
	router, st, _ := testRouter(t)
	manager, err := store.LookupUserByEmail(context.Background(), st, "department-manager@city311.example.invalid")
	require.NoError(t, err)
	path := "/api/v1/export/service-requests?page_size=1"
	for i := 0; i < 60; i++ {
		response := executeOAuthExport(t, router, path, manager.ID, contract.ScopeCRMExport)
		require.Equal(t, http.StatusOK, response.Code, "request %d: %s", i+1, response.Body.String())
	}
	limited := executeOAuthExport(t, router, path, manager.ID, contract.ScopeCRMExport)
	require.Equal(t, http.StatusTooManyRequests, limited.Code, limited.Body.String())
	retryAfter, err := strconv.Atoi(limited.Header().Get(contract.RetryAfterHeader))
	require.NoError(t, err)
	require.GreaterOrEqual(t, retryAfter, 1)
	require.LessOrEqual(t, retryAfter, 60)
	require.Contains(t, limited.Body.String(), `"error":"RATE_LIMITED"`)
	require.Contains(t, limited.Body.String(), `"retryable":true`)
}

func TestDataExportHTTPQueryValidation(t *testing.T) {
	router, st, _ := testRouter(t)
	manager, err := store.LookupUserByEmail(context.Background(), st, "department-manager@city311.example.invalid")
	require.NoError(t, err)

	for name, query := range map[string]string{
		"zero page size":    "?page_size=0",
		"large page size":   "?page_size=101",
		"invalid timestamp": "?updated_since=tomorrow",
		"invalid filters":   "?filters=%7B",
		"multiple values":   "?filters=%7B%7D%7B%7D",
	} {
		t.Run(name, func(t *testing.T) {
			response := executeOAuthExport(t, router, "/api/v1/export/service-requests"+query, manager.ID, contract.ScopeCRMExport)
			require.Equal(t, http.StatusUnprocessableEntity, response.Code, response.Body.String())
			require.Contains(t, response.Body.String(), string(contract.ErrorInvalidFilter))
		})
	}

	validTime := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	response := executeOAuthExport(t, router, "/api/v1/export/service-requests?updated_since="+url.QueryEscape(validTime)+"&filters="+url.QueryEscape(`{"department":["STREETS"]}`), manager.ID, contract.ScopeCRMExport)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
}

func executeOAuthExport(t *testing.T, router http.Handler, path string, actorID uint64, scope string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	token := jwt.New()
	require.NoError(t, token.Set("scope", scope))
	ctx := jwtauth.NewContext(request.Context(), token, nil)
	ctx = auth.SetIdentityToContext(ctx, auth.Authenticated(actorID))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request.WithContext(ctx))
	return response
}
