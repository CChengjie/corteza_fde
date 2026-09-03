package city311

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/stretchr/testify/require"
)

func TestDraftHTTPRequiresConstituentAndSupportsVersionedSubmit(t *testing.T) {
	router, _, _, _ := testIdentityRouter(t)
	unauthenticated := executeJSON(t, router, http.MethodPost, "/api/v1/portal/service-request-drafts", map[string]any{}, nil, 0)
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)

	registration := identityRegistrationBody("draft.http", "draft-http@example.invalid")
	created := executeJSON(t, router, http.MethodPost, "/api/v1/accounts", registration, nil, 0)
	require.Equal(t, http.StatusAccepted, created.Code, created.Body.String())
	signedIn := executeJSON(t, router, http.MethodPost, "/api/v1/session", map[string]any{
		"login_identifier": "draft.http", "password": "StrongPassword1!",
	}, nil, 0)
	require.Equal(t, http.StatusOK, signedIn.Code, signedIn.Body.String())
	cookie := signedIn.Result().Cookies()[0]
	headers := map[string]string{"Cookie": cookie.Name + "=" + cookie.Value}

	body := validPortalBody()
	draft := executeJSON(t, router, http.MethodPost, "/api/v1/portal/service-request-drafts", body, headers, 0)
	require.Equal(t, http.StatusCreated, draft.Code, draft.Body.String())
	require.Equal(t, `"1"`, draft.Header().Get("ETag"))
	require.Equal(t, "no-store", draft.Header().Get("Cache-Control"))
	require.NotContains(t, draft.Body.String(), `"request_number"`)
	var createdDraft contract.ServiceRequest
	require.NoError(t, decodeResponse(draft, &createdDraft))
	require.Empty(t, createdDraft.RequestNumber)

	missingVersion := executeJSON(t, router, http.MethodPatch, "/api/v1/portal/service-request-drafts/"+createdDraft.RequestID, map[string]any{"summary": "Changed draft"}, headers, 0)
	require.Equal(t, http.StatusPreconditionRequired, missingVersion.Code, missingVersion.Body.String())

	updated := executeJSON(t, router, http.MethodPatch, "/api/v1/portal/service-request-drafts/"+createdDraft.RequestID, map[string]any{"summary": "Changed draft"}, map[string]string{
		"Cookie": cookie.Name + "=" + cookie.Value, contract.IfMatchHeader: `"1"`,
	}, 0)
	require.Equal(t, http.StatusOK, updated.Code, updated.Body.String())
	require.Equal(t, `"2"`, updated.Header().Get("ETag"))

	submitted := executeJSON(t, router, http.MethodPost, "/api/v1/portal/service-request-drafts/"+createdDraft.RequestID+"/submit", nil, map[string]string{
		"Cookie": cookie.Name + "=" + cookie.Value, contract.IfMatchHeader: `"2"`,
	}, 0)
	require.Equal(t, http.StatusOK, submitted.Code, submitted.Body.String())
	require.Contains(t, submitted.Body.String(), `"status":"SUBMITTED"`)
	require.Contains(t, submitted.Body.String(), `"request_number":"SR-2026-00041"`)
	require.Equal(t, `"3"`, submitted.Header().Get("ETag"))

	getAfterSubmit := executeJSON(t, router, http.MethodGet, "/api/v1/portal/service-request-drafts/"+createdDraft.RequestID, nil, headers, 0)
	require.Equal(t, http.StatusNotFound, getAfterSubmit.Code, getAfterSubmit.Body.String())
}

func TestDraftHTTPDeleteAndInvalidIdentifiers(t *testing.T) {
	router, _, _, _ := testIdentityRouter(t)
	registration := identityRegistrationBody("draft.delete", "draft-delete@example.invalid")
	created := executeJSON(t, router, http.MethodPost, "/api/v1/accounts", registration, nil, 0)
	require.Equal(t, http.StatusAccepted, created.Code, created.Body.String())
	signedIn := executeJSON(t, router, http.MethodPost, "/api/v1/session", map[string]any{
		"login_identifier": "draft.delete", "password": "StrongPassword1!",
	}, nil, 0)
	require.Equal(t, http.StatusOK, signedIn.Code, signedIn.Body.String())
	cookie := signedIn.Result().Cookies()[0]
	headers := map[string]string{"Cookie": cookie.Name + "=" + cookie.Value}

	invalidGet := executeJSON(t, router, http.MethodGet, "/api/v1/portal/service-request-drafts/not-a-number", nil, headers, 0)
	require.Equal(t, http.StatusUnprocessableEntity, invalidGet.Code, invalidGet.Body.String())
	invalidSubmit := executeJSON(t, router, http.MethodPost, "/api/v1/portal/service-request-drafts/0/submit", nil, headers, 0)
	require.Equal(t, http.StatusUnprocessableEntity, invalidSubmit.Code, invalidSubmit.Body.String())

	draft := executeJSON(t, router, http.MethodPost, "/api/v1/portal/service-request-drafts", map[string]any{}, headers, 0)
	require.Equal(t, http.StatusCreated, draft.Code, draft.Body.String())
	var createdDraft contract.ServiceRequest
	require.NoError(t, decodeResponse(draft, &createdDraft))
	draftPath := "/api/v1/portal/service-request-drafts/" + createdDraft.RequestID

	missingVersion := executeJSON(t, router, http.MethodDelete, draftPath, nil, headers, 0)
	require.Equal(t, http.StatusPreconditionRequired, missingVersion.Code, missingVersion.Body.String())
	deleted := executeJSON(t, router, http.MethodDelete, draftPath, nil, map[string]string{
		"Cookie": cookie.Name + "=" + cookie.Value, contract.IfMatchHeader: `"1"`,
	}, 0)
	require.Equal(t, http.StatusNoContent, deleted.Code, deleted.Body.String())
	missing := executeJSON(t, router, http.MethodGet, draftPath, nil, headers, 0)
	require.Equal(t, http.StatusNotFound, missing.Code, missing.Body.String())
}

func decodeResponse(response *httptest.ResponseRecorder, target any) error {
	return json.NewDecoder(response.Body).Decode(target)
}
