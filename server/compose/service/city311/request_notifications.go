package city311

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/pkg/filter"
	"github.com/cortezaproject/corteza/server/store"
)

const (
	requestNotificationOperationKind = "REQUEST_NOTIFICATION"
	requestNotificationBatchSize     = 1000
)

type requestNotificationPayload struct {
	RequestID        uint64                        `json:"request_id,string"`
	RequestNumber    string                        `json:"request_number"`
	ConstituentID    string                        `json:"constituent_id"`
	Recipient        string                        `json:"recipient"`
	Event            RelationshipNotificationEvent `json:"event"`
	PreviousStatus   contract.ServiceRequestStatus `json:"previous_status,omitempty"`
	CurrentStatus    contract.ServiceRequestStatus `json:"current_status"`
	SourceChannel    contract.SourceChannel        `json:"source_channel"`
	DeliveryKey      string                        `json:"delivery_key"`
	DeliveryStatus   string                        `json:"delivery_status"`
	DeliveryAttempts int                           `json:"delivery_attempts"`
	DeliveredAt      *time.Time                    `json:"delivered_at,omitempty"`
}

func (svc *Service) enqueueRelationshipNotifications(
	ctx context.Context,
	tx store.Storer,
	request *composeTypes.City311ServiceRequest,
	previousStatus contract.ServiceRequestStatus,
	event RelationshipNotificationEvent,
	actorID uint64,
	source contract.SourceChannel,
) error {
	recipients, err := relationshipNotificationRecipients(ctx, tx, request.ID, event)
	if err != nil {
		return err
	}
	for _, constituentID := range recipients {
		constituent, lookupErr := store.LookupCity311ConstituentByConstituentID(ctx, tx, constituentID)
		if lookupErr != nil {
			return lookupErr
		}
		payload := requestNotificationPayload{
			RequestID: request.ID, RequestNumber: publishedRequestNumber(request), ConstituentID: constituentID,
			Recipient: normalizeEmail(requesterInput(constituent.Profile).Email), Event: event,
			PreviousStatus: previousStatus, CurrentStatus: request.Status, SourceChannel: source,
			DeliveryKey: requestNotificationDeliveryKey(request, constituentID, event), DeliveryStatus: mailStatusPending,
		}
		result, mapErr := mapFrom(payload)
		if mapErr != nil {
			return mapErr
		}
		operation := &composeTypes.City311Operation{
			ID: svc.nextID(), Kind: requestNotificationOperationKind, Status: mailStatusPending,
			ActorID: actorID, Result: result, Error: composeTypes.City311JSON{}, CreatedAt: request.UpdatedAt, UpdatedAt: request.UpdatedAt,
		}
		if err = store.CreateCity311Operation(ctx, tx, operation); err != nil {
			return err
		}
	}
	return nil
}

func relationshipNotificationEvent(to contract.ServiceRequestStatus) RelationshipNotificationEvent {
	switch to {
	case contract.ServiceRequestStatusSubmitted:
		return RelationshipNotificationSubmitted
	case contract.ServiceRequestStatusResolved:
		return RelationshipNotificationResolved
	case contract.ServiceRequestStatusClosed:
		return RelationshipNotificationClosed
	case contract.ServiceRequestStatusReopened:
		return RelationshipNotificationReopened
	default:
		return RelationshipNotificationStatusChange
	}
}

func requestNotificationDeliveryKey(request *composeTypes.City311ServiceRequest, constituentID string, event RelationshipNotificationEvent) string {
	return fmt.Sprintf("city311-request:%d:%d:%s:%s", request.ID, request.Version, event, hashKey(constituentID))
}

func requestNotificationMessage(payload requestNotificationPayload) MailMessage {
	label := strings.ToLower(strings.ReplaceAll(string(payload.Event), "_", " "))
	subject := fmt.Sprintf("Request %s %s", payload.RequestNumber, label)
	body := fmt.Sprintf("Your City 311 service request %s is now %s.", payload.RequestNumber, payload.CurrentStatus)
	if payload.PreviousStatus != "" && payload.PreviousStatus != payload.CurrentStatus {
		body = fmt.Sprintf("Your City 311 service request %s changed from %s to %s.", payload.RequestNumber, payload.PreviousStatus, payload.CurrentStatus)
	}
	return MailMessage{From: mailTemplates["service-request-update"].From, To: []string{payload.Recipient}, Subject: mailTemplates["service-request-update"].SubjectPrefix + subject, Text: body}
}

// ProcessRequestNotifications drains the transactional request-notification
// outbox. A stable delivery key lets the SMTP fixture deduplicate a replay if
// the process stops after acceptance but before the operation is committed.
func (svc *Service) ProcessRequestNotifications(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	set, _, err := store.SearchCity311Operations(ctx, svc.store, composeTypes.City311OperationFilter{
		Kind: requestNotificationOperationKind, Status: mailStatusPending,
		Paging: filter.Paging{Limit: requestNotificationBatchSize},
	})
	if err != nil {
		return err
	}
	sort.Slice(set, func(i, j int) bool { return set[i].ID < set[j].ID })
	var firstErr error
	for _, operation := range set {
		if err = svc.processRequestNotification(ctx, operation); err != nil && firstErr == nil {
			firstErr = err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return firstErr
}

func (svc *Service) processRequestNotification(ctx context.Context, operation *composeTypes.City311Operation) error {
	if operation == nil || operation.Kind != requestNotificationOperationKind || operation.Status != mailStatusPending {
		return nil
	}
	payload := requestNotificationPayload{}
	encoded, err := json.Marshal(operation.Result)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(encoded, &payload); err != nil {
		return err
	}
	status, attempts := mailStatusFailed, 0
	var deliveryErr error
	if !validEmail(payload.Recipient) {
		deliveryErr = fmt.Errorf("request notification recipient has no valid email address")
	} else {
		svc.mailMu.Lock()
		status, attempts, deliveryErr = svc.deliverMail(ctx, requestNotificationMessage(payload), payload.DeliveryKey)
		svc.mailMu.Unlock()
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	now := svc.now().UTC()
	payload.DeliveryStatus = status
	payload.DeliveryAttempts = attempts
	if status == mailStatusDelivered {
		payload.DeliveredAt = &now
	}
	result, err := mapFrom(payload)
	if err != nil {
		return err
	}
	return store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		current, lookupErr := store.LookupCity311OperationByID(ctx, tx, operation.ID)
		if lookupErr != nil {
			return lookupErr
		}
		if current.Status != mailStatusPending {
			return nil
		}
		current.Status, current.Progress, current.Result, current.UpdatedAt, current.CompletedAt = status, attempts, result, now, &now
		current.Error = composeTypes.City311JSON{}
		eventType := "REQUEST_NOTIFICATION_EMAIL_DELIVERED"
		if deliveryErr != nil {
			eventType = "REQUEST_NOTIFICATION_EMAIL_DELIVERY_FAILED"
			current.Error = composeTypes.City311JSON{
				"error": string(contract.ErrorOperationFailed), "message": "Request notification delivery failed permanently.", "retryable": false,
			}
		}
		if updateErr := store.UpdateCity311Operation(ctx, tx, current); updateErr != nil {
			return updateErr
		}
		return store.CreateCity311AuditEvent(ctx, tx, &composeTypes.City311AuditEvent{
			ID: svc.nextID(), RequestID: payload.RequestID, EntityType: "request_notification", EntityID: strconv.FormatUint(current.ID, 10),
			EventType: eventType, ActorType: contract.AuditActorSystem, SourceChannel: payload.SourceChannel,
			Before: composeTypes.City311JSON{"status": mailStatusPending}, After: composeTypes.City311JSON{
				"constituent_id": payload.ConstituentID, "event": payload.Event, "status": status,
				"attempts": attempts, "delivery_key_hash": hashKey(payload.DeliveryKey),
			}, CreatedAt: now,
		})
	})
}

func (svc *Service) StartRequestNotificationWorker(ctx context.Context) {
	svc.noticeWorkerOnce.Do(func() {
		go svc.runRequestNotificationWorker(ctx)
		svc.wakeRequestNotificationWorker()
	})
}

func (svc *Service) SetRequestNotificationWorkerErrorHandler(handler func(error)) {
	svc.noticeWorkerError = handler
}

func (svc *Service) runRequestNotificationWorker(ctx context.Context) {
	poll := svc.noticePoll
	if poll <= 0 {
		poll = 30 * time.Second
	}
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-svc.noticeWake:
		case <-ticker.C:
		}
		if err := svc.ProcessRequestNotifications(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			if svc.noticeWorkerError != nil {
				svc.noticeWorkerError(err)
			}
		}
	}
}

func (svc *Service) wakeRequestNotificationWorker() {
	select {
	case svc.noticeWake <- struct{}{}:
	default:
	}
}
