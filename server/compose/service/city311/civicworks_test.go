package city311

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

type civicWorksStub struct {
	calls       int
	inputs      []contract.CivicWorksWorkOrderCreate
	keys        []string
	workOrderID string
	err         error
}

func (client *civicWorksStub) CreateWorkOrder(_ context.Context, input contract.CivicWorksWorkOrderCreate, key string) (*contract.CivicWorksWorkOrder, error) {
	client.calls++
	client.inputs = append(client.inputs, input)
	client.keys = append(client.keys, key)
	if client.err != nil {
		return nil, client.err
	}
	workOrderID := client.workOrderID
	if workOrderID == "" {
		workOrderID = "WO-000034"
	}
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	return &contract.CivicWorksWorkOrder{
		WorkOrderID: workOrderID, SourceCaseID: input.SourceCaseID, ServiceRequestNumber: input.ServiceRequestNumber,
		Status: contract.CivicWorksStatusAssigned, ExternalStatusURL: "https://civicworks.example.invalid/ui/work-orders/" + workOrderID,
		Version: 1, CreatedAt: now, UpdatedAt: now,
	}, nil
}

func TestCivicWorksAssignmentIsAtomicAndIdempotent(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	agent := seededAssignmentActor(t, ctx, svc, st, "service-agent@city311.example.invalid")
	request, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, "SR-2026-00034")
	require.NoError(t, err)
	_, err = svc.Transition(ctx, agent, request.ID, 1, contract.RequestTransition{ToStatus: contract.ServiceRequestStatusTriaged})
	require.NoError(t, err)

	failing := &civicWorksStub{err: civicWorksUnavailableError()}
	svc.SetCivicWorks(failing, "webhook-secret")
	_, err = svc.Transition(ctx, agent, request.ID, 2, contract.RequestTransition{ToStatus: contract.ServiceRequestStatusAssigned})
	requireServiceError(t, err, http.StatusServiceUnavailable, contract.ErrorTemporarilyUnavailable)
	unchanged, err := store.LookupCity311ServiceRequestByID(ctx, st, request.ID)
	require.NoError(t, err)
	require.Equal(t, contract.ServiceRequestStatusTriaged, unchanged.Status)
	require.Equal(t, 2, unchanged.Version)
	require.Empty(t, unchanged.ExternalWorkOrder)

	client := &civicWorksStub{}
	svc.SetCivicWorks(client, "webhook-secret")
	detail, err := svc.Transition(ctx, agent, request.ID, 2, contract.RequestTransition{ToStatus: contract.ServiceRequestStatusAssigned})
	require.NoError(t, err)
	require.Equal(t, contract.ServiceRequestStatusAssigned, detail.Request.Status)
	require.Equal(t, uint64(3), detail.Request.Version)
	require.Equal(t, "WO-000034", detail.ExternalWorkOrder.WorkOrderID)
	require.Equal(t, 1, client.calls)
	require.Equal(t, civicWorksIdempotencyKey(request.ID), client.keys[0])
	require.Equal(t, civicWorksCallbackPath, client.inputs[0].CallbackURL)
	require.Equal(t, "city311-case-"+detail.Request.RequestID, client.inputs[0].SourceCaseID)

	replayed, err := svc.Transition(ctx, agent, request.ID, 3, contract.RequestTransition{ToStatus: contract.ServiceRequestStatusAssigned})
	require.NoError(t, err)
	require.Equal(t, uint64(3), replayed.Request.Version)
	require.Equal(t, 1, client.calls)

	_, err = svc.Transition(ctx, agent, request.ID, 2, contract.RequestTransition{ToStatus: contract.ServiceRequestStatusAssigned})
	requireServiceError(t, err, http.StatusConflict, contract.ErrorVersionConflict)
	created, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{EventType: civicWorksCreatedAudit})
	require.NoError(t, err)
	require.Len(t, created, 1)
}

func TestCivicWorksEventsVerifySignatureVersionAndLifecycle(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	agent := seededAssignmentActor(t, ctx, svc, st, "service-agent@city311.example.invalid")
	request, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, "SR-2026-00034")
	require.NoError(t, err)
	_, err = svc.Transition(ctx, agent, request.ID, 1, contract.RequestTransition{ToStatus: contract.ServiceRequestStatusTriaged})
	require.NoError(t, err)
	svc.SetCivicWorks(&civicWorksStub{}, "webhook-secret")
	_, err = svc.Transition(ctx, agent, request.ID, 2, contract.RequestTransition{ToStatus: contract.ServiceRequestStatusAssigned})
	require.NoError(t, err)

	inProgress := contract.CivicWorksEvent{
		EventID: "EVT-2", EventType: "work_order.status_changed", WorkOrderID: "WO-000034",
		SourceCaseID: "city311-case-" + detailID(request.ID), PreviousStatus: contract.CivicWorksStatusAssigned,
		Status: contract.CivicWorksStatusInProgress, Version: 2, OccurredAt: svc.now(),
	}
	body := civicWorksEventBody(t, inProgress)
	err = svc.HandleCivicWorksEvent(ctx, body, inProgress.EventID, "bad")
	requireServiceError(t, err, http.StatusUnauthorized, contract.ErrorInvalidSignature)
	before, err := store.LookupCity311ServiceRequestByID(ctx, st, request.ID)
	require.NoError(t, err)
	require.Equal(t, contract.ServiceRequestStatusAssigned, before.Status)

	err = svc.HandleCivicWorksEvent(ctx, body, inProgress.EventID, civicWorksSignature(body, "webhook-secret"))
	require.NoError(t, err)
	applied, err := store.LookupCity311ServiceRequestByID(ctx, st, request.ID)
	require.NoError(t, err)
	require.Equal(t, contract.ServiceRequestStatusInProgress, applied.Status)
	require.Equal(t, 4, applied.Version)
	require.Equal(t, float64(2), applied.ExternalWorkOrder["version"])

	require.NoError(t, svc.HandleCivicWorksEvent(ctx, body, inProgress.EventID, "sha256="+civicWorksSignature(body, "webhook-secret")))
	duplicate, err := store.LookupCity311ServiceRequestByID(ctx, st, request.ID)
	require.NoError(t, err)
	require.Equal(t, 4, duplicate.Version)

	low := inProgress
	low.EventID = "EVT-low"
	low.Version = 1
	lowBody := civicWorksEventBody(t, low)
	require.NoError(t, svc.HandleCivicWorksEvent(ctx, lowBody, low.EventID, civicWorksSignature(lowBody, "webhook-secret")))
	ignored, err := store.LookupCity311ServiceRequestByID(ctx, st, request.ID)
	require.NoError(t, err)
	require.Equal(t, 4, ignored.Version)

	completed := inProgress
	completed.EventID = "EVT-3"
	completed.PreviousStatus = contract.CivicWorksStatusInProgress
	completed.Status = contract.CivicWorksStatusCompleted
	completed.Version = 3
	completedBody := civicWorksEventBody(t, completed)
	require.NoError(t, svc.HandleCivicWorksEvent(ctx, completedBody, completed.EventID, civicWorksSignature(completedBody, "webhook-secret")))
	resolved, err := store.LookupCity311ServiceRequestByID(ctx, st, request.ID)
	require.NoError(t, err)
	require.Equal(t, contract.ServiceRequestStatusResolved, resolved.Status)
	require.Equal(t, 5, resolved.Version)

	appliedEvents, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{EventType: civicWorksEventApplied})
	require.NoError(t, err)
	require.Len(t, appliedEvents, 2)
	ignoredEvents, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{EventType: civicWorksEventIgnored})
	require.NoError(t, err)
	require.Len(t, ignoredEvents, 1)
}

func TestCivicWorksDirectCompletionUsesLegalAtomicTransitionPlan(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	administrator := seededAssignmentActor(t, ctx, svc, st, "platform-admin@city311.example.invalid")
	request, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, "SR-2026-00035")
	require.NoError(t, err)
	svc.SetCivicWorks(&civicWorksStub{workOrderID: "WO-DIRECT"}, "webhook-secret")
	_, err = svc.Transition(ctx, administrator, request.ID, 1, contract.RequestTransition{ToStatus: contract.ServiceRequestStatusAssigned})
	require.NoError(t, err)
	event := contract.CivicWorksEvent{
		EventID: "EVT-DIRECT", EventType: "work_order.status_changed", WorkOrderID: "WO-DIRECT",
		SourceCaseID: "city311-case-" + detailID(request.ID), PreviousStatus: contract.CivicWorksStatusAssigned,
		Status: contract.CivicWorksStatusCompleted, Version: 2, OccurredAt: svc.now(),
	}
	body := civicWorksEventBody(t, event)
	require.NoError(t, svc.HandleCivicWorksEvent(ctx, body, event.EventID, civicWorksSignature(body, "webhook-secret")))
	resolved, err := store.LookupCity311ServiceRequestByID(ctx, st, request.ID)
	require.NoError(t, err)
	require.Equal(t, contract.ServiceRequestStatusResolved, resolved.Status)
	require.Equal(t, 4, resolved.Version)
	history, _, err := store.SearchCity311PublicHistoryItems(ctx, st, composeTypes.City311PublicHistoryItemFilter{RequestID: request.ID})
	require.NoError(t, err)
	require.Equal(t, "IN_PROGRESS", history[len(history)-2].Action)
	require.Equal(t, "RESOLVED", history[len(history)-1].Action)
}

func TestCivicWorksHTTPClientRetriesWithStableContractHeaders(t *testing.T) {
	calls := 0
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, "Bearer fixture-token", r.Header.Get("Authorization"))
		require.Equal(t, "run-41", r.Header.Get("X-Benchmark-Run-Id"))
		require.Equal(t, "stable-key", r.Header.Get(contract.IdempotencyHeader))
		input := contract.CivicWorksWorkOrderCreate{}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&input))
		if calls == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
		writeJSONFixture(t, w, http.StatusCreated, contract.CivicWorksWorkOrder{
			WorkOrderID: "WO-41", SourceCaseID: input.SourceCaseID, ServiceRequestNumber: input.ServiceRequestNumber,
			Status: contract.CivicWorksStatusAssigned, ExternalStatusURL: fixtureURL(r, "/ui/work-orders/WO-41"),
			Version: 1, CreatedAt: now, UpdatedAt: now,
		})
	}))
	defer fixture.Close()
	client, err := NewCivicWorks(CivicWorksOptions{
		BaseURL: fixture.URL, APIToken: "fixture-token", WebhookSecret: "webhook-secret", BenchmarkRunID: "run-41", HTTPClient: fixture.Client(),
	})
	require.NoError(t, err)
	input := contract.CivicWorksWorkOrderCreate{
		SourceCaseID: "case-41", ServiceRequestNumber: "SR-2026-00041", ServiceType: contract.ServiceTypePothole,
		Summary: "Pothole blocking lane", DepartmentCode: contract.DepartmentStreets, CallbackURL: civicWorksCallbackPath,
	}
	result, err := client.CreateWorkOrder(context.Background(), input, "stable-key")
	require.NoError(t, err)
	require.Equal(t, "WO-41", result.WorkOrderID)
	require.Equal(t, 2, calls)
}

func TestCivicWorksConfigurationAndMalformedResponses(t *testing.T) {
	for name, options := range map[string]CivicWorksOptions{
		"url":        {APIToken: "token", WebhookSecret: "secret", BenchmarkRunID: "run"},
		"token":      {BaseURL: "https://example.invalid", WebhookSecret: "secret", BenchmarkRunID: "run"},
		"secret":     {BaseURL: "https://example.invalid", APIToken: "token", BenchmarkRunID: "run"},
		"run id":     {BaseURL: "https://example.invalid", APIToken: "token", WebhookSecret: "secret"},
		"bad scheme": {BaseURL: "file:///tmp", APIToken: "token", WebhookSecret: "secret", BenchmarkRunID: "run"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := NewCivicWorks(options)
			require.Error(t, err)
		})
	}

	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"work_order_id":""}`))
	}))
	defer fixture.Close()
	client, err := NewCivicWorks(CivicWorksOptions{BaseURL: fixture.URL, APIToken: "token", WebhookSecret: "secret", BenchmarkRunID: "run", HTTPClient: fixture.Client()})
	require.NoError(t, err)
	_, err = client.CreateWorkOrder(context.Background(), contract.CivicWorksWorkOrderCreate{}, "key")
	requireServiceError(t, err, http.StatusServiceUnavailable, contract.ErrorTemporarilyUnavailable)

	_, err = decodeCivicWorksEvent([]byte(`{}`), "event")
	requireServiceError(t, err, http.StatusUnprocessableEntity, contract.ErrorValidation)
	require.False(t, validCivicWorksSignature([]byte("body"), "not-hex", "secret"))
}

func TestCivicWorksHTTPClientMapsPermanentAndContractFailures(t *testing.T) {
	input := contract.CivicWorksWorkOrderCreate{SourceCaseID: "case-1", ServiceRequestNumber: "SR-2026-00041"}
	for name, status := range map[string]int{
		"conflict": http.StatusConflict, "validation": http.StatusUnprocessableEntity, "permanent": http.StatusUnauthorized,
	} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls++
				w.WriteHeader(status)
			}))
			defer fixture.Close()
			client, err := NewCivicWorks(CivicWorksOptions{BaseURL: fixture.URL, APIToken: "token", WebhookSecret: "secret", BenchmarkRunID: "run", HTTPClient: fixture.Client()})
			require.NoError(t, err)
			_, err = client.CreateWorkOrder(context.Background(), input, "key")
			var expected contract.ErrorCode
			switch status {
			case http.StatusConflict:
				expected = contract.ErrorIdempotencyConflict
			case http.StatusUnprocessableEntity:
				expected = contract.ErrorValidation
			default:
				expected = contract.ErrorTemporarilyUnavailable
			}
			var serviceErr *ServiceError
			require.ErrorAs(t, err, &serviceErr)
			require.Equal(t, expected, serviceErr.Payload.Error)
			require.Equal(t, 1, calls)
		})
	}

	t.Run("oversized response retries", func(t *testing.T) {
		calls := 0
		fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls++
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write(make([]byte, civicWorksResponseLimit+1))
		}))
		defer fixture.Close()
		client, err := NewCivicWorks(CivicWorksOptions{BaseURL: fixture.URL, APIToken: "token", WebhookSecret: "secret", BenchmarkRunID: "run", HTTPClient: fixture.Client()})
		require.NoError(t, err)
		_, err = client.CreateWorkOrder(context.Background(), input, "key")
		requireServiceError(t, err, http.StatusServiceUnavailable, contract.ErrorTemporarilyUnavailable)
		require.Equal(t, 2, calls)
	})
}

func TestCivicWorksAssignmentAndEventErrorBranches(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	agent := seededAssignmentActor(t, ctx, svc, st, "service-agent@city311.example.invalid")
	request, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, "SR-2026-00034")
	require.NoError(t, err)
	_, err = svc.Transition(ctx, agent, request.ID, 1, contract.RequestTransition{ToStatus: contract.ServiceRequestStatusTriaged})
	require.NoError(t, err)

	svc.SetCivicWorks(nil, "")
	_, err = svc.AssignCivicWorks(ctx, agent, request.ID, 2)
	requireServiceError(t, err, http.StatusServiceUnavailable, contract.ErrorTemporarilyUnavailable)
	_, err = svc.AssignCivicWorks(ctx, agent, 999999999, 1)
	requireServiceError(t, err, http.StatusNotFound, contract.ErrorNotFound)

	client := &civicWorksStub{}
	svc.SetCivicWorks(client, "webhook-secret")
	_, err = svc.AssignCivicWorks(ctx, contract.Actor{ID: 42, Roles: []contract.ApplicationRole{contract.ApplicationRoleConstituent}}, request.ID, 2)
	requireServiceError(t, err, http.StatusForbidden, contract.ErrorForbidden)
	_, err = svc.AssignCivicWorks(ctx, agent, request.ID, 2)
	require.NoError(t, err)

	current := contract.CivicWorksEvent{
		EventID: "EVT-current", EventType: "work_order.status_changed", WorkOrderID: "WO-000034",
		SourceCaseID: "city311-case-" + detailID(request.ID), PreviousStatus: contract.CivicWorksStatusAssigned,
		Status: contract.CivicWorksStatusAssigned, Version: 2, OccurredAt: svc.now(),
	}
	body := civicWorksEventBody(t, current)
	require.NoError(t, svc.HandleCivicWorksEvent(ctx, body, current.EventID, civicWorksSignature(body, "webhook-secret")))
	updated, err := store.LookupCity311ServiceRequestByID(ctx, st, request.ID)
	require.NoError(t, err)
	require.Equal(t, contract.ServiceRequestStatusAssigned, updated.Status)
	require.Equal(t, 4, updated.Version)
	require.Equal(t, float64(2), updated.ExternalWorkOrder["version"])

	wrongPrevious := current
	wrongPrevious.EventID = "EVT-wrong-previous"
	wrongPrevious.Version = 3
	wrongPrevious.PreviousStatus = contract.CivicWorksStatusInProgress
	wrongBody := civicWorksEventBody(t, wrongPrevious)
	err = svc.HandleCivicWorksEvent(ctx, wrongBody, wrongPrevious.EventID, civicWorksSignature(wrongBody, "webhook-secret"))
	requireServiceError(t, err, http.StatusUnprocessableEntity, contract.ErrorValidation)

	missing := current
	missing.EventID = "EVT-missing"
	missing.WorkOrderID = "WO-MISSING"
	missing.Version = 3
	missingBody := civicWorksEventBody(t, missing)
	err = svc.HandleCivicWorksEvent(ctx, missingBody, missing.EventID, civicWorksSignature(missingBody, "webhook-secret"))
	requireServiceError(t, err, http.StatusUnprocessableEntity, contract.ErrorValidation)

	svc.SetCivicWorks(client, "")
	err = svc.HandleCivicWorksEvent(ctx, body, current.EventID, civicWorksSignature(body, "webhook-secret"))
	requireServiceError(t, err, http.StatusServiceUnavailable, contract.ErrorTemporarilyUnavailable)
	require.Nil(t, projectCivicWorksWorkOrder(nil))
}

func TestCivicWorksEnvironmentHelpers(t *testing.T) {
	t.Setenv("CIVICWORKS_BASE_URL", "https://civicworks.example.invalid/prefix")
	t.Setenv("CIVICWORKS_API_TOKEN", "token")
	t.Setenv("CIVICWORKS_WEBHOOK_SECRET", "secret")
	t.Setenv("BENCHMARK_RUN_ID", "run")
	client, secret, err := NewCivicWorksFromEnvironment(&http.Client{})
	require.NoError(t, err)
	require.NotNil(t, client)
	require.Equal(t, "secret", secret)
	require.NoError(t, ValidateCivicWorksEnvironment())
}

func civicWorksEventBody(t *testing.T, event contract.CivicWorksEvent) []byte {
	t.Helper()
	body, err := json.Marshal(event)
	require.NoError(t, err)
	return body
}

func civicWorksSignature(body []byte, secret string) string {
	digest := hmac.New(sha256.New, []byte(secret))
	_, _ = digest.Write(body)
	return hex.EncodeToString(digest.Sum(nil))
}

func detailID(id uint64) string { return strconv.FormatUint(id, 10) }

func writeJSONFixture(t *testing.T, w http.ResponseWriter, status int, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	require.NoError(t, json.NewEncoder(w).Encode(value))
}

func fixtureURL(r *http.Request, path string) string {
	return "http://" + r.Host + path
}
