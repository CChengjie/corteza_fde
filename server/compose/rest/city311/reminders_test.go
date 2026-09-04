package city311

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

func TestStaffReminderHTTPContracts(t *testing.T) {
	router, st, _ := testRouter(t)
	ctx := context.Background()
	request, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, "SR-2026-00034")
	require.NoError(t, err)
	agent, err := store.LookupUserByEmail(ctx, st, "service-agent@city311.example.invalid")
	require.NoError(t, err)
	supervisor, err := store.LookupUserByEmail(ctx, st, "supervisor@city311.example.invalid")
	require.NoError(t, err)
	manager, err := store.LookupUserByEmail(ctx, st, "department-manager@city311.example.invalid")
	require.NoError(t, err)
	createPath := "/api/v1/staff/service-requests/" + strconv.FormatUint(request.ID, 10) + "/reminders"
	body := map[string]any{
		"title": "Inspect completed repair", "due_at": "2026-02-04T15:04:05Z", "timezone": "America/New_York",
		"recipient_staff_id": strconv.FormatUint(agent.ID, 10), "channel": "IN_APP",
	}

	unauthenticated := executeJSON(t, router, http.MethodPost, createPath, body, nil, 0)
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)
	forbidden := executeJSON(t, router, http.MethodPost, createPath, body, nil, manager.ID)
	require.Equal(t, http.StatusForbidden, forbidden.Code, forbidden.Body.String())
	created := executeJSON(t, router, http.MethodPost, createPath, body, nil, agent.ID)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	var reminder contract.Reminder
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &reminder))
	require.Equal(t, contract.ReminderStatusScheduled, reminder.Status)

	actionPath := "/api/v1/staff/reminders/" + reminder.ReminderID
	invalidAction := executeJSON(t, router, http.MethodPost, actionPath+"/DEFER", map[string]any{}, nil, supervisor.ID)
	require.Equal(t, http.StatusUnprocessableEntity, invalidAction.Code, invalidAction.Body.String())
	missingDueAt := executeJSON(t, router, http.MethodPost, actionPath+"/SNOOZE", map[string]any{}, nil, agent.ID)
	require.Equal(t, http.StatusUnprocessableEntity, missingDueAt.Code, missingDueAt.Body.String())
	snoozed := executeJSON(t, router, http.MethodPost, actionPath+"/SNOOZE", map[string]any{"due_at": "2026-02-05T15:04:05Z"}, nil, agent.ID)
	require.Equal(t, http.StatusOK, snoozed.Code, snoozed.Body.String())
	require.Contains(t, snoozed.Body.String(), `"status":"SNOOZED"`)

	completed := executeJSON(t, router, http.MethodPost, actionPath+"/COMPLETE", map[string]any{}, nil, supervisor.ID)
	require.Equal(t, http.StatusOK, completed.Code, completed.Body.String())
	require.Contains(t, completed.Body.String(), `"status":"COMPLETED"`)
	replayed := executeJSON(t, router, http.MethodPost, actionPath+"/COMPLETE", map[string]any{}, nil, supervisor.ID)
	require.Equal(t, completed.Code, replayed.Code)
	require.JSONEq(t, completed.Body.String(), replayed.Body.String())

	invalidID := executeJSON(t, router, http.MethodPost, "/api/v1/staff/reminders/not-a-number/COMPLETE", map[string]any{}, nil, supervisor.ID)
	require.Equal(t, http.StatusUnprocessableEntity, invalidID.Code)
	missing := executeJSON(t, router, http.MethodPost, "/api/v1/staff/reminders/999999999/COMPLETE", map[string]any{}, nil, supervisor.ID)
	require.Equal(t, http.StatusNotFound, missing.Code)
	detail := executeJSON(t, router, http.MethodGet, "/api/v1/staff/service-requests/"+strconv.FormatUint(request.ID, 10), nil, nil, supervisor.ID)
	require.Equal(t, http.StatusOK, detail.Code, detail.Body.String())
	require.Contains(t, detail.Body.String(), `"reminders":[{"reminder_id":"`+reminder.ReminderID+`"`)
}
