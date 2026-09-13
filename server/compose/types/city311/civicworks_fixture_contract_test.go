package city311

import (
	"reflect"
	"testing"
)

func TestCivicWorksContractUsesCanonicalRepresentationAndAbsoluteCallback(t *testing.T) {
	document := NewContractDocument()
	if document.ContractVersion != "3.0.0" || document.Versioning.SupportedMajor != 3 {
		t.Fatalf("CivicWorks boundary must be the sole current major-3 contract: %#v", document.Versioning)
	}
	for _, provision := range []string{"10.2.2", "10.2.3", "10.2.4", "10.2.5", "11.1.1", "11.1.2", "11.1.3", "13.2.2", "13.2.3"} {
		if !contains(document.Provisions, provision) {
			t.Errorf("CivicWorks contract omits original provision %s", provision)
		}
	}

	createProperties := document.Schemas["civicworks_work_order_create"]["properties"].(map[string]interface{})
	callback := createProperties["callback_url"].(map[string]interface{})
	if callback["type"] != "string" || callback["format"] != "uri" || callback["pattern"] != "^https?://" || callback["const"] != nil {
		t.Fatalf("callback must be one absolute server-owned URL with no relative compatibility value: %#v", callback)
	}

	workOrder := document.Schemas["civicworks_work_order"]
	required := workOrder["required"].([]string)
	for _, field := range []string{
		"work_order_id", "source_case_id", "service_request_number", "service_type", "summary",
		"department_code", "fulfilment_source", "status", "external_status_url", "version", "created_at", "updated_at",
	} {
		if !contains(required, field) {
			t.Errorf("canonical CivicWorks response does not require %s", field)
		}
	}
	properties := workOrder["properties"].(map[string]interface{})
	if properties["fulfilment_source"].(map[string]interface{})["const"] != "CIVICWORKS" || properties["location"] == nil {
		t.Fatal("canonical CivicWorks response omits fulfilment source or optional location")
	}

	mock := MockCivicWorksCreated()
	if mock.ServiceType != ServiceTypeTreeMaintenance || mock.DepartmentCode != DepartmentPublicWorks || mock.FulfilmentSource != "CIVICWORKS" || !reflect.DeepEqual(mock.Location, map[string]any{
		"address": "100 Example Street", "latitude": 42.9001, "longitude": -78.8801,
	}) {
		t.Fatalf("CivicWorks success fixture is not canonical: %#v", mock)
	}
	for name, endpoint := range document.Endpoints {
		if endpoint.Path == "/__fixture/v1/reset" || endpoint.Authentication.Credential == "CIVICWORKS_CONTROL_TOKEN" {
			t.Errorf("evaluator-only control leaked into CRM product contract at %s", name)
		}
	}
}
