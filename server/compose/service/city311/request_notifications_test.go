package city311

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"testing"
	"time"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

func TestRequestSubmissionNotificationIsTransactionalIdempotentAndRecoverable(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	sender := &scriptedMailSender{codes: []int{421, 250}}
	svc.mailSender = sender
	svc.mailWait = func(context.Context, time.Duration) error { return nil }
	options := SubmissionOptions{
		Operation: "service_request_create", SourceChannel: contract.SourceChannelAPI,
		ActorType: contract.AuditActorIntegrationClient, ActorID: 77, RequireIdempotency: true,
	}

	created, status, err := svc.Submit(ctx, validSubmission(), "notification-submit-key", options)
	require.NoError(t, err)
	require.Equal(t, 201, status)
	requestID, err := strconv.ParseUint(created.RequestID, 10, 64)
	require.NoError(t, err)
	pending := requestNotificationOperations(t, ctx, st, requestID)
	require.Len(t, pending, 1)
	payload := decodeRequestNotificationOperation(t, pending[0])
	require.Equal(t, RelationshipNotificationSubmitted, payload.Event)
	require.Equal(t, "alex@example.invalid", payload.Recipient)
	require.Equal(t, mailStatusPending, payload.DeliveryStatus)

	replayed, status, err := svc.Submit(ctx, validSubmission(), "notification-submit-key", options)
	require.NoError(t, err)
	require.Equal(t, 200, status)
	require.Equal(t, created, replayed)
	require.Len(t, requestNotificationOperations(t, ctx, st, requestID), 1)

	// A fresh service instance represents process restart after the outbox commit.
	restarted := New(st)
	restarted.now = svc.now
	restarted.nextID = svc.nextID
	restarted.mailSender = sender
	restarted.mailWait = func(context.Context, time.Duration) error { return nil }
	require.NoError(t, restarted.ProcessRequestNotifications(ctx))
	require.Len(t, sender.messages, 2)
	require.Equal(t, []string{"alex@example.invalid"}, sender.messages[0].To)
	require.Contains(t, sender.messages[0].Subject, created.RequestNumber)
	require.Equal(t, sender.deliveryKeys[0], sender.deliveryKeys[1])

	delivered := requestNotificationOperations(t, ctx, st, requestID)
	require.Len(t, delivered, 1)
	require.Equal(t, mailStatusDelivered, delivered[0].Status)
	require.Equal(t, 2, delivered[0].Progress)
	payload = decodeRequestNotificationOperation(t, delivered[0])
	require.NotNil(t, payload.DeliveredAt)
	require.NoError(t, restarted.ProcessRequestNotifications(ctx))
	require.Len(t, sender.messages, 2)
	require.Equal(t, 1, requestNotificationAuditCount(t, ctx, st, requestID, "REQUEST_NOTIFICATION_EMAIL_DELIVERED"))
}

func TestStatusNotificationHonoursRelationshipMatrixAndPermanentFailure(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	request, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, "SR-2026-00034")
	require.NoError(t, err)
	actor := relationshipServiceAgent()

	detail, err := svc.LinkConstituent(ctx, actor, request.ID, 1, contract.ConstituentLink{
		ConstituentID: "C-3", RelationshipType: contract.RelationshipAffectedResident,
		PortalVisible: true, NotifyStatus: true,
	})
	require.NoError(t, err)
	detail, err = svc.LinkConstituent(ctx, actor, request.ID, detail.Request.Version, contract.ConstituentLink{
		ConstituentID: "C-1", RelationshipType: contract.RelationshipPropertyOwner,
		PortalVisible: true, NotifyStatus: true,
	})
	require.NoError(t, err)

	_, err = svc.Transition(ctx, actor, request.ID, detail.Request.Version, contract.RequestTransition{ToStatus: contract.ServiceRequestStatusTriaged})
	require.NoError(t, err)
	operations := requestNotificationOperations(t, ctx, st, request.ID)
	require.Len(t, operations, 2)
	constituents := make([]string, 0, len(operations))
	for _, operation := range operations {
		payload := decodeRequestNotificationOperation(t, operation)
		require.Equal(t, RelationshipNotificationStatusChange, payload.Event)
		require.Equal(t, contract.ServiceRequestStatusSubmitted, payload.PreviousStatus)
		require.Equal(t, contract.ServiceRequestStatusTriaged, payload.CurrentStatus)
		constituents = append(constituents, payload.ConstituentID)
	}
	sort.Strings(constituents)
	require.Equal(t, []string{"C-2", "C-3"}, constituents)

	sender := &scriptedMailSender{codes: []int{250, 550}}
	svc.mailSender = sender
	require.NoError(t, svc.ProcessRequestNotifications(ctx))
	require.Len(t, sender.messages, 2)
	operations = requestNotificationOperations(t, ctx, st, request.ID)
	statuses := []string{operations[0].Status, operations[1].Status}
	sort.Strings(statuses)
	require.Equal(t, []string{mailStatusDelivered, mailStatusFailed}, statuses)
	require.Equal(t, 1, requestNotificationAuditCount(t, ctx, st, request.ID, "REQUEST_NOTIFICATION_EMAIL_DELIVERED"))
	require.Equal(t, 1, requestNotificationAuditCount(t, ctx, st, request.ID, "REQUEST_NOTIFICATION_EMAIL_DELIVERY_FAILED"))
	require.NoError(t, svc.ProcessRequestNotifications(ctx))
	require.Len(t, sender.messages, 2)
}

func TestRequestNotificationWorkerDrainsPendingOutbox(t *testing.T) {
	svc, st := testService(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	sender := &scriptedMailSender{}
	svc.mailSender = sender
	svc.noticePoll = 5 * time.Millisecond
	svc.StartRequestNotificationWorker(ctx)

	created, _, err := svc.Submit(ctx, validSubmission(), "worker-notification-key", SubmissionOptions{
		Operation: "service_request_create", SourceChannel: contract.SourceChannelPortalAnonymous,
		ActorType: contract.AuditActorConstituent, RequireIdempotency: true,
	})
	require.NoError(t, err)
	requestID, err := strconv.ParseUint(created.RequestID, 10, 64)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		operations, _, searchErr := store.SearchCity311Operations(ctx, st, composeTypes.City311OperationFilter{
			Kind: requestNotificationOperationKind, Status: mailStatusDelivered,
			Check: func(operation *composeTypes.City311Operation) (bool, error) {
				return decodeRequestNotificationOperationNoTest(operation).RequestID == requestID, nil
			},
		})
		return searchErr == nil && len(operations) == 1
	}, time.Second, 10*time.Millisecond)
	require.Len(t, sender.messages, 1)
}

func TestRequestNotificationHelpersCoverCancellationAndReopenedEvents(t *testing.T) {
	svc, _ := testService(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.ErrorIs(t, svc.ProcessRequestNotifications(ctx), context.Canceled)
	require.Equal(t, RelationshipNotificationReopened, relationshipNotificationEvent(contract.ServiceRequestStatusReopened))

	called := false
	svc.SetRequestNotificationWorkerErrorHandler(func(error) { called = true })
	require.NotNil(t, svc.noticeWorkerError)
	svc.noticeWorkerError(context.Canceled)
	require.True(t, called)
}

func requestNotificationOperations(t *testing.T, ctx context.Context, st store.Storer, requestID uint64) composeTypes.City311OperationSet {
	t.Helper()
	set, _, err := store.SearchCity311Operations(ctx, st, composeTypes.City311OperationFilter{
		Kind: requestNotificationOperationKind,
		Check: func(operation *composeTypes.City311Operation) (bool, error) {
			return decodeRequestNotificationOperationNoTest(operation).RequestID == requestID, nil
		},
	})
	require.NoError(t, err)
	sort.Slice(set, func(i, j int) bool { return set[i].ID < set[j].ID })
	return set
}

func decodeRequestNotificationOperation(t *testing.T, operation *composeTypes.City311Operation) requestNotificationPayload {
	t.Helper()
	encoded, err := json.Marshal(operation.Result)
	require.NoError(t, err)
	payload := requestNotificationPayload{}
	require.NoError(t, json.Unmarshal(encoded, &payload))
	return payload
}

func decodeRequestNotificationOperationNoTest(operation *composeTypes.City311Operation) requestNotificationPayload {
	payload := requestNotificationPayload{}
	encoded, _ := json.Marshal(operation.Result)
	_ = json.Unmarshal(encoded, &payload)
	return payload
}

func requestNotificationAuditCount(t *testing.T, ctx context.Context, st store.Storer, requestID uint64, eventType string) int {
	t.Helper()
	set, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{
		RequestID: requestID, EntityType: "request_notification",
	})
	require.NoError(t, err)
	count := 0
	for _, event := range set {
		if event.EventType == eventType {
			count++
		}
	}
	return count
}
