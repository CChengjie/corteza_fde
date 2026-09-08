package city311

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScopedReadContractCompleteness(t *testing.T) {
	document := NewContractDocument()
	for _, tc := range []struct{ name, response, item string }{
		{"staff_constituent_search", "constituent_list_response", "constituent"},
		{"audit_list", "audit_event_list_response", "audit_event"},
	} {
		name := tc.name
		endpoint := document.Endpoints[name]
		require.Equal(t, tc.response, endpoint.ResponseSchema)
		require.Equal(t, tc.item, document.Schemas[tc.response]["properties"].(map[string]interface{})["items"].(map[string]interface{})["items_ref"])
		filters := endpoint.QueryParameters["filters"]
		require.Equal(t, false, filters["additional_properties"])
		properties := filters["properties"].(map[string]interface{})
		require.Len(t, properties, len(RecordReadFilterNames(name)))
		for _, field := range RecordReadFilterNames(name) {
			require.Contains(t, properties, field)
		}
		require.NotEmpty(t, endpoint.QueryParameters["sort"]["allowed_fields"])
		require.NotEmpty(t, endpoint.QueryParameters["page_token"]["binding"])
	}
	require.Equal(t, "display_name,constituent_id", document.Endpoints["staff_constituent_search"].QueryParameters["sort"]["default"])
	require.Equal(t, 200, document.Endpoints["staff_constituent_search"].SuccessStatuses["success"])
	require.Equal(t, 400, document.Endpoints["staff_constituent_search"].ErrorStatuses[string(ErrorInvalidPageToken)])
	require.Equal(t, "-occurred_at", document.Endpoints["audit_list"].QueryParameters["sort"]["default"])
	constituentFilters := document.Endpoints["staff_constituent_search"].QueryParameters["filters"]["properties"].(map[string]interface{})
	emailFilter := constituentFilters["email"].(map[string]interface{})
	require.Equal(t, "array", emailFilter["type"])
	require.Equal(t, true, emailFilter["unique_items"])
	require.Equal(t, []string{"true", "false"}, constituentFilters["email_opt_out"].(map[string]interface{})["items"].(map[string]interface{})["enum"])
	require.Equal(t, "object", constituentFilters["custom_fields"].(map[string]interface{})["type"])
	auditFilters := document.Endpoints["audit_list"].QueryParameters["filters"]["properties"].(map[string]interface{})
	require.Equal(t, "date-time", auditFilters["occurred_from"].(map[string]interface{})["format"])
	for _, name := range []string{"staff_constituent_search", "staff_constituent_detail"} {
		require.Equal(t, []string{"service_agent", "supervisor", "department_manager", "platform_administrator"}, document.Endpoints[name].Authentication.Alternatives[0].ApplicationRoles)
	}
	require.Nil(t, RecordReadFilterNames("unknown"))
}

func TestScopedReadOpenAPIUsesTypedListItems(t *testing.T) {
	document := NewOpenAPIDocument()
	paths := document["paths"].(map[string]interface{})
	components := document["components"].(map[string]interface{})["schemas"].(map[string]interface{})
	for _, tc := range []struct{ path, response, item string }{
		{"/api/v1/staff/constituents", "constituent_list_response", "constituent"},
		{"/api/v1/staff/audit-events", "audit_event_list_response", "audit_event"},
	} {
		operation := paths[tc.path].(map[string]interface{})["get"].(map[string]interface{})
		response := operation["responses"].(map[string]interface{})["200"].(map[string]interface{})
		content := response["content"].(map[string]interface{})["application/json"].(map[string]interface{})
		require.Equal(t, "#/components/schemas/"+tc.response, content["schema"].(map[string]interface{})["$ref"])
		items := components[tc.response].(map[string]interface{})["properties"].(map[string]interface{})["items"].(map[string]interface{})["items"].(map[string]interface{})
		require.Equal(t, "#/components/schemas/"+tc.item, items["$ref"])
	}
	operation := paths["/api/v1/staff/constituents"].(map[string]interface{})["get"].(map[string]interface{})
	responses := operation["responses"].(map[string]interface{})
	require.Equal(t, 200, responses["200"].(map[string]interface{})["x-city311-example"].(map[string]interface{})["status"])
	failure := responses["400"].(map[string]interface{})["x-city311-example"].(map[string]interface{})
	require.Equal(t, 400, failure["status"])
	require.Equal(t, string(ErrorInvalidPageToken), failure["body"].(map[string]interface{})["error"])
	require.Equal(t, "The page token is invalid.", failure["body"].(map[string]interface{})["message"])
}
