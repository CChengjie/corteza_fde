package city311

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

func TestStaffConstituentRelationshipHTTPContract(t *testing.T) {
	router, st, _ := testRouter(t)
	ctx := context.Background()
	staff, err := store.LookupUserByEmail(ctx, st, "service-agent@city311.example.invalid")
	require.NoError(t, err)
	request, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, "SR-2026-00034")
	require.NoError(t, err)
	path := fmt.Sprintf("/api/v1/staff/service-requests/%d/constituents", request.ID)
	invalidLinkRequest := executeJSON(t, router, http.MethodPost, "/api/v1/staff/service-requests/not-a-number/constituents", map[string]any{}, nil, staff.ID)
	require.Equal(t, http.StatusUnprocessableEntity, invalidLinkRequest.Code, invalidLinkRequest.Body.String())
	invalidUnlinkRequest := executeJSON(t, router, http.MethodDelete, "/api/v1/staff/service-requests/0/constituents/C-3", map[string]any{}, nil, staff.ID)
	require.Equal(t, http.StatusUnprocessableEntity, invalidUnlinkRequest.Code, invalidUnlinkRequest.Body.String())
	missingUnlinkVersion := executeJSON(t, router, http.MethodDelete, path+"/C-3", map[string]any{"reason": "Not linked"}, nil, staff.ID)
	require.Equal(t, http.StatusPreconditionRequired, missingUnlinkVersion.Code, missingUnlinkVersion.Body.String())
	emptyConstituent := executeJSON(t, router, http.MethodDelete, path+"/%20", map[string]any{"reason": "Not linked"}, map[string]string{
		contract.IfMatchHeader: `"1"`,
	}, staff.ID)
	require.Equal(t, http.StatusUnprocessableEntity, emptyConstituent.Code, emptyConstituent.Body.String())

	missingVersion := executeJSON(t, router, http.MethodPost, path, map[string]any{
		"constituent_id": "C-3", "relationship_type": "AFFECTED_RESIDENT", "portal_visible": true, "notify_status": true,
	}, nil, staff.ID)
	require.Equal(t, http.StatusPreconditionRequired, missingVersion.Code, missingVersion.Body.String())
	missingFlags := executeJSON(t, router, http.MethodPost, path, map[string]any{
		"constituent_id": "C-3", "relationship_type": "AFFECTED_RESIDENT",
	}, map[string]string{contract.IfMatchHeader: `"1"`}, staff.ID)
	require.Equal(t, http.StatusUnprocessableEntity, missingFlags.Code, missingFlags.Body.String())

	linked := executeJSON(t, router, http.MethodPost, path, map[string]any{
		"constituent_id": "C-3", "relationship_type": "AFFECTED_RESIDENT", "portal_visible": true, "notify_status": true,
	}, map[string]string{contract.IfMatchHeader: `"1"`}, staff.ID)
	require.Equal(t, http.StatusCreated, linked.Code, linked.Body.String())
	require.Contains(t, linked.Body.String(), `"constituent_links"`)
	require.Contains(t, linked.Body.String(), `"relationship_type":"AFFECTED_RESIDENT"`)

	duplicate := executeJSON(t, router, http.MethodPost, path, map[string]any{
		"constituent_id": "C-3", "relationship_type": "AFFECTED_RESIDENT", "portal_visible": true, "notify_status": true,
	}, map[string]string{contract.IfMatchHeader: `"2"`}, staff.ID)
	require.Equal(t, http.StatusUnprocessableEntity, duplicate.Code, duplicate.Body.String())

	reason := "No longer related"
	unlinked := executeJSON(t, router, http.MethodDelete, path+"/C-3", map[string]any{"reason": reason}, map[string]string{
		contract.IfMatchHeader: `"2"`,
	}, staff.ID)
	require.Equal(t, http.StatusOK, unlinked.Code, unlinked.Body.String())
	var unlinkedDetail contract.StaffServiceRequestDetail
	require.NoError(t, json.Unmarshal(unlinked.Body.Bytes(), &unlinkedDetail))
	require.Len(t, unlinkedDetail.ConstituentLinks, 1)
	require.Equal(t, "C-2", unlinkedDetail.ConstituentLinks[0].ConstituentID)

	removePrimary := executeJSON(t, router, http.MethodDelete, path+"/C-2", map[string]any{"reason": reason}, map[string]string{
		contract.IfMatchHeader: `"3"`,
	}, staff.ID)
	require.Equal(t, http.StatusUnprocessableEntity, removePrimary.Code, removePrimary.Body.String())

	workflowDesigner, err := store.LookupUserByEmail(ctx, st, "workflow-designer@city311.example.invalid")
	require.NoError(t, err)
	forbidden := executeJSON(t, router, http.MethodPost, path, map[string]any{
		"constituent_id": "C-3", "relationship_type": "REPORTER", "portal_visible": true, "notify_status": true,
	}, map[string]string{contract.IfMatchHeader: `"3"`}, workflowDesigner.ID)
	require.Equal(t, http.StatusForbidden, forbidden.Code, forbidden.Body.String())
}

func TestPortalAnonymousLinkAndMyRequestsHTTPContract(t *testing.T) {
	router, _, _, _ := testIdentityRouter(t)
	body := validPortalBody()
	anonymous := executeJSON(t, router, http.MethodPost, "/api/v1/portal/service-requests", body, map[string]string{
		contract.IdempotencyHeader: "portal-anonymous-link-http",
	}, 0)
	require.Equal(t, http.StatusCreated, anonymous.Code, anonymous.Body.String())
	var submitted contract.ServiceRequestResponse
	require.NoError(t, json.Unmarshal(anonymous.Body.Bytes(), &submitted))

	registration := identityRegistrationBody("linked.http", "alex@example.invalid")
	created := executeJSON(t, router, http.MethodPost, "/api/v1/accounts", registration, nil, 0)
	require.Equal(t, http.StatusAccepted, created.Code, created.Body.String())
	signedIn := executeJSON(t, router, http.MethodPost, "/api/v1/session", map[string]any{
		"login_identifier": "linked.http", "password": "StrongPassword1!",
	}, nil, 0)
	require.Equal(t, http.StatusOK, signedIn.Code, signedIn.Body.String())
	cookie := signedIn.Result().Cookies()[0]
	headers := map[string]string{"Cookie": cookie.Name + "=" + cookie.Value}
	invalidPageSize := executeJSON(t, router, http.MethodGet, "/api/v1/portal/service-requests?page_size=0", nil, headers, 0)
	require.Equal(t, http.StatusUnprocessableEntity, invalidPageSize.Code, invalidPageSize.Body.String())

	wrongEmail := executeJSON(t, router, http.MethodPost, "/api/v1/portal/service-requests/link", map[string]any{
		"request_number": submitted.RequestNumber, "email": "wrong@example.invalid",
	}, headers, 0)
	require.Equal(t, http.StatusNotFound, wrongEmail.Code, wrongEmail.Body.String())

	linked := executeJSON(t, router, http.MethodPost, "/api/v1/portal/service-requests/link", map[string]any{
		"request_number": submitted.RequestNumber, "email": "alex@example.invalid",
	}, headers, 0)
	require.Equal(t, http.StatusOK, linked.Code, linked.Body.String())
	require.Contains(t, linked.Body.String(), `"request_number":"`+submitted.RequestNumber+`"`)
	require.Contains(t, linked.Body.String(), `"version":2`)

	requests := executeJSON(t, router, http.MethodGet, "/api/v1/portal/service-requests?page_size=1&sort=-updated_at", nil, headers, 0)
	require.Equal(t, http.StatusOK, requests.Code, requests.Body.String())
	require.Contains(t, requests.Body.String(), `"request_number":"`+submitted.RequestNumber+`"`)
	require.Contains(t, requests.Body.String(), `"total_count":1`)

	unauthenticated := executeJSON(t, router, http.MethodGet, "/api/v1/portal/service-requests", nil, nil, 0)
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code, unauthenticated.Body.String())
}
