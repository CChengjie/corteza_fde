package city311

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	city311Service "github.com/cortezaproject/corteza/server/compose/service/city311"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/pkg/auth"
	"github.com/go-chi/chi/v5"
)

type workflowListFilters struct {
	Trigger string `json:"trigger"`
	Active  *bool  `json:"active"`
}

type workflowExecutionFilters struct {
	WorkflowID string `json:"workflow_id"`
	RequestID  string `json:"request_id"`
	Succeeded  *bool  `json:"succeeded"`
}

func (h *handler) workflowCreate(w http.ResponseWriter, r *http.Request) {
	input := contract.WorkflowDefinition{}
	if !decodeJSON(w, r, &input) {
		return
	}
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.CreateWorkflow(r.Context(), actor, input)
	writeResult(w, http.StatusCreated, result, err)
}

func (h *handler) workflowGet(w http.ResponseWriter, r *http.Request) {
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.GetWorkflow(r.Context(), actor, chi.URLParam(r, "workflow_id"))
	writeResult(w, http.StatusOK, result, err)
}

func (h *handler) workflowList(w http.ResponseWriter, r *http.Request) {
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	filters := workflowListFilters{}
	if !decodeQueryFilters(w, r, &filters) {
		return
	}
	pageSize, ok := workflowPageSize(w, r)
	if !ok {
		return
	}
	result, err := h.service.ListWorkflows(r.Context(), actor, city311Service.WorkflowListQuery{
		Trigger: strings.TrimSpace(filters.Trigger), Active: filters.Active, PageSize: pageSize, PageToken: r.URL.Query().Get("page_token"),
	})
	writeResult(w, http.StatusOK, result, err)
}

func (h *handler) workflowUpdate(w http.ResponseWriter, r *http.Request) {
	version, ok := requiredVersion(w, r)
	if !ok {
		return
	}
	input := contract.WorkflowDefinition{}
	if !decodeJSON(w, r, &input) {
		return
	}
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.UpdateWorkflow(r.Context(), actor, chi.URLParam(r, "workflow_id"), version, input)
	writeResult(w, http.StatusOK, result, err)
}

func (h *handler) workflowActivate(w http.ResponseWriter, r *http.Request) {
	h.workflowSetActive(w, r, true)
}

func (h *handler) workflowDeactivate(w http.ResponseWriter, r *http.Request) {
	h.workflowSetActive(w, r, false)
}

func (h *handler) workflowSetActive(w http.ResponseWriter, r *http.Request, active bool) {
	version, ok := requiredVersion(w, r)
	if !ok {
		return
	}
	empty := struct{}{}
	if !decodeJSON(w, r, &empty) {
		return
	}
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.SetWorkflowActive(r.Context(), actor, chi.URLParam(r, "workflow_id"), version, active)
	writeResult(w, http.StatusOK, result, err)
}

func (h *handler) workflowTest(w http.ResponseWriter, r *http.Request) {
	input := contract.WorkflowTest{}
	if !decodeJSON(w, r, &input) {
		return
	}
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.TestWorkflow(r.Context(), actor, chi.URLParam(r, "workflow_id"), input.RequestID)
	writeResult(w, http.StatusAccepted, result, err)
}

func (h *handler) workflowExecutionGet(w http.ResponseWriter, r *http.Request) {
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.GetWorkflowExecution(r.Context(), actor, chi.URLParam(r, "execution_id"))
	writeResult(w, http.StatusOK, result, err)
}

func (h *handler) workflowExecutionList(w http.ResponseWriter, r *http.Request) {
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	filters := workflowExecutionFilters{}
	if !decodeQueryFilters(w, r, &filters) {
		return
	}
	requestID := uint64(0)
	if filters.RequestID != "" {
		requestID, err = strconv.ParseUint(filters.RequestID, 10, 64)
		if err != nil || requestID == 0 {
			writeValidation(w, "/query/filters/request_id", contract.ValidationInvalidFormat)
			return
		}
	}
	pageSize, ok := workflowPageSize(w, r)
	if !ok {
		return
	}
	result, err := h.service.ListWorkflowExecutions(r.Context(), actor, city311Service.WorkflowExecutionQuery{
		WorkflowID: strings.TrimSpace(filters.WorkflowID), RequestID: requestID, Succeeded: filters.Succeeded,
		PageSize: pageSize, PageToken: r.URL.Query().Get("page_token"),
	})
	writeResult(w, http.StatusOK, result, err)
}

func (h *handler) workflowActor(r *http.Request) (contract.Actor, error) {
	return h.service.FindActor(r.Context(), auth.GetIdentityFromContext(r.Context()).Identity())
}

func workflowPageSize(w http.ResponseWriter, r *http.Request) (uint, bool) {
	value := uint64(50)
	var err error
	if raw := r.URL.Query().Get("page_size"); raw != "" {
		value, err = strconv.ParseUint(raw, 10, 16)
		if err != nil || value == 0 {
			writeValidation(w, "/query/page_size", contract.ValidationInvalidFormat)
			return 0, false
		}
	}
	return uint(value), true
}

func decodeQueryFilters(w http.ResponseWriter, r *http.Request, target any) bool {
	raw := strings.TrimSpace(r.URL.Query().Get("filters"))
	if raw == "" {
		return true
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeValidation(w, "/query/filters", contract.ValidationInvalidFormat)
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeValidation(w, "/query/filters", contract.ValidationInvalidFormat)
		return false
	}
	return true
}
