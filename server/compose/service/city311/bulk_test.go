package city311

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

func TestDuplicateGroupMembershipLifecycle(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	request := seededBulkRequest(t, ctx, st, "SR-2026-00034")
	supervisor := seededAssignmentActor(t, ctx, svc, st, "supervisor@city311.example.invalid")
	manager := seededAssignmentActor(t, ctx, svc, st, "department-manager@city311.example.invalid")

	_, err := svc.ConfirmDuplicateGroup(ctx, manager, request.ID, 1, contract.DuplicateGroupChange{DuplicateGroupID: "DG-100", Reason: "Manager cannot confirm membership."})
	requireServiceError(t, err, 403, contract.ErrorForbidden)
	confirmed, err := svc.ConfirmDuplicateGroup(ctx, supervisor, request.ID, 1, contract.DuplicateGroupChange{
		DuplicateGroupID: " DG-100 ", Reason: " Same service type, time window, and location. ",
	})
	require.NoError(t, err)
	require.Equal(t, "DG-100", confirmed.Request.DuplicateGroupID)
	require.Equal(t, uint64(2), confirmed.Request.Version)
	require.Equal(t, "DUPLICATE_GROUP_CONFIRMED", confirmed.Audit[len(confirmed.Audit)-1].EventType)
	require.Equal(t, "DG-100", confirmed.Audit[len(confirmed.Audit)-1].After["duplicate_group_id"])

	replayed, err := svc.ConfirmDuplicateGroup(ctx, supervisor, request.ID, 2, contract.DuplicateGroupChange{
		DuplicateGroupID: "DG-100", Reason: "Membership is already confirmed.",
	})
	require.NoError(t, err)
	require.Equal(t, uint64(3), replayed.Request.Version)
	require.Len(t, replayed.Audit, len(confirmed.Audit)+1)

	removed, err := svc.RemoveDuplicateGroup(ctx, supervisor, request.ID, 3, contract.Reason{Reason: "Reports concern separate road defects."})
	require.NoError(t, err)
	require.Empty(t, removed.Request.DuplicateGroupID)
	require.Equal(t, uint64(4), removed.Request.Version)
	require.Equal(t, "DUPLICATE_GROUP_REMOVED", removed.Audit[len(removed.Audit)-1].EventType)
	_, err = svc.RemoveDuplicateGroup(ctx, supervisor, request.ID, 4, contract.Reason{Reason: "Already removed."})
	requireServiceError(t, err, 404, contract.ErrorNotFound)
	_, err = svc.ConfirmDuplicateGroup(ctx, supervisor, request.ID, 3, contract.DuplicateGroupChange{DuplicateGroupID: "DG-200", Reason: "Stale update."})
	requireServiceError(t, err, 409, contract.ErrorVersionConflict)
}

func TestSubmissionAutomaticallyQualifiesSameIssueCandidates(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	draft := seededBulkRequest(t, ctx, st, "SR-2026-00033")
	candidate := seededBulkRequest(t, ctx, st, "SR-2026-00034")

	created, _, err := svc.Submit(ctx, validSubmission(), "automatic-duplicate", SubmissionOptions{
		Operation: "service_request_create", SourceChannel: contract.SourceChannelAPI,
		ActorType: contract.AuditActorIntegrationClient, ActorID: 77, RequireIdempotency: true,
	})
	require.NoError(t, err)
	createdID, err := strconv.ParseUint(created.RequestID, 10, 64)
	require.NoError(t, err)
	stored, err := store.LookupCity311ServiceRequestByID(ctx, st, createdID)
	require.NoError(t, err)
	require.NotEmpty(t, stored.DuplicateGroupID)
	require.Equal(t, 1, stored.Version, "automatic qualification is part of initial creation")

	candidate, err = store.LookupCity311ServiceRequestByID(ctx, st, candidate.ID)
	require.NoError(t, err)
	require.Equal(t, stored.DuplicateGroupID, candidate.DuplicateGroupID)
	require.Equal(t, 2, candidate.Version)
	audits, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{
		RequestID: candidate.ID, EventType: "DUPLICATE_GROUP_AUTO_QUALIFIED",
	})
	require.NoError(t, err)
	require.Len(t, audits, 1)
	require.EqualValues(t, 24, audits[0].After["window_hours"])
	require.EqualValues(t, 50, audits[0].After["radius_metres"])

	draft, err = store.LookupCity311ServiceRequestByID(ctx, st, draft.ID)
	require.NoError(t, err)
	require.Empty(t, draft.DuplicateGroupID, "unsubmitted drafts are not same-issue candidates")
	require.Equal(t, 1, draft.Version)
}

func TestBulkUpdateIsAtomicAuditedAndIdempotent(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	supervisor := seededAssignmentActor(t, ctx, svc, st, "supervisor@city311.example.invalid")
	agent := seededAssignmentUser(t, ctx, st, "service-agent@city311.example.invalid")
	first := seededBulkRequest(t, ctx, st, "SR-2026-00034")
	second := seededBulkRequest(t, ctx, st, "SR-2026-00035")
	setDuplicateGroup(t, ctx, st, first, "DG-BULK")
	setDuplicateGroup(t, ctx, st, second, "DG-BULK")

	input := contract.BulkRequest{
		RequestItems: []contract.BulkRequestItem{
			{RequestID: strconv.FormatUint(first.ID, 10), ExpectedVersion: 1},
			{RequestID: strconv.FormatUint(second.ID, 10), ExpectedVersion: 1},
		},
		Action: contract.BulkActionUpdate,
		Changes: bulkChanges(t, map[string]any{
			"primary_assignee_id": strconv.FormatUint(agent.ID, 10), "priority": "HIGH",
			"status": string(contract.ServiceRequestStatusTriaged), "staff_note": "Road crew should inspect both reports together.",
		}),
	}
	result, err := svc.Bulk(ctx, supervisor, input, "bulk-key-1")
	require.NoError(t, err)
	require.Equal(t, 2, result.UpdatedCount)
	require.Equal(t, []string{strconv.FormatUint(first.ID, 10), strconv.FormatUint(second.ID, 10)}, result.UpdatedRequestIDs)

	for _, requestID := range []uint64{first.ID, second.ID} {
		stored, err := store.LookupCity311ServiceRequestByID(ctx, st, requestID)
		require.NoError(t, err)
		require.Equal(t, 2, stored.Version)
		require.Equal(t, agent.ID, stored.PrimaryAssigneeID)
		require.Equal(t, "HIGH", stored.CustomFields["priority"])
		require.Equal(t, contract.ServiceRequestStatusTriaged, stored.Status)
		notes, _, err := store.SearchCity311RequestNotes(ctx, st, composeTypes.City311RequestNoteFilter{RequestID: requestID})
		require.NoError(t, err)
		require.Len(t, notes, 1)
		require.False(t, notes[0].PortalVisible)
		audits, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{RequestID: requestID, EventType: "SERVICE_REQUEST_BULK_UPDATED"})
		require.NoError(t, err)
		require.Len(t, audits, 1)
		require.EqualValues(t, 2, audits[0].After["selected_count"])
	}

	replay, err := svc.Bulk(ctx, supervisor, input, "bulk-key-1")
	require.NoError(t, err)
	require.Equal(t, result, replay)
	firstAfterReplay, err := store.LookupCity311ServiceRequestByID(ctx, st, first.ID)
	require.NoError(t, err)
	require.Equal(t, 2, firstAfterReplay.Version)

	conflicting := input
	conflicting.Changes = bulkChanges(t, map[string]any{"priority": "LOW"})
	_, err = svc.Bulk(ctx, supervisor, conflicting, "bulk-key-1")
	requireServiceError(t, err, 409, contract.ErrorIdempotencyConflict)

	stale := input
	stale.Changes = bulkChanges(t, map[string]any{"priority": "MEDIUM"})
	stale.RequestItems[0].ExpectedVersion = 2
	stale.RequestItems[1].ExpectedVersion = 1
	_, err = svc.Bulk(ctx, supervisor, stale, "bulk-key-stale")
	var serviceErr *ServiceError
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, contract.ErrorVersionConflict, serviceErr.Payload.Error)
	require.Equal(t, strconv.FormatUint(second.ID, 10), serviceErr.Payload.FailingRequestID)
	unchanged, err := store.LookupCity311ServiceRequestByID(ctx, st, first.ID)
	require.NoError(t, err)
	require.Equal(t, 2, unchanged.Version)
	require.Equal(t, "HIGH", unchanged.CustomFields["priority"])
}

func TestBulkCloseRequiresResolvedAndRollsBack(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	manager := seededAssignmentActor(t, ctx, svc, st, "department-manager@city311.example.invalid")
	first := seededBulkRequest(t, ctx, st, "SR-2026-00038")
	second := seededBulkRequest(t, ctx, st, "SR-2026-00037")
	setDuplicateGroup(t, ctx, st, first, "DG-CLOSE")
	setDuplicateGroup(t, ctx, st, second, "DG-CLOSE")
	input := contract.BulkRequest{
		RequestItems: []contract.BulkRequestItem{
			{RequestID: strconv.FormatUint(first.ID, 10), ExpectedVersion: 1},
			{RequestID: strconv.FormatUint(second.ID, 10), ExpectedVersion: 1},
		},
		Action: contract.BulkActionClose, Changes: bulkChanges(t, map[string]any{}),
	}

	_, err := svc.Bulk(ctx, manager, input, "bulk-close-ineligible")
	var serviceErr *ServiceError
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, contract.ErrorInvalidStatusTransition, serviceErr.Payload.Error)
	require.Equal(t, strconv.FormatUint(second.ID, 10), serviceErr.Payload.FailingRequestID)
	unchanged, err := store.LookupCity311ServiceRequestByID(ctx, st, first.ID)
	require.NoError(t, err)
	require.Equal(t, contract.ServiceRequestStatusResolved, unchanged.Status)
	require.Equal(t, 1, unchanged.Version)

	second.Status = contract.ServiceRequestStatusResolved
	require.NoError(t, store.UpdateCity311ServiceRequest(ctx, st, second))
	result, err := svc.Bulk(ctx, manager, input, "bulk-close-success")
	require.NoError(t, err)
	require.Equal(t, 2, result.UpdatedCount)
	for _, requestID := range []uint64{first.ID, second.ID} {
		closed, err := store.LookupCity311ServiceRequestByID(ctx, st, requestID)
		require.NoError(t, err)
		require.Equal(t, contract.ServiceRequestStatusClosed, closed.Status)
		require.Equal(t, 2, closed.Version)
	}
}

func TestBulkRejectsDifferentGroupsAndUnknownChanges(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	supervisor := seededAssignmentActor(t, ctx, svc, st, "supervisor@city311.example.invalid")
	first := seededBulkRequest(t, ctx, st, "SR-2026-00034")
	second := seededBulkRequest(t, ctx, st, "SR-2026-00035")
	setDuplicateGroup(t, ctx, st, first, "DG-A")
	setDuplicateGroup(t, ctx, st, second, "DG-B")

	input := contract.BulkRequest{
		RequestItems: []contract.BulkRequestItem{
			{RequestID: strconv.FormatUint(first.ID, 10), ExpectedVersion: 1},
			{RequestID: strconv.FormatUint(second.ID, 10), ExpectedVersion: 1},
		},
		Action: contract.BulkActionUpdate, Changes: bulkChanges(t, map[string]any{"priority": "HIGH"}),
	}
	_, err := svc.Bulk(ctx, supervisor, input, "different-groups")
	var serviceErr *ServiceError
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, contract.ErrorValidation, serviceErr.Payload.Error)
	require.Equal(t, strconv.FormatUint(second.ID, 10), serviceErr.Payload.FailingRequestID)

	input.Changes = bulkChanges(t, map[string]any{"unsupported": true})
	_, err = svc.Bulk(ctx, supervisor, input, "unknown-change")
	requireServiceError(t, err, 422, contract.ErrorValidation)
}

func seededBulkRequest(t *testing.T, ctx context.Context, st store.Storer, number string) *composeTypes.City311ServiceRequest {
	t.Helper()
	request, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, number)
	require.NoError(t, err)
	return request
}

func setDuplicateGroup(t *testing.T, ctx context.Context, st store.Storer, request *composeTypes.City311ServiceRequest, group string) {
	t.Helper()
	request.DuplicateGroupID = group
	require.NoError(t, store.UpdateCity311ServiceRequest(ctx, st, request))
}

func bulkChanges(t *testing.T, input map[string]any) *contract.BulkChanges {
	t.Helper()
	out := contract.BulkChanges{}
	for key, value := range input {
		encoded, err := json.Marshal(value)
		require.NoError(t, err)
		out[key] = encoded
	}
	return &out
}
