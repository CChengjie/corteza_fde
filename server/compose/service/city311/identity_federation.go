package city311

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/pkg/errors"
	"github.com/cortezaproject/corteza/server/store"
	systemTypes "github.com/cortezaproject/corteza/server/system/types"
)

const (
	FederationFlowCookie              = "city311_federation"
	configurationIdentity             = "IDENTITY_CONFIGURATION"
	identityConfigurationKey          = "effective"
	configurationFederatedAccount     = "FEDERATED_ACCOUNT"
	federationFlowLifetime            = 10 * time.Minute
	federationAuthenticationFailedMsg = "Federated authentication failed. Please return to sign in and try again."
	federationLinkRequiredMsg         = "This verified email belongs to an existing account. Sign in locally, then start again to confirm linking."
)

var identityRoleMappings = []contract.ActorRoleMapping{
	{AssertedRole: contract.RoleServiceAgent, ApplicationRole: contract.ApplicationRoleServiceAgent},
	{AssertedRole: contract.RoleSupervisor, ApplicationRole: contract.ApplicationRoleSupervisor},
	{AssertedRole: contract.RoleDepartmentManager, ApplicationRole: contract.ApplicationRoleDepartmentManager},
	{AssertedRole: contract.RolePlatformAdministrator, ApplicationRole: contract.ApplicationRolePlatformAdministrator},
	{AssertedRole: contract.RoleWorkflowDesigner, ApplicationRole: contract.ApplicationRoleWorkflowDesigner},
}

type identityConfigurationPayload struct {
	OIDCEnabled bool `json:"oidc_enabled"`
	SAMLEnabled bool `json:"saml_enabled"`
}

type federationFlow struct {
	Provider      string    `json:"provider"`
	Client        string    `json:"client"`
	State         string    `json:"state"`
	Nonce         string    `json:"nonce,omitempty"`
	PKCEVerifier  string    `json:"pkce_verifier,omitempty"`
	SAMLRequestID string    `json:"saml_request_id,omitempty"`
	LinkUserID    string    `json:"link_user_id,omitempty"`
	IssuedAt      time.Time `json:"issued_at"`
}

type federatedAccountPayload struct {
	Provider        string   `json:"provider"`
	Subject         string   `json:"subject"`
	UserID          string   `json:"user_id"`
	Email           string   `json:"email"`
	ActorType       string   `json:"actor_type"`
	DepartmentCodes []string `json:"department_codes"`
	DistrictCodes   []string `json:"district_codes"`
	Roles           []string `json:"roles"`
}

func (svc *IdentityService) IdentityConfiguration(ctx context.Context, actor contract.Actor) (*contract.IdentityConfiguration, error) {
	if err := requirePlatformAdministrator(actor); err != nil {
		return nil, err
	}
	svc.federationMu.Lock()
	defer svc.federationMu.Unlock()
	revision, err := svc.ensureIdentityConfiguration(ctx)
	if err != nil {
		return nil, err
	}
	return svc.identityConfigurationFromRevision(revision), nil
}

func (svc *IdentityService) UpdateIdentityConfiguration(ctx context.Context, actor contract.Actor, expectedVersion uint64, input contract.IdentityConfigurationWrite) (*contract.IdentityConfiguration, error) {
	if err := requirePlatformAdministrator(actor); err != nil {
		return nil, err
	}
	if expectedVersion == 0 {
		return nil, expectedVersionRequired()
	}
	svc.federationMu.Lock()
	defer svc.federationMu.Unlock()
	current, err := svc.ensureIdentityConfiguration(ctx)
	if err != nil {
		return nil, err
	}
	if uint64(current.Version) != expectedVersion {
		return nil, versionConflict(current.Version)
	}
	payload := identityConfigurationPayload{}
	decodeConfigurationPayload(current.Payload, &payload)
	if input.OIDCEnabled != nil {
		payload.OIDCEnabled = *input.OIDCEnabled
	}
	if input.SAMLEnabled != nil {
		payload.SAMLEnabled = *input.SAMLEnabled
	}
	if fields := svc.validateIdentityEnablement(payload); len(fields) > 0 {
		return nil, validationError(fields...)
	}
	now := svc.now().UTC()
	next := &composeTypes.City311ConfigurationRevision{
		ID: svc.nextID(), ResourceType: configurationIdentity, ResourceKey: identityConfigurationKey,
		Payload: composeTypes.City311JSON{"oidc_enabled": payload.OIDCEnabled, "saml_enabled": payload.SAMLEnabled},
		Version: current.Version + 1, Published: true, CreatedAt: now,
	}
	err = store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		if err := store.CreateCity311ConfigurationRevision(ctx, tx, next); err != nil {
			return err
		}
		return store.CreateCity311AuditEvent(ctx, tx, &composeTypes.City311AuditEvent{
			ID: svc.nextID(), EntityType: "identity_configuration", EntityID: identityConfigurationKey,
			EventType: "IDENTITY_CONFIGURATION_UPDATED", ActorType: contract.AuditActorStaff, ActorID: actor.ID,
			SourceChannel: contract.SourceChannelStaffInPerson, Before: cloneMap(current.Payload), After: cloneMap(next.Payload), CreatedAt: now,
		})
	})
	if err != nil {
		return nil, err
	}
	return svc.identityConfigurationFromRevision(next), nil
}

func (svc *IdentityService) ensureIdentityConfiguration(ctx context.Context) (*composeTypes.City311ConfigurationRevision, error) {
	revision, err := svc.latestIdentityRevision(ctx, svc.store, configurationIdentity, identityConfigurationKey)
	if err == nil {
		return revision, nil
	}
	if !errors.IsNotFound(err) {
		return nil, err
	}
	now := svc.now().UTC()
	oidcConfigured := svc.oidcRuntimeConfigured()
	samlConfigured := svc.samlRuntimeConfigured()
	revision = &composeTypes.City311ConfigurationRevision{
		ID: svc.nextID(), ResourceType: configurationIdentity, ResourceKey: identityConfigurationKey,
		Payload: composeTypes.City311JSON{"oidc_enabled": oidcConfigured, "saml_enabled": samlConfigured},
		Version: 1, Published: true, CreatedAt: now,
	}
	if err = store.CreateCity311ConfigurationRevision(ctx, svc.store, revision); err != nil {
		return nil, err
	}
	return revision, nil
}

func (svc *IdentityService) latestIdentityRevision(ctx context.Context, st store.Storer, resourceType, resourceKey string) (*composeTypes.City311ConfigurationRevision, error) {
	set, _, err := store.SearchCity311ConfigurationRevisions(ctx, st, composeTypes.City311ConfigurationRevisionFilter{
		ResourceType: resourceType, ResourceKey: resourceKey,
	})
	if err != nil {
		return nil, err
	}
	var latest *composeTypes.City311ConfigurationRevision
	for _, revision := range set {
		if revision.ResourceType == resourceType && revision.ResourceKey == resourceKey && (latest == nil || revision.Version > latest.Version) {
			latest = revision
		}
	}
	if latest == nil {
		return nil, errors.NotFound("configuration revision not found")
	}
	return latest, nil
}

func (svc *IdentityService) identityConfigurationFromRevision(revision *composeTypes.City311ConfigurationRevision) *contract.IdentityConfiguration {
	payload := identityConfigurationPayload{}
	decodeConfigurationPayload(revision.Payload, &payload)
	runtime, _ := svc.federationRuntime()
	return &contract.IdentityConfiguration{
		OIDCEnabled: payload.OIDCEnabled, SAMLEnabled: payload.SAMLEnabled,
		OIDCIssuerURL: runtime.OIDCIssuerURL, OIDCStaffClientID: runtime.OIDCStaffClientID,
		OIDCPublicClientID: runtime.OIDCPublicClientID, OIDCClientSecretConfigured: runtime.OIDCClientSecret != "",
		SAMLMetadataURL: runtime.SAMLMetadataURL, SAMLSPServiceEntityID: runtime.SAMLServiceProvider,
		ActorRoleMappings: append([]contract.ActorRoleMapping(nil), identityRoleMappings...),
		Version:           uint64(revision.Version), UpdatedAt: revision.CreatedAt,
	}
}

func (svc *IdentityService) validateIdentityEnablement(payload identityConfigurationPayload) []contract.FieldError {
	fields := make([]contract.FieldError, 0, 2)
	if payload.OIDCEnabled && !svc.oidcRuntimeConfigured() {
		fields = append(fields, contract.FieldError{Field: "/oidc_enabled", Code: contract.ValidationInvalidValue})
	}
	if payload.SAMLEnabled && !svc.samlRuntimeConfigured() {
		fields = append(fields, contract.FieldError{Field: "/saml_enabled", Code: contract.ValidationInvalidValue})
	}
	return fields
}

func (svc *IdentityService) oidcRuntimeConfigured() bool {
	runtime, _ := svc.federationRuntime()
	issuer, err := url.Parse(runtime.OIDCIssuerURL)
	return err == nil && issuer.Host != "" && (issuer.Scheme == "http" || issuer.Scheme == "https") &&
		runtime.OIDCStaffClientID != "" && runtime.OIDCPublicClientID != "" &&
		runtime.OIDCClientSecret != "" && validFederationBaseURL(runtime)
}

func (svc *IdentityService) samlRuntimeConfigured() bool {
	runtime, _ := svc.federationRuntime()
	metadata, metadataErr := url.Parse(runtime.SAMLMetadataURL)
	entity, entityErr := url.Parse(runtime.SAMLServiceProvider)
	return validFederationBaseURL(runtime) && metadataErr == nil && metadata.Host != "" &&
		entityErr == nil && entity.Host != ""
}

func validFederationBaseURL(runtime IdentityRuntimeConfiguration) bool {
	parsed, err := url.Parse(runtime.BaseURL)
	return err == nil && parsed.Host != "" && (parsed.Scheme == "http" || parsed.Scheme == "https")
}

func (svc *IdentityService) StartFederatedSignIn(ctx context.Context, provider, requestedClient string, resolved *ResolvedSession) (*contract.FederatedRedirect, string, error) {
	if svc.configErr != nil {
		return nil, "", svc.configurationUnavailable()
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider != federatedProviderOIDC && provider != federatedProviderSAML {
		return nil, "", apiError(http.StatusNotFound, contract.ErrorNotFound, "The identity provider was not found.")
	}
	client, linkUserID, err := federatedClientFor(provider, requestedClient, resolved)
	if err != nil {
		return nil, "", err
	}
	svc.federationMu.Lock()
	configuration, err := svc.ensureIdentityConfiguration(ctx)
	svc.federationMu.Unlock()
	if err != nil {
		return nil, "", err
	}
	effective := svc.identityConfigurationFromRevision(configuration)
	if (provider == federatedProviderOIDC && !effective.OIDCEnabled) || (provider == federatedProviderSAML && !effective.SAMLEnabled) {
		return nil, "", svc.configurationUnavailable()
	}
	state, err := randomToken(svc.random)
	if err != nil {
		return nil, "", err
	}
	nonce, err := randomToken(svc.random)
	if err != nil {
		return nil, "", err
	}
	verifier, err := randomToken(svc.random)
	if err != nil {
		return nil, "", err
	}
	flow := federationFlow{
		Provider: provider, Client: client, State: state, Nonce: nonce, PKCEVerifier: verifier,
		LinkUserID: linkUserID, IssuedAt: svc.now().UTC(),
	}
	runtime, federation := svc.federationRuntime()
	authorization, err := federation.Start(ctx, FederationStartRequest{
		Provider: provider, Client: client, State: state, Nonce: nonce, PKCEVerifier: verifier,
		CallbackURL: federationCallbackURL(runtime, provider),
	})
	if err != nil {
		return nil, "", svc.federationError(err)
	}
	flow.SAMLRequestID = authorization.RequestID
	sealed, err := svc.sealFederationFlow(flow)
	if err != nil {
		return nil, "", err
	}
	return &contract.FederatedRedirect{AuthorizationURL: authorization.URL}, sealed, nil
}

func federatedClientFor(provider, requested string, resolved *ResolvedSession) (string, string, error) {
	client := strings.ToLower(strings.TrimSpace(requested))
	linkUserID := ""
	if resolved != nil && resolved.User != nil {
		linkUserID = strconv.FormatUint(resolved.User.ID, 10)
		if resolved.Actor != nil && identityHasRole(resolved.Actor.ApplicationRoles, contract.ApplicationRoleConstituent) {
			client = federatedClientPublic
		} else {
			client = federatedClientStaff
		}
	}
	if client == "" {
		client = federatedClientStaff
	}
	if client != federatedClientStaff && client != federatedClientPublic {
		return "", "", validationError(contract.FieldError{Field: "/query/client", Code: contract.ValidationInvalidValue})
	}
	if provider == federatedProviderSAML && client != federatedClientStaff {
		return "", "", validationError(contract.FieldError{Field: "/query/client", Code: contract.ValidationInvalidValue})
	}
	return client, linkUserID, nil
}

func (svc *IdentityService) CompleteFederatedSignIn(ctx context.Context, provider, returnedState, code, samlResponse, sealedFlow string) (string, *ResolvedSession, error) {
	if svc.configErr != nil {
		return "", nil, svc.configurationUnavailable()
	}
	flow, err := svc.openFederationFlow(sealedFlow)
	if err != nil || flow.Provider != strings.ToLower(strings.TrimSpace(provider)) ||
		!hmac.Equal([]byte(flow.State), []byte(strings.TrimSpace(returnedState))) ||
		svc.now().Before(flow.IssuedAt) || svc.now().Sub(flow.IssuedAt) > federationFlowLifetime {
		return "", nil, federatedAuthenticationFailed(federationAuthenticationFailedMsg)
	}
	svc.federationMu.Lock()
	configuration, err := svc.ensureIdentityConfiguration(ctx)
	svc.federationMu.Unlock()
	if err != nil {
		return "", nil, err
	}
	effective := svc.identityConfigurationFromRevision(configuration)
	if (flow.Provider == federatedProviderOIDC && !effective.OIDCEnabled) || (flow.Provider == federatedProviderSAML && !effective.SAMLEnabled) {
		return "", nil, svc.configurationUnavailable()
	}
	runtime, federation := svc.federationRuntime()
	claims, err := federation.Callback(ctx, FederationCallbackRequest{
		Provider: flow.Provider, Client: flow.Client, Code: strings.TrimSpace(code), Nonce: flow.Nonce,
		PKCEVerifier: flow.PKCEVerifier, CallbackURL: federationCallbackURL(runtime, flow.Provider),
		SAMLResponse: strings.TrimSpace(samlResponse), SAMLRequestID: flow.SAMLRequestID,
	})
	if err != nil {
		return "", nil, svc.federationError(err)
	}
	claims, roles, err := validateFederatedClaims(flow, claims)
	if err != nil {
		return "", nil, err
	}
	userID, err := svc.persistFederatedAccount(ctx, flow, claims, roles)
	if err != nil {
		return "", nil, err
	}
	rawToken, record, err := svc.createSession(ctx, userID)
	if err != nil {
		return "", nil, err
	}
	account, user, err := svc.resolveSessionPrincipals(ctx, record)
	if err != nil || account == nil || user == nil {
		svc.discardIdentitySession(ctx, record.ID)
		if err != nil {
			return "", nil, err
		}
		return "", nil, federatedAuthenticationFailed(federationAuthenticationFailedMsg)
	}
	actor, err := svc.currentActor(ctx, user, account)
	if err != nil {
		svc.discardIdentitySession(ctx, record.ID)
		return "", nil, err
	}
	return rawToken, &ResolvedSession{Record: record, Account: account, User: user, Actor: actor}, nil
}

func validateFederatedClaims(flow federationFlow, claims FederatedClaims) (FederatedClaims, []contract.ApplicationRole, error) {
	claims.Subject = strings.TrimSpace(claims.Subject)
	claims.Email = normalizeEmail(claims.Email)
	claims.DisplayName = strings.TrimSpace(claims.DisplayName)
	claims.ActorType = strings.ToLower(strings.TrimSpace(claims.ActorType))
	claims.DepartmentCodes = uniqueTrimmed(claims.DepartmentCodes)
	claims.DistrictCodes = uniqueTrimmed(claims.DistrictCodes)
	claims.Roles = uniqueTrimmed(claims.Roles)
	if claims.Subject == "" || !claims.EmailVerified || !validEmail(claims.Email) || len(validateDisplayName(claims.DisplayName, "/name")) > 0 {
		return claims, nil, federatedAuthenticationFailed(federationAuthenticationFailedMsg)
	}
	if len(claims.DepartmentCodes) > 1 || !allKnownStrings(claims.DepartmentCodes, departmentStrings()) ||
		!allKnownStrings(claims.DistrictCodes, districtStrings()) {
		return claims, nil, federatedAuthenticationFailed(federationAuthenticationFailedMsg)
	}
	roles := make([]contract.ApplicationRole, 0, len(claims.Roles))
	for _, asserted := range claims.Roles {
		mapped, ok := mappedFederatedRole(asserted)
		if !ok {
			return claims, nil, federatedAuthenticationFailed(federationAuthenticationFailedMsg)
		}
		roles = append(roles, mapped)
	}
	if flow.Client == federatedClientPublic {
		if flow.Provider != federatedProviderOIDC || claims.ActorType != "constituent" || len(roles) != 0 {
			return claims, nil, federatedAuthenticationFailed(federationAuthenticationFailedMsg)
		}
		roles = []contract.ApplicationRole{contract.ApplicationRoleConstituent}
	} else if claims.ActorType != federatedClientStaff || len(roles) == 0 {
		return claims, nil, federatedAuthenticationFailed(federationAuthenticationFailedMsg)
	}
	return claims, roles, nil
}

func (svc *IdentityService) persistFederatedAccount(ctx context.Context, flow federationFlow, claims FederatedClaims, roles []contract.ApplicationRole) (uint64, error) {
	svc.federationMu.Lock()
	defer svc.federationMu.Unlock()
	key := federatedAccountKey(flow.Provider, claims.Subject)
	current, err := svc.latestIdentityRevision(ctx, svc.store, configurationFederatedAccount, key)
	if err != nil && !errors.IsNotFound(err) {
		return 0, err
	}
	snapshot := federatedAccountPayload{}
	if current != nil {
		decodeConfigurationPayload(current.Payload, &snapshot)
		if snapshot.Provider != flow.Provider || snapshot.Subject != claims.Subject {
			return 0, federatedAuthenticationFailed(federationAuthenticationFailedMsg)
		}
	}
	var userID uint64
	err = store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		var account *composeTypes.City311LocalAccount
		if current != nil {
			userID, err = strconv.ParseUint(snapshot.UserID, 10, 64)
			if err != nil || userID == 0 {
				return federatedAuthenticationFailed(federationAuthenticationFailedMsg)
			}
			account, err = store.LookupCity311LocalAccountByID(ctx, tx, userID)
			if err != nil {
				return err
			}
		} else {
			account, userID, err = svc.resolveFederatedLinkTarget(ctx, tx, flow, claims)
			if err != nil {
				return err
			}
		}
		if collision, lookupErr := store.LookupCity311LocalAccountByVerifiedEmail(ctx, tx, claims.Email); lookupErr == nil && collision.ID != userID {
			return federatedAuthenticationFailed(federationLinkRequiredMsg)
		} else if lookupErr != nil && !errors.IsNotFound(lookupErr) {
			return lookupErr
		}
		newAccount := account == nil
		if newAccount {
			userID = svc.nextID()
			digest := sha256.Sum256([]byte(flow.Provider + "\x00" + claims.Subject))
			identifier := "federated-" + hex.EncodeToString(digest[:12])
			now := svc.now().UTC()
			user := &systemTypes.User{
				ID: userID, Handle: "city311-" + identifier, Username: identifier, Email: claims.Email,
				Name: claims.DisplayName, EmailConfirmed: true, Meta: &systemTypes.UserMeta{PreferredLanguage: "en"}, CreatedAt: now,
			}
			if err = store.CreateUser(ctx, tx, user); err != nil {
				return err
			}
			account = &composeTypes.City311LocalAccount{
				ID: userID, LoginIdentifier: identifier, VerifiedEmail: claims.Email,
				PreferredLanguage: string(contract.LanguageEN), CreatedAt: now, UpdatedAt: now,
			}
			if err = store.CreateCity311LocalAccount(ctx, tx, account); err != nil {
				return err
			}
			if flow.Client == federatedClientPublic {
				if err = svc.createConstituentAccount(ctx, tx, user, account, now); err != nil {
					return err
				}
			}
		} else if err = svc.updateFederatedPrincipals(ctx, tx, account, claims); err != nil {
			return err
		}
		if err = svc.replaceFederatedRoles(ctx, tx, userID, roles, claims); err != nil {
			return err
		}
		return svc.appendFederatedMapping(ctx, tx, current, key, userID, flow, claims, newAccount)
	})
	return userID, err
}

func (svc *IdentityService) resolveFederatedLinkTarget(ctx context.Context, tx store.Storer, flow federationFlow, claims FederatedClaims) (*composeTypes.City311LocalAccount, uint64, error) {
	account, err := store.LookupCity311LocalAccountByVerifiedEmail(ctx, tx, claims.Email)
	if errors.IsNotFound(err) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	linkID, parseErr := strconv.ParseUint(flow.LinkUserID, 10, 64)
	if parseErr != nil || linkID == 0 || linkID != account.ID {
		return nil, 0, federatedAuthenticationFailed(federationLinkRequiredMsg)
	}
	return account, account.ID, nil
}

func (svc *IdentityService) updateFederatedPrincipals(ctx context.Context, tx store.Storer, account *composeTypes.City311LocalAccount, claims FederatedClaims) error {
	user, err := store.LookupUserByID(ctx, tx, account.ID)
	if err != nil || !user.Valid() {
		if err != nil {
			return err
		}
		return federatedAuthenticationFailed(federationAuthenticationFailedMsg)
	}
	user.Email = claims.Email
	user.EmailConfirmed = true
	user.Name = claims.DisplayName
	if err = store.UpdateUser(ctx, tx, user); err != nil {
		return err
	}
	account.VerifiedEmail = claims.Email
	account.UpdatedAt = svc.now().UTC()
	return store.UpdateCity311LocalAccount(ctx, tx, account)
}

func (svc *IdentityService) replaceFederatedRoles(ctx context.Context, tx store.Storer, userID uint64, roles []contract.ApplicationRole, claims FederatedClaims) error {
	resource := fmt.Sprintf("corteza::system:user/%d", userID)
	desired := make(map[uint64]bool, len(roles))
	federatedRoles := []contract.ApplicationRole{
		contract.ApplicationRoleConstituent, contract.ApplicationRoleServiceAgent, contract.ApplicationRoleSupervisor,
		contract.ApplicationRoleDepartmentManager, contract.ApplicationRolePlatformAdministrator, contract.ApplicationRoleWorkflowDesigner,
	}
	cityRoleIDs := make(map[uint64]bool, len(federatedRoles))
	for _, applicationRole := range federatedRoles {
		role, err := store.LookupRoleByHandle(ctx, tx, "city311-"+string(applicationRole))
		if err != nil {
			return err
		}
		cityRoleIDs[role.ID] = true
		for _, selected := range roles {
			if applicationRole == selected {
				desired[role.ID] = true
			}
		}
	}
	memberships, _, err := store.SearchRoleMembers(ctx, tx, systemTypes.RoleMemberFilter{Resource: resource})
	if err != nil {
		return err
	}
	existing := make(map[uint64]bool, len(memberships))
	for _, membership := range memberships {
		existing[membership.RoleID] = true
		if cityRoleIDs[membership.RoleID] && !desired[membership.RoleID] {
			if err = store.DeleteRoleMember(ctx, tx, membership); err != nil {
				return err
			}
		}
	}
	for roleID := range desired {
		if !existing[roleID] {
			if err = store.CreateRoleMember(ctx, tx, &systemTypes.RoleMember{RoleID: roleID, Resource: resource}); err != nil {
				return err
			}
		}
	}
	now := svc.now().UTC()
	profile, err := store.LookupCity311ActorProfileByID(ctx, tx, userID)
	if errors.IsNotFound(err) {
		profile = &composeTypes.City311ActorProfile{ID: userID, CreatedAt: now}
		profile.ApplicationRoles = composeTypes.City311ApplicationRoleSet(roles)
		profile.Districts = composeTypes.City311DistrictCodeSet(stringDistricts(claims.DistrictCodes))
		if len(claims.DepartmentCodes) == 1 {
			profile.Department = contract.DepartmentCode(claims.DepartmentCodes[0])
		}
		profile.UpdatedAt = now
		return store.CreateCity311ActorProfile(ctx, tx, profile)
	}
	if err != nil {
		return err
	}
	profile.ApplicationRoles = composeTypes.City311ApplicationRoleSet(roles)
	profile.Department = ""
	if len(claims.DepartmentCodes) == 1 {
		profile.Department = contract.DepartmentCode(claims.DepartmentCodes[0])
	}
	profile.Districts = composeTypes.City311DistrictCodeSet(stringDistricts(claims.DistrictCodes))
	profile.UpdatedAt = now
	return store.UpdateCity311ActorProfile(ctx, tx, profile)
}

func (svc *IdentityService) appendFederatedMapping(ctx context.Context, tx store.Storer, current *composeTypes.City311ConfigurationRevision, key string, userID uint64, flow federationFlow, claims FederatedClaims, created bool) error {
	payload := federatedAccountPayload{
		Provider: flow.Provider, Subject: claims.Subject, UserID: strconv.FormatUint(userID, 10), Email: claims.Email,
		ActorType: claims.ActorType, DepartmentCodes: claims.DepartmentCodes, DistrictCodes: claims.DistrictCodes, Roles: claims.Roles,
	}
	encoded, err := mapFrom(payload)
	if err != nil {
		return err
	}
	version := 1
	before := map[string]any{}
	if current != nil {
		version = current.Version + 1
		before = cloneMap(current.Payload)
	}
	now := svc.now().UTC()
	revision := &composeTypes.City311ConfigurationRevision{
		ID: svc.nextID(), ResourceType: configurationFederatedAccount, ResourceKey: key,
		Payload: encoded, Version: version, Published: true, CreatedAt: now,
	}
	if err = store.CreateCity311ConfigurationRevision(ctx, tx, revision); err != nil {
		return err
	}
	event := "FEDERATED_ACCOUNT_UPDATED"
	if created {
		event = "FEDERATED_ACCOUNT_PROVISIONED"
	} else if current == nil {
		event = "FEDERATED_ACCOUNT_LINKED"
	}
	actorType := contract.AuditActorStaff
	if flow.Client == federatedClientPublic {
		actorType = contract.AuditActorConstituent
	}
	return store.CreateCity311AuditEvent(ctx, tx, &composeTypes.City311AuditEvent{
		ID: svc.nextID(), EntityType: "account", EntityID: strconv.FormatUint(userID, 10), EventType: event,
		ActorType: actorType, ActorID: userID, SourceChannel: contract.SourceChannelPortalAuthenticated,
		Before: before, After: cloneMap(encoded), CreatedAt: now,
	})
}

func (svc *IdentityService) sealFederationFlow(flow federationFlow) (string, error) {
	encoded, err := json.Marshal(flow)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(svc.federationEncryptionKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(svc.random, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, encoded, []byte("city311-federation-flow-v1"))
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (svc *IdentityService) openFederationFlow(value string) (federationFlow, error) {
	flow := federationFlow{}
	sealed, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return flow, err
	}
	block, err := aes.NewCipher(svc.federationEncryptionKey())
	if err != nil {
		return flow, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(sealed) < gcm.NonceSize() {
		return flow, fmt.Errorf("federation flow is malformed")
	}
	opened, err := gcm.Open(nil, sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():], []byte("city311-federation-flow-v1"))
	if err != nil {
		return flow, err
	}
	if err = json.Unmarshal(opened, &flow); err != nil {
		return flow, err
	}
	return flow, nil
}

func (svc *IdentityService) federationEncryptionKey() []byte {
	key := sha256.Sum256(append([]byte("city311-federation-flow-v1\x00"), svc.secret...))
	return key[:]
}

func federationCallbackURL(runtime IdentityRuntimeConfiguration, provider string) string {
	return runtime.BaseURL + "/api/v1/auth/" + provider + "/callback"
}

func (svc *IdentityService) federationError(err error) error {
	var unavailable *federationUnavailableError
	if stderrors.As(err, &unavailable) {
		return svc.configurationUnavailable()
	}
	return federatedAuthenticationFailed(federationAuthenticationFailedMsg)
}

func federatedAuthenticationFailed(message string) *ServiceError {
	return apiError(http.StatusUnauthorized, contract.ErrorUnauthenticated, message)
}

func federatedAccountKey(provider, subject string) string {
	digest := sha256.Sum256([]byte(provider + "\x00" + subject))
	return provider + ":" + hex.EncodeToString(digest[:])
}

func mappedFederatedRole(value string) (contract.ApplicationRole, bool) {
	for _, mapping := range identityRoleMappings {
		if string(mapping.AssertedRole) == value {
			return mapping.ApplicationRole, true
		}
	}
	return "", false
}

func uniqueTrimmed(values []string) []string {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			set[value] = true
		}
	}
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func allKnownStrings(values, known []string) bool {
	allowed := make(map[string]bool, len(known))
	for _, value := range known {
		allowed[value] = true
	}
	for _, value := range values {
		if !allowed[value] {
			return false
		}
	}
	return true
}

func departmentStrings() []string {
	out := make([]string, len(contract.DepartmentCodes))
	for index, value := range contract.DepartmentCodes {
		out[index] = string(value)
	}
	return out
}

func districtStrings() []string {
	out := make([]string, len(contract.DistrictCodes))
	for index, value := range contract.DistrictCodes {
		out[index] = string(value)
	}
	return out
}

func stringDistricts(values []string) []contract.DistrictCode {
	out := make([]contract.DistrictCode, len(values))
	for index, value := range values {
		out[index] = contract.DistrictCode(value)
	}
	return out
}
