package compose

import (
	"github.com/cortezaproject/corteza/server/codegen/schema"
)

serviceRequest: {
	model: {
		ident:            "compose_city311_service_request"
		omitGetterSetter: true
		attributes: {
			id: schema.IdField
			request_number: {goType: "string", sortable: true, dal: {type: "Text", length: 16}}
			summary: {goType: "string", sortable: true, dal: {type: "Text", length: 160}}
			description: {goType: "string", dal: {type: "Text"}}
			service_type: {goType: "types.ServiceType", sortable: true, dal: {type: "Text", length: 64}}
			owning_department: {goType: "types.DepartmentCode", sortable: true, dal: {type: "Text", length: 64}}
			council_district: {goType: "types.DistrictCode", sortable: true, dal: {type: "Text", length: 32}}
			source_channel: {goType: "types.SourceChannel", sortable: true, dal: {type: "Text", length: 32}}
			origin_class: {goType: "types.OriginClass", sortable: true, dal: {type: "Text", length: 16}}
			status: {goType: "types.ServiceRequestStatus", sortable: true, dal: {type: "Text", length: 32}}
			primary_requester: {goType: "types.City311JSON", dal: {type: "JSON", defaultEmptyObject: true}}
			location: {goType: "types.City311JSON", dal: {type: "JSON", defaultEmptyObject: true}}
			custom_fields: {goType: "types.City311JSON", dal: {type: "JSON", defaultEmptyObject: true}}
			primary_assignee_id: {ident: "primaryAssigneeID", goType: "uint64", sortable: true, dal: {type: "ID", default: 0}}
			collaborator_ids: {ident: "collaboratorIDs", goType: "types.City311Uint64Set", dal: {type: "JSON"}}
			duplicate_group_id: {ident: "duplicateGroupID", goType: "string", sortable: true, dal: {type: "Text", length: 64}}
			version: {goType: "int", sortable: true, dal: {type: "Number", meta: {"rdbms:type": "integer"}, default: 1}}
			created_at: schema.SortableTimestampNowField
			updated_at: schema.SortableTimestampNowField
		}
		indexes: {
			primary: {attribute: "id"}
			unique_request_number: {attribute: "request_number"}
			request_scope: {attributes: ["owning_department", "council_district", "status"]}
		}
	}
	filter: {
		struct: {
			request_number: {goType: "string"}
			service_type: {goType: "string"}
			owning_department: {goType: "string"}
			council_district: {goType: "string"}
			source_channel: {goType: "string"}
			origin_class: {goType: "string"}
			status: {goType: "string"}
			primary_assignee_id: {ident: "primaryAssigneeID", goType: "uint64"}
		}
		byValue: ["request_number", "service_type", "owning_department", "council_district", "source_channel", "origin_class", "status", "primary_assignee_id"]
	}
	features: {labels: false, flags: false}
	envoy: {omit: true}
	store: {
		ident: "city311ServiceRequest"
		api: lookups: [
			{fields: ["id"]},
			{fields: ["request_number"], constraintCheck: true},
		]
	}
}

constituent: {
	model: {
		ident:            "compose_city311_constituent"
		omitGetterSetter: true
		attributes: {
			id: schema.IdField
			constituent_id: {ident: "constituentID", goType: "string", sortable: true, dal: {type: "Text", length: 64}}
			profile: {goType: "types.City311JSON", dal: {type: "JSON", defaultEmptyObject: true}}
			owning_department: {goType: "types.DepartmentCode", sortable: true, dal: {type: "Text", length: 64}}
			council_district: {goType: "types.DistrictCode", sortable: true, dal: {type: "Text", length: 32}}
			created_at: schema.SortableTimestampNowField
			updated_at: schema.SortableTimestampNowField
		}
		indexes: {
			primary: {attribute: "id"}
			unique_constituent_id: {attribute: "constituent_id"}
			constituent_scope: {attributes: ["owning_department", "council_district"]}
		}
	}
	filter: {
		struct: {
			constituent_id: {ident: "constituentID", goType: "string"}
			owning_department: {goType: "string"}
			council_district: {goType: "string"}
		}
		byValue: ["constituent_id", "owning_department", "council_district"]
	}
	features: {labels: false, flags: false}
	envoy: {omit: true}
	store: {
		ident: "city311Constituent"
		api: lookups: [
			{fields: ["id"]},
			{fields: ["constituent_id"], constraintCheck: true},
		]
	}
}

requestConstituentLink: {
	model: {
		ident:            "compose_city311_request_constituent"
		omitGetterSetter: true
		attributes: {
			id: schema.IdField
			request_id: {ident: "requestID", goType: "uint64", sortable: true, dal: {type: "ID"}}
			constituent_id: {ident: "constituentID", goType: "string", sortable: true, dal: {type: "Text", length: 64}}
			relationship_type: {goType: "types.RelationshipType", sortable: true, dal: {type: "Text", length: 32}}
			portal_visible: {goType: "bool", dal: {type: "Boolean", default: false}}
			notify_status: {goType: "bool", dal: {type: "Boolean", default: false}}
			created_at: schema.SortableTimestampNowField
			updated_at: schema.SortableTimestampNowField
		}
		indexes: {
			primary: {attribute: "id"}
			unique_primary: {
				attribute: "request_id"
				predicate: "relationship_type = 'PRIMARY_REQUESTER'"
			}
			unique_relationship: {attributes: ["request_id", "constituent_id", "relationship_type"]}
			request: {attributes: ["request_id", "created_at"]}
			constituent: {attributes: ["constituent_id", "created_at"]}
		}
	}
	filter: {
		struct: {
			request_id: {ident: "requestID", goType: "uint64"}
			constituent_id: {ident: "constituentID", goType: "string"}
			relationship_type: {goType: "string"}
		}
		byValue: ["request_id", "constituent_id", "relationship_type"]
	}
	features: {labels: false, flags: false}
	envoy: {omit: true}
	store: {
		ident: "city311RequestConstituentLink"
		api: lookups: [{fields: ["id"]}]
	}
}

requestNote: {
	model: {
		ident:            "compose_city311_request_note"
		omitGetterSetter: true
		attributes: {
			id: schema.IdField
			request_id: {ident: "requestID", goType: "uint64", sortable: true, dal: {type: "ID"}}
			author_type: {goType: "types.AuditActorType", dal: {type: "Text", length: 32}}
			author_id: {ident: "authorID", goType: "uint64", dal: {type: "ID"}}
			author_constituent_id: {ident: "authorConstituentID", goType: "string", dal: {type: "Text", length: 64}}
			body: {goType: "string", dal: {type: "Text", length: 2000}}
			portal_visible: {goType: "bool", dal: {type: "Boolean", default: false}}
			created_at: schema.SortableTimestampNowField
		}
		indexes: {
			primary: {attribute: "id"}
			request: {attributes: ["request_id", "created_at"]}
		}
	}
	filter: {
		struct: {request_id: {ident: "requestID", goType: "uint64"}}
		byValue: ["request_id"]
	}
	features: {labels: false, flags: false}
	envoy: {omit: true}
	store: {ident: "city311RequestNote", api: lookups: [{fields: ["id"]}]}
}

reopenRequest: {
	model: {
		ident:            "compose_city311_reopen_request"
		omitGetterSetter: true
		attributes: {
			id: schema.IdField
			request_id: {ident: "requestID", goType: "uint64", sortable: true, dal: {type: "ID"}}
			requested_by: {goType: "string", dal: {type: "Text", length: 64}}
			request_reason: {goType: "string", dal: {type: "Text", length: 2000}}
			status: {goType: "string", sortable: true, dal: {type: "Text", length: 32}}
			requested_at: schema.SortableTimestampNowField
			approved_by: {ident: "approvedBy", goType: "uint64", dal: {type: "ID", default: 0}}
			approval_reason: {goType: "string", dal: {type: "Text", length: 2000, default: ""}}
			approved_at: schema.SortableTimestampNilField
		}
		indexes: {
			primary: {attribute: "id"}
			request: {attributes: ["request_id", "requested_at"]}
			unique_pending: {
				attribute: "request_id"
				predicate: "status = 'PENDING_APPROVAL'"
			}
		}
	}
	filter: {
		struct: {
			request_id: {ident: "requestID", goType: "uint64"}
			status: {goType: "string"}
		}
		byValue: ["request_id", "status"]
	}
	features: {labels: false, flags: false}
	envoy: {omit: true}
	store: {ident: "city311ReopenRequest", api: lookups: [{fields: ["id"]}]}
}

requestSequence: {
	model: {
		ident:            "compose_city311_request_sequence"
		omitGetterSetter: true
		attributes: {
			id: schema.IdField
			next_number: {goType: "uint64", dal: {type: "Number", meta: {"rdbms:type": "bigint"}}}
		}
		indexes: {primary: {attribute: "id"}}
	}
	filter: {struct: {}}
	features: {labels: false, flags: false}
	envoy: {omit: true}
	store: {ident: "city311RequestSequence", api: lookups: [{fields: ["id"]}]}
}

idempotencyRecord: {
	model: {
		ident:            "compose_city311_idempotency"
		omitGetterSetter: true
		attributes: {
			id: schema.IdField
			operation: {goType: "string", dal: {type: "Text", length: 96}}
			key_hash: {goType: "string", dal: {type: "Text", length: 64}}
			request_hash: {goType: "string", dal: {type: "Text", length: 64}}
			response_status: {goType: "int", dal: {type: "Number", meta: {"rdbms:type": "integer"}}}
			response_body: {goType: "types.City311JSON", dal: {type: "JSON", defaultEmptyObject: true}}
			request_id: {ident: "requestID", goType: "uint64", dal: {type: "ID"}}
			created_at: schema.SortableTimestampNowField
			expires_at: schema.SortableTimestampField
		}
		indexes: {
			primary: {attribute: "id"}
			unique_operation_key: {attributes: ["operation", "key_hash"]}
			expires: {attribute: "expires_at"}
		}
	}
	filter: {
		struct: {operation: {goType: "string"}, key_hash: {goType: "string"}}
		byValue: ["operation", "key_hash"]
	}
	features: {labels: false, flags: false}
	envoy: {omit: true}
	store: {
		ident: "city311IdempotencyRecord"
		api: lookups: [
			{fields: ["id"]},
			{fields: ["operation", "key_hash"], constraintCheck: true},
		]
	}
}

auditEvent: {
	model: {
		ident:            "compose_city311_audit_event"
		omitGetterSetter: true
		attributes: {
			id: schema.IdField
			request_id: {ident: "requestID", goType: "uint64", sortable: true, dal: {type: "ID"}}
			entity_type: {goType: "string", sortable: true, dal: {type: "Text", length: 64, default: ""}}
			entity_id: {ident: "entityID", goType: "string", sortable: true, dal: {type: "Text", length: 64, default: ""}}
			event_type: {goType: "string", sortable: true, dal: {type: "Text", length: 96}}
			actor_type: {goType: "types.AuditActorType", dal: {type: "Text", length: 32}}
			actor_id: {ident: "actorID", goType: "uint64", dal: {type: "ID"}}
			source_channel: {goType: "types.SourceChannel", dal: {type: "Text", length: 32}}
			before: {goType: "types.City311JSON", dal: {type: "JSON", defaultEmptyObject: true}}
			after: {goType: "types.City311JSON", dal: {type: "JSON", defaultEmptyObject: true}}
			created_at: schema.SortableTimestampNowField
		}
		indexes: {
			primary: {attribute: "id"}
			request: {attributes: ["request_id", "created_at"]}
			entity: {attributes: ["entity_type", "entity_id", "created_at"]}
		}
	}
	filter: {
		struct: {
			request_id: {ident: "requestID", goType: "uint64"}
			entity_type: {goType: "string"}
			entity_id: {ident: "entityID", goType: "string"}
			event_type: {goType: "string"}
		}
		byValue: ["request_id", "entity_type", "entity_id", "event_type"]
	}
	features: {labels: false, flags: false}
	envoy: {omit: true}
	store: {ident: "city311AuditEvent", api: lookups: [{fields: ["id"]}]}
}

requestAttachment: {
	model: {
		ident:            "compose_city311_request_attachment"
		omitGetterSetter: true
		attributes: {
			id: schema.IdField
			request_id: {ident: "requestID", goType: "uint64", sortable: true, dal: {type: "ID"}}
			filename: {goType: "string", dal: {type: "Text", length: 120}}
			media_type: {goType: "string", dal: {type: "Text", length: 128}}
			size: {goType: "uint64", dal: {type: "Number", meta: {"rdbms:type": "bigint"}}}
			content: {goType: "[]byte", dal: {type: "Blob"}}
			created_at: schema.SortableTimestampNowField
		}
		indexes: {primary: {attribute: "id"}, request: {attributes: ["request_id", "created_at"]}}
	}
	filter: {
		struct: {request_id: {ident: "requestID", goType: "uint64"}}
		byValue: ["request_id"]
	}
	features: {labels: false, flags: false}
	envoy: {omit: true}
	store: {ident: "city311RequestAttachment", api: lookups: [{fields: ["id"]}]}
}

publicHistoryItem: {
	model: {
		ident:            "compose_city311_public_history"
		omitGetterSetter: true
		attributes: {
			id: schema.IdField
			request_id: {ident: "requestID", goType: "uint64", sortable: true, dal: {type: "ID"}}
			action: {goType: "string", dal: {type: "Text", length: 64}}
			responsible_department: {goType: "types.DepartmentCode", dal: {type: "Text", length: 64}}
			occurred_at: schema.SortableTimestampNowField
		}
		indexes: {primary: {attribute: "id"}, request: {attributes: ["request_id", "occurred_at"]}}
	}
	filter: {
		struct: {request_id: {ident: "requestID", goType: "uint64"}}
		byValue: ["request_id"]
	}
	features: {labels: false, flags: false}
	envoy: {omit: true}
	store: {ident: "city311PublicHistoryItem", api: lookups: [{fields: ["id"]}]}
}

actorProfile: {
	model: {
		ident:            "compose_city311_actor_profile"
		omitGetterSetter: true
		attributes: {
			id: schema.IdField
			application_roles: {goType: "types.City311ApplicationRoleSet", dal: {type: "JSON"}}
			department: {goType: "types.DepartmentCode", sortable: true, dal: {type: "Text", length: 64}}
			districts: {goType: "types.City311DistrictCodeSet", dal: {type: "JSON"}}
			created_at: schema.SortableTimestampNowField
			updated_at: schema.SortableTimestampNowField
		}
		indexes: {primary: {attribute: "id"}, department: {attribute: "department"}}
	}
	filter: {struct: {department: {goType: "string"}}, byValue: ["department"]}
	features: {labels: false, flags: false}
	envoy: {omit: true}
	store: {ident: "city311ActorProfile", api: lookups: [{fields: ["id"]}]}
}

localAccount: {
	model: {
		ident:            "compose_city311_local_account"
		omitGetterSetter: true
		attributes: {
			id: schema.IdField
			login_identifier: {goType: "string", sortable: true, dal: {type: "Text", length: 64}}
			verified_email: {goType: "string", sortable: true, dal: {type: "Text", length: 254}}
			preferred_language: {goType: "string", dal: {type: "Text", length: 2}}
			created_at: schema.SortableTimestampNowField
			updated_at: schema.SortableTimestampNowField
		}
		indexes: {
			primary: {attribute: "id"}
			unique_login_identifier: {attribute: "login_identifier"}
			unique_verified_email: {attribute: "verified_email"}
		}
	}
	filter: {
		struct: {
			login_identifier: {goType: "string"}
			verified_email: {goType: "string"}
		}
		byValue: ["login_identifier", "verified_email"]
	}
	features: {labels: false, flags: false}
	envoy: {omit: true}
	store: {
		ident: "city311LocalAccount"
		api: lookups: [
			{fields: ["id"]},
			{fields: ["login_identifier"], constraintCheck: true},
			{fields: ["verified_email"], constraintCheck:   true},
		]
	}
}

identitySession: {
	model: {
		ident:            "compose_city311_identity_session"
		omitGetterSetter: true
		attributes: {
			id: schema.IdField
			token_hash: {goType: "string", dal: {type: "Text", length: 64}}
			user_id: {ident: "userID", goType: "uint64", sortable: true, dal: {type: "ID"}}
			issued_at:           schema.SortableTimestampNowField
			last_seen_at:        schema.SortableTimestampNowField
			expires_at:          schema.SortableTimestampField
			absolute_expires_at: schema.SortableTimestampField
		}
		indexes: {
			primary: {attribute: "id"}
			unique_token_hash: {attribute: "token_hash"}
			user_expiry: {attributes: ["user_id", "expires_at"]}
		}
	}
	filter: {
		struct: {
			token_hash: {goType: "string"}
			user_id: {ident: "userID", goType: "uint64"}
		}
		byValue: ["token_hash", "user_id"]
	}
	features: {labels: false, flags: false}
	envoy: {omit: true}
	store: {
		ident: "city311IdentitySession"
		api: lookups: [
			{fields: ["id"]},
			{fields: ["token_hash"], constraintCheck: true},
		]
	}
}

passwordResetToken: {
	model: {
		ident:            "compose_city311_password_reset_token"
		omitGetterSetter: true
		attributes: {
			id: schema.IdField
			token_hash: {goType: "string", dal: {type: "Text", length: 64}}
			user_id: {ident: "userID", goType: "uint64", sortable: true, dal: {type: "ID"}}
			created_at: schema.SortableTimestampNowField
			expires_at: schema.SortableTimestampField
			used_at:    schema.SortableTimestampNilField
		}
		indexes: {
			primary: {attribute: "id"}
			unique_token_hash: {attribute: "token_hash"}
			user_expiry: {attributes: ["user_id", "expires_at"]}
		}
	}
	filter: {
		struct: {user_id: {ident: "userID", goType: "uint64"}}
		byValue: ["user_id"]
	}
	features: {labels: false, flags: false}
	envoy: {omit: true}
	store: {
		ident: "city311PasswordResetToken"
		api: lookups: [
			{fields: ["id"]},
			{fields: ["token_hash"], constraintCheck: true},
		]
	}
}

identityNotification: {
	model: {
		ident:            "compose_city311_identity_notification"
		omitGetterSetter: true
		attributes: {
			id: schema.IdField
			user_id: {ident: "userID", goType: "uint64", sortable: true, dal: {type: "ID"}}
			kind: {goType: "string", sortable: true, dal: {type: "Text", length: 64}}
			recipient: {goType: "string", dal: {type: "Text", length: 254}}
			delivery_key: {goType: "string", dal: {type: "Text", length: 128}}
			payload: {goType: "types.City311JSON", dal: {type: "JSON", defaultEmptyObject: true}}
			status: {goType: "string", sortable: true, dal: {type: "Text", length: 16}}
			attempts: {goType: "int", dal: {type: "Number", meta: {"rdbms:type": "integer"}, default: 0}}
			last_error: {goType: "string", dal: {type: "Text", default: ""}}
			created_at: schema.SortableTimestampNowField
			updated_at: schema.SortableTimestampNowField
		}
		indexes: {
			primary: {attribute: "id"}
			unique_delivery_key: {attribute: "delivery_key"}
			user_status: {attributes: ["user_id", "status"]}
		}
	}
	filter: {
		struct: {
			user_id: {ident: "userID", goType: "uint64"}
			status: {goType: "string"}
		}
		byValue: ["user_id", "status"]
	}
	features: {labels: false, flags: false}
	envoy: {omit: true}
	store: {ident: "city311IdentityNotification", api: lookups: [{fields: ["id"]}]}
}
