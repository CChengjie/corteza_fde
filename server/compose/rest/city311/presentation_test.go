package city311

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/pkg/auth"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

func TestPresentationHTTPPublicIsolationAndAdminLifecycle(t *testing.T) {
	router, st, _ := testRouter(t)
	ctx := context.Background()
	admin, err := store.LookupUserByEmail(ctx, st, "platform-admin@city311.example.invalid")
	require.NoError(t, err)
	agent, err := store.LookupUserByEmail(ctx, st, "service-agent@city311.example.invalid")
	require.NoError(t, err)

	publicBranding := executeJSON(t, router, http.MethodGet, "/api/v1/public/branding", nil, nil, 0)
	require.Equal(t, http.StatusOK, publicBranding.Code, publicBranding.Body.String())
	require.Equal(t, `"1"`, publicBranding.Header().Get("ETag"))
	publicHome := executeJSON(t, router, http.MethodGet, "/api/v1/public/content/HOME", nil, nil, 0)
	require.Equal(t, http.StatusOK, publicHome.Code, publicHome.Body.String())
	require.Contains(t, publicHome.Body.String(), "Welcome to City 311")
	missing := executeJSON(t, router, http.MethodGet, "/api/v1/public/content/UNKNOWN", nil, nil, 0)
	require.Equal(t, http.StatusNotFound, missing.Code, missing.Body.String())

	unauthenticated := executeJSON(t, router, http.MethodGet, "/api/v1/admin/branding", nil, nil, 0)
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code, unauthenticated.Body.String())
	forbidden := executeJSON(t, router, http.MethodGet, "/api/v1/admin/branding", nil, nil, agent.ID)
	require.Equal(t, http.StatusForbidden, forbidden.Code, forbidden.Body.String())
	current := executeJSON(t, router, http.MethodGet, "/api/v1/admin/branding", nil, nil, admin.ID)
	require.Equal(t, http.StatusOK, current.Code, current.Body.String())

	preview := executeJSON(t, router, http.MethodPost, "/api/v1/admin/branding/preview", map[string]any{"organisation_name": "Preview City"}, nil, admin.ID)
	require.Equal(t, http.StatusOK, preview.Code, preview.Body.String())
	require.Contains(t, preview.Body.String(), `"published":false`)
	missingVersion := executeJSON(t, router, http.MethodPatch, "/api/v1/admin/branding", map[string]any{"organisation_name": "Published City"}, nil, admin.ID)
	require.Equal(t, http.StatusPreconditionRequired, missingVersion.Code, missingVersion.Body.String())
	draft := executeJSON(t, router, http.MethodPatch, "/api/v1/admin/branding", map[string]any{"organisation_name": "Published City"}, map[string]string{contract.IfMatchHeader: `"1"`}, admin.ID)
	require.Equal(t, http.StatusOK, draft.Code, draft.Body.String())
	require.Equal(t, `"2"`, draft.Header().Get("ETag"))
	publicBeforePublish := executeJSON(t, router, http.MethodGet, "/api/v1/public/branding", nil, nil, 0)
	require.NotContains(t, publicBeforePublish.Body.String(), "Published City")
	published := executeJSON(t, router, http.MethodPost, "/api/v1/admin/branding/publish", map[string]any{}, map[string]string{contract.IfMatchHeader: `"2"`}, admin.ID)
	require.Equal(t, http.StatusOK, published.Code, published.Body.String())
	publicAfterPublish := executeJSON(t, router, http.MethodGet, "/api/v1/public/branding", nil, nil, 0)
	require.Contains(t, publicAfterPublish.Body.String(), "Published City")
	versions := executeJSON(t, router, http.MethodGet, "/api/v1/admin/branding/versions?page_size=1", nil, nil, admin.ID)
	require.Equal(t, http.StatusOK, versions.Code, versions.Body.String())
	require.Contains(t, versions.Body.String(), `"total_count":3`)
	rolledBack := executeJSON(t, router, http.MethodPost, "/api/v1/admin/branding/rollback", map[string]any{"target_version": 1}, map[string]string{contract.IfMatchHeader: `"3"`}, admin.ID)
	require.Equal(t, http.StatusOK, rolledBack.Code, rolledBack.Body.String())
	require.Contains(t, rolledBack.Body.String(), "City 311")
}

func TestPresentationHTTPContentHelpSanitisationAndValidation(t *testing.T) {
	router, st, _ := testRouter(t)
	ctx := context.Background()
	admin, err := store.LookupUserByEmail(ctx, st, "platform-admin@city311.example.invalid")
	require.NoError(t, err)

	list := executeJSON(t, router, http.MethodGet, "/api/v1/admin/content?page_size=2", nil, nil, admin.ID)
	require.Equal(t, http.StatusOK, list.Code, list.Body.String())
	require.Contains(t, list.Body.String(), `"total_count":5`)
	adminHome := executeJSON(t, router, http.MethodGet, "/api/v1/admin/content/HOME", nil, nil, admin.ID)
	require.Equal(t, http.StatusOK, adminHome.Code, adminHome.Body.String())
	preview := executeJSON(t, router, http.MethodPost, "/api/v1/admin/content/HOME/preview", map[string]any{"body": `<p>Safe preview</p><script>bad()</script>`}, nil, admin.ID)
	require.Equal(t, http.StatusOK, preview.Code, preview.Body.String())
	require.NotContains(t, preview.Body.String(), "script")
	draft := executeJSON(t, router, http.MethodPatch, "/api/v1/admin/content/HOME", map[string]any{"body": `<p>Published home</p><script>bad()</script>`}, map[string]string{contract.IfMatchHeader: `"1"`}, admin.ID)
	require.Equal(t, http.StatusOK, draft.Code, draft.Body.String())
	publicBefore := executeJSON(t, router, http.MethodGet, "/api/v1/public/content/HOME", nil, nil, 0)
	require.NotContains(t, publicBefore.Body.String(), "Published home")
	published := executeJSON(t, router, http.MethodPost, "/api/v1/admin/content/HOME/publish", map[string]any{}, map[string]string{contract.IfMatchHeader: `"2"`}, admin.ID)
	require.Equal(t, http.StatusOK, published.Code, published.Body.String())
	publicAfter := executeJSON(t, router, http.MethodGet, "/api/v1/public/content/HOME", nil, nil, 0)
	require.Contains(t, publicAfter.Body.String(), "Published home")
	rolledBack := executeJSON(t, router, http.MethodPost, "/api/v1/admin/content/HOME/rollback", map[string]any{"target_version": 1}, map[string]string{contract.IfMatchHeader: `"3"`}, admin.ID)
	require.Equal(t, http.StatusOK, rolledBack.Code, rolledBack.Body.String())
	require.Contains(t, rolledBack.Body.String(), "Welcome to City 311")
	contentVersions := executeJSON(t, router, http.MethodGet, "/api/v1/admin/content/HOME/versions", nil, nil, admin.ID)
	require.Equal(t, http.StatusOK, contentVersions.Code, contentVersions.Body.String())
	require.Contains(t, contentVersions.Body.String(), `"total_count":4`)

	helpCurrent := executeJSON(t, router, http.MethodGet, "/api/v1/admin/help/public.request.submit?language=EN", nil, nil, admin.ID)
	require.Equal(t, http.StatusOK, helpCurrent.Code, helpCurrent.Body.String())
	require.Equal(t, `"1"`, helpCurrent.Header().Get("ETag"))
	require.Contains(t, helpCurrent.Body.String(), `"state":"PUBLISHED"`)
	helpPreview := executeJSON(t, router, http.MethodPost, "/api/v1/admin/help/public.request.submit/preview", map[string]any{
		"language": "ES", "body": `<p>Describa el problema.</p><img src=x onerror=bad()>`,
	}, nil, admin.ID)
	require.Equal(t, http.StatusOK, helpPreview.Code, helpPreview.Body.String())
	require.Contains(t, helpPreview.Body.String(), `"state":"DRAFT"`)
	require.NotContains(t, helpPreview.Body.String(), "onerror")
	helpMissingVersion := executeJSON(t, router, http.MethodPatch, "/api/v1/admin/help/public.request.submit", map[string]any{
		"language": "ES", "body": `<p>Describa el problema.</p>`,
	}, nil, admin.ID)
	require.Equal(t, http.StatusPreconditionRequired, helpMissingVersion.Code, helpMissingVersion.Body.String())
	helpDraft := executeJSON(t, router, http.MethodPatch, "/api/v1/admin/help/public.request.submit", map[string]any{
		"language": "ES", "body": `<p>Describa el problema.</p><img src=x onerror=bad()>`,
	}, map[string]string{contract.IfMatchHeader: `"1"`}, admin.ID)
	require.Equal(t, http.StatusOK, helpDraft.Code, helpDraft.Body.String())
	require.Equal(t, `"2"`, helpDraft.Header().Get("ETag"))
	require.Contains(t, helpDraft.Body.String(), `"published":false`)

	helpBeforePublish := executeHelpRequest(t, router, "es-MX, en;q=0.8")
	require.NotContains(t, helpBeforePublish.Body.String(), "Describa el problema")
	helpPublished := executeJSON(t, router, http.MethodPost, "/api/v1/admin/help/public.request.submit/publish?language=ES", map[string]any{}, map[string]string{contract.IfMatchHeader: `"2"`}, admin.ID)
	require.Equal(t, http.StatusOK, helpPublished.Code, helpPublished.Body.String())
	require.Equal(t, `"3"`, helpPublished.Header().Get("ETag"))
	require.Contains(t, helpPublished.Body.String(), `"state":"PUBLISHED"`)
	helpResponse := executeHelpRequest(t, router, "es-MX, en;q=0.8")
	require.Contains(t, helpResponse.Body.String(), "Describa el problema")
	require.NotContains(t, helpResponse.Body.String(), "onerror")
	helpVersions := executeJSON(t, router, http.MethodGet, "/api/v1/admin/help/public.request.submit/versions?language=ES", nil, nil, admin.ID)
	require.Equal(t, http.StatusOK, helpVersions.Code, helpVersions.Body.String())
	require.Contains(t, helpVersions.Body.String(), `"total_count":2`)

	helpSecondDraft := executeJSON(t, router, http.MethodPatch, "/api/v1/admin/help/public.request.submit", map[string]any{
		"language": "ES", "body": `<p>Segundo texto.</p>`,
	}, map[string]string{contract.IfMatchHeader: `"3"`}, admin.ID)
	require.Equal(t, http.StatusOK, helpSecondDraft.Code, helpSecondDraft.Body.String())
	helpStillPublished := executeHelpRequest(t, router, "es")
	require.Contains(t, helpStillPublished.Body.String(), "Describa el problema")
	helpRolledBack := executeJSON(t, router, http.MethodPost, "/api/v1/admin/help/public.request.submit/rollback?language=ES", map[string]any{"target_version": 3}, map[string]string{contract.IfMatchHeader: `"4"`}, admin.ID)
	require.Equal(t, http.StatusOK, helpRolledBack.Code, helpRolledBack.Body.String())
	require.Equal(t, `"5"`, helpRolledBack.Header().Get("ETag"))
	require.Contains(t, helpRolledBack.Body.String(), "Describa el problema")
	helpStale := executeJSON(t, router, http.MethodPost, "/api/v1/admin/help/public.request.submit/publish?language=ES", map[string]any{}, map[string]string{contract.IfMatchHeader: `"4"`}, admin.ID)
	require.Equal(t, http.StatusConflict, helpStale.Code, helpStale.Body.String())
	require.Contains(t, helpStale.Body.String(), `"current_version":5`)

	badBody := executeJSON(t, router, http.MethodPatch, "/api/v1/admin/content/HOME", map[string]any{"unknown": true}, map[string]string{contract.IfMatchHeader: `"4"`}, admin.ID)
	require.Equal(t, http.StatusUnprocessableEntity, badBody.Code, badBody.Body.String())
	badPage := executeJSON(t, router, http.MethodGet, "/api/v1/admin/content?page_size=0", nil, nil, admin.ID)
	require.Equal(t, http.StatusUnprocessableEntity, badPage.Code, badPage.Body.String())
}

func TestPresentationHTTPRejectsAuthenticatedActorsWithoutCityRole(t *testing.T) {
	router, _, _ := testRouter(t)
	unknownID := uint64(999999999)
	tests := []struct {
		method  string
		path    string
		body    any
		headers map[string]string
	}{
		{http.MethodGet, "/api/v1/admin/branding", nil, nil},
		{http.MethodPost, "/api/v1/admin/branding/preview", map[string]any{}, nil},
		{http.MethodGet, "/api/v1/admin/branding/versions", nil, nil},
		{http.MethodGet, "/api/v1/admin/content/HOME", nil, nil},
		{http.MethodPost, "/api/v1/admin/content/HOME/preview", map[string]any{"body": "<p>Preview</p>"}, nil},
		{http.MethodGet, "/api/v1/admin/content/HOME/versions", nil, nil},
		{http.MethodGet, "/api/v1/admin/help/public.request.submit?language=EN", nil, nil},
		{http.MethodPost, "/api/v1/admin/help/public.request.submit/preview", map[string]any{"language": "EN", "body": "<p>Help</p>"}, nil},
		{http.MethodPatch, "/api/v1/admin/help/public.request.submit", map[string]any{"language": "EN", "body": "<p>Help</p>"}, map[string]string{contract.IfMatchHeader: `"1"`}},
		{http.MethodPost, "/api/v1/admin/help/public.request.submit/publish?language=EN", map[string]any{}, map[string]string{contract.IfMatchHeader: `"1"`}},
		{http.MethodGet, "/api/v1/admin/help/public.request.submit/versions?language=EN", nil, nil},
		{http.MethodPost, "/api/v1/admin/help/public.request.submit/rollback?language=EN", map[string]any{"target_version": 1}, map[string]string{contract.IfMatchHeader: `"1"`}},
	}
	for _, test := range tests {
		response := executeJSON(t, router, test.method, test.path, test.body, test.headers, unknownID)
		require.Equal(t, http.StatusForbidden, response.Code, test.path+": "+response.Body.String())
	}
}

func executeHelpRequest(t *testing.T, router http.Handler, language string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/public/help/public.request.submit", nil)
	request.Header.Set("Accept-Language", language)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request.WithContext(auth.SetIdentityToContext(request.Context(), auth.Anonymous())))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	return response
}
