package city311

import (
	"bytes"
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

	city311Service "github.com/cortezaproject/corteza/server/compose/service/city311"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

type restCivicWorksClient struct{}

func (restCivicWorksClient) CreateWorkOrder(_ context.Context, input contract.CivicWorksWorkOrderCreate, _ string) (*contract.CivicWorksWorkOrder, error) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	return &contract.CivicWorksWorkOrder{
		WorkOrderID: "WO-REST", SourceCaseID: input.SourceCaseID, ServiceRequestNumber: input.ServiceRequestNumber,
		Status: contract.CivicWorksStatusAssigned, ExternalStatusURL: "https://civicworks.example.invalid/ui/work-orders/WO-REST",
		Version: 1, CreatedAt: now, UpdatedAt: now,
	}, nil
}

func TestCivicWorksWebhookHTTPHMACIdempotencyAndValidation(t *testing.T) {
	_, st, svc := testRouter(t)
	ctx := context.Background()
	agent, err := store.LookupUserByEmail(ctx, st, "service-agent@city311.example.invalid")
	require.NoError(t, err)
	actor, err := svc.FindActor(ctx, agent.ID)
	require.NoError(t, err)
	request, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, "SR-2026-00034")
	require.NoError(t, err)
	_, err = svc.Transition(ctx, actor, request.ID, 1, contract.RequestTransition{ToStatus: contract.ServiceRequestStatusTriaged})
	require.NoError(t, err)
	svc.SetCivicWorks(restCivicWorksClient{}, "rest-secret")
	_, err = svc.Transition(ctx, actor, request.ID, 2, contract.RequestTransition{ToStatus: contract.ServiceRequestStatusAssigned})
	require.NoError(t, err)

	router := chi.NewRouter()
	router.Route("/integrations", MountCivicWorksRoutesWithService(svc))
	event := contract.CivicWorksEvent{
		EventID: "EVT-REST", EventType: "work_order.status_changed", WorkOrderID: "WO-REST",
		SourceCaseID: "city311-case-" + strconv.FormatUint(request.ID, 10), PreviousStatus: contract.CivicWorksStatusAssigned,
		Status: contract.CivicWorksStatusInProgress, Version: 2, OccurredAt: time.Date(2026, 9, 4, 12, 30, 0, 0, time.UTC),
	}
	body, err := json.Marshal(event)
	require.NoError(t, err)

	wrongContent := executeCivicWorksEvent(t, router, body, event.EventID, signCivicWorksBody(body, "rest-secret"), "text/plain")
	require.Equal(t, http.StatusUnprocessableEntity, wrongContent.Code, wrongContent.Body.String())
	badSignature := executeCivicWorksEvent(t, router, body, event.EventID, "bad", "application/json")
	require.Equal(t, http.StatusUnauthorized, badSignature.Code, badSignature.Body.String())
	headerMismatch := executeCivicWorksEvent(t, router, body, "different-event", signCivicWorksBody(body, "rest-secret"), "application/json")
	require.Equal(t, http.StatusUnprocessableEntity, headerMismatch.Code, headerMismatch.Body.String())

	accepted := executeCivicWorksEvent(t, router, body, event.EventID, signCivicWorksBody(body, "rest-secret"), "application/json; charset=utf-8")
	require.Equal(t, http.StatusNoContent, accepted.Code, accepted.Body.String())
	require.Empty(t, accepted.Body.String())
	duplicate := executeCivicWorksEvent(t, router, body, event.EventID, signCivicWorksBody(body, "rest-secret"), "application/json")
	require.Equal(t, http.StatusNoContent, duplicate.Code, duplicate.Body.String())

	updated, err := store.LookupCity311ServiceRequestByID(ctx, st, request.ID)
	require.NoError(t, err)
	require.Equal(t, contract.ServiceRequestStatusInProgress, updated.Status)
	require.Equal(t, 4, updated.Version)

	unconfigured := chi.NewRouter()
	unconfigured.Route("/integrations", MountCivicWorksRoutesWithService(nil))
	response := executeCivicWorksEvent(t, unconfigured, body, event.EventID, signCivicWorksBody(body, "rest-secret"), "application/json")
	require.Equal(t, http.StatusServiceUnavailable, response.Code, response.Body.String())
}

func executeCivicWorksEvent(t *testing.T, router http.Handler, body []byte, eventID, signature, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/integrations/civicworks/events", bytes.NewReader(body))
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("X-CivicWorks-Event-Id", eventID)
	request.Header.Set("X-CivicWorks-Signature", signature)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func signCivicWorksBody(body []byte, secret string) string {
	digest := hmac.New(sha256.New, []byte(secret))
	_, _ = digest.Write(body)
	return hex.EncodeToString(digest.Sum(nil))
}

var _ city311Service.CivicWorksClient = restCivicWorksClient{}
