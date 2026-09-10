package city311

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFederationContractPublishesRuntimeInputsAndOutcomes(t *testing.T) {
	document := NewContractDocument()

	start := document.Endpoints["federated_sign_in_start"]
	require.Equal(t, map[string]interface{}{
		"type": "string", "enum": []string{"staff", "public"}, "default": "staff", "example": "public",
		"description": "Selects the OIDC relying-party client. SAML accepts only staff.",
	}, start.QueryParameters["client"])
	require.Equal(t, http.StatusNotFound, start.ErrorStatuses[string(ErrorNotFound)])
	require.Equal(t, http.StatusUnprocessableEntity, start.ErrorStatuses[string(ErrorValidation)])
	require.Equal(t, http.StatusServiceUnavailable, start.ErrorStatuses[string(ErrorTemporarilyUnavailable)])

	oidc := document.Endpoints["federated_sign_in_callback"]
	require.Equal(t, http.MethodGet, oidc.Method)
	require.Equal(t, "/api/v1/auth/oidc/callback", oidc.Path)
	require.Contains(t, oidc.QueryParameters, "state")
	require.Contains(t, oidc.QueryParameters, "code")
	require.Contains(t, oidc.QueryParameters, "error")
	require.Equal(t, http.StatusUnauthorized, oidc.ErrorStatuses[string(ErrorUnauthenticated)])
	require.Equal(t, http.StatusServiceUnavailable, oidc.ErrorStatuses[string(ErrorTemporarilyUnavailable)])

	saml := document.Endpoints["federated_saml_callback"]
	require.Equal(t, http.MethodPost, saml.Method)
	require.Equal(t, "/api/v1/auth/saml/callback", saml.Path)
	require.Equal(t, "federated_saml_callback", saml.RequestSchema)
	require.Equal(t, "application/x-www-form-urlencoded", saml.RequestMediaType)
	require.Equal(t, []string{"RelayState", "SAMLResponse"}, document.Schemas[saml.RequestSchema]["required"])

	for _, name := range []string{
		"federated_oidc_start_public", "federated_start_unknown_provider", "federated_start_invalid_client",
		"federated_start_unavailable", "federated_oidc_callback_authenticated", "federated_oidc_callback_rejected",
		"federated_oidc_callback_unavailable", "federated_saml_callback_request", "federated_saml_callback_authenticated",
		"federated_saml_callback_rejected", "federated_saml_callback_unavailable",
	} {
		require.Contains(t, document.Mocks, name)
	}
	for _, name := range []string{"federated_oidc_callback_authenticated", "federated_saml_callback_authenticated"} {
		body := document.Mocks[name].Body.(map[string]interface{})
		require.Equal(t, true, body["authenticated"])
	}
}

func TestOpenAPIFederationCallbacksPublishTheirTransports(t *testing.T) {
	document := NewOpenAPIDocument()
	paths := document["paths"].(map[string]interface{})

	oidc := paths["/api/v1/auth/oidc/callback"].(map[string]interface{})["get"].(map[string]interface{})
	require.Len(t, oidc["parameters"], 3)
	require.NotContains(t, oidc, "requestBody")

	saml := paths["/api/v1/auth/saml/callback"].(map[string]interface{})["post"].(map[string]interface{})
	body := saml["requestBody"].(map[string]interface{})
	content := body["content"].(map[string]interface{})
	require.Contains(t, content, "application/x-www-form-urlencoded")
	success := saml["responses"].(map[string]interface{})["200"].(map[string]interface{})
	example := success["x-city311-example"].(map[string]interface{})["body"].(map[string]interface{})
	require.Equal(t, true, example["authenticated"])
}
