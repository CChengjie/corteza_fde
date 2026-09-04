package city311

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/crewjam/saml"
	dsig "github.com/russellhaering/goxmldsig"
	"github.com/stretchr/testify/require"
	jose "gopkg.in/square/go-jose.v2"
)

func TestRuntimeOIDCProviderUsesDiscoveryPKCEAndVerifiedClaims(t *testing.T) {
	now := time.Date(2026, 2, 3, 15, 4, 5, 0, time.UTC)
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	publicKey := jose.JSONWebKey{Key: &privateKey.PublicKey, KeyID: "fixture-key", Algorithm: string(jose.RS256), Use: "sig"}
	var issuer string
	var exchangedVerifier string
	var tokenNonce string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			writeProviderJSON(t, w, map[string]any{
				"issuer": issuer, "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token",
				"jwks_uri": issuer + "/keys", "scopes_supported": []string{"openid", "profile", "email"},
			})
		case "/keys":
			writeProviderJSON(t, w, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{publicKey}})
		case "/token":
			require.NoError(t, r.ParseForm())
			exchangedVerifier = r.Form.Get("code_verifier")
			require.Equal(t, "fixture-code", r.Form.Get("code"))
			require.Equal(t, "https://city311.example.test/api/v1/auth/oidc/callback", r.Form.Get("redirect_uri"))
			signed := signedOIDCToken(t, privateKey, issuer, "city311-public", tokenNonce, now)
			writeProviderJSON(t, w, map[string]any{"access_token": "access", "token_type": "Bearer", "expires_in": 60, "id_token": signed})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	issuer = server.URL
	runtime := IdentityRuntimeConfiguration{
		BaseURL: "https://city311.example.test", OIDCIssuerURL: issuer,
		OIDCStaffClientID: "city311-staff", OIDCPublicClientID: "city311-public", OIDCClientSecret: "client-secret",
	}
	provider := NewRuntimeFederationProvider(runtime, server.Client(), func() time.Time { return now })
	start := FederationStartRequest{
		Provider: federatedProviderOIDC, Client: federatedClientPublic, State: "state-value", Nonce: "nonce-value",
		PKCEVerifier: strings.Repeat("v", 43), CallbackURL: "https://city311.example.test/api/v1/auth/oidc/callback",
	}
	tokenNonce = start.Nonce
	authorization, err := provider.Start(context.Background(), start)
	require.NoError(t, err)
	parsed, err := url.Parse(authorization.URL)
	require.NoError(t, err)
	require.Equal(t, issuer+"/authorize", parsed.Scheme+"://"+parsed.Host+parsed.Path)
	require.Equal(t, start.State, parsed.Query().Get("state"))
	require.Equal(t, start.Nonce, parsed.Query().Get("nonce"))
	require.Equal(t, "code", parsed.Query().Get("response_type"))
	require.ElementsMatch(t, []string{"openid", "profile", "email"}, strings.Fields(parsed.Query().Get("scope")))
	require.Equal(t, "S256", parsed.Query().Get("code_challenge_method"))
	digest := sha256.Sum256([]byte(start.PKCEVerifier))
	require.Equal(t, base64.RawURLEncoding.EncodeToString(digest[:]), parsed.Query().Get("code_challenge"))

	claims, err := provider.Callback(context.Background(), FederationCallbackRequest{
		Provider: start.Provider, Client: start.Client, Code: "fixture-code", Nonce: start.Nonce,
		PKCEVerifier: start.PKCEVerifier, CallbackURL: start.CallbackURL,
	})
	require.NoError(t, err)
	require.Equal(t, start.PKCEVerifier, exchangedVerifier)
	require.Equal(t, "fixture-subject", claims.Subject)
	require.Equal(t, "oidc@example.invalid", claims.Email)
	require.True(t, claims.EmailVerified)
	require.Equal(t, "constituent", claims.ActorType)
	require.NotNil(t, claims.DepartmentCodes)
	require.NotNil(t, claims.DistrictCodes)
	require.NotNil(t, claims.Roles)

	_, err = provider.Callback(context.Background(), FederationCallbackRequest{
		Provider: start.Provider, Client: start.Client, Code: "fixture-code", Nonce: "wrong-nonce",
		PKCEVerifier: start.PKCEVerifier, CallbackURL: start.CallbackURL,
	})
	require.ErrorContains(t, err, "nonce does not match")
}

func signedOIDCToken(t *testing.T, key *rsa.PrivateKey, issuer, audience, nonce string, now time.Time) string {
	t.Helper()
	options := (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "fixture-key")
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, options)
	require.NoError(t, err)
	payload, err := json.Marshal(map[string]any{
		"iss": issuer, "sub": "fixture-subject", "aud": audience, "exp": now.Add(time.Minute).Unix(), "iat": now.Unix(), "nonce": nonce,
		"email": "oidc@example.invalid", "email_verified": true, "name": "OIDC Resident", "actor_type": "constituent",
		"department_codes": []string{}, "district_codes": []string{}, "roles": []string{},
	})
	require.NoError(t, err)
	signed, err := signer.Sign(payload)
	require.NoError(t, err)
	compact, err := signed.CompactSerialize()
	require.NoError(t, err)
	return compact
}

func writeProviderJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	require.NoError(t, json.NewEncoder(w).Encode(value))
}

type staticServiceProviderMetadata struct{ metadata *saml.EntityDescriptor }

func (provider staticServiceProviderMetadata) GetServiceProvider(_ *http.Request, _ string) (*saml.EntityDescriptor, error) {
	return provider.metadata, nil
}

func TestRuntimeSAMLProviderBuildsTrackedRequestAndValidatesSignedClaims(t *testing.T) {
	now := time.Now().UTC()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "City 311 test identity provider"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)
	certificate, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	var fixtureURL string
	var identityProvider *saml.IdentityProvider
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metadata" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/samlmetadata+xml")
		require.NoError(t, xml.NewEncoder(w).Encode(identityProvider.Metadata()))
	}))
	t.Cleanup(server.Close)
	fixtureURL = server.URL
	metadataURL, err := url.Parse(fixtureURL + "/metadata")
	require.NoError(t, err)
	ssoURL, err := url.Parse(fixtureURL + "/sso")
	require.NoError(t, err)
	callbackURL, err := url.Parse("https://city311.example.test/api/v1/auth/saml/callback")
	require.NoError(t, err)
	identityProvider = &saml.IdentityProvider{
		Key: privateKey, Certificate: certificate, MetadataURL: *metadataURL, SSOURL: *ssoURL,
		SignatureMethod: dsig.RSASHA256SignatureMethod,
	}
	serviceProvider := &saml.ServiceProvider{
		EntityID: "https://city311.example.test/saml", AcsURL: *callbackURL, AuthnNameIDFormat: saml.PersistentNameIDFormat,
	}
	identityProvider.ServiceProviderProvider = staticServiceProviderMetadata{metadata: serviceProvider.Metadata()}
	runtime := IdentityRuntimeConfiguration{
		BaseURL: "https://city311.example.test", SAMLMetadataURL: fixtureURL + "/metadata",
		SAMLServiceProvider: "https://city311.example.test/saml",
	}
	provider := NewRuntimeFederationProvider(runtime, server.Client(), time.Now)
	start := FederationStartRequest{
		Provider: federatedProviderSAML, Client: federatedClientStaff, State: "relay-state",
		CallbackURL: "https://city311.example.test/api/v1/auth/saml/callback",
	}
	authorization, err := provider.Start(context.Background(), start)
	require.NoError(t, err)
	require.NotEmpty(t, authorization.RequestID)
	redirect, err := url.Parse(authorization.URL)
	require.NoError(t, err)
	require.Equal(t, fixtureURL+"/sso", redirect.Scheme+"://"+redirect.Host+redirect.Path)
	require.Equal(t, start.State, redirect.Query().Get("RelayState"))
	require.NotEmpty(t, redirect.Query().Get("SAMLRequest"))

	authenticationRequest := httptest.NewRequest(http.MethodGet, authorization.URL, nil)
	idpRequest, err := saml.NewIdpAuthnRequest(identityProvider, authenticationRequest)
	require.NoError(t, err)
	require.NoError(t, idpRequest.Validate())
	session := &saml.Session{
		ID: "session-1", CreateTime: now, ExpireTime: now.Add(time.Hour), Index: "index-1",
		NameID: "persistent-staff-subject", NameIDFormat: string(saml.PersistentNameIDFormat), SubjectID: "persistent-staff-subject",
		CustomAttributes: []saml.Attribute{
			samlStringAttribute("email", "saml@example.invalid"),
			samlStringAttribute("displayName", "SAML Staff"),
			samlStringAttribute("departmentCodes", "STREETS"),
			samlStringAttribute("districtCodes", "NORTH", "CENTRAL"),
			samlStringAttribute("roles", "service_agent", "supervisor"),
		},
	}
	require.NoError(t, (saml.DefaultAssertionMaker{}).MakeAssertion(idpRequest, session))
	response, err := idpRequest.PostBinding()
	require.NoError(t, err)
	claims, err := provider.Callback(context.Background(), FederationCallbackRequest{
		Provider: federatedProviderSAML, Client: federatedClientStaff, CallbackURL: start.CallbackURL,
		SAMLRequestID: authorization.RequestID, SAMLResponse: response.SAMLResponse,
	})
	require.NoError(t, err)
	require.Equal(t, session.NameID, claims.Subject)
	require.Equal(t, "saml@example.invalid", claims.Email)
	require.Equal(t, "SAML Staff", claims.DisplayName)
	require.ElementsMatch(t, []string{"NORTH", "CENTRAL"}, claims.DistrictCodes)
	require.ElementsMatch(t, []string{"service_agent", "supervisor"}, claims.Roles)

	_, err = provider.Callback(context.Background(), FederationCallbackRequest{
		Provider: federatedProviderSAML, Client: federatedClientStaff, CallbackURL: start.CallbackURL,
		SAMLRequestID: authorization.RequestID, SAMLResponse: base64.StdEncoding.EncodeToString([]byte("<unsigned/>")),
	})
	require.ErrorContains(t, err, "verify SAML assertion")
}

func samlStringAttribute(name string, values ...string) saml.Attribute {
	attribute := saml.Attribute{FriendlyName: name, Name: name, NameFormat: "urn:oasis:names:tc:SAML:2.0:attrname-format:basic"}
	for _, value := range values {
		attribute.Values = append(attribute.Values, saml.AttributeValue{Type: "xs:string", Value: value})
	}
	return attribute
}

func TestRuntimeFederationConfigurationAndUnsupportedProviders(t *testing.T) {
	setFederatedIdentityEnvironment(t)
	runtime := IdentityRuntimeFromEnvironment()
	require.Equal(t, "https://city311.example.test", runtime.BaseURL)
	require.Equal(t, "https://identity.example.test", runtime.OIDCIssuerURL)
	require.Equal(t, "secret", runtime.OIDCClientSecret)

	provider := NewRuntimeFederationProvider(IdentityRuntimeConfiguration{}, nil, nil)
	_, err := provider.Start(context.Background(), FederationStartRequest{Provider: federatedProviderOIDC})
	var unavailable *federationUnavailableError
	require.ErrorAs(t, err, &unavailable)
	_, err = provider.Start(context.Background(), FederationStartRequest{Provider: federatedProviderSAML})
	require.ErrorAs(t, err, &unavailable)
	_, err = provider.Start(context.Background(), FederationStartRequest{Provider: "unknown"})
	require.ErrorContains(t, err, "unsupported")
	_, err = provider.Callback(context.Background(), FederationCallbackRequest{Provider: "unknown"})
	require.ErrorContains(t, err, "unsupported")
	require.Equal(t, "value", firstAttribute(map[string][]string{"claim": {"value"}}, "claim"))
	require.Empty(t, firstAttribute(map[string][]string{}, "missing"))
}

func setFederatedIdentityEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("APP_BASE_URL", "https://city311.example.test/")
	t.Setenv("OIDC_ISSUER_URL", "https://identity.example.test/")
	t.Setenv("OIDC_STAFF_CLIENT_ID", "staff")
	t.Setenv("OIDC_PUBLIC_CLIENT_ID", "public")
	t.Setenv("OIDC_CLIENT_SECRET", "secret")
	t.Setenv("SAML_METADATA_URL", "https://identity.example.test/metadata")
	t.Setenv("SAML_SP_ENTITY_ID", "https://city311.example.test/saml")
}
