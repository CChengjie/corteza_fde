package city311

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	systemTypes "github.com/cortezaproject/corteza/server/system/types"
	"github.com/jmoiron/sqlx/types"
)

const (
	calendarImportKind       = "CALENDAR_IMPORT"
	calendarImportAuditEvent = "CALENDAR_IMPORTED"
	calendarExportAuditEvent = "CALENDAR_EXPORTED"
	calendarTimezone         = "America/New_York"
	maximumCalendarBytes     = 1 << 20
	maximumCalendarEvents    = 500
)

type calendarEvent struct {
	UID          string
	Summary      string
	Start        time.Time
	End          time.Time
	Timezone     string
	Description  string
	Status       string
	Recurrence   string
	LastModified time.Time
}

type calendarImportSummary struct {
	Total            int
	Created          int
	Updated          int
	Cancelled        int
	IgnoredCancelled int
}

func (svc *Service) ImportCalendar(ctx context.Context, actor contract.Actor, input contract.CalendarImport) (*contract.Operation, error) {
	if !isStaff(actor) {
		return nil, apiError(http.StatusForbidden, contract.ErrorForbidden, "A staff role is required.")
	}
	events, err := parseCalendar(input.ICS)
	if err != nil {
		return nil, calendarValidationError()
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	pending := &contract.Operation{}
	err = store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		now := svc.now().UTC()
		operation := &composeTypes.City311Operation{
			ID: svc.nextID(), Kind: calendarImportKind, Status: string(contract.OperationStatusPending), ActorID: actor.ID,
			Result: composeTypes.City311JSON{}, Error: composeTypes.City311JSON{}, CreatedAt: now, UpdatedAt: now,
		}
		if createErr := store.CreateCity311Operation(ctx, tx, operation); createErr != nil {
			return createErr
		}
		*pending = *toOperation(operation)

		summary, importErr := svc.importCalendarEvents(ctx, tx, actor, events)
		if importErr != nil {
			return importErr
		}
		completedAt := svc.now().UTC()
		operation.Status = string(contract.OperationStatusSucceeded)
		operation.Progress = 100
		operation.Result = composeTypes.City311JSON{
			"total_events": summary.Total, "created": summary.Created, "updated": summary.Updated,
			"cancelled": summary.Cancelled, "ignored_cancelled": summary.IgnoredCancelled,
		}
		operation.UpdatedAt = completedAt
		operation.CompletedAt = &completedAt
		if updateErr := store.UpdateCity311Operation(ctx, tx, operation); updateErr != nil {
			return updateErr
		}
		return store.CreateCity311AuditEvent(ctx, tx, &composeTypes.City311AuditEvent{
			ID: svc.nextID(), EntityType: "calendar_import", EntityID: publicOperationID(operation.ID), EventType: calendarImportAuditEvent,
			ActorType: contract.AuditActorStaff, ActorID: actor.ID, SourceChannel: contract.SourceChannelStaffInPerson,
			Before: composeTypes.City311JSON{}, After: composeTypes.City311JSON{
				"department_code": optionalActorDepartment(actor), "total_events": summary.Total, "created": summary.Created,
				"updated": summary.Updated, "cancelled": summary.Cancelled, "ignored_cancelled": summary.IgnoredCancelled,
			}, CreatedAt: completedAt,
		})
	})
	if err != nil {
		return nil, err
	}
	return pending, nil
}

func (svc *Service) importCalendarEvents(ctx context.Context, tx store.Storer, actor contract.Actor, events []calendarEvent) (calendarImportSummary, error) {
	summary := calendarImportSummary{Total: len(events)}
	set, _, err := store.SearchReminders(ctx, tx, systemTypes.ReminderFilter{AssignedTo: actor.ID})
	if err != nil {
		return summary, err
	}
	byUID := make(map[string]*systemTypes.Reminder, len(set))
	payloads := make(map[uint64]city311ReminderPayload, len(set))
	for _, reminder := range set {
		payload, decodeErr := decodeReminderPayload(reminder)
		if decodeErr != nil {
			return summary, decodeErr
		}
		uid := calendarReminderUID(reminder, payload)
		if _, exists := byUID[uid]; !exists {
			byUID[uid] = reminder
			payloads[reminder.ID] = payload
		}
	}

	for _, event := range events {
		reminder := byUID[event.UID]
		if event.Status == "CANCELLED" && reminder == nil {
			summary.IgnoredCancelled++
			continue
		}
		if reminder == nil {
			payload := calendarPayload(event, 0)
			encoded, encodeErr := json.Marshal(payload)
			if encodeErr != nil {
				return summary, encodeErr
			}
			now := svc.now().UTC()
			reminder = &systemTypes.Reminder{
				ID: svc.nextID(), Resource: calendarReminderResource(actor.ID), Payload: types.JSONText(encoded),
				AssignedTo: actor.ID, AssignedBy: actor.ID, AssignedAt: now, RemindAt: utcTime(event.Start), CreatedAt: now,
			}
			applyCalendarStatus(reminder, &payload, event.Status, actor.ID, now)
			encoded, encodeErr = json.Marshal(payload)
			if encodeErr != nil {
				return summary, encodeErr
			}
			reminder.Payload = types.JSONText(encoded)
			if createErr := store.CreateReminder(ctx, tx, reminder); createErr != nil {
				return summary, createErr
			}
			if auditErr := svc.persistReminderAudit(ctx, tx, actor.ID, reminder, payload, "REMINDER_IMPORTED", map[string]any{}, reminderSnapshot(reminder, payload)); auditErr != nil {
				return summary, auditErr
			}
			byUID[event.UID] = reminder
			payloads[reminder.ID] = payload
			summary.Created++
			continue
		}

		payload := payloads[reminder.ID]
		before := reminderSnapshot(reminder, payload)
		payload.Title = event.Summary
		payload.DueAt = event.Start.UTC()
		end := event.End.UTC()
		payload.EndAt = &end
		payload.Timezone = event.Timezone
		payload.Description = event.Description
		payload.CalendarUID = event.UID
		payload.Recurrence = event.Recurrence
		modified := event.LastModified.UTC()
		payload.LastModified = &modified
		now := svc.now().UTC()
		applyCalendarStatus(reminder, &payload, event.Status, actor.ID, now)
		encoded, encodeErr := json.Marshal(payload)
		if encodeErr != nil {
			return summary, encodeErr
		}
		reminder.Payload = types.JSONText(encoded)
		reminder.UpdatedAt = &now
		if updateErr := store.UpdateReminder(ctx, tx, reminder); updateErr != nil {
			return summary, updateErr
		}
		eventType := "REMINDER_CALENDAR_UPDATED"
		if event.Status == "CANCELLED" {
			eventType = "REMINDER_CANCELLED"
			summary.Cancelled++
		} else {
			summary.Updated++
		}
		if auditErr := svc.persistReminderAudit(ctx, tx, actor.ID, reminder, payload, eventType, before, reminderSnapshot(reminder, payload)); auditErr != nil {
			return summary, auditErr
		}
		payloads[reminder.ID] = payload
	}
	return summary, nil
}

func (svc *Service) ExportCalendar(ctx context.Context, actor contract.Actor) (*contract.CalendarExport, error) {
	if !isStaff(actor) {
		return nil, apiError(http.StatusForbidden, contract.ErrorForbidden, "A staff role is required.")
	}
	set, _, err := store.SearchReminders(ctx, svc.store, systemTypes.ReminderFilter{AssignedTo: actor.ID})
	if err != nil {
		return nil, err
	}
	type exportEvent struct {
		uid      string
		reminder *systemTypes.Reminder
		payload  city311ReminderPayload
	}
	events := make([]exportEvent, 0, len(set))
	for _, reminder := range set {
		payload, decodeErr := decodeReminderPayload(reminder)
		if decodeErr != nil {
			return nil, decodeErr
		}
		events = append(events, exportEvent{uid: calendarReminderUID(reminder, payload), reminder: reminder, payload: payload})
	}
	sort.Slice(events, func(i, j int) bool { return events[i].uid < events[j].uid })

	lines := []string{"BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//City 311 CRM//Calendar Exchange//EN", "CALSCALE:GREGORIAN"}
	for _, event := range events {
		lines = append(lines, encodeCalendarEvent(event.uid, event.reminder, event.payload)...)
	}
	lines = append(lines, "END:VCALENDAR")
	body := strings.Join(lines, "\r\n") + "\r\n"
	if err = store.CreateCity311AuditEvent(ctx, svc.store, &composeTypes.City311AuditEvent{
		ID: svc.nextID(), EntityType: "calendar_export", EntityID: strconv.FormatUint(actor.ID, 10), EventType: calendarExportAuditEvent,
		ActorType: contract.AuditActorStaff, ActorID: actor.ID, SourceChannel: contract.SourceChannelStaffInPerson,
		Before: composeTypes.City311JSON{}, After: composeTypes.City311JSON{
			"department_code": optionalActorDepartment(actor), "event_count": len(events),
		}, CreatedAt: svc.now().UTC(),
	}); err != nil {
		return nil, err
	}
	return &contract.CalendarExport{ContentType: "text/calendar", Body: body}, nil
}

func parseCalendar(raw string) ([]calendarEvent, error) {
	if len(raw) == 0 || len(raw) > maximumCalendarBytes || strings.ContainsRune(raw, '\x00') {
		return nil, fmt.Errorf("calendar payload is empty or too large")
	}
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	physical := strings.Split(raw, "\n")
	lines := make([]string, 0, len(physical))
	for _, line := range physical {
		if (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) && len(lines) > 0 {
			lines[len(lines)-1] += line[1:]
			continue
		}
		lines = append(lines, line)
	}
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "BEGIN:VCALENDAR" {
		return nil, fmt.Errorf("calendar envelope is missing")
	}
	var events []calendarEvent
	var properties map[string]calendarProperty
	inEvent := false
	calendarEnded := false
	versionSeen := false
	for _, rawLine := range lines[1:] {
		line := strings.TrimSpace(rawLine)
		if calendarEnded && line != "" {
			return nil, fmt.Errorf("content follows calendar terminator")
		}
		switch line {
		case "VERSION:2.0":
			if !inEvent {
				versionSeen = true
			}
		case "BEGIN:VEVENT":
			if inEvent {
				return nil, fmt.Errorf("nested event")
			}
			inEvent = true
			properties = map[string]calendarProperty{}
		case "END:VEVENT":
			if !inEvent {
				return nil, fmt.Errorf("event terminator without event")
			}
			event, err := calendarEventFromProperties(properties)
			if err != nil {
				return nil, err
			}
			events = append(events, event)
			if len(events) > maximumCalendarEvents {
				return nil, fmt.Errorf("too many events")
			}
			inEvent = false
			properties = nil
		case "END:VCALENDAR":
			if inEvent {
				return nil, fmt.Errorf("unterminated event")
			}
			calendarEnded = true
		default:
			if !inEvent || line == "" {
				continue
			}
			property, err := parseCalendarProperty(line)
			if err != nil {
				return nil, err
			}
			if _, duplicate := properties[property.Name]; duplicate {
				return nil, fmt.Errorf("duplicate event property")
			}
			properties[property.Name] = property
		}
	}
	if inEvent || len(events) == 0 || !calendarEnded || !versionSeen {
		return nil, fmt.Errorf("calendar is incomplete")
	}
	return events, nil
}

type calendarProperty struct {
	Name   string
	Params map[string]string
	Value  string
}

func parseCalendarProperty(line string) (calendarProperty, error) {
	separator := strings.IndexByte(line, ':')
	if separator < 1 {
		return calendarProperty{}, fmt.Errorf("property value is missing")
	}
	segments := strings.Split(line[:separator], ";")
	property := calendarProperty{Name: strings.ToUpper(strings.TrimSpace(segments[0])), Params: map[string]string{}, Value: line[separator+1:]}
	for _, segment := range segments[1:] {
		pair := strings.SplitN(segment, "=", 2)
		if len(pair) != 2 {
			return calendarProperty{}, fmt.Errorf("property parameter is invalid")
		}
		property.Params[strings.ToUpper(strings.TrimSpace(pair[0]))] = strings.Trim(strings.TrimSpace(pair[1]), `"`)
	}
	return property, nil
}

func calendarEventFromProperties(properties map[string]calendarProperty) (calendarEvent, error) {
	required := []string{"UID", "SUMMARY", "DTSTART", "DTEND", "DESCRIPTION", "STATUS", "LAST-MODIFIED"}
	for _, name := range required {
		if _, ok := properties[name]; !ok {
			return calendarEvent{}, fmt.Errorf("required property %s is missing", name)
		}
	}
	uid := strings.TrimSpace(properties["UID"].Value)
	summary := strings.TrimSpace(unescapeCalendarText(properties["SUMMARY"].Value))
	description := unescapeCalendarText(properties["DESCRIPTION"].Value)
	status := strings.ToUpper(strings.TrimSpace(properties["STATUS"].Value))
	if uid == "" || utf8.RuneCountInString(uid) > 255 || strings.ContainsAny(uid, "\r\n") || summary == "" || utf8.RuneCountInString(summary) > maximumReminderTitleLength {
		return calendarEvent{}, fmt.Errorf("event identity or summary is invalid")
	}
	if status != "CONFIRMED" && status != "TENTATIVE" && status != "COMPLETED" && status != "CANCELLED" {
		return calendarEvent{}, fmt.Errorf("event status is invalid")
	}
	start, timezone, err := parseCalendarDateTime(properties["DTSTART"], false)
	if err != nil {
		return calendarEvent{}, err
	}
	end, endTimezone, err := parseCalendarDateTime(properties["DTEND"], false)
	if err != nil || endTimezone != timezone || !end.After(start) {
		return calendarEvent{}, fmt.Errorf("event end is invalid")
	}
	lastModified, _, err := parseCalendarDateTime(properties["LAST-MODIFIED"], true)
	if err != nil {
		return calendarEvent{}, err
	}
	recurrence := strings.TrimSpace(properties["RRULE"].Value)
	if len(recurrence) > 1024 || strings.ContainsAny(recurrence, "\r\n") {
		return calendarEvent{}, fmt.Errorf("recurrence rule is invalid")
	}
	return calendarEvent{
		UID: uid, Summary: summary, Start: start, End: end, Timezone: timezone, Description: description,
		Status: status, Recurrence: recurrence, LastModified: lastModified,
	}, nil
}

func parseCalendarDateTime(property calendarProperty, requireUTC bool) (time.Time, string, error) {
	value := strings.TrimSpace(property.Value)
	tzid := property.Params["TZID"]
	if strings.HasSuffix(value, "Z") {
		if tzid != "" {
			return time.Time{}, "", fmt.Errorf("UTC time must not have TZID")
		}
		parsed, err := time.Parse("20060102T150405Z", value)
		return parsed.UTC(), "UTC", err
	}
	if requireUTC || tzid != calendarTimezone {
		return time.Time{}, "", fmt.Errorf("local time must identify America/New_York")
	}
	location, err := time.LoadLocation(calendarTimezone)
	if err != nil {
		return time.Time{}, "", err
	}
	parsed, err := time.ParseInLocation("20060102T150405", value, location)
	return parsed.UTC(), calendarTimezone, err
}

func calendarPayload(event calendarEvent, requestID uint64) city311ReminderPayload {
	end := event.End.UTC()
	modified := event.LastModified.UTC()
	return city311ReminderPayload{
		RequestID: requestID, Title: event.Summary, DueAt: event.Start.UTC(), EndAt: &end, Timezone: event.Timezone,
		Description: event.Description, CalendarUID: event.UID, Recurrence: event.Recurrence, LastModified: &modified,
		Channel: contract.ReminderChannelInApp, Status: contract.ReminderStatusScheduled,
	}
}

func applyCalendarStatus(reminder *systemTypes.Reminder, payload *city311ReminderPayload, status string, actorID uint64, now time.Time) {
	switch status {
	case "CANCELLED":
		payload.Status = contract.ReminderStatusCancelled
		payload.CompletedAt = nil
		payload.CompletedBy = 0
		reminder.DismissedAt, reminder.DismissedBy, reminder.RemindAt = &now, actorID, nil
	case "COMPLETED":
		payload.Status = contract.ReminderStatusCompleted
		payload.CompletedAt = &now
		payload.CompletedBy = actorID
		reminder.DismissedAt, reminder.DismissedBy, reminder.RemindAt = &now, actorID, nil
	default:
		payload.Status = contract.ReminderStatusScheduled
		payload.CompletedAt = nil
		payload.CompletedBy = 0
		reminder.DismissedAt, reminder.DismissedBy, reminder.RemindAt = nil, 0, utcTime(payload.DueAt)
	}
}

func calendarReminderUID(reminder *systemTypes.Reminder, payload city311ReminderPayload) string {
	if strings.TrimSpace(payload.CalendarUID) != "" {
		return strings.TrimSpace(payload.CalendarUID)
	}
	return "city311-reminder-" + strconv.FormatUint(reminder.ID, 10) + "@city.example"
}

func calendarReminderResource(actorID uint64) string {
	return "corteza::compose:city311-calendar/" + strconv.FormatUint(actorID, 10)
}

func encodeCalendarEvent(uid string, reminder *systemTypes.Reminder, payload city311ReminderPayload) []string {
	end := payload.DueAt.Add(30 * time.Minute)
	if payload.EndAt != nil && payload.EndAt.After(payload.DueAt) {
		end = payload.EndAt.UTC()
	}
	modified := reminder.CreatedAt.UTC()
	if reminder.UpdatedAt != nil {
		modified = reminder.UpdatedAt.UTC()
	}
	if payload.LastModified != nil {
		modified = payload.LastModified.UTC()
	}
	description := payload.Description
	if description == "" && payload.RequestID != 0 {
		description = "City 311 service request " + strconv.FormatUint(payload.RequestID, 10)
	}
	status := "CONFIRMED"
	if payload.Status == contract.ReminderStatusCancelled {
		status = "CANCELLED"
	} else if payload.Status == contract.ReminderStatusCompleted {
		status = "COMPLETED"
	}
	lines := []string{
		"BEGIN:VEVENT", "UID:" + escapeCalendarText(uid), "SUMMARY:" + escapeCalendarText(payload.Title),
		formatCalendarDateTime("DTSTART", payload.DueAt, payload.Timezone), formatCalendarDateTime("DTEND", end, payload.Timezone),
		"DESCRIPTION:" + escapeCalendarText(description), "STATUS:" + status,
	}
	if payload.Recurrence != "" {
		lines = append(lines, "RRULE:"+payload.Recurrence)
	}
	lines = append(lines, "LAST-MODIFIED:"+modified.Format("20060102T150405Z"), "END:VEVENT")
	return lines
}

func formatCalendarDateTime(name string, value time.Time, timezone string) string {
	if timezone == calendarTimezone {
		location, _ := time.LoadLocation(calendarTimezone)
		return name + ";TZID=" + calendarTimezone + ":" + value.In(location).Format("20060102T150405")
	}
	return name + ":" + value.UTC().Format("20060102T150405Z")
}

func escapeCalendarText(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, ";", `\;`)
	value = strings.ReplaceAll(value, ",", `\,`)
	value = strings.ReplaceAll(value, "\r\n", `\n`)
	value = strings.ReplaceAll(value, "\r", `\n`)
	return strings.ReplaceAll(value, "\n", `\n`)
}

func unescapeCalendarText(value string) string {
	reader := bufio.NewReader(strings.NewReader(value))
	var output strings.Builder
	for {
		r, _, err := reader.ReadRune()
		if err != nil {
			break
		}
		if r != '\\' {
			output.WriteRune(r)
			continue
		}
		next, _, nextErr := reader.ReadRune()
		if nextErr != nil {
			output.WriteRune(r)
			break
		}
		if next == 'n' || next == 'N' {
			output.WriteByte('\n')
		} else {
			output.WriteRune(next)
		}
	}
	return output.String()
}

func calendarValidationError() *ServiceError {
	return validationError(contract.FieldError{Field: "/ics", Code: contract.ValidationInvalidFormat})
}
