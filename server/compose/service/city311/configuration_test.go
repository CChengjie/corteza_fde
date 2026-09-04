package city311

import (
	"context"
	"net/http"
	"strings"
	"testing"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

func TestContactCategoryLifecycleRolesAndActiveFilters(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	require.NoError(t, svc.Seed(ctx, svc.now()))
	administrator := seededAssignmentActor(t, ctx, svc, st, "platform-admin@city311.example.invalid")
	manager := seededAssignmentActor(t, ctx, svc, st, "department-manager@city311.example.invalid")
	agent := seededAssignmentActor(t, ctx, svc, st, "service-agent@city311.example.invalid")

	seeded, err := svc.ListContactCategories(ctx, manager, ConfigurationListQuery{PageSize: 3})
	require.NoError(t, err)
	require.Equal(t, 7, seeded.TotalCount)
	require.Len(t, seeded.Items, 3)
	require.NotNil(t, seeded.NextPageToken)
	require.Equal(t, []string{"code"}, seeded.Sort)
	_, err = svc.ListContactCategories(ctx, agent, ConfigurationListQuery{})
	requireServiceError(t, err, http.StatusForbidden, contract.ErrorForbidden)

	created, err := svc.CreateContactCategory(ctx, manager, contract.CategoryWrite{
		Code: "NONPROFIT", Active: true, Labels: map[string]string{"EN": "Nonprofit", "ES": "Organización sin fines de lucro"},
	})
	require.NoError(t, err)
	require.Equal(t, uint64(1), created.Version)
	require.True(t, created.Active)
	_, err = svc.CreateContactCategory(ctx, administrator, contract.CategoryWrite{Code: "NONPROFIT", Active: true, Labels: map[string]string{"EN": "Duplicate"}})
	requireValidationCode(t, err, "/code", contract.ValidationDuplicate)
	_, err = svc.CreateContactCategory(ctx, administrator, contract.CategoryWrite{Code: "bad-code", Active: true, Labels: map[string]string{"EN": "Bad"}})
	requireValidationCode(t, err, "/code", contract.ValidationInvalidFormat)

	_, err = svc.UpdateContactCategory(ctx, manager, "NONPROFIT", contract.CategoryWrite{Code: "RENAMED", Active: false, Labels: map[string]string{"EN": "Renamed"}})
	requireValidationCode(t, err, "/code", contract.ValidationConflict)
	updated, err := svc.UpdateContactCategory(ctx, manager, "NONPROFIT", contract.CategoryWrite{Code: "NONPROFIT", Active: false, Labels: map[string]string{"EN": "Nonprofit organisation"}})
	require.NoError(t, err)
	require.Equal(t, uint64(2), updated.Version)
	require.False(t, updated.Active)

	_, err = svc.List(ctx, manager, RequestFilter{Categories: []contract.ContactCategory{"NONPROFIT"}})
	requireValidationCode(t, err, "/query/filters/category", contract.ValidationInvalidValue)
	audits, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{EntityID: "NONPROFIT"})
	require.NoError(t, err)
	require.Len(t, audits, 2)
}

func TestCustomFieldDefinitionLifecycleSubmissionWorkflowSearchAndExport(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	administrator := seededAssignmentActor(t, ctx, svc, st, "platform-admin@city311.example.invalid")
	manager := seededAssignmentActor(t, ctx, svc, st, "department-manager@city311.example.invalid")

	definition := contract.CustomFieldDefinition{
		Key: "impact", Labels: map[string]string{"EN": "Impact"}, Entity: "service_request",
		FieldType: contract.CustomFieldTypeSingleChoice, Required: true, Default: "MEDIUM", Active: true,
		ChoiceValues: []string{"LOW", "MEDIUM", "HIGH"},
	}
	created, err := svc.CreateCustomField(ctx, administrator, definition)
	require.NoError(t, err)
	require.Equal(t, uint64(1), created.Version)
	require.Equal(t, []string{"LOW", "MEDIUM", "HIGH"}, created.ChoiceValues)
	_, err = svc.CreateCustomField(ctx, manager, definition)
	requireServiceError(t, err, http.StatusForbidden, contract.ErrorForbidden)

	invalid := definition
	invalid.Key = "invalid_choice"
	invalid.Default = "UNKNOWN"
	_, err = svc.CreateCustomField(ctx, administrator, invalid)
	requireValidationCode(t, err, "/default", contract.ValidationInvalidValue)

	input := validSubmission()
	input.CustomFields = nil
	response, _, err := svc.Submit(ctx, input, "custom-default", SubmissionOptions{SourceChannel: contract.SourceChannelStaffInPerson, ActorType: contract.AuditActorStaff, ActorID: administrator.ID, StaffActor: &administrator})
	require.NoError(t, err)
	stored, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, response.RequestNumber)
	require.NoError(t, err)
	require.Equal(t, "MEDIUM", stored.CustomFields["impact"])

	input.CustomFields = map[string]any{"impact": "UNKNOWN"}
	_, _, err = svc.Submit(ctx, input, "custom-invalid", SubmissionOptions{SourceChannel: contract.SourceChannelStaffInPerson, ActorType: contract.AuditActorStaff, ActorID: administrator.ID, StaffActor: &administrator})
	requireValidationCode(t, err, "/custom_fields/impact", contract.ValidationInvalidValue)
	input.CustomFields = map[string]any{"unknown": true}
	_, _, err = svc.Submit(ctx, input, "custom-unknown", SubmissionOptions{SourceChannel: contract.SourceChannelStaffInPerson, ActorType: contract.AuditActorStaff, ActorID: administrator.ID, StaffActor: &administrator})
	requireValidationCode(t, err, "/custom_fields/unknown", contract.ValidationInvalidValue)

	err = svc.workflowFieldUpdate(ctx, administrator, stored, map[string]any{"field": "custom_fields.impact", "value": "UNKNOWN"})
	requireValidationCode(t, err, "/actions/value", contract.ValidationInvalidValue)
	require.NoError(t, svc.workflowFieldUpdate(ctx, administrator, stored, map[string]any{"field": "custom_fields.impact", "value": "HIGH"}))

	listed, err := svc.List(ctx, administrator, RequestFilter{CustomFields: map[string][]string{"impact": {"HIGH"}}})
	require.NoError(t, err)
	require.Equal(t, 1, listed.TotalCount)
	exported, err := svc.ExportData(ctx, administrator, "service-requests", contract.DataExportQuery{Filters: map[string][]string{"custom_fields.impact": {"HIGH"}}})
	require.NoError(t, err)
	require.Len(t, exported.Items, 1)
	require.Equal(t, "HIGH", exported.Items[0]["custom_fields"].(map[string]interface{})["impact"])

	definition.Active = false
	updated, err := svc.UpdateCustomField(ctx, administrator, "impact", created.Version, definition)
	require.NoError(t, err)
	require.False(t, updated.Active)
	require.Equal(t, uint64(2), updated.Version)
	_, err = svc.UpdateCustomField(ctx, administrator, "impact", created.Version, definition)
	requireServiceError(t, err, http.StatusConflict, contract.ErrorVersionConflict)

	stored, err = store.LookupCity311ServiceRequestByID(ctx, st, stored.ID)
	require.NoError(t, err)
	require.Equal(t, "HIGH", stored.CustomFields["impact"], "deactivation must preserve stored business values")
	definitions, err := svc.ListCustomFields(ctx, administrator, ConfigurationListQuery{})
	require.NoError(t, err)
	require.Equal(t, 1, definitions.TotalCount)
	require.False(t, definitions.Items[0].Active)
	_, err = svc.List(ctx, administrator, RequestFilter{CustomFields: map[string][]string{"impact": {"HIGH"}}})
	requireValidationCode(t, err, "/query/filters/custom_fields/impact", contract.ValidationInvalidValue)
}

func TestCustomFieldTypeAppropriateValidation(t *testing.T) {
	valid := []contract.CustomFieldDefinition{
		{Key: "text", Labels: map[string]string{"EN": "Text"}, Entity: "constituent", FieldType: contract.CustomFieldTypeText, Active: true, Validation: map[string]any{"min_length": 2, "max_length": 10}, Default: "ok"},
		{Key: "integer", Labels: map[string]string{"EN": "Integer"}, Entity: "service_request", FieldType: contract.CustomFieldTypeInteger, Active: true, Validation: map[string]any{"minimum": 1, "maximum": 10}, Default: 2},
		{Key: "decimal", Labels: map[string]string{"EN": "Decimal"}, Entity: "service_request", FieldType: contract.CustomFieldTypeDecimal, Active: true, Default: 2.5},
		{Key: "date", Labels: map[string]string{"EN": "Date"}, Entity: "service_request", FieldType: contract.CustomFieldTypeDate, Active: true, Validation: map[string]any{"minimum": "2026-01-01"}, Default: "2026-02-03"},
		{Key: "datetime", Labels: map[string]string{"EN": "Date time"}, Entity: "service_request", FieldType: contract.CustomFieldTypeDateTime, Active: true, Default: "2026-02-03T15:04:05Z"},
		{Key: "boolean", Labels: map[string]string{"EN": "Boolean"}, Entity: "service_request", FieldType: contract.CustomFieldTypeBoolean, Active: true, Default: true},
		{Key: "multi", Labels: map[string]string{"EN": "Multi"}, Entity: "service_request", FieldType: contract.CustomFieldTypeMultiChoice, Active: true, ChoiceValues: []string{"A", "B"}, Default: []string{"A"}},
	}
	for _, definition := range valid {
		require.NoError(t, validateCustomFieldDefinition(definition), definition.Key)
	}

	invalid := valid[0]
	invalid.Validation = map[string]any{"minimum": 1}
	requireValidationCode(t, validateCustomFieldDefinition(invalid), "/validation/minimum", contract.ValidationInvalidValue)
	invalid = valid[1]
	invalid.Default = 1.5
	requireValidationCode(t, validateCustomFieldDefinition(invalid), "/default", contract.ValidationInvalidValue)
	invalid = valid[3]
	invalid.Default = "03/02/2026"
	requireValidationCode(t, validateCustomFieldDefinition(invalid), "/default", contract.ValidationInvalidFormat)
	invalid = valid[6]
	invalid.ChoiceValues = []string{"A", "A"}
	requireValidationCode(t, validateCustomFieldDefinition(invalid), "/choice_values/1", contract.ValidationDuplicate)
}

func TestConfigurationValidationAndPaginationEdges(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	administrator := seededAssignmentActor(t, ctx, svc, st, "platform-admin@city311.example.invalid")
	agent := seededAssignmentActor(t, ctx, svc, st, "service-agent@city311.example.invalid")

	_, err := svc.ListContactCategories(ctx, administrator, ConfigurationListQuery{PageSize: 101})
	requireValidationCode(t, err, "/query/page_size", contract.ValidationOutOfRange)
	_, err = svc.ListContactCategories(ctx, administrator, ConfigurationListQuery{PageToken: "invalid"})
	requireServiceError(t, err, http.StatusBadRequest, contract.ErrorInvalidPageToken)
	_, err = svc.UpdateContactCategory(ctx, administrator, "MISSING", contract.CategoryWrite{Code: "MISSING", Active: true, Labels: map[string]string{"EN": "Missing"}})
	requireServiceError(t, err, http.StatusNotFound, contract.ErrorNotFound)
	_, err = svc.ListCustomFields(ctx, agent, ConfigurationListQuery{})
	requireServiceError(t, err, http.StatusForbidden, contract.ErrorForbidden)
	_, err = svc.UpdateCustomField(ctx, administrator, "missing", 1, contract.CustomFieldDefinition{Key: "missing", Labels: map[string]string{"EN": "Missing"}, Entity: "service_request", FieldType: contract.CustomFieldTypeText})
	requireServiceError(t, err, http.StatusNotFound, contract.ErrorNotFound)
	_, err = svc.UpdateCustomField(ctx, administrator, "missing", 0, contract.CustomFieldDefinition{})
	requireServiceError(t, err, http.StatusPreconditionRequired, contract.ErrorExpectedVersionRequired)

	for _, labels := range []map[string]string{
		{}, {"EN": "English", "FR": "Français"}, {"EN": "English", "ES": ""},
	} {
		require.NotEmpty(t, validateLocalizedLabels(labels, "/labels"))
	}
	require.False(t, validCategoryCode(""))
	require.False(t, validCategoryCode("lowercase"))
	require.False(t, validCategoryCode(strings.Repeat("A", 65)))

	base := contract.CustomFieldDefinition{Key: "edge", Labels: map[string]string{"EN": "Edge"}, Entity: "service_request", Active: true}
	tests := []struct {
		fieldType contract.CustomFieldType
		rules     map[string]any
		choices   []string
		field     string
		code      contract.ValidationCode
	}{
		{contract.CustomFieldTypeText, map[string]any{"min_length": -1}, nil, "/validation/min_length", contract.ValidationOutOfRange},
		{contract.CustomFieldTypeText, map[string]any{"max_length": 0}, nil, "/validation/max_length", contract.ValidationOutOfRange},
		{contract.CustomFieldTypeText, map[string]any{"min_length": 5, "max_length": 2}, nil, "/validation/min_length", contract.ValidationOutOfRange},
		{contract.CustomFieldTypeDecimal, map[string]any{"minimum": "low"}, nil, "/validation/minimum", contract.ValidationInvalidValue},
		{contract.CustomFieldTypeDecimal, map[string]any{"maximum": "high"}, nil, "/validation/maximum", contract.ValidationInvalidValue},
		{contract.CustomFieldTypeDecimal, map[string]any{"minimum": 4, "maximum": 2}, nil, "/validation/minimum", contract.ValidationOutOfRange},
		{contract.CustomFieldTypeDate, map[string]any{"minimum": "tomorrow"}, nil, "/validation/minimum", contract.ValidationInvalidFormat},
		{contract.CustomFieldTypeDate, map[string]any{"maximum": "tomorrow"}, nil, "/validation/maximum", contract.ValidationInvalidFormat},
		{contract.CustomFieldTypeDate, map[string]any{"minimum": "2026-02-04", "maximum": "2026-02-03"}, nil, "/validation/minimum", contract.ValidationOutOfRange},
		{contract.CustomFieldTypeBoolean, map[string]any{"minimum": 1}, nil, "/validation/minimum", contract.ValidationInvalidValue},
		{contract.CustomFieldTypeText, nil, []string{"A"}, "/choice_values", contract.ValidationInvalidValue},
		{contract.CustomFieldTypeSingleChoice, nil, nil, "/choice_values", contract.ValidationRequired},
		{contract.CustomFieldTypeSingleChoice, nil, []string{""}, "/choice_values/0", contract.ValidationInvalidValue},
	}
	for _, test := range tests {
		definition := base
		definition.FieldType, definition.Validation, definition.ChoiceValues = test.fieldType, test.rules, test.choices
		requireValidationCode(t, validateCustomFieldDefinition(definition), test.field, test.code)
	}
}

func TestCustomFieldValueAndFilterHelpers(t *testing.T) {
	definitions := []struct {
		definition contract.CustomFieldDefinition
		valid      any
		invalid    any
	}{
		{contract.CustomFieldDefinition{FieldType: contract.CustomFieldTypeText, Validation: map[string]any{"min_length": 2, "max_length": 3}}, "ok", "x"},
		{contract.CustomFieldDefinition{FieldType: contract.CustomFieldTypeInteger, Validation: map[string]any{"minimum": 1, "maximum": 3}}, 2, 2.5},
		{contract.CustomFieldDefinition{FieldType: contract.CustomFieldTypeDecimal, Validation: map[string]any{"minimum": 1, "maximum": 3}}, 2.5, 4},
		{contract.CustomFieldDefinition{FieldType: contract.CustomFieldTypeDate, Validation: map[string]any{"minimum": "2026-01-01", "maximum": "2026-12-31"}}, "2026-02-03", "2025-12-31"},
		{contract.CustomFieldDefinition{FieldType: contract.CustomFieldTypeDateTime}, "2026-02-03T15:04:05Z", "tomorrow"},
		{contract.CustomFieldDefinition{FieldType: contract.CustomFieldTypeBoolean}, true, "true"},
		{contract.CustomFieldDefinition{FieldType: contract.CustomFieldTypeSingleChoice, ChoiceValues: []string{"A"}}, "A", "B"},
		{contract.CustomFieldDefinition{FieldType: contract.CustomFieldTypeMultiChoice, ChoiceValues: []string{"A", "B"}}, []any{"A", "B"}, []any{"A", "A"}},
	}
	for _, test := range definitions {
		require.Empty(t, validateCustomFieldValue(test.definition, test.valid))
		require.NotEmpty(t, validateCustomFieldValue(test.definition, test.invalid))
	}
	require.Equal(t, contract.ValidationInvalidValue, validateCustomFieldValue(contract.CustomFieldDefinition{}, "value"))
	require.True(t, matchesCustomFields(map[string]any{"tags": []string{"A", "B"}}, map[string][]string{"tags": {"B"}}))
	require.True(t, matchesCustomFields(map[string]any{"tags": []any{"A", "B"}}, map[string][]string{"tags": {"A"}}))
	require.False(t, matchesCustomFields(map[string]any{}, map[string][]string{"tags": {"A"}}))
	require.False(t, matchesCustomFields(map[string]any{"tags": []string{"B"}}, map[string][]string{"tags": {"A"}}))
	require.False(t, matchesCustomFields(map[string]any{"tags": []any{"B"}}, map[string][]string{"tags": {"A"}}))
	require.Equal(t, "true", customFieldFilterValue(true))
	require.Equal(t, "1.5", customFieldFilterValue(float32(1.5)))
	require.Equal(t, "2", customFieldFilterValue(int64(2)))
	require.Equal(t, "3", customFieldFilterValue(uint64(3)))
	require.Equal(t, "A,B", customFieldFilterValue([]string{"A", "B"}))
	require.Equal(t, "A,2", customFieldFilterValue([]any{"A", 2}))
}

func requireValidationCode(t *testing.T, err error, field string, code contract.ValidationCode) {
	t.Helper()
	serviceErr, ok := err.(*ServiceError)
	require.True(t, ok, "expected ServiceError, got %T: %v", err, err)
	require.Equal(t, http.StatusUnprocessableEntity, serviceErr.Status)
	require.Contains(t, serviceErr.Payload.Errors, contract.FieldError{Field: field, Code: code})
}
