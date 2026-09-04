package city311

import (
	"context"
	"net/http"
	"testing"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

func presentationAdministrator() contract.Actor {
	return contract.Actor{ID: 801, Roles: []contract.ApplicationRole{contract.ApplicationRolePlatformAdministrator}}
}

func TestPresentationSeedIsIdempotentAndPublicDefaultsAreAvailable(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	require.NoError(t, svc.Seed(ctx, svc.now()))
	revisions, _, err := store.SearchCity311ConfigurationRevisions(ctx, st, composeTypes.City311ConfigurationRevisionFilter{})
	require.NoError(t, err)
	require.Len(t, revisions, 14)

	branding, err := svc.PublicBranding(ctx)
	require.NoError(t, err)
	require.Equal(t, "City 311", branding.OrganisationName)
	require.True(t, branding.Published)
	for _, key := range publicContentKeys {
		content, err := svc.PublicContent(ctx, key)
		require.NoError(t, err)
		require.True(t, content.Published)
		require.NotEmpty(t, content.Body)
	}
	for key, initial := range initialHelpContent {
		help, err := svc.PublicHelp(ctx, key, nil)
		require.NoError(t, err)
		require.Equal(t, contract.LanguageEN, help.Language)
		require.Contains(t, help.Body, initial)
	}
	_, err = svc.PublicContent(ctx, "UNKNOWN")
	requireServiceError(t, err, http.StatusNotFound, contract.ErrorNotFound)
	_, err = svc.PublicHelp(ctx, "unknown.help", nil)
	requireServiceError(t, err, http.StatusNotFound, contract.ErrorNotFound)
}

func TestBrandingPreviewPublishHistoryAndImmediateRollback(t *testing.T) {
	svc, _ := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	admin := presentationAdministrator()
	nonAdmin := contract.Actor{Roles: []contract.ApplicationRole{contract.ApplicationRoleDepartmentManager}}
	_, err := svc.AdminBranding(ctx, nonAdmin)
	requireServiceError(t, err, http.StatusForbidden, contract.ErrorForbidden)

	name := "Buffalo City 311"
	font := "Inter"
	preview, err := svc.PreviewBranding(ctx, admin, contract.BrandingWrite{OrganisationName: &name, FontFamily: &font})
	require.NoError(t, err)
	require.Equal(t, uint64(2), preview.Version)
	require.False(t, preview.Published)
	public, err := svc.PublicBranding(ctx)
	require.NoError(t, err)
	require.Equal(t, "City 311", public.OrganisationName)

	draft, err := svc.UpdateBranding(ctx, admin, 1, contract.BrandingWrite{OrganisationName: &name, FontFamily: &font})
	require.NoError(t, err)
	require.Equal(t, uint64(2), draft.Version)
	require.False(t, draft.Published)
	_, err = svc.UpdateBranding(ctx, admin, 1, contract.BrandingWrite{OrganisationName: &name})
	requireServiceError(t, err, http.StatusConflict, contract.ErrorVersionConflict)
	published, err := svc.PublishBranding(ctx, admin, 2)
	require.NoError(t, err)
	require.Equal(t, uint64(3), published.Version)
	require.True(t, published.Published)
	public, err = svc.PublicBranding(ctx)
	require.NoError(t, err)
	require.Equal(t, name, public.OrganisationName)

	secondName := "Buffalo Service Centre"
	secondDraft, err := svc.UpdateBranding(ctx, admin, 3, contract.BrandingWrite{OrganisationName: &secondName})
	require.NoError(t, err)
	secondPublished, err := svc.PublishBranding(ctx, admin, secondDraft.Version)
	require.NoError(t, err)
	rolledBack, err := svc.RollbackBranding(ctx, admin, secondPublished.Version, 3)
	require.NoError(t, err)
	require.Equal(t, uint64(6), rolledBack.Version)
	require.Equal(t, name, rolledBack.OrganisationName)
	_, err = svc.RollbackBranding(ctx, admin, rolledBack.Version, 1)
	requireServiceError(t, err, http.StatusUnprocessableEntity, contract.ErrorValidation)

	versions, err := svc.BrandingVersions(ctx, admin, PresentationListQuery{PageSize: 2})
	require.NoError(t, err)
	require.Equal(t, 6, versions.TotalCount)
	require.Len(t, versions.Items, 2)
	require.NotNil(t, versions.NextPageToken)
	next, err := svc.BrandingVersions(ctx, admin, PresentationListQuery{PageSize: 2, PageToken: *versions.NextPageToken})
	require.NoError(t, err)
	require.Len(t, next.Items, 2)
}

func TestBrandingValidationRejectsUnsafeOrInaccessibleValues(t *testing.T) {
	svc, _ := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	admin := presentationAdministrator()
	badURL := "javascript:alert(1)"
	badColour := "#FFFFFF"
	badFont := "Untrusted Remote Font"
	_, err := svc.UpdateBranding(ctx, admin, 1, contract.BrandingWrite{LogoURL: &badURL, PrimaryColour: &badColour, FontFamily: &badFont})
	requireServiceError(t, err, http.StatusUnprocessableEntity, contract.ErrorValidation)
	_, err = svc.UpdateBranding(ctx, admin, 1, contract.BrandingWrite{})
	requireServiceError(t, err, http.StatusUnprocessableEntity, contract.ErrorValidation)
	_, err = svc.UpdateBranding(ctx, admin, 0, contract.BrandingWrite{FontFamily: &badFont})
	requireServiceError(t, err, http.StatusPreconditionRequired, contract.ErrorExpectedVersionRequired)
}

func TestContentDraftPreviewPublicationVersionsAndRollback(t *testing.T) {
	svc, _ := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	admin := presentationAdministrator()
	publicBefore, err := svc.PublicContent(ctx, "HOME")
	require.NoError(t, err)

	input := contract.ContentWrite{Body: `<script>alert(1)</script><p><strong>Updated home</strong></p><a href="javascript:alert(2)">bad</a>`}
	preview, err := svc.PreviewContent(ctx, admin, "HOME", input)
	require.NoError(t, err)
	require.NotContains(t, preview.Body, "script")
	require.NotContains(t, preview.Body, "javascript")
	require.Contains(t, preview.Body, "Updated home")
	publicAfterPreview, err := svc.PublicContent(ctx, "HOME")
	require.NoError(t, err)
	require.Equal(t, publicBefore.Body, publicAfterPreview.Body)

	draft, err := svc.UpdateContent(ctx, admin, "HOME", 1, input)
	require.NoError(t, err)
	require.Equal(t, "DRAFT", draft.State)
	publicAfterDraft, err := svc.PublicContent(ctx, "HOME")
	require.NoError(t, err)
	require.Equal(t, publicBefore.Body, publicAfterDraft.Body)
	published, err := svc.PublishContent(ctx, admin, "HOME", draft.Version)
	require.NoError(t, err)
	require.Equal(t, "PUBLISHED", published.State)
	publicAfterPublish, err := svc.PublicContent(ctx, "HOME")
	require.NoError(t, err)
	require.Equal(t, published.Body, publicAfterPublish.Body)

	rolledBack, err := svc.RollbackContent(ctx, admin, "HOME", published.Version, 1)
	require.NoError(t, err)
	require.Equal(t, publicBefore.Body, rolledBack.Body)
	versions, err := svc.ContentVersions(ctx, admin, "HOME", PresentationListQuery{PageSize: 2})
	require.NoError(t, err)
	require.Equal(t, 4, versions.TotalCount)
	require.NotNil(t, versions.NextPageToken)
	listed, err := svc.ListContent(ctx, admin, PresentationListQuery{PageSize: 2})
	require.NoError(t, err)
	require.Equal(t, len(publicContentKeys), listed.TotalCount)
	require.Len(t, listed.Items, 2)
	_, err = svc.ContentVersions(ctx, admin, "HOME", PresentationListQuery{PageToken: "invalid"})
	requireServiceError(t, err, http.StatusBadRequest, contract.ErrorInvalidPageToken)
	_, err = svc.UpdateContent(ctx, admin, "HOME", rolledBack.Version, contract.ContentWrite{Body: `<script>alert(1)</script>`})
	requireServiceError(t, err, http.StatusUnprocessableEntity, contract.ErrorValidation)
}

func TestHelpLocalisationVersioningSanitisationAndEnglishFallback(t *testing.T) {
	svc, _ := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	admin := presentationAdministrator()
	updated, err := svc.UpdateHelp(ctx, admin, "public.request.submit", 1, contract.HelpWrite{
		Language: contract.LanguageES, Body: `<p>Describa el problema.</p><script>alert(1)</script>`,
	})
	require.NoError(t, err)
	require.Equal(t, uint64(2), updated.Version)
	require.NotContains(t, updated.Body, "script")
	spanish, err := svc.PublicHelp(ctx, "public.request.submit", []contract.Language{contract.LanguageES})
	require.NoError(t, err)
	require.Equal(t, contract.LanguageES, spanish.Language)
	englishFallback, err := svc.PublicHelp(ctx, "public.request.submit", []contract.Language{contract.LanguageVI})
	require.NoError(t, err)
	require.Equal(t, contract.LanguageEN, englishFallback.Language)
	_, err = svc.UpdateHelp(ctx, admin, "public.request.submit", 1, contract.HelpWrite{Language: contract.LanguageES, Body: "<p>Ayuda</p>"})
	requireServiceError(t, err, http.StatusConflict, contract.ErrorVersionConflict)
	_, err = svc.UpdateHelp(ctx, admin, "public.request.submit", 1, contract.HelpWrite{Language: "FR", Body: "<p>Aide</p>"})
	requireServiceError(t, err, http.StatusUnprocessableEntity, contract.ErrorValidation)
	english, err := svc.UpdateHelp(ctx, admin, "public.request.submit", 1, contract.HelpWrite{Language: contract.LanguageEN, Body: "<p>Updated English help.</p>"})
	require.NoError(t, err)
	require.Equal(t, uint64(2), english.Version)
}
