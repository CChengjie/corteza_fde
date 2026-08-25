package city311

func clientMocks() map[string]MockContract {
	return map[string]MockContract{
		"session_anonymous": mock(200, map[string]interface{}{
			"authenticated": false, "actor": nil, "preferred_language": "EN", "expires_at": nil,
		}),
		"session_staff": mock(200, map[string]interface{}{
			"authenticated": true,
			"actor": map[string]interface{}{
				"actor_id": "staff-0007", "display_name": "Sam Rivera", "application_roles": []string{"service_agent"},
				"department_codes": []string{"STREETS"}, "district_codes": []string{"NORTH", "CENTRAL"}, "scopes": []string{},
				"available_routes": []string{"staff_request_queue", "staff_request_detail"},
			},
			"preferred_language": "EN", "expires_at": "2026-08-25T20:00:00Z",
		}),
		"operation_pending": mock(200, map[string]interface{}{
			"operation_id": "op-00041", "kind": "REPORT_EXPORT", "status": "RUNNING", "progress": 40,
			"result": nil, "error": nil, "created_at": "2026-08-25T12:00:00Z", "updated_at": "2026-08-25T12:00:04Z", "completed_at": nil,
		}),
		"operation_succeeded": mock(200, map[string]interface{}{
			"operation_id": "op-00041", "kind": "REPORT_EXPORT", "status": "SUCCEEDED", "progress": 100,
			"result": map[string]interface{}{"download_url": "/api/v1/operations/op-00041/result"}, "error": nil,
			"created_at": "2026-08-25T12:00:00Z", "updated_at": "2026-08-25T12:00:06Z", "completed_at": "2026-08-25T12:00:06Z",
		}),
		"empty_list": mock(200, map[string]interface{}{
			"items": []interface{}{}, "next_page_token": nil, "total_count": 0,
			"applied_filters": map[string]interface{}{"status": []string{"CLOSED"}}, "sort": []string{"-updated_at"},
		}),
		"nested_validation_error": mock(422, APIError{
			Error: ErrorValidation, Message: "The request contains invalid fields.", Retryable: false,
			Errors: []FieldError{{Field: "/attachments/2/media_type", Code: ValidationInvalidValue}},
		}),
		"bulk_validation_failure": mock(422, APIError{
			Error: ErrorValidation, Message: "One selected request is not eligible for this bulk operation.", Retryable: false,
			FailingRequestID: "case-7c58d2", Errors: []FieldError{{Field: "/changes/status", Code: ValidationInvalidValue}},
		}),
		"expected_version_required": mock(428, APIError{
			Error: ErrorExpectedVersionRequired, Message: "If-Match is required for this update.", Retryable: false,
		}),
		"portal_attachment_staged": mock(201, map[string]interface{}{
			"attachment_token": "upload-00031", "filename": "pothole.jpg", "media_type": "image/jpeg", "size": 248031,
			"expires_at": "2026-08-25T12:15:00Z",
		}),
		"portal_service_request_created": mock(201, MockCreatedServiceRequest()),
		"public_branding": mock(200, map[string]interface{}{
			"organisation_name": "City 311", "logo_url": "/assets/city-logo.svg", "favicon_url": "/assets/favicon.ico",
			"portal_wallpaper_url": "/assets/portal-wallpaper.jpg", "login_header": "City services, one place", "public_header": "City 311",
			"public_footer": "City services", "primary_colour": "#005EA8", "accent_colour": "#FFB81C", "font_family": "Arial",
			"published": true, "version": 3, "updated_at": "2026-08-25T10:00:00Z",
		}),
		"public_content_home": mock(200, map[string]interface{}{
			"content_key": "HOME", "body": "<p>Welcome to City 311.</p>", "state": "PUBLISHED", "published": true,
			"version": 2, "updated_at": "2026-08-25T10:00:00Z",
		}),
		"public_help_submit": mock(200, map[string]interface{}{
			"help_key": "public.request.submit", "language": "EN",
			"body":    "<p>Describe the issue, choose its type, and provide the location where City service is needed.</p>",
			"version": 1, "updated_at": "2026-08-25T10:00:00Z",
		}),
		"staff_queue": mock(200, map[string]interface{}{
			"items": []interface{}{map[string]interface{}{
				"request_id": "case-7c58d2", "request_number": "SR-2026-00041", "summary": "Pothole blocking the eastbound lane",
				"service_type": "POTHOLE", "status": "IN_PROGRESS", "owning_department": "STREETS", "council_district": "CENTRAL",
				"origin_class": "EXTERNAL", "source_channel": "PORTAL_ANONYMOUS", "primary_assignee_id": "staff-0007",
				"duplicate_group_id": nil, "version": 3, "updated_at": "2026-08-25T12:00:00Z", "available_actions": []string{"RESOLVE"},
			}},
			"next_page_token": nil, "total_count": 1, "applied_filters": map[string]interface{}{"status": []string{"IN_PROGRESS"}}, "sort": []string{"-updated_at"},
		}),
		"civicworks_event_acknowledged":     {HTTPStatus: 204, Body: map[string]interface{}{}},
		"civicworks_duplicate_acknowledged": {HTTPStatus: 204, Body: map[string]interface{}{}},
		"civicworks_invalid_signature": mock(401, APIError{
			Error: ErrorInvalidSignature, Message: "The CivicWorks event signature is invalid.", Retryable: false,
		}),
	}
}
