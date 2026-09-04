package city311

import (
	"context"
	"strconv"
	"strings"
	"time"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
)

const (
	reopenStatusPending  = "PENDING_APPROVAL"
	reopenStatusApproved = "APPROVED"
)

// RequestReopen records a constituent request for approval. It deliberately
// leaves the service-request status and version unchanged.
func (svc *Service) RequestReopen(ctx context.Context, ownerID, requestID uint64, input contract.ReopenApproval) (*contract.ReopenRequest, error) {
	if ownerID == 0 {
		return nil, apiError(401, contract.ErrorUnauthenticated, "Authentication is required.")
	}
	reason, err := validateReopenReason(input.Reason)
	if err != nil {
		return nil, err
	}
	constituentID := "C-" + strconv.FormatUint(ownerID, 10)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	var created *composeTypes.City311ReopenRequest
	err = store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		if err := ensurePortalRequestAccess(ctx, tx, requestID, constituentID); err != nil {
			return err
		}
		request, err := store.LookupCity311ServiceRequestByID(ctx, tx, requestID)
		if err != nil {
			return err
		}
		if !requestCanBeReopened(request.Status) {
			return invalidStatusTransition("Only a resolved or closed service request can be reopened.")
		}
		pending, err := pendingReopenRequests(ctx, tx, requestID)
		if err != nil {
			return err
		}
		if len(pending) > 0 {
			return validationError(contract.FieldError{Field: "/request_id", Code: contract.ValidationConflict})
		}
		now := svc.now()
		created = &composeTypes.City311ReopenRequest{
			ID: svc.nextID(), RequestID: requestID, RequestedBy: constituentID,
			RequestReason: reason, Status: reopenStatusPending, RequestedAt: now,
		}
		if err = store.CreateCity311ReopenRequest(ctx, tx, created); err != nil {
			return err
		}
		return svc.persistReopenAudit(ctx, tx, created, "REOPEN_REQUESTED", contract.AuditActorConstituent, ownerID, contract.SourceChannelPortalAuthenticated, map[string]any{}, reopenSnapshot(created), now)
	})
	if err != nil {
		return nil, err
	}
	return &contract.ReopenRequest{RequestID: strconv.FormatUint(created.RequestID, 10), Status: created.Status}, nil
}

// ApproveReopen atomically approves the pending request, transitions the
// service request, appends audit events, and publishes constituent history.
func (svc *Service) ApproveReopen(ctx context.Context, actor contract.Actor, requestID, expectedVersion uint64, input contract.ReopenApproval) (*contract.StaffServiceRequestDetail, error) {
	if !canApproveReopen(actor) {
		return nil, apiError(403, contract.ErrorForbidden, "A supervisor or department manager role is required.")
	}
	if expectedVersion == 0 {
		return nil, apiError(428, contract.ErrorExpectedVersionRequired, "If-Match must identify the expected record version.")
	}
	reason, err := validateReopenReason(input.Reason)
	if err != nil {
		return nil, err
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	err = store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		request, err := lookupScopedRequest(ctx, tx, actor, requestID)
		if err != nil {
			return err
		}
		if uint64(request.Version) != expectedVersion {
			return requestVersionConflict(request.Version)
		}
		if !requestCanBeReopened(request.Status) {
			return invalidStatusTransition("Only a resolved or closed service request can be reopened.")
		}
		pending, err := pendingReopenRequests(ctx, tx, requestID)
		if err != nil {
			return err
		}
		if len(pending) == 0 {
			return apiError(404, contract.ErrorNotFound, "No pending reopen request was found.")
		}

		reopen := pending[0]
		now := svc.now()
		reopenBefore := reopenSnapshot(reopen)
		requestBefore := requestSnapshot(request)
		reopen.Status = reopenStatusApproved
		reopen.ApprovedBy = actor.ID
		reopen.ApprovalReason = reason
		reopen.ApprovedAt = &now
		if err = store.UpdateCity311ReopenRequest(ctx, tx, reopen); err != nil {
			return err
		}
		request.Status = contract.ServiceRequestStatusReopened
		request.Version++
		request.UpdatedAt = now
		if err = store.UpdateCity311ServiceRequest(ctx, tx, request); err != nil {
			return err
		}
		if err = svc.persistReopenAudit(ctx, tx, reopen, "REOPEN_REQUEST_APPROVED", contract.AuditActorStaff, actor.ID, contract.SourceChannelStaffInPerson, reopenBefore, reopenSnapshot(reopen), now); err != nil {
			return err
		}
		if err = store.CreateCity311AuditEvent(ctx, tx, &composeTypes.City311AuditEvent{
			ID: svc.nextID(), RequestID: request.ID, EntityType: "service_request", EntityID: strconv.FormatUint(request.ID, 10),
			EventType: "SERVICE_REQUEST_TRANSITIONED", ActorType: contract.AuditActorStaff, ActorID: actor.ID,
			SourceChannel: contract.SourceChannelStaffInPerson, Before: requestBefore, After: requestSnapshot(request), CreatedAt: now,
		}); err != nil {
			return err
		}
		if err = store.CreateCity311PublicHistoryItem(ctx, tx, &composeTypes.City311PublicHistoryItem{
			ID: svc.nextID(), RequestID: request.ID, Action: string(contract.ServiceRequestStatusReopened),
			ResponsibleDepartment: request.OwningDepartment, OccurredAt: now,
		}); err != nil {
			return err
		}
		return svc.enqueueRelationshipNotifications(ctx, tx, request, contract.ServiceRequestStatus(anyString(requestBefore["status"])), RelationshipNotificationReopened, actor.ID, contract.SourceChannelStaffInPerson)
	})
	if err != nil {
		return nil, err
	}
	svc.wakeRequestNotificationWorker()
	return svc.Find(ctx, actor, requestID)
}

func validateReopenReason(value string) (string, error) {
	reason := strings.TrimSpace(value)
	if fields := validateBoundedText(reason, "/reason", 1, 2000); len(fields) > 0 {
		return "", validationError(fields...)
	}
	return reason, nil
}

func requestCanBeReopened(status contract.ServiceRequestStatus) bool {
	return status == contract.ServiceRequestStatusResolved || status == contract.ServiceRequestStatusClosed
}

func canApproveReopen(actor contract.Actor) bool {
	return hasRole(actor, contract.ApplicationRoleSupervisor) || hasRole(actor, contract.ApplicationRoleDepartmentManager)
}

func pendingReopenRequests(ctx context.Context, st store.Storer, requestID uint64) (composeTypes.City311ReopenRequestSet, error) {
	set, _, err := store.SearchCity311ReopenRequests(ctx, st, composeTypes.City311ReopenRequestFilter{
		RequestID: requestID, Status: reopenStatusPending,
	})
	return set, err
}

func invalidStatusTransition(message string) *ServiceError {
	return apiError(422, contract.ErrorInvalidStatusTransition, message)
}

func (svc *Service) persistReopenAudit(
	ctx context.Context,
	tx store.Storer,
	reopen *composeTypes.City311ReopenRequest,
	eventType string,
	actorType contract.AuditActorType,
	actorID uint64,
	source contract.SourceChannel,
	before, after map[string]any,
	occurredAt time.Time,
) error {
	return store.CreateCity311AuditEvent(ctx, tx, &composeTypes.City311AuditEvent{
		ID: svc.nextID(), RequestID: reopen.RequestID, EntityType: "reopen_request", EntityID: strconv.FormatUint(reopen.ID, 10),
		EventType: eventType, ActorType: actorType, ActorID: actorID, SourceChannel: source,
		Before: before, After: after, CreatedAt: occurredAt,
	})
}

func reopenSnapshot(reopen *composeTypes.City311ReopenRequest) map[string]any {
	return map[string]any{
		"reopen_request_id": strconv.FormatUint(reopen.ID, 10), "request_id": strconv.FormatUint(reopen.RequestID, 10),
		"requested_by": reopen.RequestedBy, "request_reason": reopen.RequestReason,
		"status": reopen.Status, "requested_at": reopen.RequestedAt,
		"approved_by": optionalID(reopen.ApprovedBy), "approval_reason": reopen.ApprovalReason,
		"approved_at": reopen.ApprovedAt,
	}
}
