package city311

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"testing"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

func setIntegrationEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("SESSION_SECRET", "integration-persistence-secret")
	t.Setenv("APP_BASE_URL", "https://city311.example.test")
	t.Setenv("CIVICWORKS_BASE_URL", "https://civicworks.example.test")
	t.Setenv("CIVICWORKS_API_TOKEN", "civicworks-api-token")
	t.Setenv("CIVICWORKS_WEBHOOK_SECRET", "civicworks-webhook-secret")
	t.Setenv("BENCHMARK_RUN_ID", "integration-run")
	t.Setenv("MAP_BASE_URL", "https://mapping.example.test")
	t.Setenv("MAP_API_TOKEN", "mapping-api-token")
	t.Setenv("WORKFLOW_OAUTH_TOKEN_URL", "https://workflow.example.test/oauth/token")
	t.Setenv("WORKFLOW_API_BASE_URL", "https://workflow.example.test")
	t.Setenv("WORKFLOW_CLIENT_ID", "workflow-client")
	t.Setenv("WORKFLOW_CLIENT_SECRET", "workflow-client-secret")
	t.Setenv("MAIL_SMTP_HOST", "mail.example.test")
	t.Setenv("MAIL_SMTP_PORT", "587")
	t.Setenv("MAIL_SMTP_USERNAME", "mailer")
	t.Setenv("MAIL_SMTP_PASSWORD", "mail-password")
	t.Setenv("MAIL_API_BASE_URL", "https://mail.example.test")
	t.Setenv("MAIL_API_TOKEN", "mail-api-token")
	t.Setenv("OIDC_ISSUER_URL", "https://identity.example.test")
	t.Setenv("OIDC_STAFF_CLIENT_ID", "city311-staff")
	t.Setenv("OIDC_PUBLIC_CLIENT_ID", "city311-public")
	t.Setenv("OIDC_CLIENT_SECRET", "identity-client-secret")
	t.Setenv("SAML_METADATA_URL", "https://identity.example.test/saml/metadata")
	t.Setenv("SAML_SP_ENTITY_ID", "https://city311.example.test/saml")
}

func testIntegrationService(t *testing.T) (*Service, store.Storer, contract.Actor) {
	t.Helper()
	setIntegrationEnvironment(t)
	svc, st := testService(t)
	require.NoError(t, svc.Seed(context.Background(), svc.now()))
	return svc, st, contract.Actor{ID: 44, Roles: []contract.ApplicationRole{contract.ApplicationRolePlatformAdministrator}}
}

func TestIntegrationCatalogueAuthorizationFilteringAndPagination(t *testing.T) {
	svc, _, administrator := testIntegrationService(t)
	ctx := context.Background()

	_, err := svc.ListIntegrations(ctx, contract.Actor{ID: 45, Roles: []contract.ApplicationRole{contract.ApplicationRoleServiceAgent}}, IntegrationListQuery{})
	requireServiceError(t, err, integrationHTTPStatusForbidden, contract.ErrorForbidden)

	list, err := svc.ListIntegrations(ctx, administrator, IntegrationListQuery{PageSize: 2, Sort: "-kind,integration_id"})
	require.NoError(t, err)
	require.Len(t, list.Items, 2)
	require.Equal(t, 5, list.TotalCount)
	require.NotNil(t, list.NextPageToken)
	require.Equal(t, []string{"-kind", "integration_id"}, list.Sort)

	next, err := svc.ListIntegrations(ctx, administrator, IntegrationListQuery{
		PageSize: 2, PageToken: *list.NextPageToken, Sort: "-kind,integration_id",
	})
	require.NoError(t, err)
	require.Len(t, next.Items, 2)
	require.NotEqual(t, list.Items[0].IntegrationID, next.Items[0].IntegrationID)

	active := true
	filtered, err := svc.ListIntegrations(ctx, administrator, IntegrationListQuery{
		Kinds: []contract.IntegrationKind{contract.IntegrationKindMapping}, Active: &active,
	})
	require.NoError(t, err)
	require.Len(t, filtered.Items, 1)
	require.Equal(t, IntegrationMappingID, filtered.Items[0].IntegrationID)
	require.Equal(t, map[string]any{"active": true, "kind": []contract.IntegrationKind{contract.IntegrationKindMapping}}, filtered.AppliedFilters)

	_, err = svc.ListIntegrations(ctx, administrator, IntegrationListQuery{Sort: "unknown"})
	requireServiceError(t, err, integrationHTTPStatusUnprocessableEntity, contract.ErrorValidation)
	_, err = svc.ListIntegrations(ctx, administrator, IntegrationListQuery{Sort: "kind,active,updated_at,integration_id"})
	requireServiceError(t, err, integrationHTTPStatusUnprocessableEntity, contract.ErrorValidation)
	_, err = svc.ListIntegrations(ctx, administrator, IntegrationListQuery{PageToken: "not-a-page-token"})
	requireServiceError(t, err, integrationHTTPStatusBadRequest, contract.ErrorInvalidPageToken)

	connection, err := svc.GetIntegration(ctx, administrator, IntegrationIdentityID)
	require.NoError(t, err)
	require.Equal(t, contract.IntegrationKindIdentity, connection.Kind)
	require.True(t, connection.Active)
	require.True(t, connection.SecretConfigured)
	_, err = svc.GetIntegration(ctx, administrator, "missing")
	requireServiceError(t, err, integrationHTTPStatusNotFound, contract.ErrorNotFound)
}

func TestIntegrationUpdateEncryptsSecretsAndKeepsAuditSafe(t *testing.T) {
	svc, st, administrator := testIntegrationService(t)
	ctx := context.Background()
	active := true
	secret := "rotated-mapping-secret"
	updated, err := svc.UpdateIntegration(ctx, administrator, IntegrationMappingID, 1, contract.IntegrationConnectionWrite{
		Active: &active, Configuration: map[string]any{"base_url": "https://new-mapping.example.test"}, Secret: &secret,
	})
	require.NoError(t, err)
	require.Equal(t, uint64(2), updated.Version)
	require.True(t, updated.Active)
	require.True(t, updated.SecretConfigured)
	encoded, err := json.Marshal(updated)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), secret)
	require.NotContains(t, string(encoded), "secret\"")

	revisions, _, err := store.SearchCity311ConfigurationRevisions(ctx, st, composeTypes.City311ConfigurationRevisionFilter{
		ResourceType: configurationIntegration, ResourceKey: IntegrationMappingID,
	})
	require.NoError(t, err)
	require.Len(t, revisions, 2)
	persisted, err := json.Marshal(revisions[1].Payload)
	require.NoError(t, err)
	require.NotContains(t, string(persisted), secret)
	require.Contains(t, string(persisted), "sealed_secret")

	audits, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{EventType: "INTEGRATION_UPDATED"})
	require.NoError(t, err)
	require.Len(t, audits, 1)
	auditJSON, err := json.Marshal(audits[0])
	require.NoError(t, err)
	require.NotContains(t, string(auditJSON), secret)
	require.NotContains(t, string(auditJSON), "sealed_secret")
	require.Contains(t, string(auditJSON), "new-mapping.example.test")

	mapping, configurationError := svc.MappingRuntime()
	require.NoError(t, configurationError)
	require.NotNil(t, mapping)
	require.Contains(t, mapping.endpoint, "new-mapping.example.test")
	require.Equal(t, secret, mapping.apiToken)

	_, err = svc.UpdateIntegration(ctx, administrator, IntegrationMappingID, 1, contract.IntegrationConnectionWrite{Active: &active})
	requireServiceError(t, err, integrationHTTPStatusConflict, contract.ErrorVersionConflict)
	_, err = svc.UpdateIntegration(ctx, administrator, IntegrationMappingID, 0, contract.IntegrationConnectionWrite{Active: &active})
	requireServiceError(t, err, integrationHTTPStatusPreconditionRequired, contract.ErrorExpectedVersionRequired)
	_, err = svc.UpdateIntegration(ctx, administrator, IntegrationMappingID, 2, contract.IntegrationConnectionWrite{})
	requireServiceError(t, err, integrationHTTPStatusUnprocessableEntity, contract.ErrorValidation)
}

func TestIntegrationRotationRevocationAndRestartReload(t *testing.T) {
	svc, st, administrator := testIntegrationService(t)
	ctx := context.Background()

	rotated, err := svc.RotateIntegrationSecret(ctx, administrator, IntegrationWorkflowID, 1, contract.SecretRotation{NewSecret: "replacement-workflow-secret"})
	require.NoError(t, err)
	require.Equal(t, uint64(2), rotated.Version)
	require.True(t, rotated.Active)
	require.True(t, rotated.SecretConfigured)
	svc.runtimeMu.RLock()
	workflow := svc.workflowHTTP.(*oauthWorkflowHTTPClient)
	svc.runtimeMu.RUnlock()
	require.Equal(t, "replacement-workflow-secret", workflow.clientSecret)

	restarted := New(st)
	restarted.now = svc.now
	restarted.integrationKey = svc.integrationKey
	require.NoError(t, restarted.ReloadIntegrations(ctx))
	restarted.runtimeMu.RLock()
	reloadedWorkflow := restarted.workflowHTTP.(*oauthWorkflowHTTPClient)
	restarted.runtimeMu.RUnlock()
	require.Equal(t, "replacement-workflow-secret", reloadedWorkflow.clientSecret)

	revoked, err := svc.RevokeIntegration(ctx, administrator, IntegrationWorkflowID, 2, contract.Reason{Reason: "credential compromise"})
	require.NoError(t, err)
	require.Equal(t, uint64(3), revoked.Version)
	require.False(t, revoked.Active)
	require.False(t, revoked.SecretConfigured)
	svc.runtimeMu.RLock()
	require.Nil(t, svc.workflowHTTP)
	require.Error(t, svc.workflowConfig)
	svc.runtimeMu.RUnlock()

	revisions, _, err := store.SearchCity311ConfigurationRevisions(ctx, st, composeTypes.City311ConfigurationRevisionFilter{
		ResourceType: configurationIntegration, ResourceKey: IntegrationWorkflowID,
	})
	require.NoError(t, err)
	require.Len(t, revisions, 3)
	audits, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{EventType: "INTEGRATION_REVOKED"})
	require.NoError(t, err)
	require.Len(t, audits, 1)
	require.Equal(t, "credential compromise", audits[0].After["reason"])

	_, err = svc.RotateIntegrationSecret(ctx, administrator, IntegrationWorkflowID, 3, contract.SecretRotation{})
	requireServiceError(t, err, integrationHTTPStatusUnprocessableEntity, contract.ErrorValidation)
	_, err = svc.RevokeIntegration(ctx, administrator, IntegrationWorkflowID, 3, contract.Reason{})
	requireServiceError(t, err, integrationHTTPStatusUnprocessableEntity, contract.ErrorValidation)
}

func TestEveryIntegrationKindReloadsAndValidatesWrites(t *testing.T) {
	svc, _, administrator := testIntegrationService(t)
	ctx := context.Background()
	identity := NewIdentity(svc.store, IdentityOptions{Secret: []byte("integration-identity-session")})
	svc.BindIdentityService(identity)
	require.NoError(t, svc.ReloadIntegrations(ctx))

	svc.runtimeMu.RLock()
	require.NotNil(t, svc.civicWorksClient)
	require.NotNil(t, svc.mappingService)
	require.NotNil(t, svc.workflowHTTP)
	require.IsType(t, smtpMailSender{}, svc.mailSender)
	svc.runtimeMu.RUnlock()
	identity.runtimeMu.RLock()
	require.Equal(t, "identity-client-secret", identity.runtime.OIDCClientSecret)
	identity.runtimeMu.RUnlock()

	for _, integrationID := range []string{IntegrationCivicWorksID, IntegrationMappingID, IntegrationWorkflowID, IntegrationMailID, IntegrationIdentityID} {
		rotated, err := svc.RotateIntegrationSecret(ctx, administrator, integrationID, 1, contract.SecretRotation{NewSecret: "replacement-" + integrationID})
		require.NoError(t, err, integrationID)
		require.Equal(t, uint64(2), rotated.Version)
	}

	active := true
	badSecret := " malformed "
	_, err := svc.UpdateIntegration(ctx, administrator, IntegrationMappingID, 2, contract.IntegrationConnectionWrite{Active: &active, Secret: &badSecret})
	requireServiceError(t, err, integrationHTTPStatusUnprocessableEntity, contract.ErrorValidation)
	_, err = svc.UpdateIntegration(ctx, administrator, IntegrationMappingID, 2, contract.IntegrationConnectionWrite{
		Active: &active, Configuration: map[string]any{"unknown": "value"},
	})
	requireServiceError(t, err, integrationHTTPStatusUnprocessableEntity, contract.ErrorValidation)
	_, err = svc.UpdateIntegration(ctx, administrator, IntegrationMappingID, 2, contract.IntegrationConnectionWrite{
		Active: &active, Configuration: map[string]any{"base_url": "relative"},
	})
	requireServiceError(t, err, integrationHTTPStatusUnprocessableEntity, contract.ErrorValidation)
}

func TestIntegrationEncryptionRejectsWrongKeyAndTampering(t *testing.T) {
	svc, _, _ := testIntegrationService(t)
	sealed, err := svc.sealIntegrationSecrets(IntegrationMappingID, integrationSecretBundle{"api_token": "top-secret"})
	require.NoError(t, err)
	opened, err := svc.openIntegrationSecrets(IntegrationMappingID, sealed)
	require.NoError(t, err)
	require.Equal(t, "top-secret", opened["api_token"])

	other := New(svc.store)
	other.integrationKey = sha256.Sum256([]byte("wrong-key"))
	_, err = other.openIntegrationSecrets(IntegrationMappingID, sealed)
	require.Error(t, err)
	_, err = svc.openIntegrationSecrets(IntegrationMappingID, sealed+"x")
	require.Error(t, err)
	empty, err := svc.openIntegrationSecrets(IntegrationMappingID, "")
	require.NoError(t, err)
	require.Empty(t, empty)

	_, err = prepareIntegrationRuntime(contract.IntegrationKind("UNKNOWN"), true, nil, nil)
	require.Error(t, err)
	require.False(t, knownIntegrationID("unknown"))
}

const (
	integrationHTTPStatusBadRequest           = 400
	integrationHTTPStatusForbidden            = 403
	integrationHTTPStatusNotFound             = 404
	integrationHTTPStatusConflict             = 409
	integrationHTTPStatusUnprocessableEntity  = 422
	integrationHTTPStatusPreconditionRequired = 428
)
