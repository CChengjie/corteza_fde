package city311

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

func profileSessionHeaders(t *testing.T, router http.Handler, suffix string) map[string]string {
	t.Helper()
	input := identityRegistrationBody("profile."+suffix, "profile-"+suffix+"@example.invalid")
	created := executeJSON(t, router, http.MethodPost, "/api/v1/accounts", input, nil, 0)
	require.Equal(t, http.StatusAccepted, created.Code, created.Body.String())
	signedIn := executeJSON(t, router, http.MethodPost, "/api/v1/session", map[string]string{
		"login_identifier": input["login_identifier"].(string), "password": input["password"].(string),
	}, nil, 0)
	require.Equal(t, http.StatusOK, signedIn.Code, signedIn.Body.String())
	cookies := signedIn.Result().Cookies()
	require.Len(t, cookies, 1)
	return map[string]string{"Cookie": cookies[0].Name + "=" + cookies[0].Value, "If-Match": `"1"`}
}

func TestProfileHTTPPatchOwnAccountAndSessionProjection(t *testing.T) {
	router, st, _, _ := testIdentityRouter(t)
	ctx := context.Background()
	first := profileSessionHeaders(t, router, "first")
	second := profileSessionHeaders(t, router, "second")
	before := executeJSON(t, router, http.MethodGet, "/api/v1/account/profile", nil, first, 0)
	require.Equal(t, http.StatusOK, before.Code, before.Body.String())
	require.Equal(t, "no-store", before.Header().Get("Cache-Control"))
	var original contract.Constituent
	require.NoError(t, json.Unmarshal(before.Body.Bytes(), &original))
	otherBefore := executeJSON(t, router, http.MethodGet, "/api/v1/account/profile", nil, second, 0)
	addresses := make([]contract.Address, 5)
	for i := range addresses {
		addresses[i] = contract.Address{Line1: fmt.Sprintf("%d Main St", i+1), City: "Buffalo", Region: "NY", PostalCode: "14201", Country: "US", Primary: i == 0}
	}
	updated := executeJSON(t, router, http.MethodPatch, "/api/v1/account/profile", map[string]any{
		"display_name": "Updated Resident", "preferred_language": "ES", "primary_category": "BUSINESS_OWNER",
		"addresses": addresses, "phone_numbers": []contract.PhoneNumber{{Label: contract.PhoneLabelHome, Value: "+17165550101"}},
	}, first, 0)
	require.Equal(t, http.StatusOK, updated.Code, updated.Body.String())
	var profile contract.Constituent
	require.NoError(t, json.Unmarshal(updated.Body.Bytes(), &profile))
	require.Equal(t, original.ConstituentID, profile.ConstituentID)
	require.Equal(t, original.Emails, profile.Emails)
	require.Equal(t, original.LoginIdentifier, profile.LoginIdentifier)
	require.Equal(t, original.EmailOptOut, profile.EmailOptOut)
	require.Len(t, profile.Addresses, 5)
	require.Equal(t, "Updated Resident", profile.DisplayName)
	otherAfter := executeJSON(t, router, http.MethodGet, "/api/v1/account/profile", nil, second, 0)
	require.JSONEq(t, otherBefore.Body.String(), otherAfter.Body.String())
	current := executeJSON(t, router, http.MethodGet, "/api/v1/session", nil, first, 0)
	var session contract.Session
	require.NoError(t, json.Unmarshal(current.Body.Bytes(), &session))
	require.Equal(t, contract.LanguageES, session.PreferredLanguage)
	require.Equal(t, profile.DisplayName, session.Actor.DisplayName)
	account, err := store.LookupCity311LocalAccountByLoginIdentifier(ctx, st, original.LoginIdentifier)
	require.NoError(t, err)
	user, err := store.LookupUserByID(ctx, st, account.ID)
	require.NoError(t, err)
	require.Equal(t, "es", user.Meta.PreferredLanguage)
	// Identifier maintenance must preserve the profile and session changes.
	changedIdentifier := executeJSON(t, router, http.MethodPost, "/api/v1/account/login-identifier", map[string]string{
		"login_identifier": "renamed.first", "current_password": "StrongPassword1!",
	}, first, 0)
	require.Equal(t, http.StatusOK, changedIdentifier.Code, changedIdentifier.Body.String())
	loaded := executeJSON(t, router, http.MethodGet, "/api/v1/account/profile", nil, first, 0)
	require.NoError(t, json.Unmarshal(loaded.Body.Bytes(), &profile))
	require.Equal(t, "renamed.first", profile.LoginIdentifier)
	require.Len(t, profile.Addresses, 5)
	require.Equal(t, "Updated Resident", profile.DisplayName)
}

func TestProfileHTTPRejectsInvalidOrProtectedFieldsAtomically(t *testing.T) {
	router, _, _, _ := testIdentityRouter(t)
	headers := profileSessionHeaders(t, router, "validation")
	before := executeJSON(t, router, http.MethodGet, "/api/v1/account/profile", nil, headers, 0)
	for _, patch := range []any{
		nil, []string{}, map[string]any{"display_name": nil}, map[string]any{"addresses": nil},
		map[string]any{"emails": []string{"unverified@example.invalid"}},
		map[string]any{"login_identifier": "unauthorised"}, map[string]any{"constituent_id": "C-other"},
		map[string]any{"email_opt_out": true}, map[string]any{"owning_department": "STREETS"},
		map[string]any{"display_name": "Must not be saved", "phone_numbers": []map[string]string{{"label": "HOME", "value": "bad"}}},
		map[string]any{"preferred_language": "FR"},
	} {
		response := executeJSON(t, router, http.MethodPatch, "/api/v1/account/profile", patch, headers, 0)
		require.Equal(t, http.StatusUnprocessableEntity, response.Code, response.Body.String())
		require.Contains(t, response.Body.String(), "VALIDATION_ERROR")
		after := executeJSON(t, router, http.MethodGet, "/api/v1/account/profile", nil, headers, 0)
		require.JSONEq(t, before.Body.String(), after.Body.String())
	}
}

func TestProfileHTTPLanguageOptionalSessionAndRoleBoundary(t *testing.T) {
	router, st, _, _ := testIdentityRouter(t)
	ctx := context.Background()
	headers := profileSessionHeaders(t, router, "language")
	accountsBefore, _, err := store.SearchCity311LocalAccounts(ctx, st, composeTypes.City311LocalAccountFilter{})
	require.NoError(t, err)
	for _, headers := range []map[string]string{nil, {"Cookie": "city311_session=invalid"}} {
		response := executeJSON(t, router, http.MethodPatch, "/api/v1/preferences/language", map[string]string{"language": "VI"}, headers, 0)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		require.JSONEq(t, `{"language":"VI"}`, response.Body.String())
	}
	accountsAfter, _, err := store.SearchCity311LocalAccounts(ctx, st, composeTypes.City311LocalAccountFilter{})
	require.NoError(t, err)
	require.Equal(t, accountsBefore, accountsAfter)
	response := executeJSON(t, router, http.MethodPatch, "/api/v1/preferences/language", map[string]string{"language": "ES"}, headers, 0)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	profile := executeJSON(t, router, http.MethodGet, "/api/v1/account/profile", nil, headers, 0)
	require.Contains(t, profile.Body.String(), `"preferred_language":"ES"`)
	session := executeJSON(t, router, http.MethodGet, "/api/v1/session", nil, headers, 0)
	require.Contains(t, session.Body.String(), `"preferred_language":"ES"`)
	for _, invalid := range []any{nil, map[string]string{}, map[string]string{"language": "FR"}, map[string]any{"language": nil}} {
		response = executeJSON(t, router, http.MethodPatch, "/api/v1/preferences/language", invalid, headers, 0)
		require.Equal(t, http.StatusUnprocessableEntity, response.Code)
	}
	account, err := store.LookupCity311LocalAccountByLoginIdentifier(ctx, st, "profile.language")
	require.NoError(t, err)
	actor, err := store.LookupCity311ActorProfileByID(ctx, st, account.ID)
	require.NoError(t, err)
	actor.ApplicationRoles = composeTypes.City311ApplicationRoleSet{contract.ApplicationRoleWorkflowDesigner}
	require.NoError(t, store.UpdateCity311ActorProfile(ctx, st, actor))
	for _, method := range []string{http.MethodGet, http.MethodPatch} {
		response = executeJSON(t, router, method, "/api/v1/account/profile", map[string]string{}, headers, 0)
		require.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
		response = executeJSON(t, router, method, "/api/v1/account/profile", map[string]string{}, nil, account.ID)
		require.Equal(t, http.StatusUnauthorized, response.Code, "Corteza identity alone is not a City 311 constituent session")
	}
	response = executeJSON(t, router, http.MethodPatch, "/api/v1/preferences/language", map[string]string{"language": "VI"}, headers, 0)
	require.Equal(t, http.StatusOK, response.Code)
	account, err = store.LookupCity311LocalAccountByID(ctx, st, account.ID)
	require.NoError(t, err)
	require.Equal(t, "ES", account.PreferredLanguage, "staff browser preference must not mutate a constituent profile")
	// Signing out invalidates profile access immediately.
	response = executeJSON(t, router, http.MethodDelete, "/api/v1/session", nil, headers, 0)
	require.Equal(t, http.StatusNoContent, response.Code)
	response = executeJSON(t, router, http.MethodGet, "/api/v1/account/profile", nil, headers, 0)
	require.Equal(t, http.StatusUnauthorized, response.Code)
}

func TestProfileHTTPUsesETagAndRejectsStaleWrites(t *testing.T) {
	router, _, _, _ := testIdentityRouter(t)
	headers := profileSessionHeaders(t, router, "version")
	initial := executeJSON(t, router, http.MethodGet, "/api/v1/account/profile", nil, headers, 0)
	require.Equal(t, `"1"`, initial.Header().Get("ETag"))
	for _, value := range []string{"", "1", `W/"1"`, `"0"`, "*", `"1","2"`} {
		headers["If-Match"] = value
		response := executeJSON(t, router, http.MethodPatch, "/api/v1/account/profile", map[string]string{"display_name": "Not saved"}, headers, 0)
		require.Equal(t, 428, response.Code, response.Body.String())
		require.Contains(t, response.Body.String(), "EXPECTED_VERSION_REQUIRED")
	}
	headers["If-Match"] = initial.Header().Get("ETag")
	saved := executeJSON(t, router, http.MethodPatch, "/api/v1/account/profile", map[string]string{"display_name": "Saved"}, headers, 0)
	require.Equal(t, 200, saved.Code, saved.Body.String())
	require.Equal(t, `"2"`, saved.Header().Get("ETag"))
	require.NotContains(t, saved.Body.String(), profilePrivateRevisionForTest)
	stale := executeJSON(t, router, http.MethodPatch, "/api/v1/account/profile", map[string]string{"display_name": "Stale overwrite"}, headers, 0)
	require.Equal(t, 409, stale.Code, stale.Body.String())
	var failure contract.APIError
	require.NoError(t, json.Unmarshal(stale.Body.Bytes(), &failure))
	require.Equal(t, contract.ErrorVersionConflict, failure.Error)
	require.Equal(t, uint64(2), *failure.CurrentVersion)
	loaded := executeJSON(t, router, http.MethodGet, "/api/v1/account/profile", nil, headers, 0)
	require.JSONEq(t, saved.Body.String(), loaded.Body.String())
	headers["If-Match"] = loaded.Header().Get("ETag")
	noop := executeJSON(t, router, http.MethodPatch, "/api/v1/account/profile", map[string]string{}, headers, 0)
	require.Equal(t, 200, noop.Code)
	require.Equal(t, loaded.Header().Get("ETag"), noop.Header().Get("ETag"))
}

const profilePrivateRevisionForTest = "_profile_version"
