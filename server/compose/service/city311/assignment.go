package city311

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/pkg/errors"
	"github.com/cortezaproject/corteza/server/store"
	systemTypes "github.com/cortezaproject/corteza/server/system/types"
)

const maximumAssignmentReasonLength = 2000

func (svc *Service) Reassign(ctx context.Context, actor contract.Actor, requestID, expectedVersion uint64, input contract.Reassignment) (*contract.StaffServiceRequestDetail, error) {
	input.Reason = strings.TrimSpace(input.Reason)
	if err := validateAssignmentMutation(actor, expectedVersion, input.Reason); err != nil {
		return nil, err
	}
	assigneeID, err := parseRequiredStaffID(input.AssigneeID, "/assignee_id")
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
		target, err := lookupAssignmentTarget(ctx, tx, assigneeID, request)
		if err != nil {
			return err
		}
		if request.PrimaryAssigneeID == target.ID {
			return nil
		}

		beforeAssignee := request.PrimaryAssigneeID
		request.PrimaryAssigneeID = target.ID
		request.CollaboratorIDs = removeStaffID(request.CollaboratorIDs, target.ID)
		request.Version++
		request.UpdatedAt = svc.now()
		if err = store.UpdateCity311ServiceRequest(ctx, tx, request); err != nil {
			return err
		}
		if err = svc.persistAssignmentNotification(ctx, tx, actor.ID, target.ID, request, "You are now the primary assignee."); err != nil {
			return err
		}
		if beforeAssignee != 0 && beforeAssignee != target.ID {
			if err = svc.persistAssignmentNotification(ctx, tx, actor.ID, beforeAssignee, request, "You are no longer the primary assignee."); err != nil {
				return err
			}
		}
		return svc.persistAssignmentAudit(ctx, tx, actor.ID, request, "REQUEST_REASSIGNED", map[string]any{
			"primary_assignee_id": optionalAuditID(beforeAssignee),
		}, map[string]any{
			"primary_assignee_id": strconv.FormatUint(target.ID, 10), "reason": input.Reason,
		})
	})
	if err != nil {
		return nil, err
	}
	return svc.Find(ctx, actor, requestID)
}

func (svc *Service) AddCollaborator(ctx context.Context, actor contract.Actor, requestID, expectedVersion, staffID uint64, input contract.Reason) (*contract.StaffServiceRequestDetail, error) {
	return svc.changeCollaborator(ctx, actor, requestID, expectedVersion, staffID, input, true)
}

func (svc *Service) RemoveCollaborator(ctx context.Context, actor contract.Actor, requestID, expectedVersion, staffID uint64, input contract.Reason) (*contract.StaffServiceRequestDetail, error) {
	return svc.changeCollaborator(ctx, actor, requestID, expectedVersion, staffID, input, false)
}

func (svc *Service) changeCollaborator(ctx context.Context, actor contract.Actor, requestID, expectedVersion, staffID uint64, input contract.Reason, add bool) (*contract.StaffServiceRequestDetail, error) {
	input.Reason = strings.TrimSpace(input.Reason)
	if staffID == 0 {
		return nil, validationError(contract.FieldError{Field: "/path/staff_id", Code: contract.ValidationInvalidFormat})
	}
	if err := validateAssignmentMutation(actor, expectedVersion, input.Reason); err != nil {
		return nil, err
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	err := store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		request, err := lookupScopedRequest(ctx, tx, actor, requestID)
		if err != nil {
			return err
		}
		if uint64(request.Version) != expectedVersion {
			return requestVersionConflict(request.Version)
		}
		if _, err = lookupAssignmentTarget(ctx, tx, staffID, request); err != nil {
			return err
		}
		present := containsStaffID(request.CollaboratorIDs, staffID)
		if add {
			if request.PrimaryAssigneeID == staffID {
				return validationError(contract.FieldError{Field: "/path/staff_id", Code: contract.ValidationInvalidValue})
			}
			if present {
				return nil
			}
			request.CollaboratorIDs = append(request.CollaboratorIDs, staffID)
			sort.Slice(request.CollaboratorIDs, func(i, j int) bool { return request.CollaboratorIDs[i] < request.CollaboratorIDs[j] })
		} else {
			if !present {
				return apiError(404, contract.ErrorNotFound, "The request collaborator was not found.")
			}
			request.CollaboratorIDs = removeStaffID(request.CollaboratorIDs, staffID)
		}
		request.Version++
		request.UpdatedAt = svc.now()
		if err = store.UpdateCity311ServiceRequest(ctx, tx, request); err != nil {
			return err
		}
		if add {
			if err = svc.persistAssignmentNotification(ctx, tx, actor.ID, staffID, request, "You were added as a collaborator."); err != nil {
				return err
			}
		}
		eventType := "COLLABORATOR_REMOVED"
		before, after := map[string]any{"staff_id": strconv.FormatUint(staffID, 10)}, map[string]any{"reason": input.Reason}
		if add {
			eventType = "COLLABORATOR_ADDED"
			before, after = map[string]any{}, map[string]any{"staff_id": strconv.FormatUint(staffID, 10), "reason": input.Reason}
		}
		return svc.persistAssignmentAudit(ctx, tx, actor.ID, request, eventType, before, after)
	})
	if err != nil {
		return nil, err
	}
	return svc.Find(ctx, actor, requestID)
}

func validateAssignmentMutation(actor contract.Actor, expectedVersion uint64, reason string) error {
	if !canManageAssignments(actor) {
		return apiError(403, contract.ErrorForbidden, "A supervisor or department manager role is required.")
	}
	if expectedVersion == 0 {
		return apiError(428, contract.ErrorExpectedVersionRequired, "If-Match must identify the expected record version.")
	}
	length := utf8.RuneCountInString(reason)
	if length == 0 {
		return validationError(contract.FieldError{Field: "/reason", Code: contract.ValidationRequired})
	}
	if length > maximumAssignmentReasonLength {
		return validationError(contract.FieldError{Field: "/reason", Code: contract.ValidationTooLong})
	}
	return nil
}

func canManageAssignments(actor contract.Actor) bool {
	return hasRole(actor, contract.ApplicationRoleSupervisor) || hasRole(actor, contract.ApplicationRoleDepartmentManager)
}

func parseRequiredStaffID(raw, field string) (uint64, error) {
	value, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
	if err != nil || value == 0 {
		return 0, validationError(contract.FieldError{Field: field, Code: contract.ValidationInvalidFormat})
	}
	return value, nil
}

func lookupAssignmentTarget(ctx context.Context, tx store.Storer, staffID uint64, request *composeTypes.City311ServiceRequest) (*contract.Actor, error) {
	profile, err := store.LookupCity311ActorProfileByID(ctx, tx, staffID)
	if errors.IsNotFound(err) {
		return nil, apiError(404, contract.ErrorNotFound, "The staff user was not found.")
	}
	if err != nil {
		return nil, err
	}
	target := &contract.Actor{ID: staffID, Roles: []contract.ApplicationRole(profile.ApplicationRoles), Department: profile.Department, Districts: []contract.DistrictCode(profile.Districts)}
	if !canOperateRequest(*target) || !canRead(*target, request) {
		return nil, apiError(403, contract.ErrorForbidden, "The staff user is not authorized for this service request.")
	}
	return target, nil
}

func containsStaffID(values []uint64, target uint64) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func removeStaffID(values []uint64, target uint64) composeTypes.City311Uint64Set {
	out := make(composeTypes.City311Uint64Set, 0, len(values))
	for _, value := range values {
		if value != target {
			out = append(out, value)
		}
	}
	return out
}

func optionalAuditID(value uint64) any {
	if value == 0 {
		return nil
	}
	return strconv.FormatUint(value, 10)
}

func (svc *Service) persistAssignmentAudit(ctx context.Context, tx store.Storer, actorID uint64, request *composeTypes.City311ServiceRequest, eventType string, before, after map[string]any) error {
	return store.CreateCity311AuditEvent(ctx, tx, &composeTypes.City311AuditEvent{
		ID: svc.nextID(), RequestID: request.ID, EntityType: "service_request", EntityID: strconv.FormatUint(request.ID, 10), EventType: eventType,
		ActorType: contract.AuditActorStaff, ActorID: actorID, SourceChannel: contract.SourceChannelStaffInPerson,
		Before: before, After: after, CreatedAt: request.UpdatedAt,
	})
}

func (svc *Service) persistAssignmentNotification(ctx context.Context, tx store.Storer, actorID, recipientID uint64, request *composeTypes.City311ServiceRequest, description string) error {
	return store.CreateNotification(ctx, tx, &systemTypes.Notification{
		ID: svc.nextID(), Kind: systemTypes.NotificationKindSimple,
		Config: systemTypes.NotificationConfig{Simple: systemTypes.SimpleNotificationConfig{
			Title: "City 311 request " + publishedRequestNumber(request), Description: description,
		}},
		Recipient: recipientID, CreatedBy: actorID, CreatedAt: request.UpdatedAt,
	})
}
