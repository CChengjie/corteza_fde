package city311

func completeEmailReplacementContract(document *ContractDocument) {
	addProvisions(document, "9.1.2")
	document.Decisions = append(document.Decisions, ContractDecision{
		ID:         "VERIFIED-EMAIL-REPLACEMENT",
		Provisions: []string{"9.1.2"},
		Decision:   "a constituent requests a replacement address while authenticated; the current verified login email remains unchanged until a one-time token delivered to the replacement address is confirmed; the confirmation endpoint is public because possession of that token is the proof of control",
	})

	request := document.Endpoints["email_replacement_request"]
	request.SuccessStatuses = map[string]int{"accepted": 202, "unavailable_address_indistinguishable": 202}
	request.ErrorStatuses = map[string]int{
		string(ErrorUnauthenticated):        401,
		string(ErrorForbidden):              403,
		string(ErrorValidation):             422,
		string(ErrorTemporarilyUnavailable): 503,
	}
	request.PrivacyRule = "every syntactically valid replacement address receives the same 202 acknowledgement; an address already used as a verified login email is not disclosed and no token is issued"
	document.Endpoints["email_replacement_request"] = request

	confirm := document.Endpoints["email_replacement_confirm"]
	confirm.ErrorStatuses = map[string]int{
		string(ErrorInvalidEmailVerificationToken): 422,
		string(ErrorExpiredEmailVerificationToken): 422,
		string(ErrorValidation):                    422,
		string(ErrorTemporarilyUnavailable):        503,
	}
	document.Endpoints["email_replacement_confirm"] = confirm

	properties := document.Schemas["constituent"]["properties"].(map[string]interface{})
	emails := properties["emails"].(map[string]interface{})
	emails["replacement_operation"] = "email_replacement_request followed by email_replacement_confirm"
	emails["current_value_retained_until_verification"] = true

	document.Protocol["verified_email_replacement"] = map[string]interface{}{
		"request_endpoint":                      "email_replacement_request",
		"confirmation_endpoint":                 "email_replacement_confirm",
		"verification_lifetime_minutes":         30,
		"raw_token_persisted":                   false,
		"latest_request_invalidates_older":      true,
		"current_email_retained_before_confirm": true,
		"confirmation_is_single_use":            true,
		"idempotency_key_required":              false,
		"if_match_required":                     false,
	}

	document.Mocks["email_replacement_requested"] = MockContract{
		Endpoint: "email_replacement_request", Role: "response", HTTPStatus: 202,
		Body: EmailReplacementAcknowledgement{Accepted: true},
	}
	document.Mocks["email_replacement_request_body"] = MockContract{
		Endpoint: "email_replacement_request", Role: "request",
		Body: EmailReplacementRequest{Email: "replacement@example.test"},
	}
	document.Mocks["email_replacement_confirm_body"] = MockContract{
		Endpoint: "email_replacement_confirm", Role: "request",
		Body: EmailReplacementConfirm{Token: "one-time-verification-token"},
	}
	document.Mocks["email_replacement_confirmed"] = MockContract{
		Endpoint: "email_replacement_confirm", Role: "response", HTTPStatus: 200,
		Body: EmailReplacementResult{VerifiedEmail: "replacement@example.test"},
	}
	document.Mocks["invalid_email_verification_token"] = MockContract{
		Endpoint: "email_replacement_confirm", Role: "response", HTTPStatus: 422,
		Body: APIError{Error: ErrorInvalidEmailVerificationToken, Message: "The email verification token is invalid."},
	}
	document.Mocks["expired_email_verification_token"] = MockContract{
		Endpoint: "email_replacement_confirm", Role: "response", HTTPStatus: 422,
		Body: APIError{Error: ErrorExpiredEmailVerificationToken, Message: "The email verification token has expired."},
	}
}
