package city311

import (
	"reflect"
	"testing"
)

func TestEveryEndpointHasDirectionAuthenticationAndResolvedSchemas(t *testing.T) {
	contract := NewContractDocument()
	for name, endpoint := range contract.Endpoints {
		if endpoint.Direction != EndpointProvidedByCRM && endpoint.Direction != EndpointConsumedByCRM {
			t.Errorf("endpoint %s has invalid direction %q", name, endpoint.Direction)
		}
		if endpoint.Authentication == "" {
			t.Errorf("endpoint %s has no authentication declaration", name)
		}
		if len(endpoint.SuccessStatuses) == 0 {
			t.Errorf("endpoint %s has no success status", name)
		}
		assertSchemaReference(t, contract, name+" request", endpoint.RequestSchema)
		assertSchemaReference(t, contract, name+" response", endpoint.ResponseSchema)
		for entity, schema := range endpoint.EntityResponseSchemas {
			assertSchemaReference(t, contract, name+" entity "+entity, schema)
		}
	}
}

func TestEverySchemaAndEnumReferenceResolves(t *testing.T) {
	contract := NewContractDocument()
	for name, schema := range contract.Schemas {
		walkContractValue(t, contract, "schema "+name, schema)
	}
}

func TestCrossCuttingBrowserProtocolIsFrozen(t *testing.T) {
	contract := NewContractDocument()
	want := []string{
		"session_and_authorization", "optimistic_concurrency", "validation_errors", "lists",
		"idempotency", "asynchronous_operations", "atomic_bulk_operations", "attachment_transport",
	}
	for _, key := range want {
		if _, present := contract.Protocol[key]; !present {
			t.Errorf("missing protocol contract %s", key)
		}
	}

	concurrency := contract.Protocol["optimistic_concurrency"].(map[string]interface{})
	if concurrency["request_header"] != IfMatchHeader || concurrency["missing_status"] != 428 || concurrency["stale_status"] != 409 {
		t.Fatal("optimistic concurrency must freeze If-Match, missing-version, and stale-version behavior")
	}
	validation := contract.Protocol["validation_errors"].(map[string]interface{})
	if validation["code_enum_ref"] != "validation_code" {
		t.Fatal("field errors must use the controlled validation-code vocabulary")
	}

	errorProperties := contract.Schemas["error"]["properties"].(map[string]interface{})
	fieldError := errorProperties["errors"].(map[string]interface{})["items"].(map[string]interface{})
	fieldProperties := fieldError["properties"].(map[string]interface{})
	if fieldProperties["field"].(map[string]interface{})["format"] != "json-pointer" {
		t.Fatal("field error paths must use JSON Pointer")
	}
	if _, present := errorProperties["failing_request_id"]; !present {
		t.Fatal("bulk failures must identify the failing request")
	}
}

func TestCivicWorksCallbackAndLocationRules(t *testing.T) {
	contract := NewContractDocument()

	for _, current := range []ServiceRequestStatus{
		ServiceRequestStatusResolved, ServiceRequestStatusClosed, ServiceRequestStatusReopened,
	} {
		plan, ok := PlanCivicWorksTransition(current, CivicWorksStatusCompleted)
		if !ok || len(plan) != 0 {
			t.Errorf("redelivered COMPLETED must be acknowledged as a no-op from %s", current)
		}
		key := string(current) + "+" + string(CivicWorksStatusCompleted)
		if published, present := contract.CivicWorksTransitionPlans[key]; !present || len(published) != 0 {
			t.Errorf("published transition plans must expose %s as a no-op", key)
		}
	}

	callback := contract.Endpoints["civicworks_event_callback"]
	if callback.Direction != EndpointProvidedByCRM || callback.Path != "/integrations/civicworks/events" {
		t.Fatal("CivicWorks callback must be a CRM-provided endpoint")
	}
	for _, header := range []string{"X-CivicWorks-Event-Id", "X-CivicWorks-Signature"} {
		if !contains(callback.RequiredHeaders, header) {
			t.Errorf("CivicWorks callback is missing %s", header)
		}
	}
	if contract.Endpoints["workflow_action_execute"].Direction != EndpointConsumedByCRM || contract.Endpoints["mapping_geocode"].Direction != EndpointConsumedByCRM {
		t.Fatal("fixture endpoints called by CRM must be marked consumed_by_crm")
	}

	for serviceType, required := range map[string]bool{
		"TREE_MAINTENANCE": true, "POTHOLE": true, "MISSED_TRASH": true, "GENERAL_INQUIRY": false,
	} {
		rule, present := contract.ServiceTypeRules[serviceType]
		if !present || rule.LocationRequired != required || rule.ConfirmedCoordinatesRequired != required {
			t.Errorf("incorrect location rule for %s: %#v", serviceType, rule)
		}
	}
}

func TestApplicationAndIdentityRoleVocabulariesStayDistinct(t *testing.T) {
	contract := NewContractDocument()
	wantApplicationRoles := []string{
		"public_visitor", "constituent", "service_agent", "supervisor", "department_manager",
		"platform_administrator", "workflow_designer", "integration_client",
	}
	if !reflect.DeepEqual(contract.Enums["application_role"], wantApplicationRoles) {
		t.Fatalf("application roles do not match Table 4.1.1-A: %#v", contract.Enums["application_role"])
	}
	if !reflect.DeepEqual(contract.Enums["oidc_actor_type"], []string{"constituent"}) {
		t.Fatal("OIDC actor type must remain the narrow identity claim vocabulary")
	}
	if _, ambiguous := contract.Enums["actor_type"]; ambiguous {
		t.Fatal("ambiguous actor_type vocabulary must not be published")
	}
}

func TestRequiredClientSurfaceInventoryIsFrozen(t *testing.T) {
	contract := NewContractDocument()
	required := []string{
		"session_current", "account_register", "federated_sign_in_start", "portal_service_request_submit",
		"portal_draft_create", "portal_my_requests", "portal_link_anonymous_request", "profile_get", "password_change",
		"geocode_proxy", "portal_attachment_upload", "public_branding_get", "public_content_get", "public_help_get",
		"staff_request_queue", "staff_request_detail", "staff_request_transition", "staff_request_bulk",
		"staff_constituent_search", "admin_branding_update", "admin_content_update", "admin_custom_fields_create",
		"workflow_create", "workflow_execution_get", "identity_configuration_update", "integration_update",
		"report_run", "audit_list", "mail_send", "calendar_import", "health",
	}
	for _, name := range required {
		if _, present := contract.Endpoints[name]; !present {
			t.Errorf("required client surface %s is not frozen", name)
		}
	}
}

func TestMutatingVersionedEndpointsRequireIfMatch(t *testing.T) {
	contract := NewContractDocument()
	for _, name := range []string{
		"portal_draft_update", "profile_update", "staff_request_transition", "admin_branding_update",
		"admin_content_update", "admin_custom_fields_update", "workflow_update", "integration_update",
	} {
		endpoint := contract.Endpoints[name]
		if !contains(endpoint.RequiredHeaders, IfMatchHeader) {
			t.Errorf("versioned update %s does not require If-Match", name)
		}
		if endpoint.ErrorStatuses[string(ErrorExpectedVersionRequired)] != 428 || endpoint.ErrorStatuses[string(ErrorVersionConflict)] != 409 {
			t.Errorf("versioned update %s has incomplete concurrency errors", name)
		}
	}
}

func TestSubmissionAndRetryIdempotencyIsConsistent(t *testing.T) {
	contract := NewContractDocument()
	for _, name := range []string{"service_request_create", "portal_service_request_submit", "mail_send", "workflow_action_execute", "civicworks_work_order_create"} {
		endpoint := contract.Endpoints[name]
		if !contains(endpoint.RequiredHeaders, IdempotencyHeader) {
			t.Errorf("retryable operation %s does not require Idempotency-Key", name)
		}
	}
}

func assertSchemaReference(t *testing.T, contract ContractDocument, context, schema string) {
	t.Helper()
	if schema == "" {
		return
	}
	if _, present := contract.Schemas[schema]; !present {
		t.Errorf("%s references missing schema %s", context, schema)
	}
}

func walkContractValue(t *testing.T, contract ContractDocument, context string, value interface{}) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, child := range typed {
			switch key {
			case "schema_ref", "items_ref":
				if name, ok := child.(string); ok {
					assertSchemaReference(t, contract, context, name)
				}
			case "enum_ref":
				if name, ok := child.(string); ok {
					if _, present := contract.Enums[name]; !present {
						t.Errorf("%s references missing enum %s", context, name)
					}
				}
			}
			walkContractValue(t, contract, context, child)
		}
	case []interface{}:
		for _, child := range typed {
			walkContractValue(t, contract, context, child)
		}
	case []map[string]interface{}:
		for _, child := range typed {
			walkContractValue(t, contract, context, child)
		}
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
