package city311

func RecordReadFilterNames(endpoint string) []string {
	if endpoint == "staff_constituent_search" {
		return []string{"q", "email", "department", "district", "category"}
	}
	if endpoint == "audit_list" {
		return []string{"entity_type", "entity_id", "event_type", "actor_id", "request_id", "source_channel", "from", "to"}
	}
	return nil
}

func configureScopedReadContracts(document *ContractDocument) {
	document.Schemas["constituent_list_response"] = scopedReadListSchema("constituent")
	document.Schemas["audit_event_list_response"] = scopedReadListSchema("audit_event")
	for _, name := range []string{"staff_constituent_search", "audit_list"} {
		endpoint := document.Endpoints[name]
		if name == "staff_constituent_search" {
			endpoint.ResponseSchema = "constituent_list_response"
		} else {
			endpoint.ResponseSchema = "audit_event_list_response"
		}
		properties := map[string]interface{}{}
		for _, field := range RecordReadFilterNames(name) {
			properties[field] = stringProperty(0, 500)
		}
		endpoint.QueryParameters["filters"] = map[string]interface{}{"type": "object", "properties": properties, "additional_properties": false}
		columns := []string{"constituent_id", "display_name", "primary_category", "updated_at"}
		if name == "audit_list" {
			columns = []string{"occurred_at", "entity_type", "entity_id", "event_type", "actor_id"}
		}
		endpoint.QueryParameters["sort"]["allowed_fields"] = columns
		endpoint.QueryParameters["sort"]["default"] = columns[0]
		endpoint.QueryParameters["page_token"]["binding"] = "operation, actor/record scope, filters and sort; re-evaluate permission before each page"
		document.Endpoints[name] = endpoint
	}
	for _, name := range []string{"staff_constituent_search", "staff_constituent_detail"} {
		endpoint := document.Endpoints[name]
		endpoint.Authentication.Alternatives[0].ApplicationRoles = []string{"service_agent", "supervisor", "department_manager", "platform_administrator"}
		document.Endpoints[name] = endpoint
	}
}

func scopedReadListSchema(itemSchema string) map[string]interface{} {
	return object([]string{"items", "next_page_token", "total_count", "applied_filters", "sort"}, map[string]interface{}{
		"items":           map[string]interface{}{"type": "array", "items_ref": itemSchema},
		"next_page_token": map[string]interface{}{"type": "string", "opaque": true, "nullable": true},
		"total_count":     map[string]interface{}{"type": "integer", "minimum": 0},
		"applied_filters": map[string]interface{}{"type": "object", "additional_properties": map[string]interface{}{"type": "string"}},
		"sort":            map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
	})
}
