package city311

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	city311Service "github.com/cortezaproject/corteza/server/compose/service/city311"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/cortezaproject/corteza/server/store/adapters/rdbms/drivers/sqlite"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type restFederationProvider struct {
	start    city311Service.FederationStartRequest
	callback city311Service.FederationCallbackRequest
	claims   city311Service.FederatedClaims
}

func (provider *restFederationProvider) Start(_ context.Context, request city311Service.FederationStartRequest) (city311Service.FederationAuthorization, error) {
	provider.start = request
	return city311Service.FederationAuthorization{URL: "https://identity.example.test/authorize?state=" + url.QueryEscape(request.State)}, nil
}

func (provider *restFederationProvider) Callback(_ context.Context, request city311Service.FederationCallbackRequest) (city311Service.FederatedClaims, error) {
	provider.callback = request
	return provider.claims, nil
}

func testFederationRouter(t *testing.T) (http.Handler, store.Storer, *restFederationProvider) {
	t.Helper()
	ctx := context.Background()
	dsn := fmt.Sprintf("sqlite3://file:%s-federation?mode=memory&cache=shared", t.Name())
	st, err := sqlite.Connect(ctx, dsn)
	require.NoError(t, err)
	require.NoError(t, store.Upgrade(ctx, zap.NewNop(), st))
	svc := city311Service.New(st)
	require.NoError(t, svc.Seed(ctx, time.Date(2026, 2, 3, 15, 4, 5, 0, time.UTC)))
	provider := &restFederationProvider{claims: city311Service.FederatedClaims{
		Subject: "rest-public-subject", Email: "rest-federated@example.invalid", EmailVerified: true,
		DisplayName: "REST Federated Resident", ActorType: "constituent",
		DepartmentCodes: []string{}, DistrictCodes: []string{}, Roles: []string{},
	}}
	next := uint64(980_000_000_000_000_000)
	runtime := &city311Service.IdentityRuntimeConfiguration{
		BaseURL: "https://city311.example.test", OIDCIssuerURL: "https://identity.example.test",
		OIDCStaffClientID: "city311-staff", OIDCPublicClientID: "city311-public", OIDCClientSecret: "rest-secret-never-returned",
		SAMLMetadataURL: "https://identity.example.test/saml/metadata", SAMLServiceProvider: "https://city311.example.test/saml",
	}
	identity := city311Service.NewIdentity(st, city311Service.IdentityOptions{
		Secret: []byte("rest-federation-session-secret"), Runtime: runtime, Federation: provider,
		Now:    func() time.Time { return time.Date(2026, 2, 3, 15, 4, 5, 0, time.UTC) },
		NextID: func() uint64 { next++; return next },
	})
	router := chi.NewRouter()
	router.Route("/api/v1", MountRoutesWithServices(svc, identity))
	return router, st, provider
}

func TestIdentityConfigurationHTTPContract(t *testing.T) {
	router, st, _ := testFederationRouter(t)
	ctx := context.Background()
	administrator, err := store.LookupUserByEmail(ctx, st, "platform-admin@city311.example.invalid")
	require.NoError(t, err)
	agent, err := store.LookupUserByEmail(ctx, st, "service-agent@city311.example.invalid")
	require.NoError(t, err)

	unauthenticated := executeJSON(t, router, http.MethodGet, "/api/v1/admin/identity", nil, nil, 0)
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)
	forbidden := executeJSON(t, router, http.MethodGet, "/api/v1/admin/identity", nil, nil, agent.ID)
	require.Equal(t, http.StatusForbidden, forbidden.Code)
	configuration := executeJSON(t, router, http.MethodGet, "/api/v1/admin/identity", nil, nil, administrator.ID)
	require.Equal(t, http.StatusOK, configuration.Code, configuration.Body.String())
	require.Equal(t, `"1"`, configuration.Header().Get("ETag"))
	require.Contains(t, configuration.Body.String(), `"oidc_client_secret_configured":true`)
	require.Contains(t, configuration.Body.String(), `"saml_sp_entity_id":"https://city311.example.test/saml"`)
	require.NotContains(t, configuration.Body.String(), "rest-secret-never-returned")

	missingVersion := executeJSON(t, router, http.MethodPatch, "/api/v1/admin/identity", map[string]any{"oidc_enabled": false}, nil, administrator.ID)
	require.Equal(t, http.StatusPreconditionRequired, missingVersion.Code)
	malformed := executeJSON(t, router, http.MethodPatch, "/api/v1/admin/identity", map[string]any{"secret": "forbidden"}, map[string]string{contract.IfMatchHeader: `"1"`}, administrator.ID)
	require.Equal(t, http.StatusUnprocessableEntity, malformed.Code)
	updated := executeJSON(t, router, http.MethodPatch, "/api/v1/admin/identity", map[string]any{"oidc_enabled": false}, map[string]string{contract.IfMatchHeader: `"1"`}, administrator.ID)
	require.Equal(t, http.StatusOK, updated.Code, updated.Body.String())
	require.Equal(t, `"2"`, updated.Header().Get("ETag"))
	require.Contains(t, updated.Body.String(), `"oidc_enabled":false`)
	require.Contains(t, updated.Body.String(), `"saml_enabled":true`)
	conflict := executeJSON(t, router, http.MethodPatch, "/api/v1/admin/identity", map[string]any{"saml_enabled": false}, map[string]string{contract.IfMatchHeader: `"1"`}, administrator.ID)
	require.Equal(t, http.StatusConflict, conflict.Code)
}

func TestFederatedSignInHTTPContractAndSecureCookies(t *testing.T) {
	router, _, provider := testFederationRouter(t)
	started := executeJSON(t, router, http.MethodGet, "/api/v1/auth/oidc/start?client=public", nil, nil, 0)
	require.Equal(t, http.StatusOK, started.Code, started.Body.String())
	require.Contains(t, started.Body.String(), `"authorization_url":"https://identity.example.test/authorize`)
	require.Equal(t, "no-store", started.Header().Get("Cache-Control"))
	require.Equal(t, "public", provider.start.Client)
	require.NotEmpty(t, provider.start.PKCEVerifier)
	require.NotEmpty(t, provider.start.Nonce)
	flowCookies := started.Result().Cookies()
	require.Len(t, flowCookies, 1)
	flowCookie := flowCookies[0]
	require.Equal(t, city311Service.FederationFlowCookie, flowCookie.Name)
	require.Equal(t, "/api/v1/auth", flowCookie.Path)
	require.True(t, flowCookie.HttpOnly)
	require.True(t, flowCookie.Secure)
	require.Equal(t, http.SameSiteLaxMode, flowCookie.SameSite)

	callbackPath := "/api/v1/auth/oidc/callback?state=" + url.QueryEscape(provider.start.State) + "&code=fixture-code"
	callback := executeJSON(t, router, http.MethodGet, callbackPath, nil, map[string]string{"Cookie": flowCookie.Name + "=" + flowCookie.Value}, 0)
	require.Equal(t, http.StatusOK, callback.Code, callback.Body.String())
	require.Contains(t, callback.Body.String(), `"authenticated":true`)
	require.Contains(t, callback.Body.String(), `"application_roles":["constituent"]`)
	require.Equal(t, "fixture-code", provider.callback.Code)
	require.Equal(t, provider.start.PKCEVerifier, provider.callback.PKCEVerifier)
	var sessionCookie *http.Cookie
	flowExpired := false
	for _, cookie := range callback.Result().Cookies() {
		if cookie.Name == city311Service.IdentitySessionCookie {
			sessionCookie = cookie
		}
		if cookie.Name == city311Service.FederationFlowCookie && cookie.MaxAge < 0 {
			flowExpired = true
		}
	}
	require.NotNil(t, sessionCookie)
	require.True(t, flowExpired)

	current := executeJSON(t, router, http.MethodGet, "/api/v1/session", nil, map[string]string{"Cookie": sessionCookie.Name + "=" + sessionCookie.Value}, 0)
	require.Equal(t, http.StatusOK, current.Code)
	require.Contains(t, current.Body.String(), `"display_name":"REST Federated Resident"`)
}

func TestFederatedCallbackFailuresAreRecoverableAndClearFlow(t *testing.T) {
	router, _, provider := testFederationRouter(t)
	missing := executeJSON(t, router, http.MethodGet, "/api/v1/auth/oidc/callback?state=missing&code=code", nil, nil, 0)
	require.Equal(t, http.StatusUnauthorized, missing.Code)
	require.Contains(t, missing.Body.String(), "return to sign in")

	started := executeJSON(t, router, http.MethodGet, "/api/v1/auth/oidc/start?client=public", nil, nil, 0)
	flow := started.Result().Cookies()[0]
	providerError := executeJSON(t, router, http.MethodGet, "/api/v1/auth/oidc/callback?error=access_denied", nil, map[string]string{"Cookie": flow.Name + "=" + flow.Value}, 0)
	require.Equal(t, http.StatusUnauthorized, providerError.Code)
	require.Less(t, providerError.Result().Cookies()[0].MaxAge, 0)

	badState := executeJSON(t, router, http.MethodGet, "/api/v1/auth/oidc/callback?state=wrong&code=code", nil, map[string]string{"Cookie": flow.Name + "=" + flow.Value}, 0)
	require.Equal(t, http.StatusUnauthorized, badState.Code)
	require.NotEqual(t, "wrong", provider.start.State)
	require.Less(t, badState.Result().Cookies()[0].MaxAge, 0)
}
