package city311

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

func TestOriginAndScopeOverrideHTTPContracts(t *testing.T) {
	router, st, _ := testRouter(t)
	ctx := context.Background()
	request, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, "SR-2026-00034")
	require.NoError(t, err)
	requestID := strconv.FormatUint(request.ID, 10)
	manager, err := store.LookupUserByEmail(ctx, st, "department-manager@city311.example.invalid")
	require.NoError(t, err)
	administrator, err := store.LookupUserByEmail(ctx, st, "platform-admin@city311.example.invalid")
	require.NoError(t, err)
	agent, err := store.LookupUserByEmail(ctx, st, "service-agent@city311.example.invalid")
	require.NoError(t, err)

	originPath := "/api/v1/staff/service-requests/" + requestID + "/origin-class"
	unauthenticated := executeJSON(t, router, http.MethodPost, originPath, map[string]any{
		"origin_class": "EXTERNAL", "reason": "Correct classification.",
	}, map[string]string{contract.IfMatchHeader: `"1"`}, 0)
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)
	missingVersion := executeJSON(t, router, http.MethodPost, originPath, map[string]any{
		"origin_class": "EXTERNAL", "reason": "Correct classification.",
	}, nil, manager.ID)
	require.Equal(t, http.StatusPreconditionRequired, missingVersion.Code)
	forbidden := executeJSON(t, router, http.MethodPost, originPath, map[string]any{
		"origin_class": "EXTERNAL", "reason": "Not authorized.",
	}, map[string]string{contract.IfMatchHeader: `"1"`}, agent.ID)
	require.Equal(t, http.StatusForbidden, forbidden.Code, forbidden.Body.String())

	origin := executeJSON(t, router, http.MethodPost, originPath, map[string]any{
		"origin_class": "EXTERNAL", "reason": "Manager correction.",
	}, map[string]string{contract.IfMatchHeader: `"1"`}, manager.ID)
	require.Equal(t, http.StatusOK, origin.Code, origin.Body.String())
	require.Contains(t, origin.Body.String(), `"origin_class":"EXTERNAL"`)
	require.Contains(t, origin.Body.String(), `"version":2`)
	require.Contains(t, origin.Body.String(), `"event_type":"ORIGIN_CLASS_OVERRIDDEN"`)

	scopePath := "/api/v1/staff/service-requests/" + requestID + "/scope-override"
	scope := executeJSON(t, router, http.MethodPost, scopePath, map[string]any{
		"department_code": "SANITATION", "district_codes": []string{"SOUTH"}, "reason": "Cross-department coordination.",
	}, map[string]string{contract.IfMatchHeader: `"2"`}, administrator.ID)
	require.Equal(t, http.StatusOK, scope.Code, scope.Body.String())
	require.Contains(t, scope.Body.String(), `"event_type":"REQUEST_SCOPE_OVERRIDDEN"`)

	decoded := map[string]any{}
	require.NoError(t, json.Unmarshal(scope.Body.Bytes(), &decoded))
	requestBody := decoded["request"].(map[string]any)
	require.NotContains(t, requestBody, "scope_department")
	require.NotContains(t, requestBody, "scope_districts")

	stale := executeJSON(t, router, http.MethodPost, scopePath, map[string]any{
		"department_code": "STREETS", "district_codes": []string{}, "reason": "Stale manager write.",
	}, map[string]string{contract.IfMatchHeader: `"2"`}, manager.ID)
	require.Equal(t, http.StatusConflict, stale.Code, stale.Body.String())
}
