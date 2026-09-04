package city311

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
)

const (
	configurationIntegration = "INTEGRATION_CONNECTION"
	integrationListSort      = "integration_id"

	IntegrationCivicWorksID = "civicworks"
	IntegrationMappingID    = "mapping"
	IntegrationWorkflowID   = "workflow-oauth"
	IntegrationMailID       = "mail"
	IntegrationIdentityID   = "identity"
)

type integrationDescriptor struct {
	ID   string
	Kind contract.IntegrationKind
}

var integrationDescriptors = []integrationDescriptor{
	{ID: IntegrationCivicWorksID, Kind: contract.IntegrationKindCivicWorks},
	{ID: IntegrationIdentityID, Kind: contract.IntegrationKindIdentity},
	{ID: IntegrationMailID, Kind: contract.IntegrationKindMail},
	{ID: IntegrationMappingID, Kind: contract.IntegrationKindMapping},
	{ID: IntegrationWorkflowID, Kind: contract.IntegrationKindWorkflowOAuth},
}

type IntegrationListQuery struct {
	Kinds     []contract.IntegrationKind
	Active    *bool
	PageSize  uint
	PageToken string
	Sort      string
}

type integrationConnectionPayload struct {
	Kind          contract.IntegrationKind `json:"kind"`
	Active        bool                     `json:"active"`
	Configuration map[string]any           `json:"configuration"`
	SealedSecret  string                   `json:"sealed_secret,omitempty"`
}

type integrationSecretBundle map[string]string

type preparedIntegrationRuntime struct {
	civicWorks       CivicWorksClient
	civicWorksSecret string
	mapping          *MappingService
	workflow         WorkflowHTTPClient
	mail             MailSender
	identity         *IdentityRuntimeConfiguration
}

type disabledMailSender struct{}

func (disabledMailSender) Send(context.Context, MailMessage, string) (int, error) {
	return 0, fmt.Errorf("mail integration is inactive")
}

func integrationEncryptionKey() [32]byte {
	secret := strings.TrimSpace(os.Getenv("SESSION_SECRET"))
	if secret == "" {
		ephemeral := make([]byte, 32)
		if _, err := io.ReadFull(cryptorand.Reader, ephemeral); err != nil {
			panic(fmt.Sprintf("city311: generate integration encryption key: %v", err))
		}
		return sha256.Sum256(ephemeral)
	}
	return sha256.Sum256([]byte("city311-integration-secret-v1\x00" + secret))
}

func (svc *Service) seedIntegrationConnections(ctx context.Context, tx store.Storer, createdAt time.Time) error {
	for _, descriptor := range integrationDescriptors {
		revisions, err := svc.configurationRevisions(ctx, tx, configurationIntegration, descriptor.ID)
		if err != nil {
			return err
		}
		if len(revisions) > 0 {
			continue
		}
		configuration, secrets := integrationEnvironment(descriptor.Kind)
		active := integrationConfigurationValid(descriptor.Kind, configuration, secrets)
		sealed, err := svc.sealIntegrationSecrets(descriptor.ID, secrets)
		if err != nil {
			return err
		}
		payload, err := mapFrom(integrationConnectionPayload{
			Kind: descriptor.Kind, Active: active, Configuration: configuration, SealedSecret: sealed,
		})
		if err != nil {
			return err
		}
		if err = store.CreateCity311ConfigurationRevision(ctx, tx, &composeTypes.City311ConfigurationRevision{
			ID: svc.nextID(), ResourceType: configurationIntegration, ResourceKey: descriptor.ID,
			Payload: payload, Version: 1, Published: true, CreatedAt: createdAt.UTC(),
		}); err != nil {
			return err
		}
	}
	return nil
}

func integrationEnvironment(kind contract.IntegrationKind) (map[string]any, integrationSecretBundle) {
	configuration := map[string]any{}
	secrets := integrationSecretBundle{}
	setConfiguration := func(key, environment string) { configuration[key] = strings.TrimSpace(os.Getenv(environment)) }
	setSecret := func(key, environment string) {
		if value := strings.TrimSpace(os.Getenv(environment)); value != "" {
			secrets[key] = value
		}
	}
	switch kind {
	case contract.IntegrationKindCivicWorks:
		setConfiguration("base_url", "CIVICWORKS_BASE_URL")
		setConfiguration("benchmark_run_id", "BENCHMARK_RUN_ID")
		setSecret("api_token", "CIVICWORKS_API_TOKEN")
		setSecret("webhook_secret", "CIVICWORKS_WEBHOOK_SECRET")
	case contract.IntegrationKindMapping:
		setConfiguration("base_url", "MAP_BASE_URL")
		setSecret("api_token", "MAP_API_TOKEN")
	case contract.IntegrationKindWorkflowOAuth:
		setConfiguration("oauth_token_url", "WORKFLOW_OAUTH_TOKEN_URL")
		setConfiguration("api_base_url", "WORKFLOW_API_BASE_URL")
		setConfiguration("client_id", "WORKFLOW_CLIENT_ID")
		setSecret("client_secret", "WORKFLOW_CLIENT_SECRET")
	case contract.IntegrationKindMail:
		setConfiguration("smtp_host", "MAIL_SMTP_HOST")
		setConfiguration("smtp_port", "MAIL_SMTP_PORT")
		setConfiguration("smtp_username", "MAIL_SMTP_USERNAME")
		setConfiguration("api_base_url", "MAIL_API_BASE_URL")
		setSecret("smtp_password", "MAIL_SMTP_PASSWORD")
		setSecret("api_token", "MAIL_API_TOKEN")
	case contract.IntegrationKindIdentity:
		setConfiguration("app_base_url", "APP_BASE_URL")
		setConfiguration("oidc_issuer_url", "OIDC_ISSUER_URL")
		setConfiguration("oidc_staff_client_id", "OIDC_STAFF_CLIENT_ID")
		setConfiguration("oidc_public_client_id", "OIDC_PUBLIC_CLIENT_ID")
		setConfiguration("saml_metadata_url", "SAML_METADATA_URL")
		setConfiguration("saml_sp_entity_id", "SAML_SP_ENTITY_ID")
		setSecret("oidc_client_secret", "OIDC_CLIENT_SECRET")
	}
	return configuration, secrets
}

func (svc *Service) ListIntegrations(ctx context.Context, actor contract.Actor, query IntegrationListQuery) (*contract.IntegrationConnectionList, error) {
	if err := requirePlatformAdministrator(actor); err != nil {
		return nil, err
	}
	revisions, err := svc.latestConfigurationResources(ctx, svc.store, configurationIntegration)
	if err != nil {
		return nil, err
	}
	items := make([]contract.IntegrationConnection, 0, len(revisions))
	for _, revision := range revisions {
		item := integrationConnectionFromRevision(revision)
		if integrationMatches(item, query) {
			items = append(items, *item)
		}
	}
	sorts, err := normalizeIntegrationSort(query.Sort)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(items, func(left, right int) bool { return integrationLess(items[left], items[right], sorts) })
	pageQuery := ConfigurationListQuery{PageSize: query.PageSize, PageToken: query.PageToken}
	start, end, next, err := configurationPage(pageQuery, len(items), strings.Join(sorts, ","))
	if err != nil {
		return nil, err
	}
	applied := map[string]any{}
	if len(query.Kinds) > 0 {
		applied["kind"] = append([]contract.IntegrationKind(nil), query.Kinds...)
	}
	if query.Active != nil {
		applied["active"] = *query.Active
	}
	return &contract.IntegrationConnectionList{
		Items: items[start:end], NextPageToken: next, TotalCount: len(items), AppliedFilters: applied, Sort: sorts,
	}, nil
}

func (svc *Service) GetIntegration(ctx context.Context, actor contract.Actor, integrationID string) (*contract.IntegrationConnection, error) {
	if err := requirePlatformAdministrator(actor); err != nil {
		return nil, err
	}
	revision, err := svc.integrationRevision(ctx, integrationID)
	if err != nil {
		return nil, err
	}
	return integrationConnectionFromRevision(revision), nil
}

func (svc *Service) UpdateIntegration(ctx context.Context, actor contract.Actor, integrationID string, expectedVersion uint64, input contract.IntegrationConnectionWrite) (*contract.IntegrationConnection, error) {
	if err := requirePlatformAdministrator(actor); err != nil {
		return nil, err
	}
	if expectedVersion == 0 {
		return nil, expectedVersionRequired()
	}
	if input.Active == nil {
		return nil, validationError(contract.FieldError{Field: "/active", Code: contract.ValidationRequired})
	}
	svc.integrationMu.Lock()
	defer svc.integrationMu.Unlock()
	current, payload, secrets, err := svc.currentIntegration(ctx, integrationID, expectedVersion)
	if err != nil {
		return nil, err
	}
	payload.Active = *input.Active
	if input.Configuration != nil {
		payload.Configuration = cloneAnyMap(input.Configuration)
	}
	if input.Secret != nil {
		if err = replaceIntegrationSecret(payload.Kind, secrets, *input.Secret, "/secret"); err != nil {
			return nil, err
		}
	}
	payload.Configuration, err = validateIntegrationConfiguration(payload.Kind, payload.Configuration)
	if err != nil {
		return nil, err
	}
	runtime, err := prepareIntegrationRuntime(payload.Kind, payload.Active, payload.Configuration, secrets)
	if err != nil {
		return nil, validationError(contract.FieldError{Field: "/configuration", Code: contract.ValidationInvalidValue})
	}
	return svc.appendIntegrationRevision(ctx, actor, current, payload, secrets, runtime, "INTEGRATION_UPDATED", "")
}

func (svc *Service) RotateIntegrationSecret(ctx context.Context, actor contract.Actor, integrationID string, expectedVersion uint64, input contract.SecretRotation) (*contract.IntegrationConnection, error) {
	if err := requirePlatformAdministrator(actor); err != nil {
		return nil, err
	}
	if expectedVersion == 0 {
		return nil, expectedVersionRequired()
	}
	svc.integrationMu.Lock()
	defer svc.integrationMu.Unlock()
	current, payload, secrets, err := svc.currentIntegration(ctx, integrationID, expectedVersion)
	if err != nil {
		return nil, err
	}
	if err = replaceIntegrationSecret(payload.Kind, secrets, input.NewSecret, "/new_secret"); err != nil {
		return nil, err
	}
	runtime, err := prepareIntegrationRuntime(payload.Kind, payload.Active, payload.Configuration, secrets)
	if err != nil {
		return nil, validationError(contract.FieldError{Field: "/new_secret", Code: contract.ValidationInvalidValue})
	}
	return svc.appendIntegrationRevision(ctx, actor, current, payload, secrets, runtime, "INTEGRATION_SECRET_ROTATED", "")
}

func (svc *Service) RevokeIntegration(ctx context.Context, actor contract.Actor, integrationID string, expectedVersion uint64, input contract.Reason) (*contract.IntegrationConnection, error) {
	if err := requirePlatformAdministrator(actor); err != nil {
		return nil, err
	}
	if expectedVersion == 0 {
		return nil, expectedVersionRequired()
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		return nil, validationError(contract.FieldError{Field: "/reason", Code: contract.ValidationRequired})
	}
	svc.integrationMu.Lock()
	defer svc.integrationMu.Unlock()
	current, payload, _, err := svc.currentIntegration(ctx, integrationID, expectedVersion)
	if err != nil {
		return nil, err
	}
	payload.Active = false
	return svc.appendIntegrationRevision(ctx, actor, current, payload, integrationSecretBundle{}, preparedIntegrationRuntime{}, "INTEGRATION_REVOKED", reason)
}

func (svc *Service) currentIntegration(ctx context.Context, integrationID string, expectedVersion uint64) (*composeTypes.City311ConfigurationRevision, integrationConnectionPayload, integrationSecretBundle, error) {
	current, err := svc.integrationRevision(ctx, integrationID)
	if err != nil {
		return nil, integrationConnectionPayload{}, nil, err
	}
	if uint64(current.Version) != expectedVersion {
		return nil, integrationConnectionPayload{}, nil, versionConflict(current.Version)
	}
	payload := integrationConnectionPayload{}
	decodeConfigurationPayload(current.Payload, &payload)
	secrets, err := svc.openIntegrationSecrets(integrationID, payload.SealedSecret)
	if err != nil {
		return nil, integrationConnectionPayload{}, nil, err
	}
	return current, payload, secrets, nil
}

func (svc *Service) integrationRevision(ctx context.Context, integrationID string) (*composeTypes.City311ConfigurationRevision, error) {
	integrationID = strings.TrimSpace(integrationID)
	if !knownIntegrationID(integrationID) {
		return nil, apiError(http.StatusNotFound, contract.ErrorNotFound, "The integration connection was not found.")
	}
	revision, err := svc.latestConfigurationRevision(ctx, svc.store, configurationIntegration, integrationID, "", false)
	if err != nil {
		return nil, apiError(http.StatusNotFound, contract.ErrorNotFound, "The integration connection was not found.")
	}
	return revision, nil
}

func (svc *Service) appendIntegrationRevision(ctx context.Context, actor contract.Actor, current *composeTypes.City311ConfigurationRevision, payload integrationConnectionPayload, secrets integrationSecretBundle, runtime preparedIntegrationRuntime, eventType, reason string) (*contract.IntegrationConnection, error) {
	sealed, err := svc.sealIntegrationSecrets(current.ResourceKey, secrets)
	if err != nil {
		return nil, err
	}
	payload.SealedSecret = sealed
	encoded, err := mapFrom(payload)
	if err != nil {
		return nil, err
	}
	now := svc.now().UTC()
	next := &composeTypes.City311ConfigurationRevision{
		ID: svc.nextID(), ResourceType: configurationIntegration, ResourceKey: current.ResourceKey,
		Payload: encoded, Version: current.Version + 1, Published: true, CreatedAt: now,
	}
	err = store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		if err := store.CreateCity311ConfigurationRevision(ctx, tx, next); err != nil {
			return err
		}
		before := integrationAuditSnapshot(current)
		after := integrationAuditSnapshot(next)
		if reason != "" {
			after["reason"] = reason
		}
		return store.CreateCity311AuditEvent(ctx, tx, &composeTypes.City311AuditEvent{
			ID: svc.nextID(), EntityType: "integration_connection", EntityID: current.ResourceKey, EventType: eventType,
			ActorType: contract.AuditActorStaff, ActorID: actor.ID, SourceChannel: contract.SourceChannelStaffInPerson,
			Before: before, After: after, CreatedAt: now,
		})
	})
	if err != nil {
		return nil, err
	}
	svc.applyIntegrationRuntime(payload.Kind, payload.Active, runtime)
	return integrationConnectionFromRevision(next), nil
}

func integrationConnectionFromRevision(revision *composeTypes.City311ConfigurationRevision) *contract.IntegrationConnection {
	payload := integrationConnectionPayload{}
	decodeConfigurationPayload(revision.Payload, &payload)
	return &contract.IntegrationConnection{
		IntegrationID: revision.ResourceKey, Kind: payload.Kind, Active: payload.Active,
		SecretConfigured: payload.SealedSecret != "", Version: uint64(revision.Version), UpdatedAt: revision.CreatedAt,
	}
}

func integrationAuditSnapshot(revision *composeTypes.City311ConfigurationRevision) composeTypes.City311JSON {
	payload := integrationConnectionPayload{}
	decodeConfigurationPayload(revision.Payload, &payload)
	return composeTypes.City311JSON{
		"integration_id": revision.ResourceKey, "kind": payload.Kind, "active": payload.Active,
		"configuration": cloneAnyMap(payload.Configuration), "secret_configured": payload.SealedSecret != "", "version": revision.Version,
	}
}

func (svc *Service) sealIntegrationSecrets(integrationID string, secrets integrationSecretBundle) (string, error) {
	if len(secrets) == 0 {
		return "", nil
	}
	encoded, err := json.Marshal(secrets)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(svc.integrationKey[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(cryptorand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, encoded, []byte("city311-integration-secret-v1:"+integrationID))
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (svc *Service) openIntegrationSecrets(integrationID, value string) (integrationSecretBundle, error) {
	if value == "" {
		return integrationSecretBundle{}, nil
	}
	sealed, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("decode integration secret: %w", err)
	}
	block, err := aes.NewCipher(svc.integrationKey[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(sealed) < gcm.NonceSize() {
		return nil, fmt.Errorf("integration secret is malformed")
	}
	opened, err := gcm.Open(nil, sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():], []byte("city311-integration-secret-v1:"+integrationID))
	if err != nil {
		return nil, fmt.Errorf("open integration secret: %w", err)
	}
	secrets := integrationSecretBundle{}
	if err = json.Unmarshal(opened, &secrets); err != nil {
		return nil, fmt.Errorf("decode integration secret payload: %w", err)
	}
	return secrets, nil
}

func replaceIntegrationSecret(kind contract.IntegrationKind, secrets integrationSecretBundle, value, field string) error {
	if secrets == nil {
		secrets = integrationSecretBundle{}
	}
	if value == "" || value != strings.TrimSpace(value) || strings.ContainsAny(value, "\r\n") {
		return validationError(contract.FieldError{Field: field, Code: contract.ValidationInvalidFormat})
	}
	switch kind {
	case contract.IntegrationKindCivicWorks:
		secrets["api_token"], secrets["webhook_secret"] = value, value
	case contract.IntegrationKindMapping:
		secrets["api_token"] = value
	case contract.IntegrationKindWorkflowOAuth:
		secrets["client_secret"] = value
	case contract.IntegrationKindMail:
		secrets["smtp_password"], secrets["api_token"] = value, value
	case contract.IntegrationKindIdentity:
		secrets["oidc_client_secret"] = value
	default:
		return validationError(contract.FieldError{Field: field, Code: contract.ValidationInvalidValue})
	}
	return nil
}

func validateIntegrationConfiguration(kind contract.IntegrationKind, input map[string]any) (map[string]any, error) {
	allowed := integrationConfigurationKeys(kind)
	if allowed == nil {
		return nil, validationError(contract.FieldError{Field: "/configuration", Code: contract.ValidationInvalidValue})
	}
	configuration := make(map[string]any, len(input))
	for key, raw := range input {
		value, ok := raw.(string)
		if !ok || !allowed[key] {
			return nil, validationError(contract.FieldError{Field: "/configuration/" + key, Code: contract.ValidationInvalidValue})
		}
		configuration[key] = strings.TrimSpace(value)
	}
	return configuration, nil
}

func integrationConfigurationKeys(kind contract.IntegrationKind) map[string]bool {
	switch kind {
	case contract.IntegrationKindCivicWorks:
		return map[string]bool{"base_url": true, "benchmark_run_id": true}
	case contract.IntegrationKindMapping:
		return map[string]bool{"base_url": true}
	case contract.IntegrationKindWorkflowOAuth:
		return map[string]bool{"oauth_token_url": true, "api_base_url": true, "client_id": true}
	case contract.IntegrationKindMail:
		return map[string]bool{"smtp_host": true, "smtp_port": true, "smtp_username": true, "api_base_url": true}
	case contract.IntegrationKindIdentity:
		return map[string]bool{
			"app_base_url": true, "oidc_issuer_url": true, "oidc_staff_client_id": true, "oidc_public_client_id": true,
			"saml_metadata_url": true, "saml_sp_entity_id": true,
		}
	default:
		return nil
	}
}

func integrationConfigurationValid(kind contract.IntegrationKind, configuration map[string]any, secrets integrationSecretBundle) bool {
	_, err := prepareIntegrationRuntime(kind, true, configuration, secrets)
	return err == nil
}

func prepareIntegrationRuntime(kind contract.IntegrationKind, active bool, configuration map[string]any, secrets integrationSecretBundle) (preparedIntegrationRuntime, error) {
	if !active {
		return preparedIntegrationRuntime{}, nil
	}
	value := func(key string) string {
		text, _ := configuration[key].(string)
		return strings.TrimSpace(text)
	}
	switch kind {
	case contract.IntegrationKindCivicWorks:
		client, err := NewCivicWorks(CivicWorksOptions{
			BaseURL: value("base_url"), APIToken: secrets["api_token"], WebhookSecret: secrets["webhook_secret"], BenchmarkRunID: value("benchmark_run_id"),
		})
		return preparedIntegrationRuntime{civicWorks: client, civicWorksSecret: secrets["webhook_secret"]}, err
	case contract.IntegrationKindMapping:
		client, err := NewMapping(MappingOptions{BaseURL: value("base_url"), APIToken: secrets["api_token"]})
		return preparedIntegrationRuntime{mapping: client}, err
	case contract.IntegrationKindWorkflowOAuth:
		client, err := NewWorkflowHTTPClient(WorkflowHTTPOptions{
			TokenURL: value("oauth_token_url"), APIBaseURL: value("api_base_url"), ClientID: value("client_id"), ClientSecret: secrets["client_secret"],
		})
		return preparedIntegrationRuntime{workflow: client}, err
	case contract.IntegrationKindMail:
		port, err := strconv.ParseUint(value("smtp_port"), 10, 16)
		if err != nil || port == 0 || value("smtp_host") == "" || value("smtp_username") == "" || secrets["smtp_password"] == "" || secrets["api_token"] == "" {
			return preparedIntegrationRuntime{}, fmt.Errorf("mail runtime configuration is incomplete")
		}
		if _, err = validatedIntegrationURL(value("api_base_url")); err != nil {
			return preparedIntegrationRuntime{}, err
		}
		return preparedIntegrationRuntime{mail: smtpMailSender{
			dial: dialSMTP, host: value("smtp_host"), port: value("smtp_port"), username: value("smtp_username"), password: secrets["smtp_password"],
		}}, nil
	case contract.IntegrationKindIdentity:
		for _, key := range []string{"app_base_url", "oidc_issuer_url", "saml_metadata_url", "saml_sp_entity_id"} {
			if _, err := validatedIntegrationURL(value(key)); err != nil {
				return preparedIntegrationRuntime{}, err
			}
		}
		if value("oidc_staff_client_id") == "" || value("oidc_public_client_id") == "" || secrets["oidc_client_secret"] == "" {
			return preparedIntegrationRuntime{}, fmt.Errorf("identity runtime configuration is incomplete")
		}
		runtime := &IdentityRuntimeConfiguration{
			BaseURL: strings.TrimRight(value("app_base_url"), "/"), OIDCIssuerURL: strings.TrimRight(value("oidc_issuer_url"), "/"),
			OIDCStaffClientID: value("oidc_staff_client_id"), OIDCPublicClientID: value("oidc_public_client_id"),
			OIDCClientSecret: secrets["oidc_client_secret"], SAMLMetadataURL: value("saml_metadata_url"), SAMLServiceProvider: value("saml_sp_entity_id"),
		}
		return preparedIntegrationRuntime{identity: runtime}, nil
	default:
		return preparedIntegrationRuntime{}, fmt.Errorf("unsupported integration kind")
	}
}

func validatedIntegrationURL(value string) (*url.URL, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("integration URL must be an absolute HTTP or HTTPS URL without credentials, query, or fragment")
	}
	return parsed, nil
}

func (svc *Service) applyIntegrationRuntime(kind contract.IntegrationKind, active bool, runtime preparedIntegrationRuntime) {
	switch kind {
	case contract.IntegrationKindCivicWorks:
		if !active {
			svc.SetCivicWorks(nil, "")
		} else {
			svc.SetCivicWorks(runtime.civicWorks, runtime.civicWorksSecret)
		}
	case contract.IntegrationKindMapping:
		svc.runtimeMu.Lock()
		svc.mappingService, svc.mappingConfig = runtime.mapping, nil
		if !active || runtime.mapping == nil {
			svc.mappingService = nil
			svc.mappingConfig = fmt.Errorf("mapping integration is inactive")
		}
		svc.runtimeMu.Unlock()
	case contract.IntegrationKindWorkflowOAuth:
		if !active {
			svc.SetWorkflowHTTPClient(nil)
		} else {
			svc.SetWorkflowHTTPClient(runtime.workflow)
		}
	case contract.IntegrationKindMail:
		if !active || runtime.mail == nil {
			svc.SetMailSender(disabledMailSender{})
		} else {
			svc.SetMailSender(runtime.mail)
		}
	case contract.IntegrationKindIdentity:
		svc.runtimeMu.RLock()
		identity := svc.identityService
		svc.runtimeMu.RUnlock()
		if identity != nil {
			if !active || runtime.identity == nil {
				identity.SetFederationRuntime(IdentityRuntimeConfiguration{})
			} else {
				identity.SetFederationRuntime(*runtime.identity)
			}
		}
	}
}

func (svc *Service) BindIdentityService(identity *IdentityService) {
	svc.runtimeMu.Lock()
	defer svc.runtimeMu.Unlock()
	svc.identityService = identity
}

func (svc *Service) MappingRuntime() (*MappingService, error) {
	svc.runtimeMu.RLock()
	defer svc.runtimeMu.RUnlock()
	return svc.mappingService, svc.mappingConfig
}

func (svc *Service) ReloadIntegrations(ctx context.Context) error {
	svc.integrationMu.Lock()
	defer svc.integrationMu.Unlock()
	for _, descriptor := range integrationDescriptors {
		revision, err := svc.integrationRevision(ctx, descriptor.ID)
		if err != nil {
			return err
		}
		payload := integrationConnectionPayload{}
		decodeConfigurationPayload(revision.Payload, &payload)
		secrets, err := svc.openIntegrationSecrets(descriptor.ID, payload.SealedSecret)
		if err != nil {
			return err
		}
		runtime, err := prepareIntegrationRuntime(payload.Kind, payload.Active, payload.Configuration, secrets)
		if err != nil {
			return fmt.Errorf("prepare %s integration: %w", descriptor.ID, err)
		}
		svc.applyIntegrationRuntime(payload.Kind, payload.Active, runtime)
	}
	return nil
}

func knownIntegrationID(integrationID string) bool {
	for _, descriptor := range integrationDescriptors {
		if descriptor.ID == integrationID {
			return true
		}
	}
	return false
}

func integrationMatches(item *contract.IntegrationConnection, query IntegrationListQuery) bool {
	if query.Active != nil && item.Active != *query.Active {
		return false
	}
	if len(query.Kinds) == 0 {
		return true
	}
	for _, kind := range query.Kinds {
		if item.Kind == kind {
			return true
		}
	}
	return false
}

func normalizeIntegrationSort(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return []string{integrationListSort}, nil
	}
	parts := strings.Split(raw, ",")
	if len(parts) > 3 {
		return nil, validationError(contract.FieldError{Field: "/query/sort", Code: contract.ValidationTooManyItems})
	}
	allowed := map[string]bool{"integration_id": true, "kind": true, "active": true, "updated_at": true}
	seen := map[string]bool{}
	sorts := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		field := strings.TrimPrefix(part, "-")
		if part == "" || !allowed[field] || seen[field] {
			return nil, validationError(contract.FieldError{Field: "/query/sort", Code: contract.ValidationInvalidValue})
		}
		seen[field] = true
		sorts = append(sorts, part)
	}
	return sorts, nil
}

func integrationLess(left, right contract.IntegrationConnection, sorts []string) bool {
	for _, binding := range sorts {
		descending := strings.HasPrefix(binding, "-")
		field := strings.TrimPrefix(binding, "-")
		comparison := 0
		switch field {
		case "integration_id":
			comparison = strings.Compare(left.IntegrationID, right.IntegrationID)
		case "kind":
			comparison = strings.Compare(string(left.Kind), string(right.Kind))
		case "active":
			if left.Active != right.Active {
				if left.Active {
					comparison = 1
				} else {
					comparison = -1
				}
			}
		case "updated_at":
			if left.UpdatedAt.Before(right.UpdatedAt) {
				comparison = -1
			} else if left.UpdatedAt.After(right.UpdatedAt) {
				comparison = 1
			}
		}
		if comparison != 0 {
			return (comparison < 0) != descending
		}
	}
	return left.IntegrationID < right.IntegrationID
}

func cloneAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	encoded, _ := json.Marshal(input)
	output := map[string]any{}
	_ = json.Unmarshal(encoded, &output)
	return output
}
