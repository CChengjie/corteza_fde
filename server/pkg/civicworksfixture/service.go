package civicworksfixture

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
	"regexp"
	"strings"
	"sync"
	"time"

	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
)

const (
	defaultPublicBaseURL = "http://civicworks:8080"
	firstWorkOrderNumber = 1
	firstEventNumber     = 31
	callbackBodyLimit    = 1 << 20
)

var serviceRequestNumberPattern = regexp.MustCompile(`^SR-[0-9]{4}-[0-9]{5}$`)

type (
	Config struct {
		APIToken      string
		ControlToken  string
		WebhookSecret string
		BenchmarkRun  string
		PublicBaseURL string
		HTTPClient    *http.Client
		Wait          func(context.Context, time.Duration) error
	}

	Service struct {
		mu             sync.RWMutex
		controlMu      sync.Mutex
		apiToken       string
		controlToken   string
		webhookSecret  string
		benchmarkRun   string
		publicBaseURL  string
		httpClient     *http.Client
		wait           func(context.Context, time.Duration) error
		nextWorkOrder  int
		nextEvent      int
		orders         map[string]*storedWorkOrder
		ordersBySource map[string]string
		idempotency    map[string]idempotencyRecord
		events         map[string]storedEvent
		deliveries     []DeliveryAttempt
		outage         bool
	}

	storedWorkOrder struct {
		workOrder   contract.CivicWorksWorkOrder
		callback    string
		fingerprint string
	}

	idempotencyRecord struct {
		fingerprint string
		workOrderID string
	}

	storedEvent struct {
		event       contract.CivicWorksEvent
		body        []byte
		callbackURL string
	}

	DeliveryAttempt struct {
		EventID      string    `json:"event_id"`
		Attempt      int       `json:"attempt"`
		Acknowledged bool      `json:"acknowledged"`
		Status       *int      `json:"status,omitempty"`
		AttemptedAt  time.Time `json:"attempted_at"`
	}

	DeliverySummary struct {
		Acknowledged bool `json:"acknowledged"`
		Attempts     int  `json:"attempts"`
		Status       *int `json:"status,omitempty"`
	}

	AdvanceResult struct {
		WorkOrder contract.CivicWorksWorkOrder `json:"work_order"`
		Event     contract.CivicWorksEvent     `json:"event"`
		Delivery  DeliverySummary              `json:"delivery"`
	}

	FixtureError struct {
		Status    int    `json:"-"`
		Code      string `json:"error"`
		Message   string `json:"message"`
		Retryable bool   `json:"retryable"`
	}
)

func (e *FixtureError) Error() string { return e.Message }

func New(config Config) (*Service, error) {
	for name, value := range map[string]string{
		"CIVICWORKS_API_TOKEN": config.APIToken, "CIVICWORKS_CONTROL_TOKEN": config.ControlToken,
		"CIVICWORKS_WEBHOOK_SECRET": config.WebhookSecret, "BENCHMARK_RUN_ID": config.BenchmarkRun,
	} {
		if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\r\n") {
			return nil, fmt.Errorf("%s is required and must not contain surrounding whitespace or line breaks", name)
		}
	}
	if hmac.Equal([]byte(config.APIToken), []byte(config.ControlToken)) {
		return nil, fmt.Errorf("CIVICWORKS_API_TOKEN and CIVICWORKS_CONTROL_TOKEN must differ")
	}
	publicBaseURL := strings.TrimRight(strings.TrimSpace(config.PublicBaseURL), "/")
	if publicBaseURL == "" {
		publicBaseURL = defaultPublicBaseURL
	}
	parsed, err := url.Parse(publicBaseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("CIVICWORKS_PUBLIC_BASE_URL must be an absolute HTTP or HTTPS URL without credentials, query, or fragment")
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}
	wait := config.Wait
	if wait == nil {
		wait = waitForRetry
	}
	service := &Service{
		apiToken: config.APIToken, controlToken: config.ControlToken, webhookSecret: config.WebhookSecret,
		benchmarkRun: config.BenchmarkRun, publicBaseURL: publicBaseURL, httpClient: client, wait: wait,
	}
	service.resetLocked()
	return service, nil
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (svc *Service) Reset() {
	svc.controlMu.Lock()
	defer svc.controlMu.Unlock()
	svc.mu.Lock()
	defer svc.mu.Unlock()
	svc.resetLocked()
}

func (svc *Service) resetLocked() {
	svc.nextWorkOrder = firstWorkOrderNumber
	svc.nextEvent = firstEventNumber
	svc.orders = make(map[string]*storedWorkOrder)
	svc.ordersBySource = make(map[string]string)
	svc.idempotency = make(map[string]idempotencyRecord)
	svc.events = make(map[string]storedEvent)
	svc.deliveries = nil
	svc.outage = false
}

func (svc *Service) SetOutage(enabled bool) {
	svc.controlMu.Lock()
	defer svc.controlMu.Unlock()
	svc.mu.Lock()
	defer svc.mu.Unlock()
	svc.outage = enabled
}

func (svc *Service) Create(input contract.CivicWorksWorkOrderCreate, idempotencyKey string) (contract.CivicWorksWorkOrder, int, error) {
	fingerprintBody, err := json.Marshal(input)
	if err != nil {
		return contract.CivicWorksWorkOrder{}, 0, unavailableError()
	}
	fingerprint := hex.EncodeToString(sha256Sum(fingerprintBody))

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.outage {
		return contract.CivicWorksWorkOrder{}, 0, unavailableError()
	}
	if record, found := svc.idempotency[idempotencyKey]; found {
		if record.fingerprint != fingerprint {
			return contract.CivicWorksWorkOrder{}, 0, fixtureError(http.StatusConflict, "IDEMPOTENCY_CONFLICT", "The idempotency key has already been used with different content.", false)
		}
		return cloneWorkOrder(svc.orders[record.workOrderID].workOrder), http.StatusOK, nil
	}
	if workOrderID, found := svc.ordersBySource[input.SourceCaseID]; found {
		stored := svc.orders[workOrderID]
		if stored.fingerprint != fingerprint {
			return contract.CivicWorksWorkOrder{}, 0, fixtureError(http.StatusConflict, "SOURCE_CASE_CONFLICT", "The source case already has a materially different work order.", false)
		}
		svc.idempotency[idempotencyKey] = idempotencyRecord{fingerprint: fingerprint, workOrderID: workOrderID}
		return cloneWorkOrder(stored.workOrder), http.StatusOK, nil
	}

	sequence := svc.nextWorkOrder
	svc.nextWorkOrder++
	workOrderID := fmt.Sprintf("WO-%06d", sequence)
	createdAt := time.Date(2026, time.August, 20, 10, 0, 0, 0, time.UTC).Add(time.Duration(sequence-firstWorkOrderNumber) * time.Second)
	workOrder := contract.CivicWorksWorkOrder{
		WorkOrderID: workOrderID, SourceCaseID: input.SourceCaseID, ServiceRequestNumber: input.ServiceRequestNumber,
		ServiceType: input.ServiceType, Summary: input.Summary, DepartmentCode: input.DepartmentCode,
		FulfilmentSource: "CIVICWORKS", Status: contract.CivicWorksStatusAssigned, Location: cloneMap(input.Location),
		ExternalStatusURL: svc.publicBaseURL + "/ui/work-orders/" + workOrderID,
		Version:           1, CreatedAt: createdAt, UpdatedAt: createdAt,
	}
	svc.orders[workOrderID] = &storedWorkOrder{workOrder: workOrder, callback: input.CallbackURL, fingerprint: fingerprint}
	svc.ordersBySource[input.SourceCaseID] = workOrderID
	svc.idempotency[idempotencyKey] = idempotencyRecord{fingerprint: fingerprint, workOrderID: workOrderID}
	return cloneWorkOrder(workOrder), http.StatusCreated, nil
}

func (svc *Service) Find(workOrderID string) (contract.CivicWorksWorkOrder, error) {
	svc.mu.RLock()
	defer svc.mu.RUnlock()
	if svc.outage {
		return contract.CivicWorksWorkOrder{}, unavailableError()
	}
	stored, found := svc.orders[workOrderID]
	if !found {
		return contract.CivicWorksWorkOrder{}, workOrderNotFoundError()
	}
	return cloneWorkOrder(stored.workOrder), nil
}

func (svc *Service) FindBySource(sourceCaseID string) ([]contract.CivicWorksWorkOrder, error) {
	svc.mu.RLock()
	defer svc.mu.RUnlock()
	if svc.outage {
		return nil, unavailableError()
	}
	workOrderID, found := svc.ordersBySource[sourceCaseID]
	if !found {
		return []contract.CivicWorksWorkOrder{}, nil
	}
	return []contract.CivicWorksWorkOrder{cloneWorkOrder(svc.orders[workOrderID].workOrder)}, nil
}

func (svc *Service) Advance(ctx context.Context, workOrderID string, status contract.CivicWorksStatus) (AdvanceResult, error) {
	svc.controlMu.Lock()
	defer svc.controlMu.Unlock()

	svc.mu.Lock()
	stored, found := svc.orders[workOrderID]
	if !found {
		svc.mu.Unlock()
		return AdvanceResult{}, workOrderNotFoundError()
	}
	if !allowedTransition(stored.workOrder.Status, status) {
		svc.mu.Unlock()
		return AdvanceResult{}, fixtureError(http.StatusConflict, "INVALID_STATUS_TRANSITION", "The requested CivicWorks status transition is not allowed.", false)
	}
	previous := stored.workOrder.Status
	stored.workOrder.Status = status
	stored.workOrder.Version++
	stored.workOrder.UpdatedAt = stored.workOrder.UpdatedAt.Add(5 * time.Minute)
	eventID := fmt.Sprintf("EVT-%06d", svc.nextEvent)
	svc.nextEvent++
	event := contract.CivicWorksEvent{
		EventID: eventID, EventType: "work_order.status_changed", WorkOrderID: workOrderID,
		SourceCaseID: stored.workOrder.SourceCaseID, PreviousStatus: previous, Status: status,
		Version: stored.workOrder.Version, OccurredAt: stored.workOrder.UpdatedAt,
	}
	body, err := json.Marshal(event)
	if err != nil {
		svc.mu.Unlock()
		return AdvanceResult{}, unavailableError()
	}
	storedEvent := storedEvent{event: event, body: body, callbackURL: stored.callback}
	svc.events[eventID] = storedEvent
	workOrder := cloneWorkOrder(stored.workOrder)
	svc.mu.Unlock()

	delivery, err := svc.deliver(ctx, storedEvent)
	return AdvanceResult{WorkOrder: workOrder, Event: event, Delivery: delivery}, err
}

func (svc *Service) Redeliver(ctx context.Context, eventID string) (DeliverySummary, error) {
	svc.controlMu.Lock()
	defer svc.controlMu.Unlock()
	svc.mu.RLock()
	event, found := svc.events[eventID]
	svc.mu.RUnlock()
	if !found {
		return DeliverySummary{}, fixtureError(http.StatusNotFound, "EVENT_NOT_FOUND", "The fixture event was not found.", false)
	}
	return svc.deliver(ctx, event)
}

func (svc *Service) DeliveryAttempts() []DeliveryAttempt {
	svc.mu.RLock()
	defer svc.mu.RUnlock()
	return append([]DeliveryAttempt(nil), svc.deliveries...)
}

func (svc *Service) deliver(ctx context.Context, event storedEvent) (DeliverySummary, error) {
	if event.callbackURL == "" {
		return DeliverySummary{}, nil
	}
	retryDelay := []time.Duration{0, time.Second, 5 * time.Second}
	logicalOffset := []time.Duration{0, time.Second, 6 * time.Second}
	summary := DeliverySummary{}
	for index := range retryDelay {
		if retryDelay[index] > 0 {
			if err := svc.wait(ctx, retryDelay[index]); err != nil {
				return summary, err
			}
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, event.callbackURL, bytes.NewReader(event.body))
		if err != nil {
			return summary, err
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-CivicWorks-Event-Id", event.event.EventID)
		request.Header.Set("X-CivicWorks-Signature", "sha256="+signature(event.body, svc.webhookSecret))
		response, requestErr := svc.httpClient.Do(request)
		var responseStatus *int
		acknowledged := false
		if requestErr == nil {
			status := response.StatusCode
			responseStatus = &status
			acknowledged = status >= 200 && status < 300
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, callbackBodyLimit))
			_ = response.Body.Close()
		}
		attempt := DeliveryAttempt{
			EventID: event.event.EventID, Attempt: index + 1, Acknowledged: acknowledged,
			Status: responseStatus, AttemptedAt: event.event.OccurredAt.Add(logicalOffset[index]),
		}
		svc.mu.Lock()
		svc.deliveries = append(svc.deliveries, attempt)
		svc.mu.Unlock()
		summary.Attempts++
		if responseStatus != nil {
			summary.Status = responseStatus
		}
		if acknowledged {
			summary.Acknowledged = true
			return summary, nil
		}
	}
	return summary, nil
}

func (svc *Service) ValidConsumerCredentials(authorization, benchmarkRun string) *FixtureError {
	return svc.validCredentials(authorization, benchmarkRun, svc.apiToken)
}

func (svc *Service) ValidControlCredentials(authorization, benchmarkRun string) *FixtureError {
	return svc.validCredentials(authorization, benchmarkRun, svc.controlToken)
}

func (svc *Service) validCredentials(authorization, benchmarkRun, expectedToken string) *FixtureError {
	prefix := "Bearer "
	if !strings.HasPrefix(authorization, prefix) || !hmac.Equal([]byte(strings.TrimPrefix(authorization, prefix)), []byte(expectedToken)) {
		return fixtureError(http.StatusUnauthorized, "UNAUTHORIZED", "The CivicWorks credential is missing or invalid.", false)
	}
	if !hmac.Equal([]byte(benchmarkRun), []byte(svc.benchmarkRun)) {
		return fixtureError(http.StatusForbidden, "RUN_SCOPE_MISMATCH", "The benchmark run does not match this CivicWorks fixture.", false)
	}
	return nil
}

func ValidateCreate(input contract.CivicWorksWorkOrderCreate, idempotencyKey string) *FixtureError {
	if strings.TrimSpace(idempotencyKey) == "" {
		return validationError("Idempotency-Key is required")
	}
	if strings.TrimSpace(input.SourceCaseID) == "" {
		return validationError("source_case_id is required")
	}
	if !serviceRequestNumberPattern.MatchString(input.ServiceRequestNumber) {
		return validationError("service_request_number is malformed")
	}
	if !contains(contract.ServiceTypes, input.ServiceType) {
		return validationError("service_type is invalid")
	}
	if strings.TrimSpace(input.Summary) == "" || len([]rune(input.Summary)) > 160 {
		return validationError("summary is required and must not exceed 160 characters")
	}
	if !contains(contract.DepartmentCodes, input.DepartmentCode) {
		return validationError("department_code is invalid")
	}
	if err := validateLocation(input.Location); err != nil {
		return validationError(err.Error())
	}
	if input.CallbackURL != "" {
		callbackURL, err := url.Parse(input.CallbackURL)
		if err != nil || callbackURL.Host == "" || (callbackURL.Scheme != "http" && callbackURL.Scheme != "https") || callbackURL.User != nil || callbackURL.Fragment != "" {
			return validationError("callback_url must be an absolute HTTP or HTTPS URL without credentials or fragment")
		}
	}
	return nil
}

func validateLocation(location map[string]any) error {
	if len(location) == 0 {
		return nil
	}
	for key := range location {
		if key != "address" && key != "latitude" && key != "longitude" {
			return fmt.Errorf("location contains an unknown field")
		}
	}
	address, ok := location["address"].(string)
	if !ok || strings.TrimSpace(address) == "" {
		return fmt.Errorf("location.address is required")
	}
	latitude, latitudeOK := location["latitude"].(float64)
	longitude, longitudeOK := location["longitude"].(float64)
	if latitudeOK != longitudeOK {
		return fmt.Errorf("location latitude and longitude must be supplied together")
	}
	if latitudeOK && (latitude < -90 || latitude > 90 || longitude < -180 || longitude > 180) {
		return fmt.Errorf("location coordinates are outside WGS84 ranges")
	}
	return nil
}

func allowedTransition(from, to contract.CivicWorksStatus) bool {
	allowed := map[contract.CivicWorksStatus][]contract.CivicWorksStatus{
		contract.CivicWorksStatusAssigned:           {contract.CivicWorksStatusInProgress, contract.CivicWorksStatusCompleted},
		contract.CivicWorksStatusInProgress:         {contract.CivicWorksStatusPartiallyCompleted, contract.CivicWorksStatusCompleted},
		contract.CivicWorksStatusPartiallyCompleted: {contract.CivicWorksStatusInProgress, contract.CivicWorksStatusCompleted},
	}
	return contains(allowed[from], to)
}

func contains[T comparable](values []T, target T) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func signature(body []byte, secret string) string {
	digest := hmac.New(sha256.New, []byte(secret))
	_, _ = digest.Write(body)
	return hex.EncodeToString(digest.Sum(nil))
}

func sha256Sum(value []byte) []byte {
	digest := sha256.Sum256(value)
	return digest[:]
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	encoded, _ := json.Marshal(input)
	output := map[string]any{}
	_ = json.Unmarshal(encoded, &output)
	return output
}

func cloneWorkOrder(input contract.CivicWorksWorkOrder) contract.CivicWorksWorkOrder {
	input.Location = cloneMap(input.Location)
	return input
}

func fixtureError(status int, code, message string, retryable bool) *FixtureError {
	return &FixtureError{Status: status, Code: code, Message: message, Retryable: retryable}
}

func validationError(message string) *FixtureError {
	return fixtureError(http.StatusUnprocessableEntity, "VALIDATION_ERROR", message, false)
}

func workOrderNotFoundError() *FixtureError {
	return fixtureError(http.StatusNotFound, "WORK_ORDER_NOT_FOUND", "The CivicWorks work order was not found.", false)
}

func unavailableError() *FixtureError {
	return fixtureError(http.StatusServiceUnavailable, "TEMPORARILY_UNAVAILABLE", "CivicWorks is temporarily unavailable.", true)
}
