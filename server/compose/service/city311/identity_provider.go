package city311

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/crewjam/saml"
	"github.com/crewjam/saml/samlsp"
	oidc "github.com/crusttech/go-oidc"
	"golang.org/x/oauth2"
)

const (
	federatedProviderOIDC = "oidc"
	federatedProviderSAML = "saml"
	federatedClientStaff  = "staff"
	federatedClientPublic = "public"
)

// IdentityRuntimeConfiguration is the effective identity-provider
// configuration. Secrets are kept here for protocol operations and are never
// copied into an API response or persisted configuration revision.
type IdentityRuntimeConfiguration struct {
	BaseURL             string
	OIDCIssuerURL       string
	OIDCStaffClientID   string
	OIDCPublicClientID  string
	OIDCClientSecret    string
	SAMLMetadataURL     string
	SAMLServiceProvider string
}

func IdentityRuntimeFromEnvironment() IdentityRuntimeConfiguration {
	return IdentityRuntimeConfiguration{
		BaseURL:             strings.TrimRight(strings.TrimSpace(os.Getenv("APP_BASE_URL")), "/"),
		OIDCIssuerURL:       strings.TrimRight(strings.TrimSpace(os.Getenv("OIDC_ISSUER_URL")), "/"),
		OIDCStaffClientID:   strings.TrimSpace(os.Getenv("OIDC_STAFF_CLIENT_ID")),
		OIDCPublicClientID:  strings.TrimSpace(os.Getenv("OIDC_PUBLIC_CLIENT_ID")),
		OIDCClientSecret:    strings.TrimSpace(os.Getenv("OIDC_CLIENT_SECRET")),
		SAMLMetadataURL:     strings.TrimSpace(os.Getenv("SAML_METADATA_URL")),
		SAMLServiceProvider: strings.TrimSpace(os.Getenv("SAML_SP_ENTITY_ID")),
	}
}

type FederationStartRequest struct {
	Provider     string
	Client       string
	State        string
	Nonce        string
	PKCEVerifier string
	CallbackURL  string
}

type FederationAuthorization struct {
	URL       string
	RequestID string
}

type FederationCallbackRequest struct {
	Provider      string
	Client        string
	Code          string
	Nonce         string
	PKCEVerifier  string
	CallbackURL   string
	SAMLResponse  string
	SAMLRequestID string
}

type FederatedClaims struct {
	Subject         string
	Email           string
	EmailVerified   bool
	DisplayName     string
	ActorType       string
	DepartmentCodes []string
	DistrictCodes   []string
	Roles           []string
}

type FederationProvider interface {
	Start(context.Context, FederationStartRequest) (FederationAuthorization, error)
	Callback(context.Context, FederationCallbackRequest) (FederatedClaims, error)
}

type federationUnavailableError struct{ cause error }

func (err *federationUnavailableError) Error() string {
	return "identity provider is temporarily unavailable"
}
func (err *federationUnavailableError) Unwrap() error { return err.cause }

type runtimeFederationProvider struct {
	runtime IdentityRuntimeConfiguration
	client  *http.Client
	now     func() time.Time
}

func NewRuntimeFederationProvider(runtime IdentityRuntimeConfiguration, client *http.Client, now func() time.Time) FederationProvider {
	if client == nil {
		client = http.DefaultClient
	}
	if now == nil {
		now = time.Now
	}
	return &runtimeFederationProvider{runtime: runtime, client: client, now: now}
}

func (provider *runtimeFederationProvider) Start(ctx context.Context, input FederationStartRequest) (FederationAuthorization, error) {
	switch input.Provider {
	case federatedProviderOIDC:
		clientID := provider.oidcClientID(input.Client)
		if provider.runtime.OIDCIssuerURL == "" || clientID == "" || provider.runtime.OIDCClientSecret == "" {
			return FederationAuthorization{}, &federationUnavailableError{cause: fmt.Errorf("OIDC runtime configuration is incomplete")}
		}
		ctx = oidc.ClientContext(ctx, provider.client)
		discovery, err := oidc.NewProvider(ctx, provider.runtime.OIDCIssuerURL)
		if err != nil {
			return FederationAuthorization{}, &federationUnavailableError{cause: err}
		}
		configuration := oauth2.Config{
			ClientID: clientID, ClientSecret: provider.runtime.OIDCClientSecret,
			Endpoint: discovery.Endpoint(), RedirectURL: input.CallbackURL,
			Scopes: []string{oidc.ScopeOpenID, "profile", "email"},
		}
		return FederationAuthorization{URL: configuration.AuthCodeURL(
			input.State, oidc.Nonce(input.Nonce), oauth2.S256ChallengeOption(input.PKCEVerifier),
		)}, nil
	case federatedProviderSAML:
		serviceProvider, err := provider.samlServiceProvider(ctx, input.CallbackURL)
		if err != nil {
			return FederationAuthorization{}, &federationUnavailableError{cause: err}
		}
		request, err := serviceProvider.MakeAuthenticationRequest(
			serviceProvider.GetSSOBindingLocation(saml.HTTPRedirectBinding), saml.HTTPRedirectBinding, saml.HTTPPostBinding,
		)
		if err != nil {
			return FederationAuthorization{}, &federationUnavailableError{cause: err}
		}
		redirect, err := request.Redirect(input.State, serviceProvider)
		if err != nil {
			return FederationAuthorization{}, &federationUnavailableError{cause: err}
		}
		return FederationAuthorization{URL: redirect.String(), RequestID: request.ID}, nil
	default:
		return FederationAuthorization{}, fmt.Errorf("unsupported identity provider")
	}
}

func (provider *runtimeFederationProvider) Callback(ctx context.Context, input FederationCallbackRequest) (FederatedClaims, error) {
	switch input.Provider {
	case federatedProviderOIDC:
		return provider.oidcCallback(ctx, input)
	case federatedProviderSAML:
		return provider.samlCallback(ctx, input)
	default:
		return FederatedClaims{}, fmt.Errorf("unsupported identity provider")
	}
}

func (provider *runtimeFederationProvider) oidcCallback(ctx context.Context, input FederationCallbackRequest) (FederatedClaims, error) {
	clientID := provider.oidcClientID(input.Client)
	if provider.runtime.OIDCIssuerURL == "" || clientID == "" || provider.runtime.OIDCClientSecret == "" {
		return FederatedClaims{}, &federationUnavailableError{cause: fmt.Errorf("OIDC runtime configuration is incomplete")}
	}
	ctx = oidc.ClientContext(ctx, provider.client)
	discovery, err := oidc.NewProvider(ctx, provider.runtime.OIDCIssuerURL)
	if err != nil {
		return FederatedClaims{}, &federationUnavailableError{cause: err}
	}
	configuration := oauth2.Config{
		ClientID: clientID, ClientSecret: provider.runtime.OIDCClientSecret,
		Endpoint: discovery.Endpoint(), RedirectURL: input.CallbackURL,
		Scopes: []string{oidc.ScopeOpenID, "profile", "email"},
	}
	token, err := configuration.Exchange(ctx, input.Code, oauth2.VerifierOption(input.PKCEVerifier))
	if err != nil {
		return FederatedClaims{}, fmt.Errorf("exchange authorization code: %w", err)
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || strings.TrimSpace(rawIDToken) == "" {
		return FederatedClaims{}, fmt.Errorf("identity provider omitted id_token")
	}
	verified, err := discovery.Verifier(&oidc.Config{ClientID: clientID, Now: provider.now}).Verify(ctx, rawIDToken)
	if err != nil {
		return FederatedClaims{}, fmt.Errorf("verify id_token: %w", err)
	}
	if verified.Nonce != input.Nonce {
		return FederatedClaims{}, fmt.Errorf("identity provider nonce does not match")
	}
	claims := struct {
		Email           *string   `json:"email"`
		EmailVerified   *bool     `json:"email_verified"`
		Name            *string   `json:"name"`
		ActorType       *string   `json:"actor_type"`
		DepartmentCodes *[]string `json:"department_codes"`
		DistrictCodes   *[]string `json:"district_codes"`
		Roles           *[]string `json:"roles"`
	}{}
	if err = verified.Claims(&claims); err != nil {
		return FederatedClaims{}, fmt.Errorf("decode id_token claims: %w", err)
	}
	if claims.Email == nil || claims.EmailVerified == nil || claims.Name == nil || claims.ActorType == nil ||
		claims.DepartmentCodes == nil || claims.DistrictCodes == nil || claims.Roles == nil {
		return FederatedClaims{}, fmt.Errorf("identity provider omitted required claims")
	}
	return FederatedClaims{
		Subject: verified.Subject, Email: *claims.Email, EmailVerified: *claims.EmailVerified,
		DisplayName: *claims.Name, ActorType: *claims.ActorType,
		DepartmentCodes: *claims.DepartmentCodes, DistrictCodes: *claims.DistrictCodes, Roles: *claims.Roles,
	}, nil
}

func (provider *runtimeFederationProvider) samlCallback(ctx context.Context, input FederationCallbackRequest) (FederatedClaims, error) {
	serviceProvider, err := provider.samlServiceProvider(ctx, input.CallbackURL)
	if err != nil {
		return FederatedClaims{}, &federationUnavailableError{cause: err}
	}
	form := url.Values{"SAMLResponse": []string{input.SAMLResponse}}
	request := &http.Request{Method: http.MethodPost, Header: make(http.Header), Form: form, PostForm: form}
	request = request.WithContext(ctx)
	assertion, err := serviceProvider.ParseResponse(request, []string{input.SAMLRequestID})
	if err != nil {
		return FederatedClaims{}, fmt.Errorf("verify SAML assertion: %w", err)
	}
	if assertion.Subject == nil || assertion.Subject.NameID == nil ||
		assertion.Subject.NameID.Format != string(saml.PersistentNameIDFormat) {
		return FederatedClaims{}, fmt.Errorf("SAML assertion requires a persistent NameID")
	}
	attributes := make(map[string][]string)
	for _, statement := range assertion.AttributeStatements {
		for _, attribute := range statement.Attributes {
			name := attribute.Name
			if attribute.FriendlyName != "" {
				name = attribute.FriendlyName
			}
			for _, value := range attribute.Values {
				attributes[name] = append(attributes[name], strings.TrimSpace(value.Value))
			}
		}
	}
	return FederatedClaims{
		Subject: assertion.Subject.NameID.Value, Email: firstAttribute(attributes, "email"), EmailVerified: true,
		DisplayName: firstAttribute(attributes, "displayName"), ActorType: federatedClientStaff,
		DepartmentCodes: attributes["departmentCodes"], DistrictCodes: attributes["districtCodes"], Roles: attributes["roles"],
	}, nil
}

func (provider *runtimeFederationProvider) samlServiceProvider(ctx context.Context, callbackURL string) (*saml.ServiceProvider, error) {
	metadataURL, err := url.Parse(provider.runtime.SAMLMetadataURL)
	if err != nil || metadataURL.Host == "" || provider.runtime.SAMLServiceProvider == "" {
		return nil, fmt.Errorf("SAML runtime configuration is incomplete")
	}
	callback, err := url.Parse(callbackURL)
	if err != nil || callback.Host == "" {
		return nil, fmt.Errorf("SAML callback URL is invalid")
	}
	metadata, err := samlsp.FetchMetadata(ctx, provider.client, *metadataURL)
	if err != nil {
		return nil, err
	}
	return &saml.ServiceProvider{
		EntityID: provider.runtime.SAMLServiceProvider, AcsURL: *callback, IDPMetadata: metadata,
		HTTPClient: provider.client, AuthnNameIDFormat: saml.PersistentNameIDFormat,
	}, nil
}

func (provider *runtimeFederationProvider) oidcClientID(client string) string {
	if client == federatedClientPublic {
		return provider.runtime.OIDCPublicClientID
	}
	return provider.runtime.OIDCStaffClientID
}

func firstAttribute(attributes map[string][]string, name string) string {
	if len(attributes[name]) == 0 {
		return ""
	}
	return attributes[name][0]
}
