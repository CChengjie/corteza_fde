package city311

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/pkg/auth"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

func TestCalendarHTTPImportOperationAndExport(t *testing.T) {
	router, st, _ := testRouter(t)
	ctx := context.Background()
	agent, err := store.LookupUserByEmail(ctx, st, "service-agent@city311.example.invalid")
	require.NoError(t, err)
	constituent, err := store.LookupUserByEmail(ctx, st, "constituent1@city311.example.invalid")
	require.NoError(t, err)
	body := map[string]any{"ics": "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:http-1@city.example\r\nSUMMARY:HTTP calendar\r\nDTSTART:20260905T150000Z\r\nDTEND:20260905T153000Z\r\nDESCRIPTION:Imported over HTTP\r\nSTATUS:CONFIRMED\r\nRRULE:FREQ=DAILY;COUNT=2\r\nLAST-MODIFIED:20260904T120000Z\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"}

	unauthenticated := executeJSON(t, router, http.MethodPost, "/api/v1/staff/calendar/import", body, nil, 0)
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)
	forbidden := executeJSON(t, router, http.MethodPost, "/api/v1/staff/calendar/import", body, nil, constituent.ID)
	require.Equal(t, http.StatusForbidden, forbidden.Code, forbidden.Body.String())
	invalid := executeJSON(t, router, http.MethodPost, "/api/v1/staff/calendar/import", map[string]any{"ics": "invalid"}, nil, agent.ID)
	require.Equal(t, http.StatusUnprocessableEntity, invalid.Code, invalid.Body.String())
	request := httptest.NewRequest(http.MethodPost, "/api/v1/staff/calendar/import", bytes.NewBufferString(`{"ics":`))
	request.Header.Set("Content-Type", "application/json")
	request = request.WithContext(auth.SetIdentityToContext(request.Context(), auth.Authenticated(agent.ID)))
	malformed := httptest.NewRecorder()
	router.ServeHTTP(malformed, request)
	require.Equal(t, http.StatusUnprocessableEntity, malformed.Code, malformed.Body.String())

	accepted := executeJSON(t, router, http.MethodPost, "/api/v1/staff/calendar/import", body, nil, agent.ID)
	require.Equal(t, http.StatusAccepted, accepted.Code, accepted.Body.String())
	var pending contract.Operation
	require.NoError(t, json.Unmarshal(accepted.Body.Bytes(), &pending))
	require.Equal(t, contract.OperationStatusPending, pending.Status)
	completed := executeJSON(t, router, http.MethodGet, "/api/v1/operations/"+pending.OperationID, nil, nil, agent.ID)
	require.Equal(t, http.StatusOK, completed.Code, completed.Body.String())
	require.Contains(t, completed.Body.String(), `"status":"SUCCEEDED"`)
	require.Contains(t, completed.Body.String(), `"created":1`)

	exported := executeJSON(t, router, http.MethodGet, "/api/v1/staff/calendar/export", nil, nil, agent.ID)
	require.Equal(t, http.StatusOK, exported.Code, exported.Body.String())
	var calendar contract.CalendarExport
	require.NoError(t, json.Unmarshal(exported.Body.Bytes(), &calendar))
	require.Equal(t, "text/calendar", calendar.ContentType)
	require.Contains(t, calendar.Body, "UID:http-1@city.example\r\n")
	require.Contains(t, calendar.Body, "RRULE:FREQ=DAILY;COUNT=2\r\n")

	forbiddenExport := executeJSON(t, router, http.MethodGet, "/api/v1/staff/calendar/export", nil, nil, constituent.ID)
	require.Equal(t, http.StatusForbidden, forbiddenExport.Code, forbiddenExport.Body.String())
}
