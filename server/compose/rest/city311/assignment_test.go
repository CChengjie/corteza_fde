package city311

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

func TestStaffAssignmentAndCollaboratorHTTPContracts(t *testing.T) {
	router, st, _ := testRouter(t)
	ctx := context.Background()
	request, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, "SR-2026-00034")
	require.NoError(t, err)
	supervisor, err := store.LookupUserByEmail(ctx, st, "supervisor@city311.example.invalid")
	require.NoError(t, err)
	agent, err := store.LookupUserByEmail(ctx, st, "service-agent@city311.example.invalid")
	require.NoError(t, err)
	manager, err := store.LookupUserByEmail(ctx, st, "department-manager@city311.example.invalid")
	require.NoError(t, err)
	requestID := strconv.FormatUint(request.ID, 10)
	assignmentPath := "/api/v1/staff/service-requests/" + requestID + "/assignment"

	unauthenticated := executeJSON(t, router, http.MethodPost, assignmentPath, map[string]any{}, nil, 0)
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)
	missingVersion := executeJSON(t, router, http.MethodPost, assignmentPath, map[string]any{
		"assignee_id": strconv.FormatUint(agent.ID, 10), "reason": "Assign to response team.",
	}, nil, supervisor.ID)
	require.Equal(t, http.StatusPreconditionRequired, missingVersion.Code, missingVersion.Body.String())
	forbidden := executeJSON(t, router, http.MethodPost, assignmentPath, map[string]any{
		"assignee_id": strconv.FormatUint(agent.ID, 10), "reason": "Self assignment is not this endpoint's role.",
	}, map[string]string{contract.IfMatchHeader: `"1"`}, agent.ID)
	require.Equal(t, http.StatusForbidden, forbidden.Code, forbidden.Body.String())

	assigned := executeJSON(t, router, http.MethodPost, assignmentPath, map[string]any{
		"assignee_id": strconv.FormatUint(agent.ID, 10), "reason": "Assign to response team.",
	}, map[string]string{contract.IfMatchHeader: `"1"`}, supervisor.ID)
	require.Equal(t, http.StatusOK, assigned.Code, assigned.Body.String())
	require.Contains(t, assigned.Body.String(), `"primary_assignee_id":"`+strconv.FormatUint(agent.ID, 10)+`"`)
	require.Contains(t, assigned.Body.String(), `"version":2`)

	collaboratorPath := "/api/v1/staff/service-requests/" + requestID + "/collaborators/" + strconv.FormatUint(manager.ID, 10)
	added := executeJSON(t, router, http.MethodPut, collaboratorPath, map[string]any{"reason": "Manager coordination."}, map[string]string{
		contract.IfMatchHeader: `"2"`,
	}, supervisor.ID)
	require.Equal(t, http.StatusOK, added.Code, added.Body.String())
	require.Contains(t, added.Body.String(), `"collaborator_ids":["`+strconv.FormatUint(manager.ID, 10)+`"]`)
	require.Contains(t, added.Body.String(), `"version":3`)

	removed := executeJSON(t, router, http.MethodDelete, collaboratorPath, map[string]any{"reason": "Coordination complete."}, map[string]string{
		contract.IfMatchHeader: `"3"`,
	}, supervisor.ID)
	require.Equal(t, http.StatusOK, removed.Code, removed.Body.String())
	require.Contains(t, removed.Body.String(), `"collaborator_ids":[]`)
	require.Contains(t, removed.Body.String(), `"version":4`)

	invalidStaff := executeJSON(t, router, http.MethodPut, "/api/v1/staff/service-requests/"+requestID+"/collaborators/not-a-number", map[string]any{"reason": "Invalid"}, map[string]string{
		contract.IfMatchHeader: `"4"`,
	}, supervisor.ID)
	require.Equal(t, http.StatusUnprocessableEntity, invalidStaff.Code)
	stale := executeJSON(t, router, http.MethodPost, assignmentPath, map[string]any{
		"assignee_id": strconv.FormatUint(agent.ID, 10), "reason": "Stale update.",
	}, map[string]string{contract.IfMatchHeader: `"1"`}, supervisor.ID)
	require.Equal(t, http.StatusConflict, stale.Code)
	require.Contains(t, stale.Body.String(), string(contract.ErrorVersionConflict))
}
