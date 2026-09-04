package city311

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

func recordReadActor(role contract.ApplicationRole) contract.Actor {
	return contract.Actor{ID: 70, Roles: []contract.ApplicationRole{role}, Department: contract.DepartmentStreets, Districts: []contract.DistrictCode{contract.DistrictNorth}}
}

func readStatus(t *testing.T, err error, status int) {
	t.Helper()
	var failure *ServiceError
	require.ErrorAs(t, err, &failure)
	require.Equal(t, status, failure.Status)
}

func createReadConstituent(t *testing.T, st store.Storer, id uint64, department contract.DepartmentCode, district contract.DistrictCode) *composeTypes.City311Constituent {
	t.Helper()
	item := &composeTypes.City311Constituent{ID: id, ConstituentID: fmt.Sprintf("C-%04d", id), OwningDepartment: department, CouncilDistrict: district,
		Profile:   composeTypes.City311JSON{"constituent_id": "untrusted", "display_name": fmt.Sprintf("Resident %04d", id), "primary_category": "RESIDENT", "additional_categories": []string{}, "emails": []string{fmt.Sprintf("resident%d@example.invalid", id)}, "phone_numbers": []map[string]string{{"label": "mobile", "value": "+17165550101"}}, "preferred_language": "EN", "_profile_version": "77", "custom_fields": map[string]any{"ward_note": "visible"}},
		CreatedAt: time.Date(2026, 2, 3, 10, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 2, 3, 10, 0, 0, 0, time.UTC)}
	require.NoError(t, store.CreateCity311Constituent(context.Background(), st, item))
	return item
}

func TestScopedReadConstituentScopePaginationAndProjection(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	// More than two raw storage pages are invisible to the caller. They must
	// neither terminate the scan nor contribute to its count or cursor.
	for id := uint64(1); id <= 251; id++ {
		createReadConstituent(t, st, id, contract.DepartmentSanitation, contract.DistrictNorth)
	}
	for id := uint64(300); id < 303; id++ {
		createReadConstituent(t, st, id, contract.DepartmentStreets, contract.DistrictNorth)
	}
	createReadConstituent(t, st, 400, contract.DepartmentStreets, contract.DistrictSouth)
	agent := recordReadActor(contract.ApplicationRoleServiceAgent)
	query := RecordReadQuery{PageSize: 2}
	first, err := svc.ListConstituents(ctx, agent, query)
	require.NoError(t, err)
	require.Equal(t, 3, first.TotalCount)
	require.Len(t, first.Items, 2)
	require.Equal(t, "C-0300", first.Items[0]["constituent_id"])
	require.NotContains(t, first.Items[0], "_profile_version")
	require.Contains(t, first.Items[0], "custom_fields")
	require.NotNil(t, first.NextPageToken)
	query.PageToken = *first.NextPageToken
	second, err := New(st).ListConstituents(ctx, agent, query)
	require.NoError(t, err)
	require.Len(t, second.Items, 1)
	require.Equal(t, "C-0302", second.Items[0]["constituent_id"])
	require.Nil(t, second.NextPageToken)
	_, err = svc.ListConstituents(ctx, recordReadActor(contract.ApplicationRoleDepartmentManager), query)
	readStatus(t, err, 422)
	query.Filters = map[string]string{"q": "Resident"}
	_, err = svc.ListConstituents(ctx, agent, query)
	readStatus(t, err, 422)
	for _, tc := range []struct {
		role  contract.ApplicationRole
		count int
	}{
		{contract.ApplicationRoleSupervisor, 3}, {contract.ApplicationRoleDepartmentManager, 4}, {contract.ApplicationRolePlatformAdministrator, 255},
	} {
		result, err := svc.ListConstituents(ctx, recordReadActor(tc.role), RecordReadQuery{})
		require.NoError(t, err)
		require.Equal(t, tc.count, result.TotalCount)
	}
	detail, err := svc.FindConstituent(ctx, agent, "C-0300")
	require.NoError(t, err)
	require.Equal(t, first.Items[0], detail)
	_, err = svc.FindConstituent(ctx, agent, "C-0400")
	readStatus(t, err, 403)
	_, err = svc.FindConstituent(ctx, agent, "C-missing")
	readStatus(t, err, 404)
	for _, actor := range []contract.Actor{recordReadActor(contract.ApplicationRoleWorkflowDesigner), recordReadActor(contract.ApplicationRoleConstituent), {ID: 70, Roles: []contract.ApplicationRole{contract.ApplicationRoleDepartmentManager}}} {
		_, err = svc.ListConstituents(ctx, actor, RecordReadQuery{})
		readStatus(t, err, 403)
		_, err = svc.FindConstituent(ctx, actor, "C-0300")
		readStatus(t, err, 403)
	}
	_, err = svc.ListConstituents(ctx, contract.Actor{}, RecordReadQuery{})
	readStatus(t, err, 401)
	stored, err := store.LookupCity311ConstituentByID(ctx, st, 300)
	require.NoError(t, err)
	require.Equal(t, "77", stored.Profile["_profile_version"])
}

func TestScopedReadConstituentFiltersAndValidation(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	createReadConstituent(t, st, 1, contract.DepartmentStreets, contract.DistrictNorth)
	createReadConstituent(t, st, 2, contract.DepartmentStreets, contract.DistrictNorth)
	actor := recordReadActor(contract.ApplicationRoleServiceAgent)
	for _, filters := range []map[string]string{
		{"q": "RESIDENT 0001"}, {"email": " RESIDENT1@EXAMPLE.INVALID "},
		{"q": "+1716", "department": "STREETS", "district": "NORTH", "category": "RESIDENT", "email": "resident1@example.invalid"},
	} {
		page, err := svc.ListConstituents(ctx, actor, RecordReadQuery{Filters: filters})
		require.NoError(t, err)
		require.Equal(t, 1, page.TotalCount)
	}
	for _, filters := range []map[string]string{{"q": "absent"}, {"email": "absent@example.invalid"}, {"department": "SANITATION"}, {"district": "SOUTH"}, {"category": "BUSINESS"}} {
		page, err := svc.ListConstituents(ctx, actor, RecordReadQuery{Filters: filters})
		require.NoError(t, err)
		require.Empty(t, page.Items)
		require.NotNil(t, page.Items)
		require.Zero(t, page.TotalCount)
		require.Nil(t, page.NextPageToken)
	}
	for _, filters := range []map[string]string{{"email": "bad"}, {"category": "bad"}, {"department": "bad"}, {"district": "bad"}, {"unknown": "value"}, {"q": string(make([]byte, 501))}} {
		_, err := svc.ListConstituents(ctx, actor, RecordReadQuery{Filters: filters})
		readStatus(t, err, 422)
	}
	_, err := svc.ListConstituents(ctx, actor, RecordReadQuery{Filters: map[string]string{"a~/b": "value"}})
	var failure *ServiceError
	require.ErrorAs(t, err, &failure)
	require.Equal(t, "/query/filters/a~0~1b", failure.Payload.Errors[0].Field)
	for _, query := range []RecordReadQuery{{PageSize: 101}, {Sort: "bad"}, {Sort: "display_name,-display_name"}, {Sort: "display_name,constituent_id,primary_category,updated_at"}, {PageToken: "!"}, {PageToken: base64.RawURLEncoding.EncodeToString([]byte(`{"offset":-1}`))}} {
		_, err = svc.ListConstituents(ctx, actor, query)
		readStatus(t, err, 422)
	}
	page, err := svc.ListConstituents(ctx, actor, RecordReadQuery{Sort: " -display_name , constituent_id"})
	require.NoError(t, err)
	require.Equal(t, "C-0002", page.Items[0]["constituent_id"])
	require.Equal(t, []string{"-display_name", "constituent_id"}, page.Sort)
	_, err = svc.ListConstituents(ctx, actor, RecordReadQuery{Filters: map[string]string{"q": strings.Repeat("é", 500)}})
	require.NoError(t, err)
}

func TestScopedReadProjectionErrorsAndCursorBoundaries(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	actor := recordReadActor(contract.ApplicationRoleServiceAgent)
	item := createReadConstituent(t, st, 1, contract.DepartmentStreets, contract.DistrictNorth)
	item.Profile["emails"], item.Profile["phone_numbers"], item.Profile["addresses"] = nil, nil, nil
	require.NoError(t, store.UpdateCity311Constituent(ctx, st, item))
	detail, err := svc.FindConstituent(ctx, actor, item.ConstituentID)
	require.NoError(t, err)
	for _, key := range []string{"emails", "phone_numbers", "addresses"} {
		require.Equal(t, []any{}, detail[key])
	}
	item.Profile["emails"] = 5
	require.NoError(t, store.UpdateCity311Constituent(ctx, st, item))
	_, err = svc.FindConstituent(ctx, actor, item.ConstituentID)
	require.Error(t, err)
	_, err = svc.ListConstituents(ctx, actor, RecordReadQuery{})
	require.Error(t, err)
	item.Profile = composeTypes.City311JSON{"not_serializable": make(chan int)}
	_, _, err = constituentReadValues(item)
	require.Error(t, err)
	query := RecordReadQuery{PageSize: 1, Filters: map[string]string{}}
	records := []readableRecord{{id: 2, values: map[string]any{"id": "2"}}, {id: 1, values: map[string]any{"id": "1"}}}
	page, err := readPage("test", actor, query, []string{"name"}, records)
	require.NoError(t, err)
	require.Equal(t, "1", page.Items[0]["id"])
	data, err := base64.RawURLEncoding.DecodeString(*page.NextPageToken)
	require.NoError(t, err)
	var cursor readCursor
	require.NoError(t, json.Unmarshal(data, &cursor))
	cursor.Offset = 200
	data, err = json.Marshal(cursor)
	require.NoError(t, err)
	query.PageToken = base64.RawURLEncoding.EncodeToString(data)
	empty, err := readPage("test", actor, query, []string{"name"}, records)
	require.NoError(t, err)
	require.Empty(t, empty.Items)
	require.Equal(t, 2, empty.TotalCount)
	_, err = readPage("other-operation", actor, query, []string{"name"}, records)
	readStatus(t, err, 422)
	actor.Districts = []contract.DistrictCode{contract.DistrictSouth}
	_, err = readPage("test", actor, query, []string{"name"}, records)
	readStatus(t, err, 422)
}

func TestScopedReadAuditDepartmentAndGlobalBoundary(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	createReadConstituent(t, st, 1, contract.DepartmentStreets, contract.DistrictSouth)
	createReadConstituent(t, st, 2, contract.DepartmentSanitation, contract.DistrictNorth)
	createReadConstituent(t, st, 3, "", "")
	for _, request := range []*composeTypes.City311ServiceRequest{
		{ID: 1, RequestNumber: "SR-2026-00001", OwningDepartment: contract.DepartmentStreets, CouncilDistrict: contract.DistrictSouth, CreatedAt: svc.now(), UpdatedAt: svc.now()},
		{ID: 2, RequestNumber: "SR-2026-00002", OwningDepartment: contract.DepartmentSanitation, CouncilDistrict: contract.DistrictNorth, CreatedAt: svc.now(), UpdatedAt: svc.now()},
	} {
		require.NoError(t, store.CreateCity311ServiceRequest(ctx, st, request))
	}
	for id := uint64(1); id <= 251; id++ {
		require.NoError(t, store.CreateCity311AuditEvent(ctx, st, &composeTypes.City311AuditEvent{ID: id, RequestID: 2, EntityType: "service_request", EntityID: "2", EventType: "changed", ActorType: contract.AuditActorStaff, ActorID: 70, SourceChannel: contract.SourceChannelStaffInPerson, CreatedAt: svc.now()}))
	}
	for index, event := range []*composeTypes.City311AuditEvent{
		{RequestID: 1, EntityType: "service_request", EntityID: "1"},
		{EntityType: "constituent", EntityID: "C-0001"},
		{EntityType: "constituent", EntityID: "C-0002"},
		{EntityType: "constituent", EntityID: "C-0003"},
		{EntityType: "constituent", EntityID: "C-missing"},
		{RequestID: 999, EntityType: "service_request", EntityID: "999"},
		{EntityType: "identity", EntityID: "70", After: composeTypes.City311JSON{"department": "STREETS"}},
	} {
		event.ID, event.ActorID, event.ActorType, event.SourceChannel, event.EventType = uint64(300+index), 70, contract.AuditActorStaff, contract.SourceChannelStaffInPerson, "changed"
		event.CreatedAt = svc.now().Add(time.Duration(index) * time.Nanosecond)
		require.NoError(t, store.CreateCity311AuditEvent(ctx, st, event))
	}
	manager := recordReadActor(contract.ApplicationRoleDepartmentManager)
	page, err := svc.ListAuditEvents(ctx, manager, RecordReadQuery{PageSize: 1})
	require.NoError(t, err)
	require.Equal(t, 2, page.TotalCount)
	require.Equal(t, "service_request", page.Items[0]["entity_type"])
	require.NotContains(t, page.Items[0], "id")
	require.NotNil(t, page.NextPageToken)
	second, err := New(st).ListAuditEvents(ctx, manager, RecordReadQuery{PageSize: 1, PageToken: *page.NextPageToken})
	require.NoError(t, err)
	require.Equal(t, "C-0001", second.Items[0]["entity_id"])
	require.Nil(t, second.NextPageToken)
	admin := recordReadActor(contract.ApplicationRolePlatformAdministrator)
	all, err := svc.ListAuditEvents(ctx, admin, RecordReadQuery{})
	require.NoError(t, err)
	require.Equal(t, 258, all.TotalCount)
	for _, actor := range []contract.Actor{recordReadActor(contract.ApplicationRoleServiceAgent), recordReadActor(contract.ApplicationRoleSupervisor), recordReadActor(contract.ApplicationRoleWorkflowDesigner), recordReadActor(contract.ApplicationRoleConstituent)} {
		_, err = svc.ListAuditEvents(ctx, actor, RecordReadQuery{})
		readStatus(t, err, 403)
	}
	for _, filters := range []map[string]string{
		{"entity_type": "service_request", "entity_id": "1", "event_type": "changed", "actor_id": "70", "request_id": "1", "source_channel": "STAFF_IN_PERSON"},
		{"from": svc.now().Format(time.RFC3339Nano), "to": svc.now().Format(time.RFC3339Nano)},
	} {
		filtered, err := svc.ListAuditEvents(ctx, manager, RecordReadQuery{Filters: filters})
		require.NoError(t, err)
		require.Equal(t, 1, filtered.TotalCount)
	}
	for _, filters := range []map[string]string{{"actor_id": "0"}, {"request_id": "-1"}, {"from": "bad"}, {"to": "bad"}, {"from": "2026-02-04T00:00:00Z", "to": "2026-02-03T00:00:00Z"}, {"source_channel": "bad"}} {
		_, err = svc.ListAuditEvents(ctx, manager, RecordReadQuery{Filters: filters})
		readStatus(t, err, 422)
	}
	filtered, err := svc.ListAuditEvents(ctx, manager, RecordReadQuery{Filters: map[string]string{"event_type": "absent"}})
	require.NoError(t, err)
	require.Zero(t, filtered.TotalCount)
	// Every read is side-effect free, including denied requests and bad filters.
	again, err := svc.ListAuditEvents(ctx, admin, RecordReadQuery{})
	require.NoError(t, err)
	require.Equal(t, all.TotalCount, again.TotalCount)
}
