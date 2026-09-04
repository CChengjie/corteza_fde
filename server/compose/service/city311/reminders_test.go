package city311

import (
	"context"
	"strconv"
	"testing"
	"time"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	systemTypes "github.com/cortezaproject/corteza/server/system/types"
	"github.com/stretchr/testify/require"
)

func TestReminderLifecyclePersistsHistoryAndAudit(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	request, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, "SR-2026-00034")
	require.NoError(t, err)
	agent := seededAssignmentActor(t, ctx, svc, st, "service-agent@city311.example.invalid")
	manager := seededAssignmentUser(t, ctx, st, "department-manager@city311.example.invalid")
	supervisor := seededAssignmentActor(t, ctx, svc, st, "supervisor@city311.example.invalid")
	dueAt := svc.now().Add(2 * time.Hour)

	created, err := svc.CreateReminder(ctx, agent, request.ID, contract.ReminderWrite{
		Title: " Confirm repair with resident ", DueAt: dueAt, Timezone: "America/New_York",
		RecipientStaffID: strconv.FormatUint(manager.ID, 10), Channel: contract.ReminderChannelInApp,
	})
	require.NoError(t, err)
	require.Equal(t, "Confirm repair with resident", created.Title)
	require.Equal(t, contract.ReminderStatusScheduled, created.Status)
	require.Equal(t, dueAt, created.DueAt)

	reminderID, err := strconv.ParseUint(created.ReminderID, 10, 64)
	require.NoError(t, err)
	snoozedDueAt := dueAt.Add(24 * time.Hour)
	snoozed, err := svc.ActionReminder(ctx, supervisor, reminderID, contract.ReminderActionSnooze, contract.ReminderActionInput{DueAt: &snoozedDueAt})
	require.NoError(t, err)
	require.Equal(t, contract.ReminderStatusSnoozed, snoozed.Status)
	require.Equal(t, snoozedDueAt, snoozed.DueAt)

	completed, err := svc.ActionReminder(ctx, supervisor, reminderID, contract.ReminderActionComplete, contract.ReminderActionInput{})
	require.NoError(t, err)
	require.Equal(t, contract.ReminderStatusCompleted, completed.Status)
	require.Equal(t, snoozedDueAt, completed.DueAt)
	require.NotNil(t, completed.CompletedAt)
	require.Equal(t, strconv.FormatUint(supervisor.ID, 10), completed.CompletedBy)

	// COMPLETE is terminal and idempotent; replay does not append another audit.
	replayed, err := svc.ActionReminder(ctx, supervisor, reminderID, contract.ReminderActionComplete, contract.ReminderActionInput{})
	require.NoError(t, err)
	require.Equal(t, completed, replayed)
	_, err = svc.ActionReminder(ctx, supervisor, reminderID, contract.ReminderActionCancel, contract.ReminderActionInput{})
	requireServiceError(t, err, 422, contract.ErrorValidation)

	persisted, err := store.LookupReminderByID(ctx, st, reminderID)
	require.NoError(t, err)
	require.Equal(t, uint(1), persisted.SnoozeCount)
	require.Nil(t, persisted.RemindAt)
	require.NotNil(t, persisted.DismissedAt)
	payload, err := decodeReminderPayload(persisted)
	require.NoError(t, err)
	require.Equal(t, []time.Time{dueAt}, payload.PreviousDueAt)

	audits, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{RequestID: request.ID, EntityType: "reminder"})
	require.NoError(t, err)
	require.Len(t, audits, 3)
	require.Equal(t, "REMINDER_CREATED", audits[0].EventType)
	require.Equal(t, "REMINDER_SNOOZED", audits[1].EventType)
	require.Equal(t, "REMINDER_COMPLETED", audits[2].EventType)
	require.Equal(t, "02/03/2026 10:04 am ET", audits[2].After["local_display_time"])

	detail, err := svc.Find(ctx, supervisor, request.ID)
	require.NoError(t, err)
	require.Len(t, detail.Reminders, 1)
	require.Equal(t, contract.ReminderStatusCompleted, detail.Reminders[0].Status)
}

func TestReminderValidationAuthorizationAndCancellation(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	request, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, "SR-2026-00034")
	require.NoError(t, err)
	agent := seededAssignmentActor(t, ctx, svc, st, "service-agent@city311.example.invalid")
	manager := seededAssignmentActor(t, ctx, svc, st, "department-manager@city311.example.invalid")
	dueAt := svc.now().Add(time.Hour)
	valid := contract.ReminderWrite{Title: "Follow up", DueAt: dueAt, Timezone: "America/New_York", RecipientStaffID: strconv.FormatUint(agent.ID, 10), Channel: contract.ReminderChannelEmail}

	_, err = svc.CreateReminder(ctx, manager, request.ID, valid)
	requireServiceError(t, err, 403, contract.ErrorForbidden)
	invalid := valid
	invalid.Title, invalid.Timezone, invalid.Channel = "", "Not/AZone", contract.ReminderChannel("SMS")
	_, err = svc.CreateReminder(ctx, agent, request.ID, invalid)
	requireServiceError(t, err, 422, contract.ErrorValidation)
	missingTarget := valid
	missingTarget.RecipientStaffID = "999999999"
	_, err = svc.CreateReminder(ctx, agent, request.ID, missingTarget)
	requireServiceError(t, err, 404, contract.ErrorNotFound)

	created, err := svc.CreateReminder(ctx, agent, request.ID, valid)
	require.NoError(t, err)
	reminderID, err := strconv.ParseUint(created.ReminderID, 10, 64)
	require.NoError(t, err)
	_, err = svc.ActionReminder(ctx, manager, reminderID, contract.ReminderActionCancel, contract.ReminderActionInput{})
	requireServiceError(t, err, 403, contract.ErrorForbidden)
	_, err = svc.ActionReminder(ctx, agent, reminderID, contract.ReminderActionSnooze, contract.ReminderActionInput{})
	requireServiceError(t, err, 422, contract.ErrorValidation)
	earlier := dueAt.Add(-time.Minute)
	_, err = svc.ActionReminder(ctx, agent, reminderID, contract.ReminderActionSnooze, contract.ReminderActionInput{DueAt: &earlier})
	requireServiceError(t, err, 422, contract.ErrorValidation)
	_, err = svc.ActionReminder(ctx, agent, reminderID, contract.ReminderAction("DEFER"), contract.ReminderActionInput{})
	requireServiceError(t, err, 422, contract.ErrorValidation)

	cancelled, err := svc.ActionReminder(ctx, agent, reminderID, contract.ReminderActionCancel, contract.ReminderActionInput{})
	require.NoError(t, err)
	require.Equal(t, contract.ReminderStatusCancelled, cancelled.Status)
	require.Nil(t, cancelled.CompletedAt)
	replayed, err := svc.ActionReminder(ctx, agent, reminderID, contract.ReminderActionCancel, contract.ReminderActionInput{})
	require.NoError(t, err)
	require.Equal(t, cancelled, replayed)

	set, _, err := store.SearchReminders(ctx, st, systemTypes.ReminderFilter{Resource: requestReminderResource(request.ID)})
	require.NoError(t, err)
	require.Len(t, set, 1)
}
