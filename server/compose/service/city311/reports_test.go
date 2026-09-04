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

func validReportDefinition(reportID, entity string, columns ...string) contract.ReportDefinition {
	return contract.ReportDefinition{
		ReportID: reportID, Name: "Operational report", Entity: entity,
		Columns: columns, Filters: map[string]any{}, Sort: []string{},
	}
}

func TestReportCatalogueValidationAndSavedReportLifecycle(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	supervisor := seededAssignmentActor(t, ctx, svc, st, "supervisor@city311.example.invalid")
	agent := seededAssignmentActor(t, ctx, svc, st, "service-agent@city311.example.invalid")
	constituent := contract.Actor{ID: 42, Roles: []contract.ApplicationRole{contract.ApplicationRoleConstituent}}

	catalogue, err := svc.ReportCatalogue(ctx, agent, ConfigurationListQuery{PageSize: 2})
	require.NoError(t, err)
	require.Equal(t, 5, catalogue.TotalCount)
	require.Len(t, catalogue.Items, 2)
	require.NotNil(t, catalogue.NextPageToken)
	require.Equal(t, "request_volume", catalogue.Items[0].ReportKey)
	_, err = svc.ReportCatalogue(ctx, constituent, ConfigurationListQuery{})
	requireServiceError(t, err, http.StatusForbidden, contract.ErrorForbidden)

	definition := validReportDefinition("north-work", "service_requests", "request_number", "status")
	created, err := svc.CreateSavedReport(ctx, supervisor, definition)
	require.NoError(t, err)
	require.Equal(t, uint64(1), created.Version)
	require.False(t, created.UpdatedAt.IsZero())
	_, err = svc.CreateSavedReport(ctx, supervisor, definition)
	requireValidationCode(t, err, "/report_id", contract.ValidationDuplicate)

	owned, err := svc.ListSavedReports(ctx, supervisor, ConfigurationListQuery{})
	require.NoError(t, err)
	require.Len(t, owned.Items, 1)
	notShared, err := svc.ListSavedReports(ctx, agent, ConfigurationListQuery{})
	require.NoError(t, err)
	require.Empty(t, notShared.Items)

	definition.Name = "North workload"
	_, err = svc.UpdateSavedReport(ctx, supervisor, definition.ReportID, 9, definition)
	requireServiceError(t, err, http.StatusConflict, contract.ErrorVersionConflict)
	updated, err := svc.UpdateSavedReport(ctx, supervisor, definition.ReportID, 1, definition)
	require.NoError(t, err)
	require.Equal(t, uint64(2), updated.Version)
	_, err = svc.UpdateSavedReport(ctx, agent, definition.ReportID, 2, definition)
	requireServiceError(t, err, http.StatusForbidden, contract.ErrorForbidden)

	shared, err := svc.ShareSavedReport(ctx, supervisor, definition.ReportID, 2, contract.ReportShare{Roles: []contract.ApplicationRole{contract.ApplicationRoleServiceAgent}})
	require.NoError(t, err)
	require.Equal(t, uint64(3), shared.Version)
	visible, err := svc.ListSavedReports(ctx, agent, ConfigurationListQuery{})
	require.NoError(t, err)
	require.Len(t, visible.Items, 1)

	_, err = svc.ShareSavedReport(ctx, agent, definition.ReportID, 3, contract.ReportShare{})
	requireServiceError(t, err, http.StatusForbidden, contract.ErrorForbidden)
	_, err = svc.ShareSavedReport(ctx, supervisor, definition.ReportID, 3, contract.ReportShare{Roles: []contract.ApplicationRole{contract.ApplicationRoleDepartmentManager}})
	requireValidationCode(t, err, "/roles/0", contract.ValidationInvalidValue)
}

func TestReportDefinitionRejectsUnsupportedAndOversizedBuilderInput(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	administrator := seededAssignmentActor(t, ctx, svc, st, "platform-admin@city311.example.invalid")

	tests := []struct {
		name       string
		definition contract.ReportDefinition
		field      string
		code       contract.ValidationCode
	}{
		{name: "entity", definition: validReportDefinition("bad-entity", "unknown", "request_id"), field: "/entity", code: contract.ValidationInvalidValue},
		{name: "column", definition: validReportDefinition("bad-column", "service_requests", "unknown"), field: "/columns/0", code: contract.ValidationInvalidValue},
		{name: "duplicate", definition: validReportDefinition("duplicate", "service_requests", "status", "status"), field: "/columns/1", code: contract.ValidationDuplicate},
		{name: "filter", definition: contract.ReportDefinition{ReportID: "bad-filter", Name: "Bad", Entity: "service_requests", Columns: []string{"status"}, Filters: map[string]any{"unknown": "x"}}, field: "/filters/unknown", code: contract.ValidationInvalidValue},
		{name: "vocabulary", definition: contract.ReportDefinition{ReportID: "bad-vocabulary", Name: "Bad", Entity: "service_requests", Columns: []string{"status"}, Filters: map[string]any{"status": "UNKNOWN"}}, field: "/filters/status", code: contract.ValidationInvalidValue},
		{name: "date range", definition: contract.ReportDefinition{ReportID: "bad-range", Name: "Bad", Entity: "service_requests", Columns: []string{"status"}, Filters: map[string]any{"created_from": "2026-02-04T00:00:00Z", "created_to": "2026-02-03T00:00:00Z"}}, field: "/filters/created_from", code: contract.ValidationOutOfRange},
		{name: "group", definition: contract.ReportDefinition{ReportID: "bad-group", Name: "Bad", Entity: "service_requests", Columns: []string{"status"}, Filters: map[string]any{}, Grouping: stringPointer("unknown")}, field: "/grouping", code: contract.ValidationInvalidValue},
		{name: "sort", definition: contract.ReportDefinition{ReportID: "bad-sort", Name: "Bad", Entity: "service_requests", Columns: []string{"status"}, Filters: map[string]any{}, Sort: []string{"unknown"}}, field: "/sort/0", code: contract.ValidationInvalidValue},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := svc.CreateSavedReport(ctx, administrator, test.definition)
			requireValidationCode(t, err, test.field, test.code)
		})
	}

	tooManyColumns := validReportDefinition("many-columns", "service_requests")
	for index := 0; index < 21; index++ {
		tooManyColumns.Columns = append(tooManyColumns.Columns, "status")
	}
	_, err := svc.CreateSavedReport(ctx, administrator, tooManyColumns)
	requireValidationCode(t, err, "/columns", contract.ValidationTooManyItems)
	tooManySorts := validReportDefinition("many-sorts", "service_requests", "status")
	tooManySorts.Sort = []string{"status", "created_at", "updated_at", "request_number"}
	_, err = svc.CreateSavedReport(ctx, administrator, tooManySorts)
	requireValidationCode(t, err, "/sort", contract.ValidationTooManyItems)

	unknownCustom := validReportDefinition("unknown-custom", "service_requests", "custom_fields.missing")
	_, err = svc.CreateSavedReport(ctx, administrator, unknownCustom)
	requireValidationCode(t, err, "/columns/0", contract.ValidationInvalidValue)
	inactive, err := svc.CreateCustomField(ctx, administrator, contract.CustomFieldDefinition{
		Key: "inactive_report", Labels: map[string]string{"EN": "Inactive"}, Entity: "service_request",
		FieldType: contract.CustomFieldTypeText, Active: false,
	})
	require.NoError(t, err)
	require.False(t, inactive.Active)
	unknownCustom.Columns = []string{"custom_fields.inactive_report"}
	_, err = svc.CreateSavedReport(ctx, administrator, unknownCustom)
	requireValidationCode(t, err, "/columns/0", contract.ValidationInvalidValue)
	_, err = svc.CreateCustomField(ctx, administrator, contract.CustomFieldDefinition{
		Key: "report_score", Labels: map[string]string{"EN": "Score"}, Entity: "service_request",
		FieldType: contract.CustomFieldTypeInteger, Active: true,
	})
	require.NoError(t, err)
	typedFilter := validReportDefinition("typed-filter", "service_requests", "custom_fields.report_score")
	typedFilter.Filters["custom_fields.report_score"] = "not-an-integer"
	_, err = svc.CreateSavedReport(ctx, administrator, typedFilter)
	requireValidationCode(t, err, "/filters/custom_fields.report_score", contract.ValidationInvalidValue)
}

func TestReportRunScopeCustomFieldsAggregationAndCSVExport(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	administrator := seededAssignmentActor(t, ctx, svc, st, "platform-admin@city311.example.invalid")
	agent := seededAssignmentActor(t, ctx, svc, st, "service-agent@city311.example.invalid")

	custom, err := svc.CreateCustomField(ctx, administrator, contract.CustomFieldDefinition{
		Key: "report_tag", Labels: map[string]string{"EN": "Report tag"}, Entity: "service_request",
		FieldType: contract.CustomFieldTypeText, Active: true,
	})
	require.NoError(t, err)
	require.Equal(t, uint64(1), custom.Version)
	request, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, "SR-2026-00034")
	require.NoError(t, err)
	request.Summary = `Road, "unsafe"`
	request.CustomFields["report_tag"] = "priority"
	require.NoError(t, store.UpdateCity311ServiceRequest(ctx, st, request))
	outOfScope, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, "SR-2026-00035")
	require.NoError(t, err)
	outOfScope.OwningDepartment = contract.DepartmentSanitation
	outOfScope.CouncilDistrict = contract.DistrictSouth
	require.NoError(t, store.UpdateCity311ServiceRequest(ctx, st, outOfScope))

	definition := validReportDefinition("tagged-requests", "service_requests", "request_number", "summary", "custom_fields.report_tag")
	definition.Filters["custom_fields.report_tag"] = "priority"
	created, err := svc.CreateSavedReport(ctx, administrator, definition)
	require.NoError(t, err)

	pending, err := svc.StartReportRun(ctx, administrator, contract.ReportRun{Definition: definition})
	require.NoError(t, err)
	require.Equal(t, contract.OperationStatusPending, pending.Status)
	completed, err := svc.GetOperation(ctx, administrator, pending.OperationID)
	require.NoError(t, err)
	require.Equal(t, contract.OperationStatusSucceeded, completed.Status)
	require.Equal(t, float64(1), completed.Result["row_count"])
	rows := completed.Result["rows"].([]any)
	require.Equal(t, "priority", rows[0].(map[string]any)["custom_fields.report_tag"])

	aggregated := validReportDefinition("status-count", "service_requests", "status", "count")
	aggregated.Grouping = stringPointer("status")
	groupRun, err := svc.StartReportRun(ctx, administrator, contract.ReportRun{Definition: aggregated})
	require.NoError(t, err)
	groupOperation, err := svc.GetOperation(ctx, administrator, groupRun.OperationID)
	require.NoError(t, err)
	require.NotEmpty(t, groupOperation.Result["rows"])

	for _, entity := range []struct {
		name    string
		columns []string
	}{
		{name: "constituents", columns: []string{"constituent_id", "display_name"}},
		{name: "follow_up_actions", columns: []string{"request_number", "action_type"}},
	} {
		runDefinition := validReportDefinition("run-"+entity.name, entity.name, entity.columns...)
		operation, runErr := svc.StartReportRun(ctx, administrator, contract.ReportRun{Definition: runDefinition})
		require.NoError(t, runErr, entity.name)
		completed, runErr := svc.GetOperation(ctx, administrator, operation.OperationID)
		require.NoError(t, runErr, entity.name)
		require.Greater(t, completed.Result["row_count"].(float64), float64(0), entity.name)
	}

	adminScope := validReportDefinition("admin-scope", "service_requests", "request_number")
	adminRun, err := svc.StartReportRun(ctx, administrator, contract.ReportRun{Definition: adminScope})
	require.NoError(t, err)
	agentRun, err := svc.StartReportRun(ctx, agent, contract.ReportRun{Definition: adminScope})
	require.NoError(t, err)
	adminCompleted, err := svc.GetOperation(ctx, administrator, adminRun.OperationID)
	require.NoError(t, err)
	agentCompleted, err := svc.GetOperation(ctx, agent, agentRun.OperationID)
	require.NoError(t, err)
	require.Greater(t, adminCompleted.Result["row_count"].(float64), agentCompleted.Result["row_count"].(float64))

	exportPending, err := svc.StartReportExport(ctx, administrator, created.ReportID, contract.ReportExport{Format: "CSV"})
	require.NoError(t, err)
	exportOperation, err := svc.GetOperation(ctx, administrator, exportPending.OperationID)
	require.NoError(t, err)
	require.Equal(t, contract.OperationStatusSucceeded, exportOperation.Status)
	download, err := svc.DownloadOperation(ctx, administrator, exportPending.OperationID)
	require.NoError(t, err)
	require.Equal(t, "text/csv; charset=utf-8", download.ContentType)
	require.True(t, strings.HasPrefix(string(download.Content), "request_number,summary,custom_fields.report_tag\r\n"))
	require.Contains(t, string(download.Content), `"Road, ""unsafe"""`)
	require.NotContains(t, strings.ReplaceAll(string(download.Content), "\r\n", ""), "\n")

	audits, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{EventType: "REPORT_EXPORTED"})
	require.NoError(t, err)
	require.Len(t, audits, 1)
	require.Equal(t, created.ReportID, audits[0].EntityID)
	_, err = svc.StartReportExport(ctx, administrator, created.ReportID, contract.ReportExport{Format: "PDF"})
	requireValidationCode(t, err, "/format", contract.ValidationInvalidValue)
}

func TestReportBuilderValueFilterAndVisibilityHelpers(t *testing.T) {
	rows := []map[string]any{{"group": "B", "active_request_count": 2}, {"group": "A", "active_request_count": 1}, {"group": "A", "active_request_count": 1}}
	require.Len(t, distinctReportRows(rows, "group"), 2)
	group := "group"
	aggregated := aggregateReportRows(rows, contract.ReportDefinition{Columns: []string{"active_request_count"}, Grouping: &group})
	require.Len(t, aggregated, 2)
	aggregated = aggregateReportRows(rows, contract.ReportDefinition{Columns: []string{"count"}})
	require.Equal(t, 3, aggregated[0]["count"])

	require.True(t, reportValueMatches("A", []string{"B", "A"}))
	require.True(t, reportValueMatches(2, []any{1, 2}))
	require.False(t, reportValueMatches("A", []any{"B"}))
	require.False(t, reportValueMatches("A", []string{"B"}))
	require.True(t, reportValueMatches(true, true))
	for _, value := range []any{"value", true, float32(1.5), float64(2.5), int(1), int64(2), uint64(3), []string{"A"}, []any{"B"}} {
		require.True(t, validReportFilterValue(value), value)
	}
	for _, value := range []any{"", []string{}, []string{""}, []any{}, []any{nil}, map[string]any{"bad": true}} {
		require.False(t, validReportFilterValue(value), value)
	}

	row := map[string]any{"created_at": "2026-02-03T12:00:00Z", "status": "OPEN"}
	require.True(t, matchesReportFilters(row, map[string]any{
		"created_from": "2026-02-03T11:00:00Z", "created_to": "2026-02-03T13:00:00Z", "status": "OPEN",
	}, map[string]string{"created_from": "created_at", "created_to": "created_at"}))
	require.False(t, matchesReportFilters(row, map[string]any{"created_from": "2026-02-03T13:00:00Z"}, map[string]string{"created_from": "created_at"}))
	require.False(t, matchesReportFilters(row, map[string]any{"created_to": "2026-02-03T11:00:00Z"}, map[string]string{"created_to": "created_at"}))
	require.False(t, matchesReportFilters(row, map[string]any{"created_from": "invalid"}, map[string]string{"created_from": "created_at"}))
	require.False(t, matchesReportFilters(row, map[string]any{"status": "CLOSED"}, nil))

	require.Equal(t, "", reportValueString(nil))
	require.Equal(t, "true", reportValueString(true))
	require.Equal(t, "3", reportValueString(int64(3)))
	require.Equal(t, "4", reportValueString(uint64(4)))
	require.Equal(t, "2.5", reportValueString(2.5))
	require.Less(t, compareReportValues(1, 2), 0)
	require.Greater(t, compareReportValues("B", "A"), 0)
	require.True(t, reportSortContains([]string{"-status"}, "status"))
	require.False(t, reportSortContains([]string{"status"}, "created_at"))

	owner := contract.Actor{ID: 1, Roles: []contract.ApplicationRole{contract.ApplicationRoleSupervisor}, Department: contract.DepartmentStreets, Districts: []contract.DistrictCode{contract.DistrictNorth, contract.DistrictCentral}}
	payload := newSavedReportPayload(validReportDefinition("scope", "service_requests", "status"), owner)
	payload.SharedRoles = []contract.ApplicationRole{contract.ApplicationRoleServiceAgent}
	require.True(t, reportVisibleTo(contract.Actor{ID: 2, Roles: []contract.ApplicationRole{contract.ApplicationRoleServiceAgent}, Department: contract.DepartmentStreets, Districts: []contract.DistrictCode{contract.DistrictNorth}}, payload))
	require.False(t, reportVisibleTo(contract.Actor{ID: 3, Roles: []contract.ApplicationRole{contract.ApplicationRoleServiceAgent}, Department: contract.DepartmentSanitation, Districts: []contract.DistrictCode{contract.DistrictNorth}}, payload))
	require.False(t, reportVisibleTo(contract.Actor{ID: 4, Roles: []contract.ApplicationRole{contract.ApplicationRoleServiceAgent}, Department: contract.DepartmentStreets, Districts: []contract.DistrictCode{contract.DistrictSouth}}, payload))
}
