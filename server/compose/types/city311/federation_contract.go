package city311

func federationMocks() map[string]MockContract {
	return map[string]MockContract{
		"federated_oidc_start_public": federationResponse("federated_sign_in_start", 200, map[string]interface{}{
			"authorization_url": "https://identity.example.test/authorize?client_id=city311-public&state=returned-state",
		}),
		"federated_start_unknown_provider": federationResponse("federated_sign_in_start", 404, APIError{
			Error: ErrorNotFound, Message: "The identity provider was not found.", Retryable: false,
		}),
		"federated_start_invalid_client": federationResponse("federated_sign_in_start", 422, APIError{
			Error: ErrorValidation, Message: "The request contains invalid fields.", Retryable: false,
			Errors: []FieldError{{Field: "/query/client", Code: ValidationInvalidValue}},
		}),
		"federated_start_unavailable": federationResponse("federated_sign_in_start", 503, APIError{
			Error: ErrorTemporarilyUnavailable, Message: "Federated identity is temporarily unavailable.", Retryable: true,
		}),
		"federated_oidc_callback_authenticated": federationResponse("federated_sign_in_callback", 200, federatedSessionMock()),
		"federated_oidc_callback_rejected": federationResponse("federated_sign_in_callback", 401, APIError{
			Error: ErrorUnauthenticated, Message: "Federated authentication failed. Please return to sign in and try again.", Retryable: false,
		}),
		"federated_oidc_callback_unavailable": federationResponse("federated_sign_in_callback", 503, APIError{
			Error: ErrorTemporarilyUnavailable, Message: "Federated identity is temporarily unavailable.", Retryable: true,
		}),
		"federated_saml_callback_request": {
			Endpoint: "federated_saml_callback", Role: "request",
			Body: map[string]interface{}{"RelayState": "returned-state", "SAMLResponse": "signed-assertion"},
		},
		"federated_saml_callback_authenticated": federationResponse("federated_saml_callback", 200, federatedSessionMock()),
		"federated_saml_callback_rejected": federationResponse("federated_saml_callback", 401, APIError{
			Error: ErrorUnauthenticated, Message: "Federated authentication failed. Please return to sign in and try again.", Retryable: false,
		}),
		"federated_saml_callback_unavailable": federationResponse("federated_saml_callback", 503, APIError{
			Error: ErrorTemporarilyUnavailable, Message: "Federated identity is temporarily unavailable.", Retryable: true,
		}),
	}
}

func federationResponse(endpoint string, status int, body interface{}) MockContract {
	return MockContract{Endpoint: endpoint, Role: "response", HTTPStatus: status, Body: body}
}

func federatedSessionMock() map[string]interface{} {
	return map[string]interface{}{
		"authenticated": true,
		"actor": map[string]interface{}{
			"actor_id": "staff-0042", "display_name": "Federated Staff", "application_roles": []string{"service_agent"},
			"department_codes": []string{"STREETS"}, "district_codes": []string{"NORTH"}, "scopes": []string{},
			"capabilities": []string{"staff_request_queue"}, "available_routes": []string{"staff_request_queue"},
		},
		"preferred_language": "EN", "expires_at": "2026-08-25T20:00:00Z",
	}
}
