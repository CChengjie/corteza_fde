package city311

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

func TestAuditListExportAndOperationHTTPContracts(t *testing.T) {
	router, st, _ := testRouter(t)
	ctx := context.Background()
	manager, err := store.LookupUserByEmail(ctx, st, "department-manager@city311.example.invalid")
	require.NoError(t, err)
	agent, err := store.LookupUserByEmail(ctx, st, "service-agent@city311.example.invalid")
	require.NoError(t, err)

	listPath := "/api/v1/staff/audit-events?filters=%7B%22event_type%22%3A%22SEED_CREATED%22%7D&page_size=1&sort=occurred_at"
	unauthenticated := executeJSON(t, router, http.MethodGet, listPath, nil, nil, 0)
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)
	forbidden := executeJSON(t, router, http.MethodGet, listPath, nil, nil, agent.ID)
	require.Equal(t, http.StatusForbidden, forbidden.Code, forbidden.Body.String())
	listed := executeJSON(t, router, http.MethodGet, listPath, nil, nil, manager.ID)
	require.Equal(t, http.StatusOK, listed.Code, listed.Body.String())
	require.Contains(t, listed.Body.String(), `"event_type":"SEED_CREATED"`)
	require.Contains(t, listed.Body.String(), `"next_page_token"`)
	require.Contains(t, listed.Body.String(), `"applied_filters":{"event_type":["SEED_CREATED"]}`)

	exportPath := "/api/v1/staff/audit-events/export"
	missingFilters := executeJSON(t, router, http.MethodPost, exportPath, map[string]any{}, nil, manager.ID)
	require.Equal(t, http.StatusUnprocessableEntity, missingFilters.Code, missingFilters.Body.String())
	accepted := executeJSON(t, router, http.MethodPost, exportPath, map[string]any{
		"filters": map[string]any{"event_type": "SEED_CREATED"},
	}, nil, manager.ID)
	require.Equal(t, http.StatusAccepted, accepted.Code, accepted.Body.String())
	var pending contract.Operation
	require.NoError(t, json.Unmarshal(accepted.Body.Bytes(), &pending))
	require.Equal(t, contract.OperationStatusPending, pending.Status)
	require.NotEmpty(t, pending.OperationID)

	operationPath := "/api/v1/operations/" + pending.OperationID
	completed := executeJSON(t, router, http.MethodGet, operationPath, nil, nil, manager.ID)
	require.Equal(t, http.StatusOK, completed.Code, completed.Body.String())
	require.Contains(t, completed.Body.String(), `"status":"SUCCEEDED"`)
	require.Contains(t, completed.Body.String(), `"download_url":"`+operationPath+`/result"`)

	result := executeJSON(t, router, http.MethodGet, operationPath+"/result", nil, nil, manager.ID)
	require.Equal(t, http.StatusOK, result.Code, result.Body.String())
	require.Equal(t, "text/csv; charset=utf-8", result.Header().Get("Content-Type"))
	require.Contains(t, result.Header().Get("Content-Disposition"), "audit-events-")
	require.True(t, strings.HasPrefix(result.Body.String(), "entity_type,entity_id,event_type"))
	require.Contains(t, result.Body.String(), "\r\n")

	immutable := executeJSON(t, router, http.MethodDelete, "/api/v1/staff/audit-events", nil, nil, manager.ID)
	require.Contains(t, []int{http.StatusMethodNotAllowed, http.StatusNotFound}, immutable.Code)
}

func TestAuditHTTPFilterFormsAndValidationFailures(t *testing.T) {
	router, st, _ := testRouter(t)
	ctx := context.Background()
	manager, err := store.LookupUserByEmail(ctx, st, "department-manager@city311.example.invalid")
	require.NoError(t, err)
	agent, err := store.LookupUserByEmail(ctx, st, "service-agent@city311.example.invalid")
	require.NoError(t, err)

	flat := "/api/v1/staff/audit-events?request_id=1&entity_type=service_request&entity_id=1" +
		"&event_type=SEED_CREATED&actor_type=system&actor_id=0&source_channel=API" +
		"&occurred_from=2026-02-03T14%3A00%3A00-05%3A00&occurred_to=2026-02-03T22%3A00%3A00Z"
	response := executeJSON(t, router, http.MethodGet, flat, nil, nil, manager.ID)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	jsonFilter := url.QueryEscape(`{"request_id":["1"],"entity_type":"service_request","actor_type":"system","source_channel":"API","occurred_from":"2026-02-03T19:00:00Z"}`)
	response = executeJSON(t, router, http.MethodGet, "/api/v1/staff/audit-events?filters="+jsonFilter, nil, nil, manager.ID)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	for _, path := range []string{
		"/api/v1/staff/audit-events?page_size=0",
		"/api/v1/staff/audit-events?page_size=101",
		"/api/v1/staff/audit-events?filters=" + url.QueryEscape(`{"unknown":"value"}`),
		"/api/v1/staff/audit-events?filters=" + url.QueryEscape(`{"event_type":"A"}{"event_type":"B"}`),
		"/api/v1/staff/audit-events?occurred_from=not-a-time",
		"/api/v1/staff/audit-events?occurred_to=not-a-time",
	} {
		response = executeJSON(t, router, http.MethodGet, path, nil, nil, manager.ID)
		require.Equal(t, http.StatusUnprocessableEntity, response.Code, "%s: %s", path, response.Body.String())
		require.Contains(t, response.Body.String(), string(contract.ErrorValidation))
	}

	exportPath := "/api/v1/staff/audit-events/export"
	response = executeJSON(t, router, http.MethodPost, exportPath, map[string]any{
		"filters": map[string]any{"occurred_from": "not-a-time"},
	}, nil, manager.ID)
	require.Equal(t, http.StatusUnprocessableEntity, response.Code, response.Body.String())
	response = executeJSON(t, router, http.MethodPost, exportPath, map[string]any{
		"filters": map[string]any{},
	}, nil, agent.ID)
	require.Equal(t, http.StatusForbidden, response.Code, response.Body.String())

	for path, expected := range map[string]int{
		"/api/v1/operations/invalid":             http.StatusUnprocessableEntity,
		"/api/v1/operations/op-999999999":        http.StatusNotFound,
		"/api/v1/operations/invalid/result":      http.StatusUnprocessableEntity,
		"/api/v1/operations/op-999999999/result": http.StatusNotFound,
	} {
		response = executeJSON(t, router, http.MethodGet, path, nil, nil, manager.ID)
		require.Equal(t, expected, response.Code, "%s: %s", path, response.Body.String())
	}

	parsed, err := parseAuditTime("2026-09-04T12:00:00-04:00", "/time")
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, time.September, 4, 16, 0, 0, 0, time.UTC), *parsed)
}
