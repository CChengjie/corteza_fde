package civicworksfixture

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
)

const requestBodyLimit = 1 << 20

type (
	statusAdvanceRequest struct {
		Status contract.CivicWorksStatus `json:"status"`
	}

	outageRequest struct {
		Enabled bool `json:"enabled"`
	}

	workOrderListItem struct {
		WorkOrderID  string                    `json:"work_order_id"`
		SourceCaseID string                    `json:"source_case_id"`
		Status       contract.CivicWorksStatus `json:"status"`
		Version      uint64                    `json:"version"`
		UpdatedAt    time.Time                 `json:"updated_at"`
	}
)

func (svc *Service) Handler() http.Handler {
	router := http.NewServeMux()
	router.HandleFunc("GET /healthz", svc.health)
	router.HandleFunc("POST /api/v1/work-orders", svc.createWorkOrder)
	router.HandleFunc("GET /api/v1/work-orders", svc.findWorkOrderBySource)
	router.HandleFunc("GET /api/v1/work-orders/{work_order_id}", svc.findWorkOrder)
	router.HandleFunc("POST /__fixture/v1/reset", svc.resetFixture)
	router.HandleFunc("POST /__fixture/v1/work-orders/{work_order_id}/status", svc.advanceWorkOrder)
	router.HandleFunc("POST /__fixture/v1/events/{event_id}/redeliver", svc.redeliverEvent)
	router.HandleFunc("GET /__fixture/v1/deliveries", svc.listDeliveries)
	router.HandleFunc("PUT /__fixture/v1/outage", svc.updateOutage)
	return router
}

func (svc *Service) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (svc *Service) createWorkOrder(w http.ResponseWriter, request *http.Request) {
	if !svc.authorizeConsumer(w, request) {
		return
	}
	input := contract.CivicWorksWorkOrderCreate{}
	if err := decodeRequest(request, &input); err != nil {
		writeFixtureError(w, err)
		return
	}
	key := request.Header.Get(contract.IdempotencyHeader)
	if err := ValidateCreate(input, key); err != nil {
		writeFixtureError(w, err)
		return
	}
	workOrder, status, err := svc.Create(input, key)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, status, workOrder)
}

func (svc *Service) findWorkOrder(w http.ResponseWriter, request *http.Request) {
	if !svc.authorizeConsumer(w, request) {
		return
	}
	workOrder, err := svc.Find(strings.TrimSpace(request.PathValue("work_order_id")))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, workOrder)
}

func (svc *Service) findWorkOrderBySource(w http.ResponseWriter, request *http.Request) {
	if !svc.authorizeConsumer(w, request) {
		return
	}
	if len(request.URL.Query()) != 1 || len(request.URL.Query()["source_case_id"]) != 1 || strings.TrimSpace(request.URL.Query().Get("source_case_id")) == "" {
		writeFixtureError(w, validationError("source_case_id is required and must be the only query parameter"))
		return
	}
	workOrders, err := svc.FindBySource(strings.TrimSpace(request.URL.Query().Get("source_case_id")))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	items := make([]workOrderListItem, 0, len(workOrders))
	for _, workOrder := range workOrders {
		items = append(items, workOrderListItem{
			WorkOrderID: workOrder.WorkOrderID, SourceCaseID: workOrder.SourceCaseID,
			Status: workOrder.Status, Version: workOrder.Version, UpdatedAt: workOrder.UpdatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (svc *Service) resetFixture(w http.ResponseWriter, request *http.Request) {
	if !svc.authorizeControl(w, request) {
		return
	}
	svc.Reset()
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "reset", "next_work_order_id": "WO-000001", "next_event_id": "EVT-000031",
	})
}

func (svc *Service) advanceWorkOrder(w http.ResponseWriter, request *http.Request) {
	if !svc.authorizeControl(w, request) {
		return
	}
	input := statusAdvanceRequest{}
	if err := decodeRequest(request, &input); err != nil {
		writeFixtureError(w, err)
		return
	}
	if !contains(contract.CivicWorksStatuses, input.Status) {
		writeFixtureError(w, validationError("status is required and must use the CivicWorks status vocabulary"))
		return
	}
	result, err := svc.Advance(request.Context(), strings.TrimSpace(request.PathValue("work_order_id")), input.Status)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (svc *Service) redeliverEvent(w http.ResponseWriter, request *http.Request) {
	if !svc.authorizeControl(w, request) {
		return
	}
	result, err := svc.Redeliver(request.Context(), strings.TrimSpace(request.PathValue("event_id")))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (svc *Service) listDeliveries(w http.ResponseWriter, request *http.Request) {
	if !svc.authorizeControl(w, request) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": svc.DeliveryAttempts()})
}

func (svc *Service) updateOutage(w http.ResponseWriter, request *http.Request) {
	if !svc.authorizeControl(w, request) {
		return
	}
	input := outageRequest{}
	if err := decodeRequest(request, &input); err != nil {
		writeFixtureError(w, err)
		return
	}
	svc.SetOutage(input.Enabled)
	writeJSON(w, http.StatusOK, input)
}

func (svc *Service) authorizeConsumer(w http.ResponseWriter, request *http.Request) bool {
	return svc.authorize(w, request, svc.ValidConsumerCredentials)
}

func (svc *Service) authorizeControl(w http.ResponseWriter, request *http.Request) bool {
	return svc.authorize(w, request, svc.ValidControlCredentials)
}

func (svc *Service) authorize(w http.ResponseWriter, request *http.Request, validate func(string, string) *FixtureError) bool {
	if err := validate(request.Header.Get("Authorization"), request.Header.Get("X-Benchmark-Run-Id")); err != nil {
		writeFixtureError(w, err)
		return false
	}
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeFixtureError(w, validationError("Content-Type must be application/json"))
		return false
	}
	return true
}

func decodeRequest(request *http.Request, target any) *FixtureError {
	decoder := json.NewDecoder(io.LimitReader(request.Body, requestBodyLimit+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return validationError("The request body must be a valid JSON object with only declared fields.")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return validationError("The request body must contain exactly one JSON object.")
	}
	return nil
}

func writeServiceError(w http.ResponseWriter, err error) {
	fixtureErr, ok := err.(*FixtureError)
	if !ok {
		fixtureErr = unavailableError()
	}
	writeFixtureError(w, fixtureErr)
}

func writeFixtureError(w http.ResponseWriter, err *FixtureError) {
	writeJSON(w, err.Status, err)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
