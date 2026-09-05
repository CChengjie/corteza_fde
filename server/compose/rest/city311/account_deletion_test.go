package city311

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccountDeletionHTTPRevokesCookieAndSession(t *testing.T) {
	router, _, _, _ := testIdentityRouter(t)
	registration := identityRegistrationBody("delete.http", "delete-http@example.invalid")
	require.Equal(t, http.StatusAccepted, executeJSON(t, router, http.MethodPost, "/api/v1/accounts", registration, nil, 0).Code)
	signedIn := executeJSON(t, router, http.MethodPost, "/api/v1/session", map[string]any{
		"login_identifier": "delete.http", "password": "StrongPassword1!",
	}, nil, 0)
	require.Equal(t, http.StatusOK, signedIn.Code)
	cookie := signedIn.Result().Cookies()[0]
	headers := map[string]string{"Cookie": cookie.Name + "=" + cookie.Value}
	deleted := executeJSON(t, router, http.MethodDelete, "/api/v1/account", nil, headers, 0)
	require.Equal(t, http.StatusNoContent, deleted.Code)
	require.Equal(t, "no-store", deleted.Header().Get("Cache-Control"))
	require.Len(t, deleted.Result().Cookies(), 1)
	require.Less(t, deleted.Result().Cookies()[0].MaxAge, 0)
	profile := executeJSON(t, router, http.MethodGet, "/api/v1/account/profile", nil, headers, 0)
	require.Equal(t, http.StatusUnauthorized, profile.Code)
	login := executeJSON(t, router, http.MethodPost, "/api/v1/session", map[string]any{
		"login_identifier": "delete.http", "password": "StrongPassword1!",
	}, nil, 0)
	require.Equal(t, http.StatusUnauthorized, login.Code)

	unauthenticated := executeJSON(t, router, http.MethodDelete, "/api/v1/account", nil, nil, 0)
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)
}
