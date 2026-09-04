package city311

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	systemTypes "github.com/cortezaproject/corteza/server/system/types"
	"github.com/stretchr/testify/require"
)

func TestCalendarImportsUpdatesCancelsAndExportsICS(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	agent := seededAssignmentActor(t, ctx, svc, st, "service-agent@city311.example.invalid")
	request, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, "SR-2026-00034")
	require.NoError(t, err)
	legacy, err := svc.CreateReminder(ctx, agent, request.ID, contract.ReminderWrite{
		Title: "Legacy request reminder", DueAt: svc.now().Add(time.Hour), Timezone: calendarTimezone,
		RecipientStaffID: strconv.FormatUint(agent.ID, 10), Channel: contract.ReminderChannelInApp,
	})
	require.NoError(t, err)

	initial, err := svc.ExportCalendar(ctx, agent)
	require.NoError(t, err)
	require.Equal(t, "text/calendar", initial.ContentType)
	require.Contains(t, initial.Body, "UID:city311-reminder-"+legacy.ReminderID+"@city.example\r\n")
	require.Contains(t, initial.Body, "DTSTART;TZID=America/New_York:")
	require.Contains(t, initial.Body, "DTEND;TZID=America/New_York:")
	require.Contains(t, initial.Body, "DESCRIPTION:City 311 service request "+strconv.FormatUint(request.ID, 10))
	require.Contains(t, initial.Body, "STATUS:CONFIRMED\r\n")
	require.Contains(t, initial.Body, "LAST-MODIFIED:")

	ics := calendarFixture(
		calendarEventFixture("utc-1@city.example", "UTC reminder", "DTSTART:20260905T150000Z", "DTEND:20260905T153000Z", "Review pothole, then call", "CONFIRMED", "RRULE:FREQ=DAILY;COUNT=2"),
		calendarEventFixture("local-1@city.example", "Local reminder", "DTSTART;TZID=America/New_York:20260906T090000", "DTEND;TZID=America/New_York:20260906T100000", "Meet resident", "TENTATIVE", ""),
		calendarEventFixture("unknown-cancelled@city.example", "Cancelled elsewhere", "DTSTART:20260907T150000Z", "DTEND:20260907T153000Z", "Ignore me", "CANCELLED", ""),
	)
	pending, err := svc.ImportCalendar(ctx, agent, contract.CalendarImport{ICS: ics})
	require.NoError(t, err)
	require.Equal(t, contract.OperationStatusPending, pending.Status)
	completed, err := svc.GetOperation(ctx, agent, pending.OperationID)
	require.NoError(t, err)
	require.Equal(t, contract.OperationStatusSucceeded, completed.Status)
	require.Equal(t, float64(3), completed.Result["total_events"])
	require.Equal(t, float64(2), completed.Result["created"])
	require.Equal(t, float64(1), completed.Result["ignored_cancelled"])

	set, _, err := store.SearchReminders(ctx, st, systemTypes.ReminderFilter{AssignedTo: agent.ID})
	require.NoError(t, err)
	require.Len(t, set, 3)
	utcReminder, utcPayload := reminderWithCalendarUID(t, set, "utc-1@city.example")
	require.Equal(t, "UTC reminder", utcPayload.Title)
	require.Equal(t, "Review pothole, then call", utcPayload.Description)
	require.Equal(t, "FREQ=DAILY;COUNT=2", utcPayload.Recurrence)
	require.Equal(t, "UTC", utcPayload.Timezone)
	require.NotNil(t, utcReminder.RemindAt)

	updatedICS := calendarFixture(calendarEventFixture("utc-1@city.example", "UTC reminder updated", "DTSTART:20260908T160000Z", "DTEND:20260908T170000Z", "Updated description, call", "CONFIRMED", "RRULE:FREQ=WEEKLY;COUNT=3"))
	updated, err := svc.ImportCalendar(ctx, agent, contract.CalendarImport{ICS: updatedICS})
	require.NoError(t, err)
	updatedOperation, err := svc.GetOperation(ctx, agent, updated.OperationID)
	require.NoError(t, err)
	require.Equal(t, float64(1), updatedOperation.Result["updated"])
	set, _, err = store.SearchReminders(ctx, st, systemTypes.ReminderFilter{AssignedTo: agent.ID})
	require.NoError(t, err)
	require.Len(t, set, 3)
	_, utcPayload = reminderWithCalendarUID(t, set, "utc-1@city.example")
	require.Equal(t, "UTC reminder updated", utcPayload.Title)

	cancelledICS := calendarFixture(calendarEventFixture("utc-1@city.example", "UTC reminder updated", "DTSTART:20260908T160000Z", "DTEND:20260908T170000Z", "Updated description, call", "CANCELLED", "RRULE:FREQ=WEEKLY;COUNT=3"))
	cancelled, err := svc.ImportCalendar(ctx, agent, contract.CalendarImport{ICS: cancelledICS})
	require.NoError(t, err)
	cancelledOperation, err := svc.GetOperation(ctx, agent, cancelled.OperationID)
	require.NoError(t, err)
	require.Equal(t, float64(1), cancelledOperation.Result["cancelled"])
	set, _, err = store.SearchReminders(ctx, st, systemTypes.ReminderFilter{AssignedTo: agent.ID})
	require.NoError(t, err)
	utcReminder, utcPayload = reminderWithCalendarUID(t, set, "utc-1@city.example")
	require.Equal(t, contract.ReminderStatusCancelled, utcPayload.Status)
	require.Nil(t, utcReminder.RemindAt)

	exported, err := svc.ExportCalendar(ctx, agent)
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(exported.Body, "END:VCALENDAR\r\n"))
	require.Contains(t, exported.Body, "UID:utc-1@city.example\r\n")
	require.Contains(t, exported.Body, "DTSTART:20260908T160000Z\r\n")
	require.Contains(t, exported.Body, "RRULE:FREQ=WEEKLY;COUNT=3\r\n")
	require.Contains(t, exported.Body, "STATUS:CANCELLED\r\n")
	require.Contains(t, exported.Body, "UID:local-1@city.example\r\n")
	require.Contains(t, exported.Body, "DTSTART;TZID=America/New_York:20260906T090000\r\n")
	require.Contains(t, exported.Body, `DESCRIPTION:Updated description\, call`)

	audits, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{EventType: calendarImportAuditEvent})
	require.NoError(t, err)
	require.Len(t, audits, 3)
	exportAudits, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{EventType: calendarExportAuditEvent})
	require.NoError(t, err)
	require.Len(t, exportAudits, 2)
}

func TestCalendarRejectsInvalidInputAndNonStaff(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	agent := seededAssignmentActor(t, ctx, svc, st, "service-agent@city311.example.invalid")
	constituent := contract.Actor{ID: 9, Roles: []contract.ApplicationRole{contract.ApplicationRoleConstituent}}

	_, err := svc.ImportCalendar(ctx, constituent, contract.CalendarImport{ICS: calendarFixture(calendarEventFixture("one", "One", "DTSTART:20260905T150000Z", "DTEND:20260905T153000Z", "Description", "CONFIRMED", ""))})
	requireServiceError(t, err, httpStatusForbidden, contract.ErrorForbidden)
	_, err = svc.ExportCalendar(ctx, constituent)
	requireServiceError(t, err, httpStatusForbidden, contract.ErrorForbidden)

	validEvent := calendarEventFixture("one", "One", "DTSTART:20260905T150000Z", "DTEND:20260905T153000Z", "Description", "CONFIRMED", "")
	for name, raw := range map[string]string{
		"empty":              "",
		"missing envelope":   validEvent,
		"missing version":    strings.Replace(calendarFixture(validEvent), "VERSION:2.0\r\n", "", 1),
		"missing uid":        calendarFixture(strings.Replace(validEvent, "UID:one\r\n", "", 1)),
		"floating time":      calendarFixture(strings.Replace(validEvent, "DTSTART:20260905T150000Z", "DTSTART:20260905T150000", 1)),
		"mixed timezone":     calendarFixture(strings.Replace(validEvent, "DTEND:20260905T153000Z", "DTEND;TZID=America/New_York:20260905T113000", 1)),
		"end before start":   calendarFixture(strings.Replace(validEvent, "DTEND:20260905T153000Z", "DTEND:20260905T140000Z", 1)),
		"bad last modified":  calendarFixture(strings.Replace(validEvent, "LAST-MODIFIED:20260904T120000Z", "LAST-MODIFIED;TZID=America/New_York:20260904T080000", 1)),
		"invalid status":     calendarFixture(strings.Replace(validEvent, "STATUS:CONFIRMED", "STATUS:UNKNOWN", 1)),
		"duplicate property": calendarFixture(strings.Replace(validEvent, "UID:one\r\n", "UID:one\r\nUID:two\r\n", 1)),
	} {
		t.Run(name, func(t *testing.T) {
			_, importErr := svc.ImportCalendar(ctx, agent, contract.CalendarImport{ICS: raw})
			requireServiceError(t, importErr, 422, contract.ErrorValidation)
		})
	}
}

func TestCalendarCompletedEventAndTextUnescaping(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	agent := seededAssignmentActor(t, ctx, svc, st, "service-agent@city311.example.invalid")
	event := calendarEventFixture("completed@city.example", "Completed\r\n event", "DTSTART:20260905T150000Z", "DTEND:20260905T153000Z", `First\nSecond\, third\; fourth\\fifth`, "COMPLETED", "")
	_, err := svc.ImportCalendar(ctx, agent, contract.CalendarImport{ICS: calendarFixture(event)})
	require.NoError(t, err)
	set, _, err := store.SearchReminders(ctx, st, systemTypes.ReminderFilter{AssignedTo: agent.ID})
	require.NoError(t, err)
	reminder, payload := reminderWithCalendarUID(t, set, "completed@city.example")
	require.Equal(t, "Completedevent", payload.Title)
	require.Equal(t, "First\nSecond, third; fourth\\fifth", payload.Description)
	require.Equal(t, contract.ReminderStatusCompleted, payload.Status)
	require.Equal(t, agent.ID, payload.CompletedBy)
	require.NotNil(t, payload.CompletedAt)
	require.Nil(t, reminder.RemindAt)

	_, err = parseCalendarProperty("DTSTART;BROKEN:20260905T150000Z")
	require.Error(t, err)
	require.Equal(t, "trailing\\", unescapeCalendarText(`trailing\`))
}

func calendarFixture(events ...string) string {
	return "BEGIN:VCALENDAR\r\nVERSION:2.0\r\n" + strings.Join(events, "") + "END:VCALENDAR\r\n"
}

func calendarEventFixture(uid, summary, start, end, description, status, rrule string) string {
	lines := []string{"BEGIN:VEVENT", "UID:" + uid, "SUMMARY:" + summary, start, end, "DESCRIPTION:" + description, "STATUS:" + status}
	if rrule != "" {
		lines = append(lines, rrule)
	}
	lines = append(lines, "LAST-MODIFIED:20260904T120000Z", "END:VEVENT")
	return strings.Join(lines, "\r\n") + "\r\n"
}

func reminderWithCalendarUID(t *testing.T, set systemTypes.ReminderSet, uid string) (*systemTypes.Reminder, city311ReminderPayload) {
	t.Helper()
	for _, reminder := range set {
		payload, err := decodeReminderPayload(reminder)
		require.NoError(t, err)
		if payload.CalendarUID == uid {
			return reminder, payload
		}
	}
	require.FailNow(t, fmt.Sprintf("reminder with UID %s not found", uid))
	return nil, city311ReminderPayload{}
}

const httpStatusForbidden = 403
