package city311

import (
	"context"
	"testing"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

func TestReopenRequiresConstituentRequestAndStaffApproval(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	request, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, "SR-2026-00038")
	require.NoError(t, err)
	require.Equal(t, contract.ServiceRequestStatusResolved, request.Status)

	pending, err := svc.RequestReopen(ctx, 6, request.ID, contract.ReopenApproval{Reason: "  The pothole has opened again.  "})
	require.NoError(t, err)
	require.Equal(t, reopenStatusPending, pending.Status)
	require.Equal(t, request.RequestNumber, "SR-2026-00038")
	persisted, err := store.LookupCity311ServiceRequestByID(ctx, st, request.ID)
	require.NoError(t, err)
	require.Equal(t, contract.ServiceRequestStatusResolved, persisted.Status)
	require.Equal(t, 1, persisted.Version)

	supervisor := relationshipServiceAgent()
	supervisor.ID = 200
	supervisor.Roles = []contract.ApplicationRole{contract.ApplicationRoleSupervisor}
	detail, err := svc.Find(ctx, supervisor, request.ID)
	require.NoError(t, err)
	require.Contains(t, detail.AvailableActions, "APPROVE_REOPEN")

	approved, err := svc.ApproveReopen(ctx, supervisor, request.ID, 1, contract.ReopenApproval{Reason: "Resident evidence confirmed."})
	require.NoError(t, err)
	require.Equal(t, contract.ServiceRequestStatusReopened, approved.Request.Status)
	require.Equal(t, uint64(2), approved.Request.Version)
	require.NotContains(t, approved.AvailableActions, "APPROVE_REOPEN")
	require.Contains(t, approved.AvailableActions, "START_PROGRESS")
	require.Equal(t, string(contract.ServiceRequestStatusReopened), approved.History[len(approved.History)-1].Action)

	reopens, _, err := store.SearchCity311ReopenRequests(ctx, st, composeTypes.City311ReopenRequestFilter{RequestID: request.ID})
	require.NoError(t, err)
	require.Len(t, reopens, 1)
	require.Equal(t, reopenStatusApproved, reopens[0].Status)
	require.Equal(t, supervisor.ID, reopens[0].ApprovedBy)
	require.Equal(t, "Resident evidence confirmed.", reopens[0].ApprovalReason)
	require.NotNil(t, reopens[0].ApprovedAt)

	audits, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{RequestID: request.ID})
	require.NoError(t, err)
	require.Equal(t, []string{"SEED_CREATED", "REOPEN_REQUESTED", "REOPEN_REQUEST_APPROVED", "SERVICE_REQUEST_TRANSITIONED"}, auditEventTypes(audits))
	require.Equal(t, "RESOLVED", audits[3].Before["status"])
	require.Equal(t, "REOPENED", audits[3].After["status"])
}

func TestReopenRejectsBypassesDuplicatesAndUnauthorisedActors(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	resolved, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, "SR-2026-00038")
	require.NoError(t, err)
	serviceAgent := relationshipServiceAgent()

	_, err = svc.Transition(ctx, serviceAgent, resolved.ID, 1, contract.RequestTransition{ToStatus: contract.ServiceRequestStatusReopened})
	var serviceErr *ServiceError
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, 422, serviceErr.Status)
	require.Equal(t, contract.ErrorInvalidStatusTransition, serviceErr.Payload.Error)
	unchanged, err := store.LookupCity311ServiceRequestByID(ctx, st, resolved.ID)
	require.NoError(t, err)
	require.Equal(t, contract.ServiceRequestStatusResolved, unchanged.Status)
	require.Equal(t, 1, unchanged.Version)

	_, err = svc.RequestReopen(ctx, 5, resolved.ID, contract.ReopenApproval{Reason: "Not linked"})
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, 404, serviceErr.Status)
	_, err = svc.RequestReopen(ctx, 6, resolved.ID, contract.ReopenApproval{Reason: "   "})
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, 422, serviceErr.Status)
	_, err = svc.RequestReopen(ctx, 6, resolved.ID, contract.ReopenApproval{Reason: "First request"})
	require.NoError(t, err)
	_, err = svc.RequestReopen(ctx, 6, resolved.ID, contract.ReopenApproval{Reason: "Duplicate request"})
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, 422, serviceErr.Status)

	_, err = svc.ApproveReopen(ctx, serviceAgent, resolved.ID, 1, contract.ReopenApproval{Reason: "Agent attempt"})
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, 403, serviceErr.Status)

	supervisor := relationshipServiceAgent()
	supervisor.Roles = []contract.ApplicationRole{contract.ApplicationRoleSupervisor}
	_, err = svc.ApproveReopen(ctx, supervisor, resolved.ID, 1, contract.ReopenApproval{Reason: "   "})
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, 422, serviceErr.Status)
	_, err = svc.ApproveReopen(ctx, supervisor, resolved.ID, 2, contract.ReopenApproval{Reason: "Stale version"})
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, 409, serviceErr.Status)
	require.Equal(t, uint64(1), *serviceErr.Payload.CurrentVersion)

	wrongDepartment := supervisor
	wrongDepartment.Department = contract.DepartmentSanitation
	_, err = svc.ApproveReopen(ctx, wrongDepartment, resolved.ID, 1, contract.ReopenApproval{Reason: "Wrong scope"})
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, 403, serviceErr.Status)

	submitted, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, "SR-2026-00034")
	require.NoError(t, err)
	_, err = svc.RequestReopen(ctx, 2, submitted.ID, contract.ReopenApproval{Reason: "Not eligible"})
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, contract.ErrorInvalidStatusTransition, serviceErr.Payload.Error)

	closed, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, "SR-2026-00039")
	require.NoError(t, err)
	manager := relationshipServiceAgent()
	manager.Roles = []contract.ApplicationRole{contract.ApplicationRoleDepartmentManager}
	_, err = svc.ApproveReopen(ctx, manager, closed.ID, 1, contract.ReopenApproval{Reason: "No pending request"})
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, 404, serviceErr.Status)
}

func auditEventTypes(events composeTypes.City311AuditEventSet) []string {
	out := make([]string, 0, len(events))
	for _, event := range events {
		out = append(out, event.EventType)
	}
	return out
}
