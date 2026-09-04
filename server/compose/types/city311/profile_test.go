package city311

import (
	"encoding/json"
	"testing"
)

func TestProfileUpdateRejectsNonContractInput(t *testing.T) {
	for _, raw := range []string{
		`null`, `[]`, `"text"`, `{"display_name":null}`, `{"phone_numbers":null}`,
		`{"addresses":[null]}`, `{"addresses":[{"primary":null}]}`,
		`{"phone_numbers":[{"label":"HOME","value":null}]}`,
		`{"emails":["other@example.invalid"]}`, `{"login_identifier":"other"}`,
		`{"email_opt_out":true}`, `{"constituent_id":"C-other"}`,
		`{"addresses":[{"hidden":true}]}`, `{"preferred_language":1}`,
	} {
		t.Run(raw, func(t *testing.T) {
			var input ProfileUpdate
			if err := json.Unmarshal([]byte(raw), &input); err == nil {
				t.Fatal("expected malformed or non-editable field rejection")
			}
		})
	}
}

func TestProfileUpdateDistinguishesOmissionAndClearing(t *testing.T) {
	var input ProfileUpdate
	if err := json.Unmarshal([]byte(`{}`), &input); err != nil {
		t.Fatal(err)
	}
	if input.PhoneNumbers != nil || input.Addresses != nil || input.PreferredLanguage != nil {
		t.Fatal("omission must preserve existing profile values")
	}
	if err := json.Unmarshal([]byte(`{"phone_numbers":[],"addresses":[]}`), &input); err != nil {
		t.Fatal(err)
	}
	if input.PhoneNumbers == nil || input.Addresses == nil || *input.PhoneNumbers == nil || *input.Addresses == nil {
		t.Fatal("empty arrays must explicitly clear collections, not mean omitted or null")
	}
	raw, err := json.Marshal(input)
	if err != nil || string(raw) != `{"phone_numbers":[],"addresses":[]}` {
		t.Fatalf("PATCH DTO drift: %s, %v", raw, err)
	}
}

func TestProfileVersionAndCookieHandoff(t *testing.T) {
	contract := NewContractDocument()
	for _, name := range []string{"profile_get", "profile_update"} {
		if contract.Endpoints[name].ResponseHeaders["ETag"] == "" {
			t.Fatal("missing profile ETag handoff")
		}
	}
	document := NewOpenAPIDocument()
	security := document["components"].(map[string]interface{})["securitySchemes"].(map[string]interface{})["sessionCookie"].(map[string]interface{})
	if security["name"] != SessionCookieName {
		t.Fatal("OpenAPI cookie differs from runtime contract")
	}
	path := document["paths"].(map[string]interface{})["/api/v1/account/profile"].(map[string]interface{})
	for _, method := range []string{"get", "patch"} {
		response := path[method].(map[string]interface{})["responses"].(map[string]interface{})["200"].(map[string]interface{})
		if response["headers"].(map[string]interface{})["ETag"] == nil {
			t.Fatal("missing OpenAPI profile ETag")
		}
	}
}
