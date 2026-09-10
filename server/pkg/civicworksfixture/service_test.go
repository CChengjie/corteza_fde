package civicworksfixture

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func newTestFixture(t *testing.T, client *http.Client, wait func(context.Context, time.Duration) error) *Service {
	t.Helper()
	fixture, err := New(Config{
		APIToken: "consumer-token", ControlToken: "control-token", WebhookSecret: "webhook-secret",
		BenchmarkRun: "run-41", PublicBaseURL: "http://civicworks:8080", HTTPClient: client, Wait: wait,
	})
	require.NoError(t, err)
	return fixture
}

func validCreateInput() contract.CivicWorksWorkOrderCreate {
	return contract.CivicWorksWorkOrderCreate{
		SourceCaseID: "case-7c58d2", ServiceRequestNumber: "SR-2026-00041",
		ServiceType: contract.ServiceTypeTreeMaintenance, Summary: "Fallen branch obstructing pavement",
		DepartmentCode: contract.DepartmentPublicWorks,
		Location:       map[string]any{"address": "100 Example Street", "latitude": 42.9001, "longitude": -78.8801},
		CallbackURL:    "http://application:8080/integrations/civicworks/events",
	}
}

func TestConsumerCreateReplayLookupConflictAndOutage(t *testing.T) {
	fixture := newTestFixture(t, nil, nil)
	handler := fixture.Handler()
	input := validCreateInput()

	created := performJSON(t, handler, http.MethodPost, "/api/v1/work-orders", input, consumerHeaders("key-41"))
	require.Equal(t, http.StatusCreated, created.Code)
	workOrder := contract.CivicWorksWorkOrder{}
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &workOrder))
	require.Equal(t, "WO-000001", workOrder.WorkOrderID)
	require.Equal(t, input.ServiceType, workOrder.ServiceType)
	require.Equal(t, input.Summary, workOrder.Summary)
	require.Equal(t, input.DepartmentCode, workOrder.DepartmentCode)
	require.Equal(t, "CIVICWORKS", workOrder.FulfilmentSource)
	require.Equal(t, input.Location, workOrder.Location)
	require.Equal(t, time.Date(2026, time.August, 20, 10, 0, 0, 0, time.UTC), workOrder.CreatedAt)

	replay := performJSON(t, handler, http.MethodPost, "/api/v1/work-orders", input, consumerHeaders("key-41"))
	require.Equal(t, http.StatusOK, replay.Code)
	require.JSONEq(t, created.Body.String(), replay.Body.String())

	secondKey := performJSON(t, handler, http.MethodPost, "/api/v1/work-orders", input, consumerHeaders("key-41-equivalent"))
	require.Equal(t, http.StatusOK, secondKey.Code)
	require.JSONEq(t, created.Body.String(), secondKey.Body.String())

	changed := input
	changed.Summary = "A materially different summary"
	keyConflict := performJSON(t, handler, http.MethodPost, "/api/v1/work-orders", changed, consumerHeaders("key-41"))
	require.Equal(t, http.StatusConflict, keyConflict.Code)
	require.Contains(t, keyConflict.Body.String(), "IDEMPOTENCY_CONFLICT")
	sourceConflict := performJSON(t, handler, http.MethodPost, "/api/v1/work-orders", changed, consumerHeaders("other-key"))
	require.Equal(t, http.StatusConflict, sourceConflict.Code)
	require.Contains(t, sourceConflict.Body.String(), "SOURCE_CASE_CONFLICT")

	lookup := performJSON(t, handler, http.MethodGet, "/api/v1/work-orders/WO-000001", nil, consumerHeaders(""))
	require.Equal(t, http.StatusOK, lookup.Code)
	require.JSONEq(t, created.Body.String(), lookup.Body.String())
	query := performJSON(t, handler, http.MethodGet, "/api/v1/work-orders?source_case_id=case-7c58d2", nil, consumerHeaders(""))
	require.Equal(t, http.StatusOK, query.Code)
	var result struct {
		Items []workOrderListItem `json:"items"`
	}
	require.NoError(t, json.Unmarshal(query.Body.Bytes(), &result))
	require.Len(t, result.Items, 1)
	require.Equal(t, "WO-000001", result.Items[0].WorkOrderID)

	outage := performJSON(t, handler, http.MethodPut, "/__fixture/v1/outage", outageRequest{Enabled: true}, controlHeaders())
	require.Equal(t, http.StatusOK, outage.Code)
	unavailable := performJSON(t, handler, http.MethodGet, "/api/v1/work-orders/WO-000001", nil, consumerHeaders(""))
	require.Equal(t, http.StatusServiceUnavailable, unavailable.Code)
	require.Contains(t, unavailable.Body.String(), "TEMPORARILY_UNAVAILABLE")
	health := performJSON(t, handler, http.MethodGet, "/healthz", nil, nil)
	require.Equal(t, http.StatusOK, health.Code)

	reset := performJSON(t, handler, http.MethodPost, "/__fixture/v1/reset", nil, controlHeaders())
	require.Equal(t, http.StatusOK, reset.Code)
	require.JSONEq(t, `{"status":"reset","next_work_order_id":"WO-000001","next_event_id":"EVT-000031"}`, reset.Body.String())
	notFound := performJSON(t, handler, http.MethodGet, "/api/v1/work-orders/WO-000001", nil, consumerHeaders(""))
	require.Equal(t, http.StatusNotFound, notFound.Code)
}

func TestControlAdvanceSignsRetriesInspectsAndRedelivers(t *testing.T) {
	statuses := []int{http.StatusServiceUnavailable, http.StatusInternalServerError, http.StatusNoContent, http.StatusOK}
	var requests []*http.Request
	var bodies [][]byte
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(request.Body)
		require.NoError(t, err)
		requests = append(requests, request.Clone(request.Context()))
		bodies = append(bodies, body)
		status := statuses[len(requests)-1]
		return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewBufferString(`{}`)), Header: make(http.Header)}, nil
	})}
	var waits []time.Duration
	fixture := newTestFixture(t, client, func(_ context.Context, delay time.Duration) error {
		waits = append(waits, delay)
		return nil
	})
	_, status, err := fixture.Create(validCreateInput(), "key-41")
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)

	result, err := fixture.Advance(context.Background(), "WO-000001", contract.CivicWorksStatusInProgress)
	require.NoError(t, err)
	require.Equal(t, "EVT-000031", result.Event.EventID)
	require.Equal(t, uint64(2), result.WorkOrder.Version)
	require.Equal(t, time.Date(2026, time.August, 20, 10, 5, 0, 0, time.UTC), result.Event.OccurredAt)
	require.True(t, result.Delivery.Acknowledged)
	require.Equal(t, 3, result.Delivery.Attempts)
	require.Equal(t, []time.Duration{time.Second, 5 * time.Second}, waits)
	require.Len(t, requests, 3)

	for index, request := range requests {
		require.Equal(t, "EVT-000031", request.Header.Get("X-CivicWorks-Event-Id"))
		require.Equal(t, "application/json", request.Header.Get("Content-Type"))
		digest := hmac.New(sha256.New, []byte("webhook-secret"))
		_, _ = digest.Write(bodies[index])
		require.Equal(t, "sha256="+hex.EncodeToString(digest.Sum(nil)), request.Header.Get("X-CivicWorks-Signature"))
		require.Equal(t, bodies[0], bodies[index])
	}
	attempts := fixture.DeliveryAttempts()
	require.Len(t, attempts, 3)
	require.Equal(t, result.Event.OccurredAt, attempts[0].AttemptedAt)
	require.Equal(t, result.Event.OccurredAt.Add(time.Second), attempts[1].AttemptedAt)
	require.Equal(t, result.Event.OccurredAt.Add(6*time.Second), attempts[2].AttemptedAt)

	redelivery, err := fixture.Redeliver(context.Background(), "EVT-000031")
	require.NoError(t, err)
	require.True(t, redelivery.Acknowledged)
	require.Equal(t, 1, redelivery.Attempts)
	require.Len(t, requests, 4)
	require.Equal(t, bodies[0], bodies[3])
	require.Equal(t, requests[0].Header.Get("X-CivicWorks-Signature"), requests[3].Header.Get("X-CivicWorks-Signature"))

	_, err = fixture.Advance(context.Background(), "WO-000001", contract.CivicWorksStatusAssigned)
	assertFixtureError(t, err, http.StatusConflict, "INVALID_STATUS_TRANSITION")
	_, err = fixture.Redeliver(context.Background(), "EVT-999999")
	assertFixtureError(t, err, http.StatusNotFound, "EVENT_NOT_FOUND")
}

func TestFixtureAuthenticationValidationAndConfiguration(t *testing.T) {
	fixture := newTestFixture(t, nil, nil)
	handler := fixture.Handler()
	input := validCreateInput()

	missing := performJSON(t, handler, http.MethodPost, "/api/v1/work-orders", input, map[string]string{"Content-Type": "application/json"})
	require.Equal(t, http.StatusUnauthorized, missing.Code)
	wrongRunHeaders := consumerHeaders("key")
	wrongRunHeaders["X-Benchmark-Run-Id"] = "other-run"
	wrongRun := performJSON(t, handler, http.MethodPost, "/api/v1/work-orders", input, wrongRunHeaders)
	require.Equal(t, http.StatusForbidden, wrongRun.Code)
	wrongMediaHeaders := consumerHeaders("key")
	wrongMediaHeaders["Content-Type"] = "text/plain"
	wrongMedia := performJSON(t, handler, http.MethodPost, "/api/v1/work-orders", input, wrongMediaHeaders)
	require.Equal(t, http.StatusUnprocessableEntity, wrongMedia.Code)

	invalid := input
	invalid.CallbackURL = "/integrations/civicworks/events"
	invalidResponse := performJSON(t, handler, http.MethodPost, "/api/v1/work-orders", invalid, consumerHeaders("key"))
	require.Equal(t, http.StatusUnprocessableEntity, invalidResponse.Code)
	require.Contains(t, invalidResponse.Body.String(), "callback_url")

	_, err := New(Config{APIToken: "same", ControlToken: "same", WebhookSecret: "secret", BenchmarkRun: "run"})
	require.EqualError(t, err, "CIVICWORKS_API_TOKEN and CIVICWORKS_CONTROL_TOKEN must differ")
	_, err = New(Config{APIToken: "api", ControlToken: "control", WebhookSecret: "secret", BenchmarkRun: "run", PublicBaseURL: "relative"})
	require.EqualError(t, err, "CIVICWORKS_PUBLIC_BASE_URL must be an absolute HTTP or HTTPS URL without credentials, query, or fragment")
}

func TestPollingOnlyWorkOrderAdvancesWithoutCallbackDelivery(t *testing.T) {
	fixture := newTestFixture(t, nil, nil)
	input := validCreateInput()
	input.SourceCaseID = "polling-only-case"
	input.ServiceRequestNumber = "SR-2026-00042"
	input.CallbackURL = ""
	require.Nil(t, ValidateCreate(input, "polling-only-key"))
	workOrder, status, err := fixture.Create(input, "polling-only-key")
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)
	result, err := fixture.Advance(context.Background(), workOrder.WorkOrderID, contract.CivicWorksStatusInProgress)
	require.NoError(t, err)
	require.False(t, result.Delivery.Acknowledged)
	require.Zero(t, result.Delivery.Attempts)
	require.Empty(t, fixture.DeliveryAttempts())
}

func TestControlHTTPAdvanceRedeliveryAndDeliveryInspection(t *testing.T) {
	callbackClient := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(bytes.NewReader(nil)), Header: make(http.Header)}, nil
	})}
	fixture := newTestFixture(t, callbackClient, func(context.Context, time.Duration) error { return nil })
	handler := fixture.Handler()
	created := performJSON(t, handler, http.MethodPost, "/api/v1/work-orders", validCreateInput(), consumerHeaders("key-41"))
	require.Equal(t, http.StatusCreated, created.Code)

	advanced := performJSON(t, handler, http.MethodPost, "/__fixture/v1/work-orders/WO-000001/status", statusAdvanceRequest{Status: contract.CivicWorksStatusInProgress}, controlHeaders())
	require.Equal(t, http.StatusOK, advanced.Code)
	require.Contains(t, advanced.Body.String(), `"event_id":"EVT-000031"`)
	deliveries := performJSON(t, handler, http.MethodGet, "/__fixture/v1/deliveries", nil, controlHeaders())
	require.Equal(t, http.StatusOK, deliveries.Code)
	require.Contains(t, deliveries.Body.String(), `"attempt":1`)

	redelivered := performJSON(t, handler, http.MethodPost, "/__fixture/v1/events/EVT-000031/redeliver", nil, controlHeaders())
	require.Equal(t, http.StatusOK, redelivered.Code)
	require.JSONEq(t, `{"acknowledged":true,"attempts":1,"status":204}`, redelivered.Body.String())
	missingEvent := performJSON(t, handler, http.MethodPost, "/__fixture/v1/events/EVT-999999/redeliver", nil, controlHeaders())
	require.Equal(t, http.StatusNotFound, missingEvent.Code)
	require.NotContains(t, missingEvent.Body.String(), "Status")

	invalidVocabulary := performJSON(t, handler, http.MethodPost, "/__fixture/v1/work-orders/WO-000001/status", map[string]string{"status": "UNKNOWN"}, controlHeaders())
	require.Equal(t, http.StatusUnprocessableEntity, invalidVocabulary.Code)
	invalidTransition := performJSON(t, handler, http.MethodPost, "/__fixture/v1/work-orders/WO-000001/status", statusAdvanceRequest{Status: contract.CivicWorksStatusAssigned}, controlHeaders())
	require.Equal(t, http.StatusConflict, invalidTransition.Code)
	missingOrder := performJSON(t, handler, http.MethodPost, "/__fixture/v1/work-orders/WO-999999/status", statusAdvanceRequest{Status: contract.CivicWorksStatusCompleted}, controlHeaders())
	require.Equal(t, http.StatusNotFound, missingOrder.Code)
}

func TestConsumerValidationRejectsMalformedInputs(t *testing.T) {
	fixture := newTestFixture(t, nil, nil)
	handler := fixture.Handler()
	missingKey := performJSON(t, handler, http.MethodPost, "/api/v1/work-orders", validCreateInput(), consumerHeaders(""))
	require.Equal(t, http.StatusUnprocessableEntity, missingKey.Code)
	badQuery := performJSON(t, handler, http.MethodGet, "/api/v1/work-orders?source_case_id=case&extra=value", nil, consumerHeaders(""))
	require.Equal(t, http.StatusUnprocessableEntity, badQuery.Code)
	badJSON := httptest.NewRequest(http.MethodPost, "/api/v1/work-orders", bytes.NewBufferString(`{"source_case_id":`))
	for key, value := range consumerHeaders("key") {
		badJSON.Header.Set(key, value)
	}
	badJSONResponse := httptest.NewRecorder()
	handler.ServeHTTP(badJSONResponse, badJSON)
	require.Equal(t, http.StatusUnprocessableEntity, badJSONResponse.Code)

	for name, location := range map[string]map[string]any{
		"unknown field":  {"address": "100 Example Street", "floor": 2},
		"one coordinate": {"address": "100 Example Street", "latitude": 42.9},
		"outside range":  {"address": "100 Example Street", "latitude": 142.9, "longitude": -78.8},
	} {
		t.Run(name, func(t *testing.T) {
			input := validCreateInput()
			input.SourceCaseID = name
			input.Location = location
			response := performJSON(t, handler, http.MethodPost, "/api/v1/work-orders", input, consumerHeaders("key-"+name))
			require.Equal(t, http.StatusUnprocessableEntity, response.Code)
		})
	}
}

func performJSON(t *testing.T, handler http.Handler, method, target string, body any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var encoded []byte
	var err error
	if body != nil {
		encoded, err = json.Marshal(body)
		require.NoError(t, err)
	}
	request := httptest.NewRequest(method, target, bytes.NewReader(encoded))
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func consumerHeaders(idempotencyKey string) map[string]string {
	headers := map[string]string{
		"Authorization": "Bearer consumer-token", "X-Benchmark-Run-Id": "run-41", "Content-Type": "application/json",
	}
	if idempotencyKey != "" {
		headers[contract.IdempotencyHeader] = idempotencyKey
	}
	return headers
}

func controlHeaders() map[string]string {
	return map[string]string{
		"Authorization": "Bearer control-token", "X-Benchmark-Run-Id": "run-41", "Content-Type": "application/json",
	}
}

func assertFixtureError(t *testing.T, err error, status int, code string) {
	t.Helper()
	fixtureErr, ok := err.(*FixtureError)
	require.True(t, ok)
	require.Equal(t, status, fixtureErr.Status)
	require.Equal(t, code, fixtureErr.Code)
}
