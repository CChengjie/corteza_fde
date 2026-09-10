package city311

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/pkg/civicworksfixture"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

type fixtureRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn fixtureRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestCivicWorksFixtureCompletesTheServerOwnedIntegrationBoundary(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	agent := seededAssignmentActor(t, ctx, svc, st, "service-agent@city311.example.invalid")
	request, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, "SR-2026-00034")
	require.NoError(t, err)
	_, err = svc.Transition(ctx, agent, request.ID, 1, contract.RequestTransition{ToStatus: contract.ServiceRequestStatusTriaged})
	require.NoError(t, err)

	var callbackTargets []string
	callbackClient := &http.Client{Transport: fixtureRoundTripFunc(func(callback *http.Request) (*http.Response, error) {
		body, readErr := io.ReadAll(callback.Body)
		require.NoError(t, readErr)
		callbackTargets = append(callbackTargets, callback.URL.String())
		status := http.StatusNoContent
		if handleErr := svc.HandleCivicWorksEvent(
			callback.Context(), body, callback.Header.Get("X-CivicWorks-Event-Id"), callback.Header.Get("X-CivicWorks-Signature"),
		); handleErr != nil {
			status = http.StatusInternalServerError
			if serviceErr, ok := handleErr.(*ServiceError); ok {
				status = serviceErr.Status
			}
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader(nil)), Header: make(http.Header)}, nil
	})}
	fixture, err := civicworksfixture.New(civicworksfixture.Config{
		APIToken: "fixture-api-token", ControlToken: "fixture-control-token", WebhookSecret: "webhook-secret",
		BenchmarkRun: "fixture-run", PublicBaseURL: "http://civicworks:8080", HTTPClient: callbackClient,
		Wait: func(context.Context, time.Duration) error { return nil },
	})
	require.NoError(t, err)
	fixtureHandler := fixture.Handler()
	consumerHTTP := &http.Client{Transport: fixtureRoundTripFunc(func(outbound *http.Request) (*http.Response, error) {
		body, readErr := io.ReadAll(outbound.Body)
		require.NoError(t, readErr)
		requestCopy := httptest.NewRequest(outbound.Method, outbound.URL.String(), bytes.NewReader(body))
		requestCopy.Header = outbound.Header.Clone()
		response := httptest.NewRecorder()
		fixtureHandler.ServeHTTP(response, requestCopy)
		return response.Result(), nil
	})}
	consumer, err := NewCivicWorks(CivicWorksOptions{
		BaseURL: "http://civicworks:8080", APIToken: "fixture-api-token", WebhookSecret: "webhook-secret",
		BenchmarkRunID: "fixture-run", HTTPClient: consumerHTTP,
	})
	require.NoError(t, err)
	svc.SetCivicWorks(consumer, "webhook-secret")

	assigned, err := svc.Transition(ctx, agent, request.ID, 2, contract.RequestTransition{ToStatus: contract.ServiceRequestStatusAssigned})
	require.NoError(t, err)
	require.Equal(t, "WO-000001", assigned.ExternalWorkOrder.WorkOrderID)
	require.Equal(t, contract.ServiceTypePothole, assigned.ExternalWorkOrder.ServiceType)
	require.Equal(t, "CIVICWORKS", assigned.ExternalWorkOrder.FulfilmentSource)

	progress, err := fixture.Advance(ctx, "WO-000001", contract.CivicWorksStatusInProgress)
	require.NoError(t, err)
	require.True(t, progress.Delivery.Acknowledged)
	inProgress, err := store.LookupCity311ServiceRequestByID(ctx, st, request.ID)
	require.NoError(t, err)
	require.Equal(t, contract.ServiceRequestStatusInProgress, inProgress.Status)

	completion, err := fixture.Advance(ctx, "WO-000001", contract.CivicWorksStatusCompleted)
	require.NoError(t, err)
	require.True(t, completion.Delivery.Acknowledged)
	resolved, err := store.LookupCity311ServiceRequestByID(ctx, st, request.ID)
	require.NoError(t, err)
	require.Equal(t, contract.ServiceRequestStatusResolved, resolved.Status)
	require.Equal(t, []string{
		"https://city311.example.test/integrations/civicworks/events",
		"https://city311.example.test/integrations/civicworks/events",
	}, callbackTargets)

	version := resolved.Version
	redelivery, err := fixture.Redeliver(ctx, completion.Event.EventID)
	require.NoError(t, err)
	require.True(t, redelivery.Acknowledged)
	replayed, err := store.LookupCity311ServiceRequestByID(ctx, st, request.ID)
	require.NoError(t, err)
	require.Equal(t, version, replayed.Version)
}
