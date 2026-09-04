package city311

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

func TestStaffRequestNoteHTTPContract(t *testing.T) {
	router, st, _ := testRouter(t)
	ctx := context.Background()
	staff, err := store.LookupUserByEmail(ctx, st, "service-agent@city311.example.invalid")
	require.NoError(t, err)
	request, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, "SR-2026-00034")
	require.NoError(t, err)
	path := fmt.Sprintf("/api/v1/staff/service-requests/%d/notes", request.ID)

	missingVisibility := executeJSON(t, router, http.MethodPost, path, map[string]any{"body": "Missing visibility"}, nil, staff.ID)
	require.Equal(t, http.StatusUnprocessableEntity, missingVisibility.Code, missingVisibility.Body.String())
	require.Contains(t, missingVisibility.Body.String(), "/portal_visible")

	created := executeJSON(t, router, http.MethodPost, path, map[string]any{
		"body": "Visible progress note", "portal_visible": true,
	}, nil, staff.ID)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	var note contract.RequestNote
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &note))
	require.Equal(t, strconv.FormatUint(request.ID, 10), note.RequestID)
	require.Equal(t, "Visible progress note", note.Body)
	require.True(t, note.PortalVisible)
	require.Equal(t, contract.AuditActorStaff, note.AuthorType)

	deleted := executeJSON(t, router, http.MethodDelete, path, nil, nil, staff.ID)
	require.Equal(t, http.StatusMethodNotAllowed, deleted.Code)

	workflowDesigner, err := store.LookupUserByEmail(ctx, st, "workflow-designer@city311.example.invalid")
	require.NoError(t, err)
	forbidden := executeJSON(t, router, http.MethodPost, path, map[string]any{
		"body": "Unauthorised note", "portal_visible": false,
	}, nil, workflowDesigner.ID)
	require.Equal(t, http.StatusForbidden, forbidden.Code, forbidden.Body.String())
}

func TestPortalConstituentNoteHTTPContract(t *testing.T) {
	router, _, _, _ := testIdentityRouter(t)
	require.Equal(t, http.StatusAccepted, executeJSON(t, router, http.MethodPost, "/api/v1/accounts", identityRegistrationBody(
		"note.owner", "note-owner@example.invalid",
	), nil, 0).Code)
	signedIn := executeJSON(t, router, http.MethodPost, "/api/v1/session", map[string]any{
		"login_identifier": "note.owner", "password": "StrongPassword1!",
	}, nil, 0)
	require.Equal(t, http.StatusOK, signedIn.Code, signedIn.Body.String())
	cookie := signedIn.Result().Cookies()[0]
	headers := map[string]string{
		"Cookie": cookie.Name + "=" + cookie.Value, contract.IdempotencyHeader: "portal-note-owner-request",
	}
	submitted := executeJSON(t, router, http.MethodPost, "/api/v1/portal/service-requests", validPortalBody(), headers, 0)
	require.Equal(t, http.StatusCreated, submitted.Code, submitted.Body.String())
	var request contract.ServiceRequestResponse
	require.NoError(t, json.Unmarshal(submitted.Body.Bytes(), &request))
	path := "/api/v1/portal/service-requests/" + request.RequestID + "/notes"

	created := executeJSON(t, router, http.MethodPost, path, map[string]any{
		"body": "The pothole has become wider.", "portal_visible": true,
	}, headers, 0)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	var note contract.RequestNote
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &note))
	require.Equal(t, request.RequestID, note.RequestID)
	require.NotEmpty(t, note.AuthorConstituentID)
	require.Equal(t, contract.AuditActorConstituent, note.AuthorType)

	unauthenticated := executeJSON(t, router, http.MethodPost, path, map[string]any{
		"body": "No session", "portal_visible": true,
	}, nil, 0)
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code, unauthenticated.Body.String())

	require.Equal(t, http.StatusAccepted, executeJSON(t, router, http.MethodPost, "/api/v1/accounts", identityRegistrationBody(
		"note.other", "note-other@example.invalid",
	), nil, 0).Code)
	otherSignIn := executeJSON(t, router, http.MethodPost, "/api/v1/session", map[string]any{
		"login_identifier": "note.other", "password": "StrongPassword1!",
	}, nil, 0)
	require.Equal(t, http.StatusOK, otherSignIn.Code, otherSignIn.Body.String())
	otherCookie := otherSignIn.Result().Cookies()[0]
	notFound := executeJSON(t, router, http.MethodPost, path, map[string]any{
		"body": "Cross-account attempt", "portal_visible": true,
	}, map[string]string{"Cookie": otherCookie.Name + "=" + otherCookie.Value}, 0)
	require.Equal(t, http.StatusNotFound, notFound.Code, notFound.Body.String())
}
