package city311

import (
	"context"
	"strings"
	"testing"
	"time"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

func TestDataExportScopesFiltersAndPaginatesAllEntities(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	manager := seededAssignmentActor(t, ctx, svc, st, "department-manager@city311.example.invalid")
	administrator := seededAssignmentActor(t, ctx, svc, st, "platform-admin@city311.example.invalid")

	first, err := svc.ExportData(ctx, manager, "service-requests", contract.DataExportQuery{
		PageSize: 1, Filters: map[string][]string{"department": {"STREETS"}},
	})
	require.NoError(t, err)
	require.Len(t, first.Items, 1)
	require.NotNil(t, first.NextPageToken)
	require.NotEmpty(t, first.Items[0]["request_id"])
	require.Equal(t, string(contract.DepartmentStreets), first.Items[0]["owning_department"])

	second, err := svc.ExportData(ctx, manager, "service-requests", contract.DataExportQuery{
		PageSize: 1, PageToken: *first.NextPageToken, Filters: map[string][]string{"department": {"STREETS"}},
	})
	require.NoError(t, err)
	require.Len(t, second.Items, 1)
	require.NotEqual(t, first.Items[0]["request_id"], second.Items[0]["request_id"])

	outside, err := svc.ExportData(ctx, manager, "service-requests", contract.DataExportQuery{
		Filters: map[string][]string{"department": {"SANITATION"}},
	})
	require.NoError(t, err)
	require.Empty(t, outside.Items)

	for entity, requiredField := range map[string]string{
		"constituents": "constituent_id", "audit-events": "event_type", "follow-up-actions": "action_type",
	} {
		result, exportErr := svc.ExportData(ctx, administrator, entity, contract.DataExportQuery{PageSize: 100})
		require.NoError(t, exportErr, entity)
		require.NotEmpty(t, result.Items, entity)
		require.Contains(t, result.Items[0], requiredField, entity)
		require.Nil(t, result.NextPageToken, entity)
		require.Equal(t, svc.now(), result.GeneratedAt)
	}

	future := svc.now().Add(time.Hour)
	empty, err := svc.ExportData(ctx, administrator, "constituents", contract.DataExportQuery{UpdatedSince: &future})
	require.NoError(t, err)
	require.Empty(t, empty.Items)
	require.Nil(t, empty.NextPageToken)

	audits, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{EventType: dataExportAuditEvent})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(audits), 7)
	require.Equal(t, contract.AuditActorIntegrationClient, audits[0].ActorType)
}

func TestDataExportRejectsInvalidEntityFilterAndPageToken(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	administrator := seededAssignmentActor(t, ctx, svc, st, "platform-admin@city311.example.invalid")

	for _, test := range []struct {
		entity string
		query  contract.DataExportQuery
		status int
		code   contract.ErrorCode
	}{
		{entity: "unknown", status: 422, code: contract.ErrorInvalidFilter},
		{entity: "service-requests", query: contract.DataExportQuery{PageSize: 101}, status: 422, code: contract.ErrorInvalidFilter},
		{entity: "service-requests", query: contract.DataExportQuery{Filters: map[string][]string{"unknown": {"value"}}}, status: 422, code: contract.ErrorInvalidFilter},
		{entity: "service-requests", query: contract.DataExportQuery{Filters: map[string][]string{"status": {"UNKNOWN"}}}, status: 422, code: contract.ErrorInvalidFilter},
		{entity: "service-requests", query: contract.DataExportQuery{PageToken: "invalid"}, status: 400, code: contract.ErrorInvalidPageToken},
	} {
		_, err := svc.ExportData(ctx, administrator, test.entity, test.query)
		requireServiceError(t, err, test.status, test.code)
	}

	page, err := svc.ExportData(ctx, administrator, "service-requests", contract.DataExportQuery{PageSize: 1})
	require.NoError(t, err)
	require.NotNil(t, page.NextPageToken)
	_, err = svc.ExportData(ctx, administrator, "service-requests", contract.DataExportQuery{
		PageSize: 1, PageToken: *page.NextPageToken, Filters: map[string][]string{"status": {"SUBMITTED"}},
	})
	requireServiceError(t, err, 400, contract.ErrorInvalidPageToken)
}

func TestDataExportRateLimitIsPerClientAndResetsByMinute(t *testing.T) {
	svc, _ := testService(t)
	for i := 0; i < dataExportLimitPerMinute; i++ {
		require.Zero(t, svc.CheckDataExportLimit(41))
	}
	require.Equal(t, 55, svc.CheckDataExportLimit(41))
	require.Zero(t, svc.CheckDataExportLimit(42))

	previous := svc.now
	svc.now = func() time.Time { return previous().Add(time.Minute) }
	require.Zero(t, svc.CheckDataExportLimit(41))
}

func TestDataExportFilterValidationCoversEveryTypedFilter(t *testing.T) {
	valid := map[string][]string{
		"request_id": {"1"}, "actor_id": {"2"},
		"status":             {string(contract.ServiceRequestStatuses[0])},
		"service_type":       {string(contract.ServiceTypes[0])},
		"department":         {string(contract.DepartmentCodes[0])},
		"district":           {string(contract.DistrictCodes[0])},
		"origin_class":       {string(contract.OriginClasses[0])},
		"source_channel":     {string(contract.SourceChannels[0])},
		"primary_category":   {string(contract.ContactCategories[0])},
		"category":           {string(contract.ContactCategories[0])},
		"preferred_language": {string(contract.Languages[0])},
		"actor_type":         {string(contract.AuditActorTypes[0])},
		"email_opt_out":      {"false"},
		"visibility":         {"PUBLIC"},
	}
	require.NoError(t, validateDataExportFilters("constituents", valid))

	for name, filters := range map[string]map[string][]string{
		"request identifier": {"request_id": {"0"}},
		"actor identifier":   {"actor_id": {"invalid"}},
		"status":             {"status": {"UNKNOWN"}},
		"service type":       {"service_type": {"UNKNOWN"}},
		"department":         {"department": {"UNKNOWN"}},
		"district":           {"district": {"UNKNOWN"}},
		"origin class":       {"origin_class": {"UNKNOWN"}},
		"source channel":     {"source_channel": {"UNKNOWN"}},
		"primary category":   {"primary_category": {"UNKNOWN"}},
		"category":           {"category": {"UNKNOWN"}},
		"language":           {"preferred_language": {"UNKNOWN"}},
		"actor type":         {"actor_type": {"UNKNOWN"}},
		"email opt out":      {"email_opt_out": {"yes"}},
		"visibility":         {"visibility": {"PRIVATE"}},
	} {
		t.Run(name, func(t *testing.T) {
			requireServiceError(t, validateDataExportFilters("constituents", filters), 422, contract.ErrorInvalidFilter)
		})
	}
}

func TestNormalizeDataExportFiltersRejectsEmptyAndOversizedValues(t *testing.T) {
	allowed := map[string]bool{"department": true}
	normalized, err := normalizeDataExportFilters(map[string][]string{" department ": {" STREETS "}}, allowed)
	require.NoError(t, err)
	require.Equal(t, []string{"STREETS"}, normalized["department"])

	for name, filters := range map[string]map[string][]string{
		"unsupported": {"unknown": {"value"}},
		"no values":   {"department": nil},
		"empty":       {"department": {" "}},
		"oversized":   {"department": {strings.Repeat("x", 161)}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err = normalizeDataExportFilters(filters, allowed)
			requireServiceError(t, err, 422, contract.ErrorInvalidFilter)
		})
	}
}

func TestDataExportProjectionAndScopeHelpers(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	administrator := seededAssignmentActor(t, ctx, svc, st, "platform-admin@city311.example.invalid")

	projected := projectExportConstituent(&composeTypes.City311Constituent{ConstituentID: "fallback"})
	require.Equal(t, "fallback", projected.ConstituentID)
	require.Empty(t, projected.Emails)
	require.Empty(t, projected.PhoneNumbers)
	require.Empty(t, projected.Addresses)

	event := &composeTypes.City311AuditEvent{
		RequestID: 9, EventType: "NOTE_CREATED", ActorType: contract.AuditActorStaff, ActorID: 7,
		After: composeTypes.City311JSON{"portal_visible": true}, CreatedAt: svc.now(),
	}
	action := projectFollowUpAction(event)
	require.Equal(t, "PUBLIC", action.Visibility)
	require.Equal(t, "staff:7", action.Actor)
	require.NotEmpty(t, action.LocalDisplayTime)

	inScope, err := svc.dataExportEventInScope(ctx, administrator, "follow-up-actions", &composeTypes.City311AuditEvent{}, map[uint64]*composeTypes.City311ServiceRequest{})
	require.NoError(t, err)
	require.False(t, inScope)
}
