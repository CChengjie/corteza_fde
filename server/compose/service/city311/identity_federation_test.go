package city311

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

type federationProviderCapture struct {
	starts    []FederationStartRequest
	callbacks []FederationCallbackRequest
	claims    FederatedClaims
	startErr  error
	finishErr error
}

func (capture *federationProviderCapture) Start(_ context.Context, request FederationStartRequest) (FederationAuthorization, error) {
	capture.starts = append(capture.starts, request)
	if capture.startErr != nil {
		return FederationAuthorization{}, capture.startErr
	}
	return FederationAuthorization{URL: "https://identity.example.test/authorize?state=" + request.State, RequestID: "saml-request-1"}, nil
}

func (capture *federationProviderCapture) Callback(_ context.Context, request FederationCallbackRequest) (FederatedClaims, error) {
	capture.callbacks = append(capture.callbacks, request)
	if capture.finishErr != nil {
		return FederatedClaims{}, capture.finishErr
	}
	return capture.claims, nil
}

func testFederatedIdentityService(t *testing.T) (*IdentityService, store.Storer, *federationProviderCapture, *time.Time) {
	t.Helper()
	base, st := testService(t)
	ctx := context.Background()
	require.NoError(t, base.Seed(ctx, base.now()))
	now := base.now()
	next := uint64(970_000_000_000_000_000)
	provider := &federationProviderCapture{claims: validPublicFederatedClaims("federated@example.invalid")}
	runtime := &IdentityRuntimeConfiguration{
		BaseURL: "https://city311.example.test", OIDCIssuerURL: "https://identity.example.test",
		OIDCStaffClientID: "city311-staff", OIDCPublicClientID: "city311-public", OIDCClientSecret: "never-return-this-secret",
		SAMLMetadataURL: "https://identity.example.test/saml/metadata", SAMLServiceProvider: "https://city311.example.test/saml",
	}
	identity := NewIdentity(st, IdentityOptions{
		Secret: []byte("federation-test-session-secret"), Runtime: runtime, Federation: provider,
		Now: func() time.Time { return now }, NextID: func() uint64 { next++; return next },
	})
	return identity, st, provider, &now
}

func validPublicFederatedClaims(email string) FederatedClaims {
	return FederatedClaims{
		Subject: "public-subject-1", Email: email, EmailVerified: true, DisplayName: "Federated Resident",
		ActorType: "constituent", DepartmentCodes: []string{}, DistrictCodes: []string{}, Roles: []string{},
	}
}

func boolPointer(value bool) *bool { return &value }

func localAccountCount(t *testing.T, st store.Storer) int {
	t.Helper()
	accounts, _, err := store.SearchCity311LocalAccounts(context.Background(), st, composeTypes.City311LocalAccountFilter{})
	require.NoError(t, err)
	return len(accounts)
}

func TestIdentityConfigurationIsVersionedAndNeverReturnsSecrets(t *testing.T) {
	identity, st, _, _ := testFederatedIdentityService(t)
	ctx := context.Background()
	administrator := contract.Actor{ID: 44, Roles: []contract.ApplicationRole{contract.ApplicationRolePlatformAdministrator}}
	_, err := identity.IdentityConfiguration(ctx, contract.Actor{ID: 45, Roles: []contract.ApplicationRole{contract.ApplicationRoleServiceAgent}})
	requireIdentityError(t, err, 403, contract.ErrorForbidden)

	configuration, err := identity.IdentityConfiguration(ctx, administrator)
	require.NoError(t, err)
	require.True(t, configuration.OIDCEnabled)
	require.True(t, configuration.SAMLEnabled)
	require.True(t, configuration.OIDCClientSecretConfigured)
	require.Equal(t, "city311-public", configuration.OIDCPublicClientID)
	require.Len(t, configuration.ActorRoleMappings, 5)
	require.Equal(t, uint64(1), configuration.Version)
	encoded, err := json.Marshal(configuration)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "never-return-this-secret")

	_, err = identity.UpdateIdentityConfiguration(ctx, administrator, 0, contract.IdentityConfigurationWrite{})
	requireIdentityError(t, err, 428, contract.ErrorExpectedVersionRequired)
	updated, err := identity.UpdateIdentityConfiguration(ctx, administrator, 1, contract.IdentityConfigurationWrite{OIDCEnabled: boolPointer(false)})
	require.NoError(t, err)
	require.False(t, updated.OIDCEnabled)
	require.True(t, updated.SAMLEnabled)
	require.Equal(t, uint64(2), updated.Version)
	_, err = identity.UpdateIdentityConfiguration(ctx, administrator, 1, contract.IdentityConfigurationWrite{SAMLEnabled: boolPointer(false)})
	requireIdentityError(t, err, 409, contract.ErrorVersionConflict)

	revisions, _, err := store.SearchCity311ConfigurationRevisions(ctx, st, composeTypes.City311ConfigurationRevisionFilter{
		ResourceType: configurationIdentity, ResourceKey: identityConfigurationKey,
	})
	require.NoError(t, err)
	require.Len(t, revisions, 2)
	audits, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{EventType: "IDENTITY_CONFIGURATION_UPDATED"})
	require.NoError(t, err)
	require.Len(t, audits, 1)
}

func TestIdentityConfigurationRejectsEnablingMissingRuntimeValues(t *testing.T) {
	_, st, _, _ := testIdentityService(t)
	runtime := &IdentityRuntimeConfiguration{BaseURL: "https://city311.example.test"}
	identity := NewIdentity(st, IdentityOptions{Secret: []byte("missing-runtime-test"), Runtime: runtime})
	administrator := contract.Actor{ID: 44, Roles: []contract.ApplicationRole{contract.ApplicationRolePlatformAdministrator}}
	configuration, err := identity.IdentityConfiguration(context.Background(), administrator)
	require.NoError(t, err)
	require.False(t, configuration.OIDCEnabled)
	_, err = identity.UpdateIdentityConfiguration(context.Background(), administrator, 1, contract.IdentityConfigurationWrite{OIDCEnabled: boolPointer(true)})
	validation := requireIdentityError(t, err, 422, contract.ErrorValidation)
	require.Equal(t, "/oidc_enabled", validation.Payload.Errors[0].Field)
}

func TestFederatedOIDCPKCEProvisioningAndImmutableSubjectUpdate(t *testing.T) {
	identity, st, provider, _ := testFederatedIdentityService(t)
	ctx := context.Background()
	before := localAccountCount(t, st)
	redirect, flowCookie, err := identity.StartFederatedSignIn(ctx, federatedProviderOIDC, federatedClientPublic, nil)
	require.NoError(t, err)
	require.Contains(t, redirect.AuthorizationURL, "https://identity.example.test/authorize")
	require.NotEmpty(t, flowCookie)
	require.Len(t, provider.starts, 1)
	start := provider.starts[0]
	require.Equal(t, federatedClientPublic, start.Client)
	require.NotEmpty(t, start.State)
	require.NotEmpty(t, start.Nonce)
	require.Len(t, start.PKCEVerifier, 43)
	require.Equal(t, "https://city311.example.test/api/v1/auth/oidc/callback", start.CallbackURL)

	_, _, err = identity.CompleteFederatedSignIn(ctx, federatedProviderOIDC, "wrong-state", "code", "", flowCookie)
	requireIdentityError(t, err, 401, contract.ErrorUnauthenticated)
	require.Equal(t, before, localAccountCount(t, st))

	token, resolved, err := identity.CompleteFederatedSignIn(ctx, federatedProviderOIDC, start.State, "code", "", flowCookie)
	require.NoError(t, err)
	require.NotEmpty(t, token)
	require.Equal(t, []contract.ApplicationRole{contract.ApplicationRoleConstituent}, resolved.Actor.ApplicationRoles)
	require.Equal(t, before+1, localAccountCount(t, st))
	firstUserID := resolved.User.ID
	mappingKey := federatedAccountKey(federatedProviderOIDC, provider.claims.Subject)
	mapping, err := identity.latestIdentityRevision(ctx, st, configurationFederatedAccount, mappingKey)
	require.NoError(t, err)
	require.Equal(t, 1, mapping.Version)

	provider.claims.DisplayName = "Updated Federated Resident"
	_, secondFlow, err := identity.StartFederatedSignIn(ctx, federatedProviderOIDC, federatedClientPublic, nil)
	require.NoError(t, err)
	secondStart := provider.starts[len(provider.starts)-1]
	_, updated, err := identity.CompleteFederatedSignIn(ctx, federatedProviderOIDC, secondStart.State, "second-code", "", secondFlow)
	require.NoError(t, err)
	require.Equal(t, firstUserID, updated.User.ID)
	require.Equal(t, provider.claims.DisplayName, updated.User.Name)
	require.Equal(t, before+1, localAccountCount(t, st))
	mapping, err = identity.latestIdentityRevision(ctx, st, configurationFederatedAccount, mappingKey)
	require.NoError(t, err)
	require.Equal(t, 2, mapping.Version)
}

func TestFederatedValidationFailureLeavesAccountsUnchanged(t *testing.T) {
	identity, st, provider, now := testFederatedIdentityService(t)
	ctx := context.Background()
	before := localAccountCount(t, st)
	provider.claims.EmailVerified = false
	_, cookie, err := identity.StartFederatedSignIn(ctx, federatedProviderOIDC, federatedClientPublic, nil)
	require.NoError(t, err)
	start := provider.starts[len(provider.starts)-1]
	_, _, err = identity.CompleteFederatedSignIn(ctx, federatedProviderOIDC, start.State, "code", "", cookie)
	requireIdentityError(t, err, 401, contract.ErrorUnauthenticated)
	require.Equal(t, before, localAccountCount(t, st))

	provider.claims.EmailVerified = true
	provider.finishErr = errors.New("invalid signature")
	_, cookie, err = identity.StartFederatedSignIn(ctx, federatedProviderOIDC, federatedClientPublic, nil)
	require.NoError(t, err)
	start = provider.starts[len(provider.starts)-1]
	_, _, err = identity.CompleteFederatedSignIn(ctx, federatedProviderOIDC, start.State, "code", "", cookie)
	requireIdentityError(t, err, 401, contract.ErrorUnauthenticated)
	require.Equal(t, before, localAccountCount(t, st))

	provider.finishErr = nil
	_, cookie, err = identity.StartFederatedSignIn(ctx, federatedProviderOIDC, federatedClientPublic, nil)
	require.NoError(t, err)
	start = provider.starts[len(provider.starts)-1]
	*now = now.Add(federationFlowLifetime + time.Second)
	_, _, err = identity.CompleteFederatedSignIn(ctx, federatedProviderOIDC, start.State, "code", "", cookie)
	requireIdentityError(t, err, 401, contract.ErrorUnauthenticated)
	require.Equal(t, before, localAccountCount(t, st))
}

func TestFederatedVerifiedEmailRequiresExplicitAuthenticatedLink(t *testing.T) {
	identity, st, provider, _ := testFederatedIdentityService(t)
	ctx := context.Background()
	registration := validAccountRegistration("link.owner", "link-owner@example.invalid")
	_, err := identity.Register(ctx, registration)
	require.NoError(t, err)
	before := localAccountCount(t, st)
	provider.claims = validPublicFederatedClaims(registration.Email)

	_, cookie, err := identity.StartFederatedSignIn(ctx, federatedProviderOIDC, federatedClientPublic, nil)
	require.NoError(t, err)
	start := provider.starts[len(provider.starts)-1]
	_, _, err = identity.CompleteFederatedSignIn(ctx, federatedProviderOIDC, start.State, "code", "", cookie)
	linkRequired := requireIdentityError(t, err, 401, contract.ErrorUnauthenticated)
	require.Contains(t, linkRequired.Payload.Message, "Sign in locally")
	require.Equal(t, before, localAccountCount(t, st))

	_, localSession, err := identity.SignIn(ctx, contract.LocalSignIn{LoginIdentifier: registration.LoginIdentifier, Password: registration.Password})
	require.NoError(t, err)
	_, cookie, err = identity.StartFederatedSignIn(ctx, federatedProviderOIDC, federatedClientStaff, localSession)
	require.NoError(t, err)
	start = provider.starts[len(provider.starts)-1]
	require.Equal(t, federatedClientPublic, start.Client)
	_, linked, err := identity.CompleteFederatedSignIn(ctx, federatedProviderOIDC, start.State, "code", "", cookie)
	require.NoError(t, err)
	require.Equal(t, localSession.User.ID, linked.User.ID)
	require.Equal(t, before, localAccountCount(t, st))
}

func TestFederatedStaffClaimsMapRolesAndRecordScope(t *testing.T) {
	identity, _, provider, _ := testFederatedIdentityService(t)
	provider.claims = FederatedClaims{
		Subject: "staff-subject-1", Email: "staff-federated@example.invalid", EmailVerified: true,
		DisplayName: "Federated Agent", ActorType: "staff", DepartmentCodes: []string{"STREETS"},
		DistrictCodes: []string{"NORTH", "CENTRAL"}, Roles: []string{"service_agent", "supervisor"},
	}
	redirect, cookie, err := identity.StartFederatedSignIn(context.Background(), federatedProviderSAML, federatedClientStaff, nil)
	require.NoError(t, err)
	require.NotEmpty(t, redirect.AuthorizationURL)
	start := provider.starts[len(provider.starts)-1]
	_, resolved, err := identity.CompleteFederatedSignIn(context.Background(), federatedProviderSAML, start.State, "", "signed-assertion", cookie)
	require.NoError(t, err)
	require.ElementsMatch(t, []contract.ApplicationRole{contract.ApplicationRoleServiceAgent, contract.ApplicationRoleSupervisor}, resolved.Actor.ApplicationRoles)
	require.Equal(t, []contract.DepartmentCode{contract.DepartmentStreets}, resolved.Actor.DepartmentCodes)
	require.ElementsMatch(t, []contract.DistrictCode{contract.DistrictNorth, contract.DistrictCentral}, resolved.Actor.DistrictCodes)
	require.Equal(t, "saml-request-1", provider.callbacks[len(provider.callbacks)-1].SAMLRequestID)
}
