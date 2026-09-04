package city311

import (
	"github.com/stretchr/testify/require"
	"testing"
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
	for _, name := range []string{"staff_constituent_search", "staff_constituent_detail"} {
		require.Equal(t, []string{"service_agent", "supervisor", "department_manager", "platform_administrator"}, document.Endpoints[name].Authentication.Alternatives[0].ApplicationRoles)
	}
	require.Nil(t, RecordReadFilterNames("unknown"))
}
