package city311

import (
	"context"
	"encoding/csv"
	"strconv"
	"strings"
	"testing"
	"time"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

func TestAuditListScopesFiltersAndPaginates(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	manager := seededAssignmentActor(t, ctx, svc, st, "department-manager@city311.example.invalid")
	administrator := seededAssignmentActor(t, ctx, svc, st, "platform-admin@city311.example.invalid")
	agent := seededAssignmentActor(t, ctx, svc, st, "service-agent@city311.example.invalid")
	outside := createAuditFixture(t, ctx, svc, st, contract.DepartmentSanitation, contract.DistrictSouth, "OUTSIDE_SCOPE")

	_, err := svc.ListAuditEvents(ctx, agent, contract.AuditQuery{})
	requireServiceError(t, err, 403, contract.ErrorForbidden)

	managerPage, err := svc.ListAuditEvents(ctx, manager, contract.AuditQuery{
		Filters: contract.AuditFilter{EventTypes: []string{"SEED_CREATED"}}, PageSize: 1, Sort: "occurred_at,entity_id",
	})
	require.NoError(t, err)
	require.Len(t, managerPage.Items, 1)
	require.NotNil(t, managerPage.NextPageToken)
	require.Greater(t, managerPage.TotalCount, 1)
	require.Equal(t, []string{"occurred_at", "entity_id"}, managerPage.Sort)

	nextPage, err := svc.ListAuditEvents(ctx, manager, contract.AuditQuery{
		Filters: contract.AuditFilter{EventTypes: []string{"SEED_CREATED"}}, PageSize: 1,
		PageToken: *managerPage.NextPageToken, Sort: "occurred_at,entity_id",
	})
	require.NoError(t, err)
	require.Len(t, nextPage.Items, 1)
	require.NotEqual(t, managerPage.Items[0].EntityID, nextPage.Items[0].EntityID)

	hidden, err := svc.ListAuditEvents(ctx, manager, contract.AuditQuery{
		Filters: contract.AuditFilter{EntityIDs: []string{outside.EntityID}},
	})
	require.NoError(t, err)
	require.Empty(t, hidden.Items)

	visible, err := svc.ListAuditEvents(ctx, administrator, contract.AuditQuery{
		Filters: contract.AuditFilter{
			EntityIDs: []string{outside.EntityID}, ActorTypes: []contract.AuditActorType{contract.AuditActorSystem},
			OccurredFrom: timePointer(svc.now().Add(-time.Minute)), OccurredTo: timePointer(svc.now().Add(time.Minute)),
		},
	})
	require.NoError(t, err)
	require.Len(t, visible.Items, 1)
	require.Equal(t, "OUTSIDE_SCOPE", visible.Items[0].EventType)
	require.Equal(t, contract.AuditActorSystem, visible.Items[0].ActorType)
}

func TestAuditCSVExportAndOperationOwnership(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	manager := seededAssignmentActor(t, ctx, svc, st, "department-manager@city311.example.invalid")
	administrator := seededAssignmentActor(t, ctx, svc, st, "platform-admin@city311.example.invalid")
	otherManager := manager
	otherManager.ID++

	pending, err := svc.StartAuditExport(ctx, manager, contract.AuditExport{Filters: contract.AuditFilter{
		EventTypes: []string{"SEED_CREATED"},
	}})
	require.NoError(t, err)
	require.Equal(t, contract.OperationStatusPending, pending.Status)
	require.Equal(t, 0, pending.Progress)
	require.Nil(t, pending.Result)
	require.Nil(t, pending.CompletedAt)

	completed, err := svc.GetOperation(ctx, manager, pending.OperationID)
	require.NoError(t, err)
	require.Equal(t, contract.OperationStatusSucceeded, completed.Status)
	require.Equal(t, 100, completed.Progress)
	require.NotNil(t, completed.CompletedAt)
	require.Equal(t, "/api/v1/operations/"+pending.OperationID+"/result", completed.Result["download_url"])

	_, err = svc.GetOperation(ctx, otherManager, pending.OperationID)
	requireServiceError(t, err, 403, contract.ErrorForbidden)
	_, err = svc.GetOperation(ctx, administrator, pending.OperationID)
	require.NoError(t, err)

	download, err := svc.DownloadOperation(ctx, manager, pending.OperationID)
	require.NoError(t, err)
	require.Equal(t, "text/csv; charset=utf-8", download.ContentType)
	require.True(t, strings.HasSuffix(download.Filename, ".csv"))
	require.Contains(t, string(download.Content), "\r\n")
	require.NotContains(t, strings.ReplaceAll(string(download.Content), "\r\n", ""), "\n")
	reader := csv.NewReader(strings.NewReader(string(download.Content)))
	rows, err := reader.ReadAll()
	require.NoError(t, err)
	require.Greater(t, len(rows), 1)
	require.Equal(t, []string{"entity_type", "entity_id", "event_type", "actor_type", "actor_id", "occurred_at", "source_channel", "before", "after"}, rows[0])
	for _, row := range rows[1:] {
		require.Equal(t, "SEED_CREATED", row[2])
		_, err = time.Parse(time.RFC3339Nano, row[5])
		require.NoError(t, err)
		require.True(t, strings.HasSuffix(row[5], "Z"))
	}

	exportEvents, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{EventType: "AUDIT_EXPORTED"})
	require.NoError(t, err)
	require.Len(t, exportEvents, 1)
	require.Equal(t, pending.OperationID, exportEvents[0].EntityID)
}

func TestAuditValidation(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	manager := seededAssignmentActor(t, ctx, svc, st, "department-manager@city311.example.invalid")
	agent := seededAssignmentActor(t, ctx, svc, st, "service-agent@city311.example.invalid")

	for _, query := range []contract.AuditQuery{
		{Filters: contract.AuditFilter{ActorTypes: []contract.AuditActorType{"UNKNOWN"}}},
		{Filters: contract.AuditFilter{SourceChannels: []contract.SourceChannel{"UNKNOWN"}}},
		{Filters: contract.AuditFilter{RequestIDs: []string{"invalid"}}},
		{Filters: contract.AuditFilter{ActorIDs: []string{"invalid"}}},
		{Filters: contract.AuditFilter{EntityTypes: []string{""}}},
		{Filters: contract.AuditFilter{EventTypes: []string{strings.Repeat("x", 97)}}},
		{Filters: contract.AuditFilter{OccurredFrom: timePointer(svc.now()), OccurredTo: timePointer(svc.now().Add(-time.Hour))}},
		{Sort: "unknown"},
		{Sort: "occurred_at,entity_type,entity_id,event_type"},
		{PageSize: 101},
		{PageToken: "invalid"},
	} {
		_, err := svc.ListAuditEvents(ctx, manager, query)
		requireServiceError(t, err, 422, contract.ErrorValidation)
	}
	offsetToken, err := encodePageToken(10000, []string{"-occurred_at"})
	require.NoError(t, err)
	_, err = svc.ListAuditEvents(ctx, manager, contract.AuditQuery{PageToken: offsetToken})
	requireServiceError(t, err, 422, contract.ErrorValidation)

	_, err = svc.StartAuditExport(ctx, agent, contract.AuditExport{})
	requireServiceError(t, err, 403, contract.ErrorForbidden)
	_, err = svc.StartAuditExport(ctx, manager, contract.AuditExport{Filters: contract.AuditFilter{EntityIDs: []string{""}}})
	requireServiceError(t, err, 422, contract.ErrorValidation)
	_, err = svc.GetOperation(ctx, manager, "invalid")
	requireServiceError(t, err, 422, contract.ErrorValidation)
	_, err = svc.GetOperation(ctx, manager, "op-0")
	requireServiceError(t, err, 422, contract.ErrorValidation)
	_, err = svc.GetOperation(ctx, manager, "op-999999999")
	requireServiceError(t, err, 404, contract.ErrorNotFound)
}

func TestAuditOperationUnavailableAndProjection(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	actor := contract.Actor{ID: 41, Roles: []contract.ApplicationRole{contract.ApplicationRoleDepartmentManager}}
	now := svc.now()
	operation := &composeTypes.City311Operation{
		ID: svc.nextID(), Kind: auditExportKind, Status: string(contract.OperationStatusPending), ActorID: actor.ID,
		Result: composeTypes.City311JSON{}, Error: composeTypes.City311JSON{}, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, store.CreateCity311Operation(ctx, st, operation))

	_, err := svc.DownloadOperation(ctx, actor, publicOperationID(operation.ID))
	requireServiceError(t, err, 404, contract.ErrorNotFound)
	_, err = svc.DownloadOperation(ctx, actor, "invalid")
	requireServiceError(t, err, 422, contract.ErrorValidation)

	operation.Result = composeTypes.City311JSON{"download_url": "/result"}
	operation.Error = composeTypes.City311JSON{
		"error": string(contract.ErrorValidation), "message": "invalid", "retryable": false,
	}
	projected := toOperation(operation)
	require.Equal(t, "/result", projected.Result["download_url"])
	require.NotNil(t, projected.Error)
	require.Equal(t, contract.ErrorValidation, projected.Error.Error)

	operation.Error = composeTypes.City311JSON{"retryable": "not-a-boolean"}
	require.Nil(t, toOperation(operation).Error)
	operation.Error = composeTypes.City311JSON{"unsupported": func() {}}
	require.Nil(t, toOperation(operation).Error)
}

func TestAuditSortingCSVAndScopeHelpers(t *testing.T) {
	now := time.Date(2026, time.September, 4, 12, 0, 0, 0, time.UTC)
	left := &composeTypes.City311AuditEvent{
		ID: 1, EntityType: "a", EntityID: "a", EventType: "a", ActorType: contract.AuditActorType("a"),
		ActorID: 1, SourceChannel: contract.SourceChannel("a"), CreatedAt: now,
		Before: composeTypes.City311JSON{}, After: composeTypes.City311JSON{},
	}
	right := &composeTypes.City311AuditEvent{
		ID: 2, EntityType: "b", EntityID: "b", EventType: "b", ActorType: contract.AuditActorType("b"),
		ActorID: 2, SourceChannel: contract.SourceChannel("b"), CreatedAt: now.Add(time.Second),
		Before: composeTypes.City311JSON{}, After: composeTypes.City311JSON{},
	}
	for _, field := range []string{"occurred_at", "entity_type", "entity_id", "event_type", "actor_type", "actor_id", "source_channel"} {
		require.Equal(t, -1, compareAuditField(left, right, field), field)
		require.Equal(t, 1, compareAuditField(right, left, field), field)
	}
	require.Zero(t, compareAuditField(left, right, "unknown"))
	require.Zero(t, compareUint64(4, 4))

	set := composeTypes.City311AuditEventSet{left, right}
	sortAuditEvents(set, []string{"-actor_id"})
	require.Equal(t, uint64(2), set[0].ID)
	sortAuditEvents(set, []string{"actor_id"})
	require.Equal(t, uint64(1), set[0].ID)
	tie := *left
	tie.ID = 3
	set = composeTypes.City311AuditEventSet{&tie, left}
	sortAuditEvents(set, []string{"entity_type"})
	require.Equal(t, uint64(1), set[0].ID)

	_, err := encodeAuditCSV(composeTypes.City311AuditEventSet{{Before: composeTypes.City311JSON{"bad": func() {}}}})
	require.Error(t, err)
	_, err = encodeAuditCSV(composeTypes.City311AuditEventSet{{Before: composeTypes.City311JSON{}, After: composeTypes.City311JSON{"bad": func() {}}}})
	require.Error(t, err)

	require.Equal(t, contract.DepartmentStreets, auditDepartment(&composeTypes.City311AuditEvent{After: composeTypes.City311JSON{"department_code": "STREETS"}}))
	require.Equal(t, contract.DepartmentSanitation, auditDepartment(&composeTypes.City311AuditEvent{After: composeTypes.City311JSON{"owning_department": "SANITATION"}}))
	require.Equal(t, contract.DepartmentGeneralServices, auditDepartment(&composeTypes.City311AuditEvent{Before: composeTypes.City311JSON{"department_code": "GENERAL_SERVICES"}}))
	require.Empty(t, auditDepartment(&composeTypes.City311AuditEvent{Before: composeTypes.City311JSON{}, After: composeTypes.City311JSON{}}))
	require.Nil(t, optionalActorDepartment(contract.Actor{}))
	require.Equal(t, contract.DepartmentStreets, optionalActorDepartment(contract.Actor{Department: contract.DepartmentStreets}))
}

func TestAuditNonRequestDepartmentAndMissingRequestScope(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	manager := contract.Actor{
		ID: 42, Roles: []contract.ApplicationRole{contract.ApplicationRoleDepartmentManager}, Department: contract.DepartmentStreets,
	}
	cache := map[uint64]*composeTypes.City311ServiceRequest{}
	allowed, err := svc.auditEventInScope(ctx, st, manager, &composeTypes.City311AuditEvent{
		After: composeTypes.City311JSON{"department_code": "STREETS"},
	}, cache)
	require.NoError(t, err)
	require.True(t, allowed)
	allowed, err = svc.auditEventInScope(ctx, st, manager, &composeTypes.City311AuditEvent{
		Before: composeTypes.City311JSON{"owning_department": "SANITATION"},
	}, cache)
	require.NoError(t, err)
	require.False(t, allowed)

	missing := &composeTypes.City311AuditEvent{RequestID: 999999999}
	allowed, err = svc.auditEventInScope(ctx, st, manager, missing, cache)
	require.NoError(t, err)
	require.False(t, allowed)
	allowed, err = svc.auditEventInScope(ctx, st, manager, missing, cache)
	require.NoError(t, err)
	require.False(t, allowed)

	agent := contract.Actor{ID: 43, Roles: []contract.ApplicationRole{contract.ApplicationRoleServiceAgent}}
	allowed, err = svc.auditEventInScope(ctx, st, agent, &composeTypes.City311AuditEvent{}, cache)
	require.NoError(t, err)
	require.False(t, allowed)
	administrator := contract.Actor{ID: 44, Roles: []contract.ApplicationRole{contract.ApplicationRolePlatformAdministrator}}
	allowed, err = svc.auditEventInScope(ctx, st, administrator, &composeTypes.City311AuditEvent{}, cache)
	require.NoError(t, err)
	require.True(t, allowed)
}

func TestAuditAppliedFiltersIncludesEveryFrozenField(t *testing.T) {
	from := time.Date(2026, time.September, 4, 8, 0, 0, 0, time.FixedZone("EDT", -4*60*60))
	to := from.Add(time.Hour)
	filters := contract.AuditFilter{
		RequestIDs: []string{"1"}, EntityTypes: []string{"request"}, EntityIDs: []string{"1"}, EventTypes: []string{"UPDATED"},
		ActorTypes: []contract.AuditActorType{contract.AuditActorStaff}, ActorIDs: []string{"2"},
		SourceChannels: []contract.SourceChannel{contract.SourceChannelAPI}, OccurredFrom: &from, OccurredTo: &to,
	}
	applied := auditAppliedFilters(filters)
	require.Len(t, applied, 9)
	require.Equal(t, "2026-09-04T12:00:00Z", applied["occurred_from"])
	require.Equal(t, "2026-09-04T13:00:00Z", applied["occurred_to"])
}

func createAuditFixture(t *testing.T, ctx context.Context, svc *Service, st store.Storer, department contract.DepartmentCode, district contract.DistrictCode, eventType string) *composeTypes.City311AuditEvent {
	t.Helper()
	request := &composeTypes.City311ServiceRequest{
		ID: svc.nextID(), RequestNumber: "SR-2026-00999", Summary: "Scoped audit fixture", Description: "A fixture used to verify department-scoped audit visibility.",
		ServiceType: contract.ServiceTypeMissedTrash, OwningDepartment: department, CouncilDistrict: district,
		SourceChannel: contract.SourceChannelAPI, OriginClass: contract.OriginClassExternal, Status: contract.ServiceRequestStatusSubmitted,
		PrimaryRequester: composeTypes.City311JSON{}, Location: composeTypes.City311JSON{}, CustomFields: composeTypes.City311JSON{},
		CollaboratorIDs: composeTypes.City311Uint64Set{}, Version: 1, CreatedAt: svc.now(), UpdatedAt: svc.now(),
	}
	require.NoError(t, store.CreateCity311ServiceRequest(ctx, st, request))
	event := &composeTypes.City311AuditEvent{
		ID: svc.nextID(), RequestID: request.ID, EntityType: "service_request", EntityID: strconv.FormatUint(request.ID, 10), EventType: eventType,
		ActorType: contract.AuditActorSystem, SourceChannel: contract.SourceChannelAPI,
		Before: composeTypes.City311JSON{}, After: composeTypes.City311JSON{"summary": "comma, quote \" value"}, CreatedAt: svc.now(),
	}
	require.NoError(t, store.CreateCity311AuditEvent(ctx, st, event))
	return event
}

func timePointer(value time.Time) *time.Time { return &value }
