package city311

import (
	"context"
	"strconv"
	"testing"

	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	systemTypes "github.com/cortezaproject/corteza/server/system/types"
	"github.com/stretchr/testify/require"
)

func TestStaffAssignmentAndCollaboratorLifecycle(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	request, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, "SR-2026-00034")
	require.NoError(t, err)
	supervisor := seededAssignmentActor(t, ctx, svc, st, "supervisor@city311.example.invalid")
	agent := seededAssignmentUser(t, ctx, st, "service-agent@city311.example.invalid")
	manager := seededAssignmentUser(t, ctx, st, "department-manager@city311.example.invalid")

	assigned, err := svc.Reassign(ctx, supervisor, request.ID, 1, contract.Reassignment{
		AssigneeID: strconv.FormatUint(agent.ID, 10), Reason: "Route to the streets response team.",
	})
	require.NoError(t, err)
	require.Equal(t, contract.ServiceRequestStatusSubmitted, assigned.Request.Status)
	require.Equal(t, strconv.FormatUint(agent.ID, 10), *assigned.PrimaryAssigneeID)
	require.Equal(t, uint64(2), assigned.Request.Version)
	require.Equal(t, "REQUEST_REASSIGNED", assigned.Audit[len(assigned.Audit)-1].EventType)
	require.Nil(t, assigned.Audit[len(assigned.Audit)-1].Before["primary_assignee_id"])
	agentNotifications, _, err := store.SearchNotifications(ctx, st, systemTypes.NotificationFilter{Recipient: agent.ID})
	require.NoError(t, err)
	require.Len(t, agentNotifications, 1)
	require.Equal(t, "You are now the primary assignee.", agentNotifications[0].Config.Simple.Description)

	withCollaborator, err := svc.AddCollaborator(ctx, supervisor, request.ID, 2, manager.ID, contract.Reason{Reason: "Manager will coordinate the repair."})
	require.NoError(t, err)
	require.Equal(t, []string{strconv.FormatUint(manager.ID, 10)}, withCollaborator.CollaboratorIDs)
	require.Equal(t, uint64(3), withCollaborator.Request.Version)
	managerNotifications, _, err := store.SearchNotifications(ctx, st, systemTypes.NotificationFilter{Recipient: manager.ID})
	require.NoError(t, err)
	require.Len(t, managerNotifications, 1)
	require.Equal(t, "You were added as a collaborator.", managerNotifications[0].Config.Simple.Description)

	// PUT is idempotent: adding the same collaborator again does not create a
	// second relationship, version, or audit event.
	repeated, err := svc.AddCollaborator(ctx, supervisor, request.ID, 3, manager.ID, contract.Reason{Reason: "Manager will coordinate the repair."})
	require.NoError(t, err)
	require.Equal(t, uint64(3), repeated.Request.Version)
	require.Len(t, repeated.CollaboratorIDs, 1)

	removed, err := svc.RemoveCollaborator(ctx, supervisor, request.ID, 3, manager.ID, contract.Reason{Reason: "Coordination is complete."})
	require.NoError(t, err)
	require.Empty(t, removed.CollaboratorIDs)
	require.Equal(t, uint64(4), removed.Request.Version)
	require.Equal(t, "COLLABORATOR_REMOVED", removed.Audit[len(removed.Audit)-1].EventType)

	persisted, err := store.LookupCity311ServiceRequestByID(ctx, st, request.ID)
	require.NoError(t, err)
	require.Equal(t, agent.ID, persisted.PrimaryAssigneeID)
	require.Empty(t, persisted.CollaboratorIDs)
}

func TestAssignmentRejectsUnauthorizedTargetsAndConflicts(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	request, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, "SR-2026-00034")
	require.NoError(t, err)
	supervisor := seededAssignmentActor(t, ctx, svc, st, "supervisor@city311.example.invalid")
	agentActor := seededAssignmentActor(t, ctx, svc, st, "service-agent@city311.example.invalid")
	agent := seededAssignmentUser(t, ctx, st, "service-agent@city311.example.invalid")
	workflowDesigner := seededAssignmentUser(t, ctx, st, "workflow-designer@city311.example.invalid")

	_, err = svc.Reassign(ctx, agentActor, request.ID, 1, contract.Reassignment{AssigneeID: strconv.FormatUint(agent.ID, 10), Reason: "Not authorized"})
	requireServiceError(t, err, 403, contract.ErrorForbidden)
	_, err = svc.Reassign(ctx, supervisor, request.ID, 1, contract.Reassignment{AssigneeID: "invalid", Reason: "Invalid target"})
	requireServiceError(t, err, 422, contract.ErrorValidation)
	_, err = svc.Reassign(ctx, supervisor, request.ID, 1, contract.Reassignment{AssigneeID: strconv.FormatUint(agent.ID, 10), Reason: "   "})
	requireServiceError(t, err, 422, contract.ErrorValidation)
	_, err = svc.Reassign(ctx, supervisor, request.ID, 1, contract.Reassignment{AssigneeID: strconv.FormatUint(workflowDesigner.ID, 10), Reason: "Wrong role and scope"})
	requireServiceError(t, err, 403, contract.ErrorForbidden)

	assigned, err := svc.Reassign(ctx, supervisor, request.ID, 1, contract.Reassignment{AssigneeID: strconv.FormatUint(agent.ID, 10), Reason: "Authorized assignment"})
	require.NoError(t, err)
	_, err = svc.AddCollaborator(ctx, supervisor, request.ID, assigned.Request.Version, agent.ID, contract.Reason{Reason: "Cannot duplicate the primary assignee"})
	requireServiceError(t, err, 422, contract.ErrorValidation)
	_, err = svc.RemoveCollaborator(ctx, supervisor, request.ID, assigned.Request.Version, workflowDesigner.ID, contract.Reason{Reason: "Not present"})
	requireServiceError(t, err, 403, contract.ErrorForbidden)
	_, err = svc.Reassign(ctx, supervisor, request.ID, 1, contract.Reassignment{AssigneeID: strconv.FormatUint(agent.ID, 10), Reason: "Stale version"})
	requireServiceError(t, err, 409, contract.ErrorVersionConflict)

	unchanged, err := store.LookupCity311ServiceRequestByID(ctx, st, request.ID)
	require.NoError(t, err)
	require.Equal(t, 2, unchanged.Version)
	require.Equal(t, agent.ID, unchanged.PrimaryAssigneeID)
}

func seededAssignmentUser(t *testing.T, ctx context.Context, st store.Storer, email string) *systemTypes.User {
	t.Helper()
	user, err := store.LookupUserByEmail(ctx, st, email)
	require.NoError(t, err)
	return user
}

func seededAssignmentActor(t *testing.T, ctx context.Context, svc *Service, st store.Storer, email string) contract.Actor {
	t.Helper()
	user := seededAssignmentUser(t, ctx, st, email)
	actor, err := svc.FindActor(ctx, user.ID)
	require.NoError(t, err)
	return actor
}

func requireServiceError(t *testing.T, err error, status int, code contract.ErrorCode) {
	t.Helper()
	var serviceErr *ServiceError
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, status, serviceErr.Status)
	require.Equal(t, code, serviceErr.Payload.Error)
}
