package city311

import (
	"context"
	"net/http"
	"testing"

	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/stretchr/testify/require"
)

func registerAndSignInEmailReplacement(t *testing.T, router http.Handler, identifier, email string) map[string]string {
	t.Helper()
	registered := executeJSON(t, router, http.MethodPost, "/api/v1/accounts", identityRegistrationBody(identifier, email), nil, 0)
	require.Equal(t, http.StatusAccepted, registered.Code, registered.Body.String())
	signedIn := executeJSON(t, router, http.MethodPost, "/api/v1/session", map[string]any{
		"login_identifier": identifier,
		"password":         "StrongPassword1!",
	}, nil, 0)
	require.Equal(t, http.StatusOK, signedIn.Code, signedIn.Body.String())
	cookies := signedIn.Result().Cookies()
	require.Len(t, cookies, 1)
	return map[string]string{"Cookie": cookies[0].Name + "=" + cookies[0].Value}
}

func TestVerifiedEmailReplacementHTTPFlow(t *testing.T) {
	router, _, notifier, identity := testIdentityRouter(t)
	headers := registerAndSignInEmailReplacement(t, router, "email.http", "email-http@example.invalid")

	unauthenticated := executeJSON(t, router, http.MethodPost, contract.EmailReplacementRequestPath, map[string]any{
		"email": "new-http@example.invalid",
	}, nil, 0)
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)
	require.Equal(t, "no-store", unauthenticated.Header().Get("Cache-Control"))

	malformed := executeJSON(t, router, http.MethodPost, contract.EmailReplacementRequestPath, map[string]any{
		"email": "new-http@example.invalid", "unexpected": true,
	}, headers, 0)
	require.Equal(t, http.StatusUnprocessableEntity, malformed.Code)
	require.Contains(t, malformed.Body.String(), string(contract.ErrorValidation))

	requested := executeJSON(t, router, http.MethodPost, contract.EmailReplacementRequestPath, map[string]any{
		"email": "New-HTTP@Example.Invalid",
	}, headers, 0)
	require.Equal(t, http.StatusAccepted, requested.Code, requested.Body.String())
	require.JSONEq(t, `{"accepted":true}`, requested.Body.String())
	require.Equal(t, "no-store", requested.Header().Get("Cache-Control"))

	oldStillWorks := executeJSON(t, router, http.MethodPost, "/api/v1/session", map[string]any{
		"login_identifier": "email-http@example.invalid", "password": "StrongPassword1!",
	}, nil, 0)
	require.Equal(t, http.StatusOK, oldStillWorks.Code, oldStillWorks.Body.String())
	require.NoError(t, identity.RetryPendingNotifications(context.Background()))
	require.Equal(t, []string{"new-http@example.invalid"}, notifier.replacementEmails)
	require.Len(t, notifier.replacementTokens, 1)

	unknown := executeJSON(t, router, http.MethodPost, contract.EmailReplacementConfirmPath, map[string]any{
		"token": "unknown-token",
	}, nil, 0)
	require.Equal(t, http.StatusUnprocessableEntity, unknown.Code)
	require.Contains(t, unknown.Body.String(), string(contract.ErrorInvalidEmailVerificationToken))
	require.Equal(t, "no-store", unknown.Header().Get("Cache-Control"))

	confirmed := executeJSON(t, router, http.MethodPost, contract.EmailReplacementConfirmPath, map[string]any{
		"token": notifier.replacementTokens[0],
	}, nil, 0)
	require.Equal(t, http.StatusOK, confirmed.Code, confirmed.Body.String())
	require.JSONEq(t, `{"verified_email":"new-http@example.invalid"}`, confirmed.Body.String())
	require.Equal(t, "no-store", confirmed.Header().Get("Cache-Control"))

	profile := executeJSON(t, router, http.MethodGet, "/api/v1/account/profile", nil, headers, 0)
	require.Equal(t, http.StatusOK, profile.Code, profile.Body.String())
	require.Contains(t, profile.Body.String(), `"emails":["new-http@example.invalid"]`)
	require.Equal(t, `"2"`, profile.Header().Get("ETag"))
	oldRejected := executeJSON(t, router, http.MethodPost, "/api/v1/session", map[string]any{
		"login_identifier": "email-http@example.invalid", "password": "StrongPassword1!",
	}, nil, 0)
	require.Equal(t, http.StatusUnauthorized, oldRejected.Code)
	newAccepted := executeJSON(t, router, http.MethodPost, "/api/v1/session", map[string]any{
		"login_identifier": "new-http@example.invalid", "password": "StrongPassword1!",
	}, nil, 0)
	require.Equal(t, http.StatusOK, newAccepted.Code, newAccepted.Body.String())

	require.NoError(t, identity.RetryPendingNotifications(context.Background()))
	require.Equal(t, 1, notifier.notices, "the old address must receive a security notice")
	reused := executeJSON(t, router, http.MethodPost, contract.EmailReplacementConfirmPath, map[string]any{
		"token": notifier.replacementTokens[0],
	}, nil, 0)
	require.Equal(t, http.StatusUnprocessableEntity, reused.Code)
	require.Contains(t, reused.Body.String(), string(contract.ErrorInvalidEmailVerificationToken))
}

func TestEmailReplacementHTTPDoesNotDiscloseClaimedAddresses(t *testing.T) {
	router, _, notifier, identity := testIdentityRouter(t)
	ownerHeaders := registerAndSignInEmailReplacement(t, router, "email.privacy.http", "privacy-http@example.invalid")
	_ = registerAndSignInEmailReplacement(t, router, "email.claimed.http", "claimed-http@example.invalid")

	claimed := executeJSON(t, router, http.MethodPost, contract.EmailReplacementRequestPath, map[string]any{
		"email": "claimed-http@example.invalid",
	}, ownerHeaders, 0)
	current := executeJSON(t, router, http.MethodPost, contract.EmailReplacementRequestPath, map[string]any{
		"email": "privacy-http@example.invalid",
	}, ownerHeaders, 0)
	require.Equal(t, http.StatusAccepted, claimed.Code)
	require.Equal(t, claimed.Code, current.Code)
	require.JSONEq(t, claimed.Body.String(), current.Body.String())
	require.NoError(t, identity.RetryPendingNotifications(context.Background()))
	require.Empty(t, notifier.replacementTokens)
}
