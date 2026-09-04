package city311

func RecordReadFilterNames(endpoint string) []string {
	if endpoint == "staff_constituent_search" {
		return []string{"constituent_id", "query", "display_name", "email", "phone", "primary_category", "preferred_language", "email_opt_out", "department", "district", "custom_fields"}
	}
	if endpoint == "audit_list" {
		return []string{"request_id", "entity_type", "entity_id", "event_type", "actor_type", "actor_id", "source_channel", "occurred_from", "occurred_to"}
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
		properties := scopedReadFilterProperties(name)
		endpoint.QueryParameters["filters"] = map[string]interface{}{"type": "object", "properties": properties, "additional_properties": false}
		columns := []string{"constituent_id", "display_name", "primary_category", "preferred_language", "department", "district"}
		defaultSort := "display_name,constituent_id"
		if name == "audit_list" {
			columns = []string{"occurred_at", "entity_type", "entity_id", "event_type", "actor_type", "actor_id", "source_channel"}
			defaultSort = "-occurred_at"
		}
		endpoint.QueryParameters["sort"]["allowed_fields"] = columns
		endpoint.QueryParameters["sort"]["default"] = defaultSort
		endpoint.QueryParameters["page_token"]["binding"] = "operation, filters and sort; permission re-evaluated for every request"
		document.Endpoints[name] = endpoint
	}
	for _, name := range []string{"staff_constituent_search", "staff_constituent_detail"} {
		endpoint := document.Endpoints[name]
		endpoint.Authentication.Alternatives[0].ApplicationRoles = []string{"service_agent", "supervisor", "department_manager", "platform_administrator"}
		document.Endpoints[name] = endpoint
	}
}

func scopedReadFilterProperties(endpoint string) map[string]interface{} {
	if endpoint == "staff_constituent_search" {
		properties := map[string]interface{}{}
		for _, name := range []string{"constituent_id", "query", "display_name", "email", "phone"} {
			properties[name] = scopedReadStringList(nil)
		}
		properties["primary_category"] = scopedReadStringList(map[string]interface{}{"enum_ref": "contact_category"})
		properties["preferred_language"] = scopedReadStringList(map[string]interface{}{"enum_ref": "language"})
		properties["email_opt_out"] = scopedReadStringList(map[string]interface{}{"enum": []string{"true", "false"}})
		properties["department"] = scopedReadStringList(map[string]interface{}{"enum_ref": "department_code"})
		properties["district"] = scopedReadStringList(map[string]interface{}{"enum_ref": "district_code"})
		properties["custom_fields"] = map[string]interface{}{"type": "object", "additional_properties": scopedReadStringList(nil)}
		return properties
	}
	properties := map[string]interface{}{}
	for _, name := range []string{"request_id", "entity_type", "entity_id", "event_type", "actor_id"} {
		properties[name] = scopedReadStringList(nil)
	}
	properties["actor_type"] = scopedReadStringList(map[string]interface{}{"enum_ref": "audit_actor_type"})
	properties["source_channel"] = scopedReadStringList(map[string]interface{}{"enum_ref": "source_channel"})
	properties["occurred_from"] = map[string]interface{}{"type": "string", "format": "date-time"}
	properties["occurred_to"] = map[string]interface{}{"type": "string", "format": "date-time"}
	return properties
}

func scopedReadStringList(item map[string]interface{}) map[string]interface{} {
	if item == nil {
		item = stringProperty(1, 254)
	}
	return map[string]interface{}{"type": "array", "min_items": 1, "unique_items": true, "items": item}
}

func scopedReadListSchema(itemSchema string) map[string]interface{} {
	return object([]string{"items", "next_page_token", "total_count", "applied_filters", "sort"}, map[string]interface{}{
		"items":           map[string]interface{}{"type": "array", "items_ref": itemSchema},
		"next_page_token": map[string]interface{}{"type": "string", "opaque": true, "nullable": true},
		"total_count":     map[string]interface{}{"type": "integer", "minimum": 0},
		"applied_filters": map[string]interface{}{"type": "object", "additional_properties": map[string]interface{}{"one_of": []map[string]interface{}{{"type": "string"}, {"type": "array", "items": map[string]interface{}{"type": "string"}}}}},
		"sort":            map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
	})
}
