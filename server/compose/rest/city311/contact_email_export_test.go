package city311

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

func TestContactEmailExportHTTPContract(t *testing.T) {
	router, st, _ := testRouter(t)
	ctx := context.Background()
	manager, err := store.LookupUserByEmail(ctx, st, "department-manager@city311.example.invalid")
	require.NoError(t, err)
	agent, err := store.LookupUserByEmail(ctx, st, "service-agent@city311.example.invalid")
	require.NoError(t, err)
	path := "/api/v1/staff/contact-email-export"

	unauthenticated := executeJSON(t, router, http.MethodPost, path, map[string]any{"filters": map[string]any{}}, nil, 0)
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)
	missing := executeJSON(t, router, http.MethodPost, path, map[string]any{}, nil, manager.ID)
	require.Equal(t, http.StatusUnprocessableEntity, missing.Code, missing.Body.String())
	forbidden := executeJSON(t, router, http.MethodPost, path, map[string]any{"filters": map[string]any{}}, nil, agent.ID)
	require.Equal(t, http.StatusForbidden, forbidden.Code, forbidden.Body.String())
	invalid := executeJSON(t, router, http.MethodPost, path, map[string]any{"filters": map[string]any{"department": "UNKNOWN"}}, nil, manager.ID)
	require.Equal(t, http.StatusUnprocessableEntity, invalid.Code, invalid.Body.String())

	accepted := executeJSON(t, router, http.MethodPost, path, map[string]any{
		"filters": map[string]any{"department": []string{"STREETS"}, "primary_category": "RESIDENT"},
	}, nil, manager.ID)
	require.Equal(t, http.StatusAccepted, accepted.Code, accepted.Body.String())
	var pending contract.Operation
	require.NoError(t, json.Unmarshal(accepted.Body.Bytes(), &pending))
	require.Equal(t, contract.OperationStatusPending, pending.Status)

	operation := executeJSON(t, router, http.MethodGet, "/api/v1/operations/"+pending.OperationID, nil, nil, manager.ID)
	require.Equal(t, http.StatusOK, operation.Code, operation.Body.String())
	require.Contains(t, operation.Body.String(), `"status":"SUCCEEDED"`)
	result := executeJSON(t, router, http.MethodGet, "/api/v1/operations/"+pending.OperationID+"/result", nil, nil, manager.ID)
	require.Equal(t, http.StatusOK, result.Code, result.Body.String())
	require.Equal(t, "text/csv; charset=utf-8", result.Header().Get("Content-Type"))
	require.Contains(t, result.Header().Get("Content-Disposition"), "contact-emails-")
	require.True(t, strings.HasPrefix(result.Body.String(), "email,display_name,primary_category,preferred_language,opt_out\r\n"))
}
