package city311

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/pkg/errors"
	"github.com/cortezaproject/corteza/server/store"
)

const (
	civicWorksCreatePath      = "/api/v1/work-orders"
	civicWorksCallbackPath    = "/integrations/civicworks/events"
	civicWorksResponseLimit   = 1 << 20
	civicWorksRequestTimeout  = 5 * time.Second
	civicWorksCreateAttempts  = 2
	civicWorksCreatedAudit    = "CIVICWORKS_WORK_ORDER_CREATED"
	civicWorksUpdatedAudit    = "CIVICWORKS_WORK_ORDER_UPDATED"
	civicWorksEventApplied    = "CIVICWORKS_EVENT_APPLIED"
	civicWorksEventIgnored    = "CIVICWORKS_EVENT_IGNORED"
	civicWorksEventEntity     = "civicworks_event"
	civicWorksWorkOrderEntity = "civicworks_work_order"
)

type (
	CivicWorksClient interface {
		CreateWorkOrder(context.Context, contract.CivicWorksWorkOrderCreate, string) (*contract.CivicWorksWorkOrder, error)
	}

	CivicWorksOptions struct {
		BaseURL        string
		APIToken       string
		WebhookSecret  string
		BenchmarkRunID string
		HTTPClient     *http.Client
	}

	CivicWorksService struct {
		endpoint       string
		apiToken       string
		webhookSecret  string
		benchmarkRunID string
		httpClient     *http.Client
	}
)

func NewCivicWorks(options CivicWorksOptions) (*CivicWorksService, error) {
	baseURL, err := validateCivicWorksOptions(options)
	if err != nil {
		return nil, err
	}
	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + civicWorksCreatePath
	client := options.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: civicWorksRequestTimeout}
	}
	return &CivicWorksService{
		endpoint: baseURL.String(), apiToken: options.APIToken, webhookSecret: options.WebhookSecret,
		benchmarkRunID: options.BenchmarkRunID, httpClient: client,
	}, nil
}

func NewCivicWorksFromEnvironment(client *http.Client) (CivicWorksClient, string, error) {
	service, err := NewCivicWorks(CivicWorksOptions{
		BaseURL: os.Getenv("CIVICWORKS_BASE_URL"), APIToken: os.Getenv("CIVICWORKS_API_TOKEN"),
		WebhookSecret: os.Getenv("CIVICWORKS_WEBHOOK_SECRET"), BenchmarkRunID: os.Getenv("BENCHMARK_RUN_ID"), HTTPClient: client,
	})
	if err != nil {
		return nil, "", err
	}
	return service, service.webhookSecret, nil
}

func ValidateCivicWorksEnvironment() error {
	_, _, err := NewCivicWorksFromEnvironment(nil)
	return err
}

func validateCivicWorksOptions(options CivicWorksOptions) (*url.URL, error) {
	if strings.TrimSpace(options.BaseURL) == "" {
		return nil, fmt.Errorf("CIVICWORKS_BASE_URL is required")
	}
	baseURL, err := url.Parse(options.BaseURL)
	if err != nil || baseURL.Host == "" || (baseURL.Scheme != "http" && baseURL.Scheme != "https") {
		return nil, fmt.Errorf("CIVICWORKS_BASE_URL must be an absolute HTTP or HTTPS URL")
	}
	if baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return nil, fmt.Errorf("CIVICWORKS_BASE_URL must not contain credentials, a query, or a fragment")
	}
	for name, value := range map[string]string{
		"CIVICWORKS_API_TOKEN": options.APIToken, "CIVICWORKS_WEBHOOK_SECRET": options.WebhookSecret, "BENCHMARK_RUN_ID": options.BenchmarkRunID,
	} {
		if strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("%s is required", name)
		}
		if value != strings.TrimSpace(value) || strings.ContainsAny(value, "\r\n") {
			return nil, fmt.Errorf("%s is malformed", name)
		}
	}
	return baseURL, nil
}

func (svc *CivicWorksService) CreateWorkOrder(ctx context.Context, input contract.CivicWorksWorkOrderCreate, idempotencyKey string) (*contract.CivicWorksWorkOrder, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return nil, civicWorksUnavailableError()
	}
	for attempt := 0; attempt < civicWorksCreateAttempts; attempt++ {
		request, requestErr := http.NewRequestWithContext(ctx, http.MethodPost, svc.endpoint, bytes.NewReader(body))
		if requestErr != nil {
			return nil, civicWorksUnavailableError()
		}
		request.Header.Set("Authorization", "Bearer "+svc.apiToken)
		request.Header.Set("X-Benchmark-Run-Id", svc.benchmarkRunID)
		request.Header.Set(contract.IdempotencyHeader, idempotencyKey)
		request.Header.Set("Content-Type", "application/json")
		response, requestErr := svc.httpClient.Do(request)
		if requestErr != nil {
			if ctx.Err() != nil || attempt+1 == civicWorksCreateAttempts {
				return nil, civicWorksUnavailableError()
			}
			continue
		}
		responseBody, readErr := io.ReadAll(io.LimitReader(response.Body, civicWorksResponseLimit+1))
		_ = response.Body.Close()
		if readErr != nil || len(responseBody) > civicWorksResponseLimit {
			if attempt+1 == civicWorksCreateAttempts {
				return nil, civicWorksUnavailableError()
			}
			continue
		}
		switch response.StatusCode {
		case http.StatusCreated, http.StatusOK:
			workOrder := &contract.CivicWorksWorkOrder{}
			if decodeErr := decodeCivicWorksResponse(responseBody, workOrder, input); decodeErr != nil {
				if attempt+1 == civicWorksCreateAttempts {
					return nil, civicWorksUnavailableError()
				}
				continue
			}
			return workOrder, nil
		case http.StatusConflict:
			return nil, &ServiceError{Status: http.StatusConflict, Payload: contract.APIError{
				Error: contract.ErrorIdempotencyConflict, Message: "CivicWorks rejected the stable work-order key.", Retryable: false,
			}}
		case http.StatusUnprocessableEntity:
			return nil, validationError(contract.FieldError{Field: "/to_status", Code: contract.ValidationInvalidValue})
		default:
			if response.StatusCode < 500 || attempt+1 == civicWorksCreateAttempts {
				return nil, civicWorksUnavailableError()
			}
		}
	}
	return nil, civicWorksUnavailableError()
}

func decodeCivicWorksResponse(data []byte, workOrder *contract.CivicWorksWorkOrder, input contract.CivicWorksWorkOrderCreate) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(workOrder); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("CivicWorks response contains trailing data")
	}
	statusURL, err := url.Parse(workOrder.ExternalStatusURL)
	if err != nil || statusURL.Host == "" || (statusURL.Scheme != "http" && statusURL.Scheme != "https") {
		return fmt.Errorf("CivicWorks response has an invalid status URL")
	}
	if strings.TrimSpace(workOrder.WorkOrderID) == "" || workOrder.SourceCaseID != input.SourceCaseID ||
		workOrder.ServiceRequestNumber != input.ServiceRequestNumber || workOrder.Status != contract.CivicWorksStatusAssigned ||
		workOrder.Version == 0 || workOrder.CreatedAt.IsZero() || workOrder.UpdatedAt.IsZero() {
		return fmt.Errorf("CivicWorks response does not satisfy the work-order contract")
	}
	return nil
}

func (svc *Service) SetCivicWorks(client CivicWorksClient, webhookSecret string) {
	svc.civicWorksClient = client
	svc.civicWorksSecret = strings.TrimSpace(webhookSecret)
	svc.civicWorksConfig = nil
	if client == nil || svc.civicWorksSecret == "" {
		svc.civicWorksConfig = fmt.Errorf("CivicWorks client and webhook secret are required")
	}
}

func (svc *Service) AssignCivicWorks(ctx context.Context, actor contract.Actor, requestID, expectedVersion uint64) (*contract.StaffServiceRequestDetail, error) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	request, err := store.LookupCity311ServiceRequestByID(ctx, svc.store, requestID)
	if err != nil {
		return nil, requestLookupError(err)
	}
	if len(request.ExternalWorkOrder) > 0 {
		if !canRead(actor, request) || !canOperateRequest(actor) {
			return nil, apiError(http.StatusForbidden, contract.ErrorForbidden, requestScopeDeniedMessage)
		}
		if uint64(request.Version) != expectedVersion {
			return nil, versionConflict(request.Version)
		}
		if request.Status == contract.ServiceRequestStatusAssigned {
			return svc.Find(ctx, actor, requestID)
		}
		return nil, invalidStatusTransition("The request already has a CivicWorks work order.")
	}
	if err = authorizeTransition(actor, request, expectedVersion, contract.ServiceRequestStatusAssigned); err != nil {
		return nil, err
	}
	if svc.civicWorksConfig != nil || svc.civicWorksClient == nil {
		return nil, civicWorksUnavailableError()
	}
	input := civicWorksCreateInput(request)
	workOrder, err := svc.civicWorksClient.CreateWorkOrder(ctx, input, civicWorksIdempotencyKey(request.ID))
	if err != nil {
		return nil, err
	}
	if err = decodeCivicWorksResponseMustMatch(workOrder, input); err != nil {
		return nil, civicWorksUnavailableError()
	}

	err = store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		stored, lookupErr := store.LookupCity311ServiceRequestByID(ctx, tx, requestID)
		if lookupErr != nil {
			return requestLookupError(lookupErr)
		}
		if authErr := authorizeTransition(actor, stored, expectedVersion, contract.ServiceRequestStatusAssigned); authErr != nil {
			return authErr
		}
		before := requestSnapshot(stored)
		mapped, mapErr := mapFrom(workOrder)
		if mapErr != nil {
			return mapErr
		}
		stored.ExternalWorkOrder = mapped
		stored.Status = contract.ServiceRequestStatusAssigned
		stored.Version++
		stored.UpdatedAt = svc.now().UTC()
		if updateErr := store.UpdateCity311ServiceRequest(ctx, tx, stored); updateErr != nil {
			return updateErr
		}
		if auditErr := svc.persistTransitionEvents(ctx, tx, actor.ID, stored, before); auditErr != nil {
			return auditErr
		}
		return store.CreateCity311AuditEvent(ctx, tx, &composeTypes.City311AuditEvent{
			ID: svc.nextID(), RequestID: stored.ID, EntityType: civicWorksWorkOrderEntity, EntityID: workOrder.WorkOrderID,
			EventType: civicWorksCreatedAudit, ActorType: contract.AuditActorStaff, ActorID: actor.ID, SourceChannel: contract.SourceChannelStaffInPerson,
			Before: composeTypes.City311JSON{}, After: cloneMap(mapped), CreatedAt: stored.UpdatedAt,
		})
	})
	if err != nil {
		return nil, err
	}
	return svc.Find(ctx, actor, requestID)
}

func (svc *Service) HandleCivicWorksEvent(ctx context.Context, body []byte, headerEventID, signature string) error {
	if svc.civicWorksConfig != nil || svc.civicWorksSecret == "" {
		return civicWorksUnavailableError()
	}
	if !validCivicWorksSignature(body, signature, svc.civicWorksSecret) {
		return &ServiceError{Status: http.StatusUnauthorized, Payload: contract.APIError{
			Error: contract.ErrorInvalidSignature, Message: "The CivicWorks event signature is invalid.", Retryable: false,
		}}
	}
	event, err := decodeCivicWorksEvent(body, headerEventID)
	if err != nil {
		return err
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	return store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		processed, _, searchErr := store.SearchCity311AuditEvents(ctx, tx, composeTypes.City311AuditEventFilter{
			EntityType: civicWorksEventEntity, EntityID: event.EventID,
		})
		if searchErr != nil {
			return searchErr
		}
		if len(processed) > 0 {
			return nil
		}
		request, workOrder, lookupErr := lookupCivicWorksRequest(ctx, tx, event)
		if lookupErr != nil {
			return lookupErr
		}
		if event.Version <= workOrder.Version {
			return svc.persistCivicWorksReceipt(ctx, tx, request, event, civicWorksEventIgnored)
		}
		if event.PreviousStatus != workOrder.Status {
			return validationError(contract.FieldError{Field: "/previous_status", Code: contract.ValidationInvalidValue})
		}
		plan, ok := contract.PlanCivicWorksTransition(request.Status, event.Status)
		if !ok {
			return validationError(contract.FieldError{Field: "/status", Code: contract.ValidationInvalidValue})
		}
		beforeOrder := requestSnapshot(request)
		workOrder.Status = event.Status
		workOrder.Version = event.Version
		workOrder.UpdatedAt = event.OccurredAt.UTC()
		mapped, mapErr := mapFrom(workOrder)
		if mapErr != nil {
			return mapErr
		}
		request.ExternalWorkOrder = mapped
		if len(plan) == 0 {
			request.Version++
			request.UpdatedAt = svc.now().UTC()
			if updateErr := store.UpdateCity311ServiceRequest(ctx, tx, request); updateErr != nil {
				return updateErr
			}
			if auditErr := svc.persistCivicWorksWorkOrderUpdate(ctx, tx, request, event, beforeOrder); auditErr != nil {
				return auditErr
			}
		} else {
			if auditErr := svc.persistCivicWorksWorkOrderUpdate(ctx, tx, request, event, beforeOrder); auditErr != nil {
				return auditErr
			}
			for _, status := range plan {
				before := requestSnapshot(request)
				request.Status = status
				request.Version++
				request.UpdatedAt = svc.now().UTC()
				if updateErr := store.UpdateCity311ServiceRequest(ctx, tx, request); updateErr != nil {
					return updateErr
				}
				if auditErr := svc.persistCivicWorksTransition(ctx, tx, request, event, before); auditErr != nil {
					return auditErr
				}
			}
		}
		return svc.persistCivicWorksReceipt(ctx, tx, request, event, civicWorksEventApplied)
	})
}

func decodeCivicWorksEvent(body []byte, headerEventID string) (contract.CivicWorksEvent, error) {
	event := contract.CivicWorksEvent{}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return event, civicWorksEventValidationError("/")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return event, civicWorksEventValidationError("/")
	}
	headerEventID = strings.TrimSpace(headerEventID)
	if headerEventID == "" || event.EventID != headerEventID || strings.TrimSpace(event.WorkOrderID) == "" ||
		strings.TrimSpace(event.SourceCaseID) == "" || event.EventType != "work_order.status_changed" || event.Version == 0 || event.OccurredAt.IsZero() ||
		!containsEnums([]contract.CivicWorksStatus{event.PreviousStatus, event.Status}, contract.CivicWorksStatuses) {
		return event, civicWorksEventValidationError("/")
	}
	return event, nil
}

func validCivicWorksSignature(body []byte, signature, secret string) bool {
	signature = strings.TrimSpace(signature)
	signature = strings.TrimPrefix(signature, "sha256=")
	provided, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	digest := hmac.New(sha256.New, []byte(secret))
	_, _ = digest.Write(body)
	return hmac.Equal(digest.Sum(nil), provided)
}

func lookupCivicWorksRequest(ctx context.Context, tx store.Storer, event contract.CivicWorksEvent) (*composeTypes.City311ServiceRequest, *contract.CivicWorksWorkOrder, error) {
	requests, _, err := store.SearchCity311ServiceRequests(ctx, tx, composeTypes.City311ServiceRequestFilter{})
	if err != nil {
		return nil, nil, err
	}
	for _, request := range requests {
		if len(request.ExternalWorkOrder) == 0 {
			continue
		}
		workOrder := &contract.CivicWorksWorkOrder{}
		encoded, encodeErr := json.Marshal(request.ExternalWorkOrder)
		if encodeErr != nil || json.Unmarshal(encoded, workOrder) != nil {
			continue
		}
		if workOrder.WorkOrderID == event.WorkOrderID && workOrder.SourceCaseID == event.SourceCaseID {
			return request, workOrder, nil
		}
	}
	return nil, nil, validationError(contract.FieldError{Field: "/work_order_id", Code: contract.ValidationInvalidValue})
}

func projectCivicWorksWorkOrder(input map[string]any) *contract.CivicWorksWorkOrder {
	if len(input) == 0 {
		return nil
	}
	workOrder := &contract.CivicWorksWorkOrder{}
	encoded, err := json.Marshal(input)
	if err != nil || json.Unmarshal(encoded, workOrder) != nil {
		return nil
	}
	return workOrder
}

func (svc *Service) persistCivicWorksTransition(ctx context.Context, tx store.Storer, request *composeTypes.City311ServiceRequest, event contract.CivicWorksEvent, before map[string]any) error {
	if err := store.CreateCity311AuditEvent(ctx, tx, &composeTypes.City311AuditEvent{
		ID: svc.nextID(), RequestID: request.ID, EntityType: "service_request", EntityID: strconv.FormatUint(request.ID, 10),
		EventType: "SERVICE_REQUEST_TRANSITIONED", ActorType: contract.AuditActorIntegrationClient, SourceChannel: contract.SourceChannelAPI,
		Before: before, After: requestSnapshot(request), CreatedAt: svc.now().UTC(),
	}); err != nil {
		return err
	}
	return store.CreateCity311PublicHistoryItem(ctx, tx, &composeTypes.City311PublicHistoryItem{
		ID: svc.nextID(), RequestID: request.ID, Action: string(request.Status), ResponsibleDepartment: request.OwningDepartment, OccurredAt: event.OccurredAt.UTC(),
	})
}

func (svc *Service) persistCivicWorksWorkOrderUpdate(ctx context.Context, tx store.Storer, request *composeTypes.City311ServiceRequest, event contract.CivicWorksEvent, before map[string]any) error {
	return store.CreateCity311AuditEvent(ctx, tx, &composeTypes.City311AuditEvent{
		ID: svc.nextID(), RequestID: request.ID, EntityType: civicWorksWorkOrderEntity, EntityID: event.WorkOrderID,
		EventType: civicWorksUpdatedAudit, ActorType: contract.AuditActorIntegrationClient, SourceChannel: contract.SourceChannelAPI,
		Before: before, After: requestSnapshot(request), CreatedAt: svc.now().UTC(),
	})
}

func (svc *Service) persistCivicWorksReceipt(ctx context.Context, tx store.Storer, request *composeTypes.City311ServiceRequest, event contract.CivicWorksEvent, eventType string) error {
	return store.CreateCity311AuditEvent(ctx, tx, &composeTypes.City311AuditEvent{
		ID: svc.nextID(), RequestID: request.ID, EntityType: civicWorksEventEntity, EntityID: event.EventID, EventType: eventType,
		ActorType: contract.AuditActorIntegrationClient, SourceChannel: contract.SourceChannelAPI,
		Before: composeTypes.City311JSON{}, After: composeTypes.City311JSON{
			"work_order_id": event.WorkOrderID, "source_case_id": event.SourceCaseID, "status": event.Status, "version": event.Version,
		}, CreatedAt: svc.now().UTC(),
	})
}

func civicWorksCreateInput(request *composeTypes.City311ServiceRequest) contract.CivicWorksWorkOrderCreate {
	return contract.CivicWorksWorkOrderCreate{
		SourceCaseID: "city311-case-" + strconv.FormatUint(request.ID, 10), ServiceRequestNumber: request.RequestNumber,
		ServiceType: request.ServiceType, Summary: request.Summary, DepartmentCode: request.OwningDepartment,
		Location: cloneOptionalMap(request.Location), CallbackURL: civicWorksCallbackPath,
	}
}

func civicWorksIdempotencyKey(requestID uint64) string {
	return "city311-civicworks-" + strconv.FormatUint(requestID, 10)
}

func decodeCivicWorksResponseMustMatch(workOrder *contract.CivicWorksWorkOrder, input contract.CivicWorksWorkOrderCreate) error {
	if workOrder == nil {
		return fmt.Errorf("CivicWorks returned no work order")
	}
	encoded, err := json.Marshal(workOrder)
	if err != nil {
		return err
	}
	return decodeCivicWorksResponse(encoded, &contract.CivicWorksWorkOrder{}, input)
}

func requestLookupError(err error) error {
	if err == nil {
		return nil
	}
	if errors.IsNotFound(err) {
		return apiError(http.StatusNotFound, contract.ErrorNotFound, "The service request was not found.")
	}
	return err
}

func versionConflict(version int) *ServiceError {
	current := uint64(version)
	return &ServiceError{Status: http.StatusConflict, Payload: contract.APIError{
		Error: contract.ErrorVersionConflict, Message: "The service request was updated by another operation.", Retryable: false, CurrentVersion: &current,
	}}
}

func civicWorksUnavailableError() *ServiceError {
	return &ServiceError{Status: http.StatusServiceUnavailable, Payload: contract.APIError{
		Error: contract.ErrorTemporarilyUnavailable, Message: "CivicWorks is temporarily unavailable.", Retryable: true,
	}}
}

func civicWorksEventValidationError(field string) *ServiceError {
	return validationError(contract.FieldError{Field: field, Code: contract.ValidationInvalidValue})
}
