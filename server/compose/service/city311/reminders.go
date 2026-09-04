package city311

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/pkg/errors"
	"github.com/cortezaproject/corteza/server/pkg/filter"
	"github.com/cortezaproject/corteza/server/store"
	systemTypes "github.com/cortezaproject/corteza/server/system/types"
	"github.com/jmoiron/sqlx/types"
)

const (
	maximumReminderTitleLength      = 160
	city311ReminderResourcePrefix   = "corteza::compose:city311-"
	reminderDeliveryPending         = "PENDING"
	reminderDeliveryDelivered       = "DELIVERED"
	reminderDeliveryTerminalFailure = "TERMINAL_FAILURE"
	reminderDeliveryBatchSize       = 1000
)

type city311ReminderPayload struct {
	RequestID         uint64                   `json:"request_id"`
	Title             string                   `json:"title"`
	DueAt             time.Time                `json:"due_at"`
	EndAt             *time.Time               `json:"end_at,omitempty"`
	Timezone          string                   `json:"timezone"`
	Description       string                   `json:"description,omitempty"`
	CalendarUID       string                   `json:"calendar_uid,omitempty"`
	Recurrence        string                   `json:"recurrence_rule,omitempty"`
	LastModified      *time.Time               `json:"last_modified,omitempty"`
	Channel           contract.ReminderChannel `json:"channel"`
	Status            contract.ReminderStatus  `json:"status"`
	PreviousDueAt     []time.Time              `json:"previous_due_at,omitempty"`
	CompletedAt       *time.Time               `json:"completed_at,omitempty"`
	CompletedBy       uint64                   `json:"completed_by,omitempty"`
	DeliveryKey       string                   `json:"delivery_key,omitempty"`
	DeliveryStatus    string                   `json:"delivery_status,omitempty"`
	DeliveryAttempts  int                      `json:"delivery_attempts,omitempty"`
	DeliveredAt       *time.Time               `json:"delivered_at,omitempty"`
	LastDeliveryError string                   `json:"last_delivery_error,omitempty"`
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
		reminder := &systemTypes.Reminder{
			ID: svc.nextID(), Resource: requestReminderResource(request.ID),
			AssignedTo: recipientID, AssignedBy: actor.ID, AssignedAt: now, RemindAt: utcTime(input.DueAt), CreatedAt: now,
		}
		payload := city311ReminderPayload{
			RequestID: request.ID, Title: input.Title, DueAt: input.DueAt.UTC(), Timezone: input.Timezone,
			Channel: input.Channel, Status: contract.ReminderStatusScheduled,
		}
		resetReminderDelivery(reminder, &payload)
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		reminder.Payload = types.JSONText(encoded)
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
	if err == nil {
		svc.wakeReminderWorker()
	}
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
			resetReminderDelivery(reminder, &payload)
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
	if err == nil && action == contract.ReminderActionSnooze {
		svc.wakeReminderWorker()
	}
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

// ProcessDueReminders delivers each due City 311 reminder occurrence once.
// The stable delivery key lets the mail fixture deduplicate a replay if the
// process stops after SMTP acceptance but before the reminder state is saved.
func (svc *Service) ProcessDueReminders(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()

	now := svc.now().UTC()
	set, _, err := store.SearchReminders(ctx, svc.store, systemTypes.ReminderFilter{
		Resource: city311ReminderResourcePrefix, ScheduledUntil: &now, ExcludeDismissed: true, ScheduledOnly: true,
		Paging: filter.Paging{Limit: reminderDeliveryBatchSize},
	})
	if err != nil {
		return err
	}
	sort.Slice(set, func(i, j int) bool { return set[i].ID < set[j].ID })
	var firstErr error
	for _, reminder := range set {
		if err = svc.processDueReminder(ctx, reminder); err != nil && firstErr == nil {
			firstErr = err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return firstErr
}

func (svc *Service) processDueReminder(ctx context.Context, reminder *systemTypes.Reminder) error {
	payload, err := decodeReminderPayload(reminder)
	if err != nil {
		return err
	}
	if reminderTerminal(payload.Status) {
		return nil
	}
	if payload.DueAt.After(svc.now().UTC()) {
		return nil
	}
	resetReminderDelivery(reminder, &payload)
	if payload.DeliveryStatus == reminderDeliveryDelivered || payload.DeliveryStatus == reminderDeliveryTerminalFailure {
		return nil
	}
	if payload.DeliveryStatus != reminderDeliveryPending {
		return fmt.Errorf("unsupported reminder delivery status %q", payload.DeliveryStatus)
	}

	before := reminderSnapshot(reminder, payload)
	deliveryState, attempts, deliveryErr := svc.dispatchDueReminder(ctx, reminder, payload)
	if deliveryState == "" {
		return deliveryErr
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	payload.DeliveryStatus = deliveryState
	payload.DeliveryAttempts += attempts
	payload.LastDeliveryError = ""
	now := svc.now().UTC()
	eventType := "REMINDER_IN_APP_DELIVERED"
	if deliveryState == reminderDeliveryDelivered {
		payload.DeliveredAt = &now
		if payload.Channel == contract.ReminderChannelEmail {
			eventType = "REMINDER_EMAIL_DELIVERED"
		}
	} else {
		payload.DeliveredAt = nil
		if deliveryErr != nil {
			payload.LastDeliveryError = deliveryErr.Error()
		}
		eventType = "REMINDER_EMAIL_DELIVERY_FAILED"
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	reminder.Payload = types.JSONText(encoded)
	reminder.UpdatedAt = &now
	return store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		if err := store.UpdateReminder(ctx, tx, reminder); err != nil {
			return err
		}
		return svc.persistReminderSystemAudit(ctx, tx, reminder, payload, eventType, before, reminderSnapshot(reminder, payload))
	})
}

func (svc *Service) dispatchDueReminder(ctx context.Context, reminder *systemTypes.Reminder, payload city311ReminderPayload) (string, int, error) {
	switch payload.Channel {
	case contract.ReminderChannelInApp:
		// The persisted system reminder is the single visible in-app notification.
		return reminderDeliveryDelivered, 1, nil
	case contract.ReminderChannelEmail:
		message, err := svc.reminderMailMessage(ctx, reminder, payload)
		if err != nil {
			return "", 0, err
		}
		svc.mailMu.Lock()
		status, attempts, deliveryErr := svc.deliverMail(ctx, message, payload.DeliveryKey)
		svc.mailMu.Unlock()
		if status == mailStatusDelivered {
			return reminderDeliveryDelivered, attempts, nil
		}
		return reminderDeliveryTerminalFailure, attempts, deliveryErr
	default:
		return "", 0, fmt.Errorf("unsupported reminder channel %q", payload.Channel)
	}
}

func (svc *Service) reminderMailMessage(ctx context.Context, reminder *systemTypes.Reminder, payload city311ReminderPayload) (MailMessage, error) {
	recipient, err := store.LookupUserByID(ctx, svc.store, reminder.AssignedTo)
	if err != nil {
		return MailMessage{}, err
	}
	email := normalizeEmail(recipient.Email)
	if !validEmail(email) {
		return MailMessage{}, fmt.Errorf("reminder recipient %d has no valid email address", reminder.AssignedTo)
	}
	subject := "City 311 reminder: " + payload.Title
	body := payload.Title
	if payload.RequestID != 0 {
		request, lookupErr := store.LookupCity311ServiceRequestByID(ctx, svc.store, payload.RequestID)
		if lookupErr != nil {
			return MailMessage{}, lookupErr
		}
		body = fmt.Sprintf("Reminder for service request %s: %s", request.RequestNumber, payload.Title)
	}
	return MailMessage{From: "noreply@city.example", To: []string{email}, Subject: subject, Text: body}, nil
}

func (svc *Service) StartReminderWorker(ctx context.Context) {
	svc.reminderWorkerOnce.Do(func() {
		go svc.runReminderWorker(ctx)
		svc.wakeReminderWorker()
	})
}

func (svc *Service) SetReminderWorkerErrorHandler(handler func(error)) {
	svc.reminderWorkerError = handler
}

func (svc *Service) runReminderWorker(ctx context.Context) {
	poll := svc.reminderPoll
	if poll <= 0 {
		poll = 30 * time.Second
	}
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-svc.reminderWake:
		case <-ticker.C:
		}
		if err := svc.ProcessDueReminders(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			if svc.reminderWorkerError != nil {
				svc.reminderWorkerError(err)
			}
		}
	}
}

func (svc *Service) wakeReminderWorker() {
	select {
	case svc.reminderWake <- struct{}{}:
	default:
	}
}

func resetReminderDelivery(reminder *systemTypes.Reminder, payload *city311ReminderPayload) {
	key := reminderDeliveryKey(reminder.ID, payload.DueAt)
	if payload.DeliveryKey == key && payload.DeliveryStatus != "" {
		return
	}
	payload.DeliveryKey = key
	payload.DeliveryStatus = reminderDeliveryPending
	payload.DeliveryAttempts = 0
	payload.DeliveredAt = nil
	payload.LastDeliveryError = ""
}

func reminderDeliveryKey(reminderID uint64, dueAt time.Time) string {
	return "city311-reminder:" + strconv.FormatUint(reminderID, 10) + ":" + dueAt.UTC().Format(time.RFC3339Nano)
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
		"delivery_status": payload.DeliveryStatus, "delivery_attempts": payload.DeliveryAttempts,
		"delivered_at": payload.DeliveredAt, "last_delivery_error": payload.LastDeliveryError,
	}
}

func (svc *Service) persistReminderAudit(ctx context.Context, tx store.Storer, actorID uint64, reminder *systemTypes.Reminder, payload city311ReminderPayload, eventType string, before, after map[string]any) error {
	return svc.persistReminderAuditAs(ctx, tx, contract.AuditActorStaff, actorID, contract.SourceChannelStaffInPerson, reminder, payload, eventType, before, after)
}

func (svc *Service) persistReminderSystemAudit(ctx context.Context, tx store.Storer, reminder *systemTypes.Reminder, payload city311ReminderPayload, eventType string, before, after map[string]any) error {
	return svc.persistReminderAuditAs(ctx, tx, contract.AuditActorSystem, 0, contract.SourceChannelAPI, reminder, payload, eventType, before, after)
}

func (svc *Service) persistReminderAuditAs(ctx context.Context, tx store.Storer, actorType contract.AuditActorType, actorID uint64, source contract.SourceChannel, reminder *systemTypes.Reminder, payload city311ReminderPayload, eventType string, before, after map[string]any) error {
	now := svc.now()
	after["local_display_time"] = reminderLocalDisplay(now)
	after["visibility"] = "STAFF"
	return store.CreateCity311AuditEvent(ctx, tx, &composeTypes.City311AuditEvent{
		ID: svc.nextID(), RequestID: payload.RequestID, EntityType: "reminder", EntityID: strconv.FormatUint(reminder.ID, 10), EventType: eventType,
		ActorType: actorType, ActorID: actorID, SourceChannel: source,
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
