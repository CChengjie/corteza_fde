package city311

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

func integrationActors(t *testing.T, st store.Storer) (uint64, uint64) {
	t.Helper()
	administrator, err := store.LookupUserByEmail(context.Background(), st, "platform-admin@city311.example.invalid")
	require.NoError(t, err)
	agent, err := store.LookupUserByEmail(context.Background(), st, "service-agent@city311.example.invalid")
	require.NoError(t, err)
	return administrator.ID, agent.ID
}

func TestIntegrationAdministrationHTTPContract(t *testing.T) {
	router, st, _ := testRouter(t)
	administratorID, agentID := integrationActors(t, st)

	unauthenticated := executeJSON(t, router, http.MethodGet, "/api/v1/admin/integrations", nil, nil, 0)
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)

	forbidden := executeJSON(t, router, http.MethodGet, "/api/v1/admin/integrations", nil, nil, agentID)
	require.Equal(t, http.StatusForbidden, forbidden.Code)

	list := executeJSON(t, router, http.MethodGet, "/api/v1/admin/integrations?page_size=2&sort=kind", nil, nil, administratorID)
	require.Equal(t, http.StatusOK, list.Code, list.Body.String())
	var catalogue contract.IntegrationConnectionList
	require.NoError(t, decodeResponse(list, &catalogue))
	require.Len(t, catalogue.Items, 2)
	require.Equal(t, 5, catalogue.TotalCount)
	require.NotNil(t, catalogue.NextPageToken)

	filters := url.QueryEscape(`{"kind":"MAPPING","active":false}`)
	filtered := executeJSON(t, router, http.MethodGet, "/api/v1/admin/integrations?filters="+filters, nil, nil, administratorID)
	require.Equal(t, http.StatusOK, filtered.Code, filtered.Body.String())
	require.NoError(t, decodeResponse(filtered, &catalogue))
	require.Len(t, catalogue.Items, 1)
	require.Equal(t, "mapping", catalogue.Items[0].IntegrationID)

	get := executeJSON(t, router, http.MethodGet, "/api/v1/admin/integrations/mapping", nil, nil, administratorID)
	require.Equal(t, http.StatusOK, get.Code, get.Body.String())
	require.Equal(t, `"1"`, get.Header().Get("ETag"))
	require.NotContains(t, get.Body.String(), `"secret"`)

	missingVersion := executeJSON(t, router, http.MethodPatch, "/api/v1/admin/integrations/mapping", map[string]any{"active": false}, nil, administratorID)
	require.Equal(t, http.StatusPreconditionRequired, missingVersion.Code)

	newSecret := "http-mapping-secret"
	updated := executeJSON(t, router, http.MethodPatch, "/api/v1/admin/integrations/mapping", map[string]any{
		"active": true, "configuration": map[string]any{"base_url": "https://mapping-http.example.test"}, "secret": newSecret,
	}, map[string]string{contract.IfMatchHeader: `"1"`}, administratorID)
	require.Equal(t, http.StatusOK, updated.Code, updated.Body.String())
	require.Equal(t, `"2"`, updated.Header().Get("ETag"))
	require.NotContains(t, updated.Body.String(), newSecret)
	require.NotContains(t, updated.Body.String(), `"secret"`)

	stale := executeJSON(t, router, http.MethodPatch, "/api/v1/admin/integrations/mapping", map[string]any{"active": false}, map[string]string{contract.IfMatchHeader: `"1"`}, administratorID)
	require.Equal(t, http.StatusConflict, stale.Code)
	require.Contains(t, stale.Body.String(), string(contract.ErrorVersionConflict))

	rotatedSecret := "http-rotated-secret"
	rotated := executeJSON(t, router, http.MethodPost, "/api/v1/admin/integrations/mapping/rotate", map[string]any{
		"new_secret": rotatedSecret,
	}, map[string]string{contract.IfMatchHeader: `"2"`}, administratorID)
	require.Equal(t, http.StatusOK, rotated.Code, rotated.Body.String())
	require.Equal(t, `"3"`, rotated.Header().Get("ETag"))
	require.NotContains(t, rotated.Body.String(), rotatedSecret)

	revoked := executeJSON(t, router, http.MethodPost, "/api/v1/admin/integrations/mapping/revoke", map[string]any{
		"reason": "scheduled credential retirement",
	}, map[string]string{contract.IfMatchHeader: `"3"`}, administratorID)
	require.Equal(t, http.StatusOK, revoked.Code, revoked.Body.String())
	require.Equal(t, `"4"`, revoked.Header().Get("ETag"))
	var connection contract.IntegrationConnection
	require.NoError(t, decodeResponse(revoked, &connection))
	require.False(t, connection.Active)
	require.False(t, connection.SecretConfigured)
}

func TestIntegrationAdministrationHTTPValidation(t *testing.T) {
	router, st, _ := testRouter(t)
	administratorID, agentID := integrationActors(t, st)

	for _, request := range []struct {
		method string
		path   string
		body   map[string]any
	}{
		{method: http.MethodGet, path: "/api/v1/admin/integrations/mapping"},
		{method: http.MethodPatch, path: "/api/v1/admin/integrations/mapping", body: map[string]any{"active": false}},
		{method: http.MethodPost, path: "/api/v1/admin/integrations/mapping/rotate", body: map[string]any{"new_secret": "agent-secret"}},
		{method: http.MethodPost, path: "/api/v1/admin/integrations/mapping/revoke", body: map[string]any{"reason": "not authorised"}},
	} {
		response := executeJSON(t, router, request.method, request.path, request.body, map[string]string{contract.IfMatchHeader: `"1"`}, agentID)
		require.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
	}

	for _, request := range []struct {
		method string
		path   string
		body   map[string]any
	}{
		{method: http.MethodGet, path: "/api/v1/admin/integrations"},
		{method: http.MethodGet, path: "/api/v1/admin/integrations/mapping"},
		{method: http.MethodPatch, path: "/api/v1/admin/integrations/mapping", body: map[string]any{"active": false}},
		{method: http.MethodPost, path: "/api/v1/admin/integrations/mapping/rotate", body: map[string]any{"new_secret": "unknown-actor-secret"}},
		{method: http.MethodPost, path: "/api/v1/admin/integrations/mapping/revoke", body: map[string]any{"reason": "unknown actor"}},
	} {
		response := executeJSON(t, router, request.method, request.path, request.body, map[string]string{contract.IfMatchHeader: `"1"`}, 999_999_999_999)
		require.NotEqual(t, http.StatusOK, response.Code, response.Body.String())
	}

	tests := []struct {
		name    string
		method  string
		path    string
		body    map[string]any
		headers map[string]string
		status  int
	}{
		{name: "required active", method: http.MethodPatch, path: "/api/v1/admin/integrations/mapping", body: map[string]any{}, headers: map[string]string{contract.IfMatchHeader: `"1"`}, status: http.StatusUnprocessableEntity},
		{name: "malformed secret", method: http.MethodPatch, path: "/api/v1/admin/integrations/mapping", body: map[string]any{"active": false, "secret": " malformed "}, headers: map[string]string{contract.IfMatchHeader: `"1"`}, status: http.StatusUnprocessableEntity},
		{name: "required rotation", method: http.MethodPost, path: "/api/v1/admin/integrations/mapping/rotate", body: map[string]any{}, headers: map[string]string{contract.IfMatchHeader: `"1"`}, status: http.StatusUnprocessableEntity},
		{name: "required reason", method: http.MethodPost, path: "/api/v1/admin/integrations/mapping/revoke", body: map[string]any{}, headers: map[string]string{contract.IfMatchHeader: `"1"`}, status: http.StatusUnprocessableEntity},
		{name: "unknown integration", method: http.MethodGet, path: "/api/v1/admin/integrations/missing", status: http.StatusNotFound},
		{name: "bad filter", method: http.MethodGet, path: "/api/v1/admin/integrations?filters=" + url.QueryEscape(`{"kind":"UNKNOWN"}`), status: http.StatusUnprocessableEntity},
		{name: "bad sort", method: http.MethodGet, path: "/api/v1/admin/integrations?sort=secret", status: http.StatusUnprocessableEntity},
		{name: "unknown update field", method: http.MethodPatch, path: "/api/v1/admin/integrations/mapping", body: map[string]any{"active": false, "unknown": true}, headers: map[string]string{contract.IfMatchHeader: `"1"`}, status: http.StatusUnprocessableEntity},
		{name: "unknown rotation field", method: http.MethodPost, path: "/api/v1/admin/integrations/mapping/rotate", body: map[string]any{"new_secret": "valid", "unknown": true}, headers: map[string]string{contract.IfMatchHeader: `"1"`}, status: http.StatusUnprocessableEntity},
		{name: "unknown revocation field", method: http.MethodPost, path: "/api/v1/admin/integrations/mapping/revoke", body: map[string]any{"reason": "valid", "unknown": true}, headers: map[string]string{contract.IfMatchHeader: `"1"`}, status: http.StatusUnprocessableEntity},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := executeJSON(t, router, test.method, test.path, test.body, test.headers, administratorID)
			require.Equal(t, test.status, response.Code, response.Body.String())
		})
	}
}
