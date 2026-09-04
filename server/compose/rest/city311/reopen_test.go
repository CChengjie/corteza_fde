package city311

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

func TestReopenRequestAndApprovalHTTPContract(t *testing.T) {
	router, st, _, _ := testIdentityRouter(t)
	ctx := context.Background()
	require.Equal(t, http.StatusAccepted, executeJSON(t, router, http.MethodPost, "/api/v1/accounts", identityRegistrationBody(
		"reopen.owner", "reopen-owner@example.invalid",
	), nil, 0).Code)
	signedIn := executeJSON(t, router, http.MethodPost, "/api/v1/session", map[string]any{
		"login_identifier": "reopen.owner", "password": "StrongPassword1!",
	}, nil, 0)
	require.Equal(t, http.StatusOK, signedIn.Code, signedIn.Body.String())
	cookie := signedIn.Result().Cookies()[0]
	portalHeaders := map[string]string{
		"Cookie": cookie.Name + "=" + cookie.Value, contract.IdempotencyHeader: "portal-reopen-owner-request",
	}
	submitted := executeJSON(t, router, http.MethodPost, "/api/v1/portal/service-requests", validPortalBody(), portalHeaders, 0)
	require.Equal(t, http.StatusCreated, submitted.Code, submitted.Body.String())
	var created contract.ServiceRequestResponse
	require.NoError(t, json.Unmarshal(submitted.Body.Bytes(), &created))
	requestID, err := strconv.ParseUint(created.RequestID, 10, 64)
	require.NoError(t, err)
	request, err := store.LookupCity311ServiceRequestByID(ctx, st, requestID)
	require.NoError(t, err)
	request.Status = contract.ServiceRequestStatusResolved
	require.NoError(t, store.UpdateCity311ServiceRequest(ctx, st, request))

	portalPath := "/api/v1/portal/service-requests/" + created.RequestID + "/reopen"
	requested := executeJSON(t, router, http.MethodPost, portalPath, map[string]any{
		"reason": "The repair has failed.",
	}, portalHeaders, 0)
	require.Equal(t, http.StatusAccepted, requested.Code, requested.Body.String())
	require.JSONEq(t, `{"request_id":"`+created.RequestID+`","status":"PENDING_APPROVAL"}`, requested.Body.String())

	duplicate := executeJSON(t, router, http.MethodPost, portalPath, map[string]any{
		"reason": "A second pending request.",
	}, portalHeaders, 0)
	require.Equal(t, http.StatusUnprocessableEntity, duplicate.Code, duplicate.Body.String())

	serviceAgent, err := store.LookupUserByEmail(ctx, st, "service-agent@city311.example.invalid")
	require.NoError(t, err)
	direct := executeJSON(t, router, http.MethodPost, "/api/v1/staff/service-requests/"+created.RequestID+"/transitions", map[string]any{
		"to_status": "REOPENED", "reason": "Bypass approval",
	}, map[string]string{contract.IfMatchHeader: `"1"`}, serviceAgent.ID)
	require.Equal(t, http.StatusUnprocessableEntity, direct.Code, direct.Body.String())
	require.Contains(t, direct.Body.String(), string(contract.ErrorInvalidStatusTransition))

	approvePath := "/api/v1/staff/service-requests/" + created.RequestID + "/reopen/approve"
	missingVersion := executeJSON(t, router, http.MethodPost, approvePath, map[string]any{"reason": "Reviewed"}, nil, serviceAgent.ID)
	require.Equal(t, http.StatusPreconditionRequired, missingVersion.Code, missingVersion.Body.String())
	forbidden := executeJSON(t, router, http.MethodPost, approvePath, map[string]any{"reason": "Reviewed"}, map[string]string{
		contract.IfMatchHeader: `"1"`,
	}, serviceAgent.ID)
	require.Equal(t, http.StatusForbidden, forbidden.Code, forbidden.Body.String())

	supervisor, err := store.LookupUserByEmail(ctx, st, "supervisor@city311.example.invalid")
	require.NoError(t, err)
	approved := executeJSON(t, router, http.MethodPost, approvePath, map[string]any{
		"reason": "Resident evidence confirmed.",
	}, map[string]string{contract.IfMatchHeader: `"1"`}, supervisor.ID)
	require.Equal(t, http.StatusOK, approved.Code, approved.Body.String())
	require.Contains(t, approved.Body.String(), `"status":"REOPENED"`)
	require.Contains(t, approved.Body.String(), `"version":2`)
	require.Contains(t, approved.Body.String(), `"action":"REOPENED"`)

	persisted, err := store.LookupCity311ServiceRequestByID(ctx, st, requestID)
	require.NoError(t, err)
	require.Equal(t, contract.ServiceRequestStatusReopened, persisted.Status)
	reopens, _, err := store.SearchCity311ReopenRequests(ctx, st, composeTypes.City311ReopenRequestFilter{RequestID: requestID})
	require.NoError(t, err)
	require.Len(t, reopens, 1)
	require.Equal(t, "APPROVED", reopens[0].Status)
}
