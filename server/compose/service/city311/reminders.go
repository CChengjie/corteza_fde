package city311

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/pkg/errors"
	"github.com/cortezaproject/corteza/server/store"
	systemTypes "github.com/cortezaproject/corteza/server/system/types"
	"github.com/jmoiron/sqlx/types"
)

const maximumReminderTitleLength = 160

type city311ReminderPayload struct {
	RequestID     uint64                   `json:"request_id"`
	Title         string                   `json:"title"`
	DueAt         time.Time                `json:"due_at"`
	Timezone      string                   `json:"timezone"`
	Channel       contract.ReminderChannel `json:"channel"`
	Status        contract.ReminderStatus  `json:"status"`
	PreviousDueAt []time.Time              `json:"previous_due_at,omitempty"`
	CompletedAt   *time.Time               `json:"completed_at,omitempty"`
	CompletedBy   uint64                   `json:"completed_by,omitempty"`
}

func (svc *Service) CreateReminder(ctx context.Context, actor contract.Actor, requestID uint64, input contract.ReminderWrite) (*contract.Reminder, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.Timezone = strings.TrimSpace(input.Timezone)
	if err := validateReminderWrite(actor, input); err != nil {
		return nil, err
	}
	recipientID, err := parseRequiredStaffID(input.RecipientStaffID, "/recipient_staff_id")
	if err != nil {
		return nil, err
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	var result *contract.Reminder
	err = store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		request, err := lookupScopedRequest(ctx, tx, actor, requestID)
		if err != nil {
			return err
		}
		if _, err = lookupAssignmentTarget(ctx, tx, recipientID, request); err != nil {
			return err
		}
		now := svc.now()
		payload := city311ReminderPayload{
			RequestID: request.ID, Title: input.Title, DueAt: input.DueAt.UTC(), Timezone: input.Timezone,
			Channel: input.Channel, Status: contract.ReminderStatusScheduled,
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		reminder := &systemTypes.Reminder{
			ID: svc.nextID(), Resource: requestReminderResource(request.ID), Payload: types.JSONText(encoded),
			AssignedTo: recipientID, AssignedBy: actor.ID, AssignedAt: now, RemindAt: utcTime(input.DueAt), CreatedAt: now,
		}
		if err = store.CreateReminder(ctx, tx, reminder); err != nil {
			return err
		}
		if err = svc.persistReminderAudit(ctx, tx, actor.ID, reminder, payload, "REMINDER_CREATED", map[string]any{}, reminderSnapshot(reminder, payload)); err != nil {
			return err
		}
		value := reminderContract(reminder, payload)
		result = &value
		return nil
	})
	return result, err
}

func (svc *Service) ActionReminder(ctx context.Context, actor contract.Actor, reminderID uint64, action contract.ReminderAction, input contract.ReminderActionInput) (*contract.Reminder, error) {
	if !containsEnums([]contract.ReminderAction{action}, contract.ReminderActions) {
		return nil, validationError(contract.FieldError{Field: "/path/action", Code: contract.ValidationInvalidValue})
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	var result *contract.Reminder
	err := store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		reminder, payload, request, err := svc.lookupReminderContext(ctx, tx, reminderID)
		if err != nil {
			return err
		}
		if actor.ID != reminder.AssignedTo && !hasRole(actor, contract.ApplicationRoleSupervisor) {
			return apiError(403, contract.ErrorForbidden, "Only the reminder recipient or a supervisor can perform this action.")
		}
		if !canRead(actor, request) {
			return apiError(403, contract.ErrorForbidden, requestScopeDeniedMessage)
		}
		if reminderActionAlreadyApplied(payload.Status, action) {
			value := reminderContract(reminder, payload)
			result = &value
			return nil
		}
		before := reminderSnapshot(reminder, payload)
		now := svc.now()
		switch action {
		case contract.ReminderActionSnooze:
			if reminderTerminal(payload.Status) {
				return invalidReminderAction()
			}
			if input.DueAt == nil {
				return validationError(contract.FieldError{Field: "/due_at", Code: contract.ValidationRequired})
			}
			if reminder.RemindAt == nil || !input.DueAt.After(*reminder.RemindAt) {
				return validationError(contract.FieldError{Field: "/due_at", Code: contract.ValidationOutOfRange})
			}
			payload.PreviousDueAt = append(payload.PreviousDueAt, payload.DueAt.UTC())
			payload.DueAt = input.DueAt.UTC()
			payload.Status = contract.ReminderStatusSnoozed
			reminder.RemindAt = utcTime(*input.DueAt)
			reminder.SnoozeCount++
		case contract.ReminderActionComplete:
			if reminderTerminal(payload.Status) {
				return invalidReminderAction()
			}
			payload.Status = contract.ReminderStatusCompleted
			payload.CompletedAt = &now
			payload.CompletedBy = actor.ID
			reminder.DismissedAt, reminder.DismissedBy, reminder.RemindAt = &now, actor.ID, nil
		case contract.ReminderActionCancel:
			if reminderTerminal(payload.Status) {
				return invalidReminderAction()
			}
			payload.Status = contract.ReminderStatusCancelled
			reminder.DismissedAt, reminder.DismissedBy, reminder.RemindAt = &now, actor.ID, nil
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		reminder.Payload = types.JSONText(encoded)
		reminder.UpdatedAt = &now
		if err = store.UpdateReminder(ctx, tx, reminder); err != nil {
			return err
		}
		if err = svc.persistReminderAudit(ctx, tx, actor.ID, reminder, payload, reminderActionEvent(action), before, reminderSnapshot(reminder, payload)); err != nil {
			return err
		}
		value := reminderContract(reminder, payload)
		result = &value
		return nil
	})
	return result, err
}

func validateReminderWrite(actor contract.Actor, input contract.ReminderWrite) error {
	if !hasRole(actor, contract.ApplicationRoleServiceAgent) {
		return apiError(403, contract.ErrorForbidden, "A service agent role is required.")
	}
	var fields []contract.FieldError
	length := utf8.RuneCountInString(input.Title)
	if length == 0 {
		fields = append(fields, contract.FieldError{Field: "/title", Code: contract.ValidationRequired})
	} else if length > maximumReminderTitleLength {
		fields = append(fields, contract.FieldError{Field: "/title", Code: contract.ValidationTooLong})
	}
	if input.DueAt.IsZero() {
		fields = append(fields, contract.FieldError{Field: "/due_at", Code: contract.ValidationRequired})
	}
	if input.Timezone == "" {
		fields = append(fields, contract.FieldError{Field: "/timezone", Code: contract.ValidationRequired})
	} else if _, err := time.LoadLocation(input.Timezone); err != nil {
		fields = append(fields, contract.FieldError{Field: "/timezone", Code: contract.ValidationInvalidValue})
	}
	if !containsEnums([]contract.ReminderChannel{input.Channel}, contract.ReminderChannels) {
		fields = append(fields, contract.FieldError{Field: "/channel", Code: contract.ValidationInvalidValue})
	}
	if len(fields) > 0 {
		return validationError(fields...)
	}
	return nil
}

func (svc *Service) requestReminders(ctx context.Context, requestID uint64) ([]contract.Reminder, error) {
	set, _, err := store.SearchReminders(ctx, svc.store, systemTypes.ReminderFilter{Resource: requestReminderResource(requestID)})
	if err != nil {
		return nil, err
	}
	sort.Slice(set, func(i, j int) bool {
		if set[i].RemindAt == nil {
			return false
		}
		if set[j].RemindAt == nil {
			return true
		}
		return set[i].RemindAt.Before(*set[j].RemindAt)
	})
	out := make([]contract.Reminder, 0, len(set))
	for _, reminder := range set {
		payload, err := decodeReminderPayload(reminder)
		if err != nil {
			return nil, err
		}
		out = append(out, reminderContract(reminder, payload))
	}
	return out, nil
}

func (svc *Service) lookupReminderContext(ctx context.Context, tx store.Storer, reminderID uint64) (*systemTypes.Reminder, city311ReminderPayload, *composeTypes.City311ServiceRequest, error) {
	reminder, err := store.LookupReminderByID(ctx, tx, reminderID)
	if errors.IsNotFound(err) {
		return nil, city311ReminderPayload{}, nil, apiError(404, contract.ErrorNotFound, "The reminder was not found.")
	}
	if err != nil {
		return nil, city311ReminderPayload{}, nil, err
	}
	payload, err := decodeReminderPayload(reminder)
	if err != nil || reminder.Resource != requestReminderResource(payload.RequestID) {
		return nil, city311ReminderPayload{}, nil, apiError(404, contract.ErrorNotFound, "The reminder was not found.")
	}
	request, err := store.LookupCity311ServiceRequestByID(ctx, tx, payload.RequestID)
	if errors.IsNotFound(err) {
		return nil, city311ReminderPayload{}, nil, apiError(404, contract.ErrorNotFound, "The service request was not found.")
	}
	return reminder, payload, request, err
}

func requestReminderResource(requestID uint64) string {
	return "corteza::compose:city311-service-request/" + strconv.FormatUint(requestID, 10)
}

func decodeReminderPayload(reminder *systemTypes.Reminder) (city311ReminderPayload, error) {
	payload := city311ReminderPayload{}
	if err := json.Unmarshal(reminder.Payload, &payload); err != nil {
		return payload, err
	}
	return payload, nil
}

func reminderContract(reminder *systemTypes.Reminder, payload city311ReminderPayload) contract.Reminder {
	completedBy := ""
	if payload.CompletedBy != 0 {
		completedBy = strconv.FormatUint(payload.CompletedBy, 10)
	}
	return contract.Reminder{
		ReminderID: strconv.FormatUint(reminder.ID, 10), RequestID: strconv.FormatUint(payload.RequestID, 10),
		Title: payload.Title, DueAt: payload.DueAt.UTC(), Timezone: payload.Timezone,
		RecipientStaffID: strconv.FormatUint(reminder.AssignedTo, 10), Channel: payload.Channel, Status: payload.Status,
		CompletedAt: payload.CompletedAt, CompletedBy: completedBy,
	}
}

func reminderSnapshot(reminder *systemTypes.Reminder, payload city311ReminderPayload) map[string]any {
	value := reminderContract(reminder, payload)
	return map[string]any{
		"reminder_id": value.ReminderID, "request_id": value.RequestID, "title": value.Title,
		"due_at": value.DueAt, "timezone": value.Timezone, "recipient_staff_id": value.RecipientStaffID,
		"channel": value.Channel, "status": value.Status, "completed_at": value.CompletedAt,
		"completed_by": value.CompletedBy, "previous_due_at": payload.PreviousDueAt,
	}
}

func (svc *Service) persistReminderAudit(ctx context.Context, tx store.Storer, actorID uint64, reminder *systemTypes.Reminder, payload city311ReminderPayload, eventType string, before, after map[string]any) error {
	now := svc.now()
	after["local_display_time"] = reminderLocalDisplay(now)
	after["visibility"] = "STAFF"
	return store.CreateCity311AuditEvent(ctx, tx, &composeTypes.City311AuditEvent{
		ID: svc.nextID(), RequestID: payload.RequestID, EntityType: "reminder", EntityID: strconv.FormatUint(reminder.ID, 10), EventType: eventType,
		ActorType: contract.AuditActorStaff, ActorID: actorID, SourceChannel: contract.SourceChannelStaffInPerson,
		Before: before, After: after, CreatedAt: now,
	})
}

func reminderLocalDisplay(value time.Time) string {
	location, _ := time.LoadLocation("America/New_York")
	return value.In(location).Format("01/02/2006 3:04 pm") + " ET"
}

func reminderActionAlreadyApplied(status contract.ReminderStatus, action contract.ReminderAction) bool {
	return status == contract.ReminderStatusCompleted && action == contract.ReminderActionComplete ||
		status == contract.ReminderStatusCancelled && action == contract.ReminderActionCancel
}

func reminderTerminal(status contract.ReminderStatus) bool {
	return status == contract.ReminderStatusCompleted || status == contract.ReminderStatusCancelled
}

func invalidReminderAction() error {
	return validationError(contract.FieldError{Field: "/path/action", Code: contract.ValidationInvalidValue})
}

func reminderActionEvent(action contract.ReminderAction) string {
	switch action {
	case contract.ReminderActionSnooze:
		return "REMINDER_SNOOZED"
	case contract.ReminderActionComplete:
		return "REMINDER_COMPLETED"
	default:
		return "REMINDER_CANCELLED"
	}
}

func utcTime(value time.Time) *time.Time {
	value = value.UTC()
	return &value
}
