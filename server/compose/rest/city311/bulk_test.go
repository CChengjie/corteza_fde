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

func TestDuplicateGroupAndBulkHTTPContracts(t *testing.T) {
	router, st, _ := testRouter(t)
	ctx := context.Background()
	first, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, "SR-2026-00034")
	require.NoError(t, err)
	second, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, "SR-2026-00035")
	require.NoError(t, err)
	supervisor, err := store.LookupUserByEmail(ctx, st, "supervisor@city311.example.invalid")
	require.NoError(t, err)
	manager, err := store.LookupUserByEmail(ctx, st, "department-manager@city311.example.invalid")
	require.NoError(t, err)
	firstID, secondID := strconv.FormatUint(first.ID, 10), strconv.FormatUint(second.ID, 10)
	firstGroupPath := "/api/v1/staff/service-requests/" + firstID + "/duplicate-group"
	secondGroupPath := "/api/v1/staff/service-requests/" + secondID + "/duplicate-group"

	unauthenticated := executeJSON(t, router, http.MethodPost, firstGroupPath, map[string]any{}, nil, 0)
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)
	missingVersion := executeJSON(t, router, http.MethodPost, firstGroupPath, map[string]any{
		"duplicate_group_id": "DG-HTTP", "reason": "Same issue.",
	}, nil, supervisor.ID)
	require.Equal(t, http.StatusPreconditionRequired, missingVersion.Code)
	forbidden := executeJSON(t, router, http.MethodPost, firstGroupPath, map[string]any{
		"duplicate_group_id": "DG-HTTP", "reason": "Managers do not confirm grouping.",
	}, map[string]string{contract.IfMatchHeader: `"1"`}, manager.ID)
	require.Equal(t, http.StatusForbidden, forbidden.Code, forbidden.Body.String())

	for path, expectedRequestID := range map[string]string{firstGroupPath: firstID, secondGroupPath: secondID} {
		confirmed := executeJSON(t, router, http.MethodPost, path, map[string]any{
			"duplicate_group_id": "DG-HTTP", "reason": "Same service type and location.",
		}, map[string]string{contract.IfMatchHeader: `"1"`}, supervisor.ID)
		require.Equal(t, http.StatusOK, confirmed.Code, confirmed.Body.String())
		require.Contains(t, confirmed.Body.String(), `"request_id":"`+expectedRequestID+`"`)
		require.Contains(t, confirmed.Body.String(), `"duplicate_group_id":"DG-HTTP"`)
	}

	bulkPath := "/api/v1/staff/service-requests/bulk"
	body := map[string]any{
		"request_items": []map[string]any{
			{"request_id": firstID, "expected_version": 2},
			{"request_id": secondID, "expected_version": 2},
		},
		"action": "UPDATE", "changes": map[string]any{"priority": "HIGH", "staff_note": "Coordinate inspection."},
	}
	missingKey := executeJSON(t, router, http.MethodPost, bulkPath, body, nil, supervisor.ID)
	require.Equal(t, http.StatusUnprocessableEntity, missingKey.Code, missingKey.Body.String())
	updated := executeJSON(t, router, http.MethodPost, bulkPath, body, map[string]string{contract.IdempotencyHeader: "http-bulk-key"}, supervisor.ID)
	require.Equal(t, http.StatusOK, updated.Code, updated.Body.String())
	require.JSONEq(t, `{"updated_request_ids":["`+firstID+`","`+secondID+`"],"updated_count":2}`, updated.Body.String())

	replay := executeJSON(t, router, http.MethodPost, bulkPath, body, map[string]string{contract.IdempotencyHeader: "http-bulk-key"}, supervisor.ID)
	require.Equal(t, http.StatusOK, replay.Code)
	require.JSONEq(t, updated.Body.String(), replay.Body.String())

	staleBody := map[string]any{
		"request_items": []map[string]any{
			{"request_id": firstID, "expected_version": 3},
			{"request_id": secondID, "expected_version": 2},
		},
		"action": "UPDATE", "changes": map[string]any{"priority": "LOW"},
	}
	stale := executeJSON(t, router, http.MethodPost, bulkPath, staleBody, map[string]string{contract.IdempotencyHeader: "http-bulk-stale"}, supervisor.ID)
	require.Equal(t, http.StatusConflict, stale.Code, stale.Body.String())
	require.Contains(t, stale.Body.String(), `"failing_request_id":"`+secondID+`"`)

	removed := executeJSON(t, router, http.MethodDelete, firstGroupPath, map[string]any{"reason": "Reports are unrelated."}, map[string]string{
		contract.IfMatchHeader: `"3"`,
	}, supervisor.ID)
	require.Equal(t, http.StatusOK, removed.Code, removed.Body.String())
	decoded := map[string]any{}
	require.NoError(t, json.Unmarshal(removed.Body.Bytes(), &decoded))
	requestBody := decoded["request"].(map[string]any)
	_, grouped := requestBody["duplicate_group_id"]
	require.False(t, grouped)
}
