package city311

import (
	"reflect"
	"testing"
)

func TestVerifiedEmailReplacementContractIsComplete(t *testing.T) {
	contract := NewContractDocument()
	if contract.ContractVersion != "2.1.0" {
		t.Fatalf("email replacement contract must be published as additive version 2.1.0, got %s", contract.ContractVersion)
	}
	if !contains(contract.Provisions, "9.1.2") {
		t.Fatal("contract does not identify original provision 9.1.2")
	}

	request := contract.Endpoints["email_replacement_request"]
	if request.Method != "POST" || request.Path != EmailReplacementRequestPath || request.Authentication.Mode != "session_cookie" || request.Authentication.ActorClass != "constituent" {
		t.Fatalf("replacement request endpoint drifted: %#v", request)
	}
	if request.RequiredCapability != "email_replacement_request" || request.SuccessStatuses["accepted"] != 202 || request.SuccessStatuses["unavailable_address_indistinguishable"] != 202 {
		t.Fatal("replacement request lacks its capability or privacy-preserving 202 outcomes")
	}
	if request.PrivacyRule == "" || request.ErrorStatuses[string(ErrorUnauthenticated)] != 401 || request.ErrorStatuses[string(ErrorForbidden)] != 403 || request.ErrorStatuses[string(ErrorValidation)] != 422 || request.ErrorStatuses[string(ErrorTemporarilyUnavailable)] != 503 {
		t.Fatal("replacement request has incomplete privacy or error behavior")
	}
	if contains(request.RequiredHeaders, IdempotencyHeader) || contains(request.RequiredHeaders, IfMatchHeader) {
		t.Fatal("token issuance must not publish idempotency or record-version headers")
	}

	confirm := contract.Endpoints["email_replacement_confirm"]
	if confirm.Method != "POST" || confirm.Path != EmailReplacementConfirmPath || confirm.Authentication.Mode != "none" || confirm.RequiredCapability != "" || confirm.SuccessStatuses["success"] != 200 {
		t.Fatalf("replacement confirmation endpoint drifted: %#v", confirm)
	}
	if confirm.ErrorStatuses[string(ErrorInvalidEmailVerificationToken)] != 422 || confirm.ErrorStatuses[string(ErrorExpiredEmailVerificationToken)] != 422 || confirm.ErrorStatuses[string(ErrorValidation)] != 422 || confirm.ErrorStatuses[string(ErrorTemporarilyUnavailable)] != 503 {
		t.Fatal("replacement confirmation has incomplete token error behavior")
	}

	protocol := contract.Protocol["verified_email_replacement"].(map[string]interface{})
	wantProtocol := map[string]interface{}{
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
	if !reflect.DeepEqual(protocol, wantProtocol) {
		t.Fatalf("replacement protocol drifted: %#v", protocol)
	}

	for _, schema := range []string{"email_replacement_request", "email_replacement_acknowledgement", "email_replacement_confirm", "email_replacement_result"} {
		if contract.Schemas[schema] == nil {
			t.Errorf("missing replacement schema %s", schema)
		}
	}
	for _, mock := range []string{
		"email_replacement_request_body", "email_replacement_requested", "email_replacement_confirm_body",
		"email_replacement_confirmed", "invalid_email_verification_token", "expired_email_verification_token",
	} {
		if _, present := contract.Mocks[mock]; !present {
			t.Errorf("missing replacement example %s", mock)
		}
	}
}

func TestVerifiedEmailReplacementOpenAPIHandoff(t *testing.T) {
	document := NewOpenAPIDocument()
	paths := document["paths"].(map[string]interface{})
	request := paths[EmailReplacementRequestPath].(map[string]interface{})["post"].(map[string]interface{})
	if request["operationId"] != "email_replacement_request" || request["x-city311-required-capability"] != "email_replacement_request" {
		t.Fatal("OpenAPI omits the authenticated replacement request handoff")
	}
	if !reflect.DeepEqual(request["security"], []map[string][]string{{"sessionCookie": {}}}) {
		t.Fatalf("replacement request must use the session cookie: %#v", request["security"])
	}
	requestResponses := request["responses"].(map[string]interface{})
	accepted := requestResponses["202"].(map[string]interface{})
	if accepted["x-city311-example"] == nil || request["x-city311-privacy-rule"] == nil {
		t.Fatal("OpenAPI omits the privacy-safe request response example")
	}

	confirm := paths[EmailReplacementConfirmPath].(map[string]interface{})["post"].(map[string]interface{})
	if confirm["operationId"] != "email_replacement_confirm" || !reflect.DeepEqual(confirm["security"], []map[string][]string{}) {
		t.Fatal("OpenAPI confirmation must be public and named for generated clients")
	}
	confirmResponses := confirm["responses"].(map[string]interface{})
	for _, status := range []string{"200", "422", "503"} {
		if confirmResponses[status] == nil {
			t.Errorf("OpenAPI confirmation omits %s response", status)
		}
	}
	invalidExamples := confirmResponses["422"].(map[string]interface{})["content"].(map[string]interface{})["application/json"].(map[string]interface{})["examples"].(map[string]interface{})
	for _, name := range []string{"invalid_email_verification_token", "expired_email_verification_token"} {
		if invalidExamples[name] == nil {
			t.Errorf("OpenAPI confirmation omits %s example", name)
		}
	}

	schemas := document["components"].(map[string]interface{})["schemas"].(map[string]interface{})
	token := schemas["email_replacement_confirm"].(map[string]interface{})["properties"].(map[string]interface{})["token"].(map[string]interface{})
	if token["writeOnly"] != true || token["minLength"] != 1 || token["maxLength"] != 512 {
		t.Fatalf("confirmation token schema is incomplete: %#v", token)
	}
	verifiedEmail := schemas["email_replacement_result"].(map[string]interface{})["properties"].(map[string]interface{})["verified_email"].(map[string]interface{})
	if verifiedEmail["format"] != "email" || verifiedEmail["maxLength"] != 254 {
		t.Fatalf("replacement result email schema is incomplete: %#v", verifiedEmail)
	}
}
