package city311

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestAttachmentContractRequiresBinarySafeRepresentation(t *testing.T) {
	document := NewContractDocument()
	if document.ContractVersion != "2.1.0" || document.Versioning.SupportedMajor != 2 {
		t.Fatal("the mandatory base64 body contract must identify supported major 2")
	}
	schema := document.Schemas["binary_attachment"]
	wantRequired := []string{"content_type", "content_disposition", "body", "body_encoding"}
	if !reflect.DeepEqual(schema["required"], wantRequired) {
		t.Fatal("every download must include the encoding discriminator")
	}
	properties := schema["properties"].(map[string]interface{})
	if properties["body"].(map[string]interface{})["contentEncoding"] != "base64" || !reflect.DeepEqual(properties["body_encoding"].(map[string]interface{})["enum"], []string{"base64"}) {
		t.Fatal("the body has exactly one supported representation: RFC 4648 base64")
	}
	for _, content := range [][]byte{[]byte("hello"), {0, 255, 128}} {
		encoded, err := json.Marshal(BinaryAttachment{ContentType: "text/plain", ContentDisposition: "attachment; filename=file.txt", Body: base64.StdEncoding.EncodeToString(content), BodyEncoding: "base64"})
		if err != nil {
			t.Fatal(err)
		}
		var wire map[string]interface{}
		if err = json.Unmarshal(encoded, &wire); err != nil || wire["body_encoding"] != "base64" {
			t.Fatalf("DTO omitted its required encoding: %s, %v", encoded, err)
		}
		decoded, err := base64.StdEncoding.DecodeString(wire["body"].(string))
		if err != nil || !reflect.DeepEqual(decoded, content) {
			t.Fatal("current consumer must reconstruct exact file bytes")
		}
	}
	example := normalizedJSON(document.Mocks["attachment_download_binary"].Body).(map[string]interface{})
	decoded, err := base64.StdEncoding.DecodeString(example["body"].(string))
	if err != nil || example["body_encoding"] != "base64" || !reflect.DeepEqual(decoded, []byte{0, 255, 128}) {
		t.Fatal("download mock must demonstrate arbitrary binary bytes")
	}
}

func TestContractSnapshotMatchesEntireGoDocument(t *testing.T) {
	actual := decodeJSON(t, mustReadContract(t))

	expectedRaw, err := json.Marshal(NewContractDocument())
	if err != nil {
		t.Fatal(err)
	}
	expected := decodeJSON(t, expectedRaw)

	if !reflect.DeepEqual(actual, expected) {
		t.Fatal("contract.json drifted from NewContractDocument; regenerate the complete handoff")
	}
}

func TestEveryMockStatusAndBodyMatchesContract(t *testing.T) {
	var actual ContractDocument
	if err := json.Unmarshal(mustReadContract(t), &actual); err != nil {
		t.Fatal(err)
	}
	expected := NewContractDocument()

	if len(actual.Mocks) != len(expected.Mocks) {
		t.Fatal("mock key set drifted from the Go fixtures")
	}
	for name, want := range expected.Mocks {
		got, present := actual.Mocks[name]
		if !present || got.HTTPStatus != want.HTTPStatus || !reflect.DeepEqual(got.Headers, want.Headers) {
			t.Fatalf("mock status or headers drifted for %s", name)
		}
	}
	if got := actual.Mocks["rate_limited"].Headers[RetryAfterHeader]; got == "" {
		t.Fatal("HTTP 429 fixture must carry Retry-After")
	}
}

func TestEveryMockMatchesItsLinkedEndpoint(t *testing.T) {
	contract := NewContractDocument()
	for name, item := range contract.Mocks {
		endpoint, present := contract.Endpoints[item.Endpoint]
		if !present {
			t.Errorf("mock %s links to missing endpoint %s", name, item.Endpoint)
			continue
		}
		switch item.Role {
		case "request":
			if item.HTTPStatus != 0 || endpoint.RequestSchema == "" {
				t.Errorf("request mock %s must omit HTTP status and link to an endpoint request schema", name)
			}
		case "response":
			body, err := json.Marshal(item.Body)
			if err != nil {
				t.Errorf("mock %s body cannot be inspected: %v", name, err)
				continue
			}
			var errorBody struct {
				Error string `json:"error"`
			}
			if err = json.Unmarshal(body, &errorBody); err != nil {
				t.Errorf("mock %s body cannot be projected: %v", name, err)
				continue
			}
			if errorBody.Error != "" {
				if endpoint.ErrorStatuses[errorBody.Error] != item.HTTPStatus {
					t.Errorf("mock %s has error %s status %d outside endpoint %s", name, errorBody.Error, item.HTTPStatus, item.Endpoint)
				}
			} else if !statusPresent(endpoint.SuccessStatuses, item.HTTPStatus) {
				t.Errorf("mock %s has success status %d outside endpoint %s", name, item.HTTPStatus, item.Endpoint)
			}
		default:
			t.Errorf("mock %s has invalid role %q", name, item.Role)
		}
	}
}

func statusPresent(statuses map[string]int, target int) bool {
	for _, status := range statuses {
		if status == target {
			return true
		}
	}
	return false
}

func TestLifecycleAndCivicWorksMapping(t *testing.T) {
	if !CanTransition(ServiceRequestStatusDraft, ServiceRequestStatusSubmitted) {
		t.Fatal("DRAFT to SUBMITTED must be allowed")
	}
	if CanTransition(ServiceRequestStatusAssigned, ServiceRequestStatusResolved) {
		t.Fatal("CRM ASSIGNED to RESOLVED must not widen the lifecycle in provision 10.1.2")
	}

	for external, expected := range map[CivicWorksStatus]ServiceRequestStatus{
		CivicWorksStatusAssigned:           ServiceRequestStatusAssigned,
		CivicWorksStatusInProgress:         ServiceRequestStatusInProgress,
		CivicWorksStatusPartiallyCompleted: ServiceRequestStatusInProgress,
		CivicWorksStatusCompleted:          ServiceRequestStatusResolved,
	} {
		actual, ok := MapCivicWorksStatus(external)
		if !ok || actual != expected {
			t.Fatalf("unexpected CivicWorks mapping for %s: %s, %t", external, actual, ok)
		}
	}
}

func TestCivicWorksAssignedToCompletedIsNormalisedThroughLegalEdges(t *testing.T) {
	plan, ok := PlanCivicWorksTransition(ServiceRequestStatusAssigned, CivicWorksStatusCompleted)
	if !ok {
		t.Fatal("legal CivicWorks ASSIGNED to COMPLETED event must be representable")
	}
	want := []ServiceRequestStatus{ServiceRequestStatusInProgress, ServiceRequestStatusResolved}
	if !reflect.DeepEqual(plan, want) {
		t.Fatalf("unexpected normalisation plan: %#v", plan)
	}

	current := ServiceRequestStatusAssigned
	for _, next := range plan {
		if !CanTransition(current, next) {
			t.Fatalf("normalisation contains illegal CRM edge %s to %s", current, next)
		}
		current = next
	}
	if current != ServiceRequestStatusResolved {
		t.Fatalf("completion plan ended at %s", current)
	}
}

func TestAnonymousLookupNotFoundIsPrivacySafe(t *testing.T) {
	notFound := MockAnonymousStatusNotFound()
	if notFound.RequestDetail != nil {
		t.Fatal("anonymous lookup mismatch must return an empty request-detail payload")
	}

	contract := NewContractDocument()
	endpoint := contract.Endpoints["anonymous_status_lookup"]
	if endpoint.SuccessStatuses["generic_not_found"] != 404 || endpoint.PrivacyRule == "" {
		t.Fatal("anonymous lookup must freeze the generic not-found behavior")
	}
	properties := contract.Schemas["public_service_request_detail"]["properties"].(map[string]interface{})
	if _, present := properties["primary_requester"]; present {
		t.Fatal("public request projection must not expose requester personal information")
	}
}

func TestWireRolesAndErrorVocabularyAreComplete(t *testing.T) {
	contract := NewContractDocument()
	wantRoles := []string{"service_agent", "supervisor", "department_manager", "platform_administrator", "workflow_designer"}
	if !reflect.DeepEqual(contract.Enums["actor_role"], wantRoles) {
		t.Fatalf("actor_role must contain only identity-provider asserted staff roles: %#v", contract.Enums["actor_role"])
	}

	wantErrors := []ErrorCode{
		ErrorInvalidResetToken, ErrorExpiredResetToken, ErrorInsufficientScope,
		ErrorInvalidClient, ErrorInvalidToken, ErrorTemporarilyUnavailable, ErrorNotFound,
	}
	available := make(map[string]bool, len(contract.Enums["error_code"]))
	for _, code := range contract.Enums["error_code"] {
		available[code] = true
	}
	for _, code := range wantErrors {
		if !available[string(code)] {
			t.Fatalf("missing consumer-visible error code %s", code)
		}
	}
}

func TestErrorEnvelopeOmitsOptionalFields(t *testing.T) {
	raw, err := json.Marshal(MockIdempotencyConflict())
	if err != nil {
		t.Fatal(err)
	}

	var body map[string]interface{}
	if err = json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if _, exists := body["errors"]; exists {
		t.Fatal("errors must be omitted for non-field errors")
	}
	if _, exists := body["current_version"]; exists {
		t.Fatal("current_version must be omitted outside version conflicts")
	}
}

func mustReadContract(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("contract.json")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func decodeJSON(t *testing.T, raw []byte) interface{} {
	t.Helper()
	var value interface{}
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	return value
}
