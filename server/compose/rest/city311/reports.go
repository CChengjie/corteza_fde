package city311

import (
	"net/http"

	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/go-chi/chi/v5"
)

func (h *handler) reportCatalogue(w http.ResponseWriter, r *http.Request) {
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	query, ok := configurationListQuery(w, r)
	if !ok {
		return
	}
	result, err := h.service.ReportCatalogue(r.Context(), actor, query)
	writeResult(w, http.StatusOK, result, err)
}

func (h *handler) reportRun(w http.ResponseWriter, r *http.Request) {
	input := contract.ReportRun{}
	if !decodeJSON(w, r, &input) {
		return
	}
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.StartReportRun(r.Context(), actor, input)
	writeResult(w, http.StatusAccepted, result, err)
}

func (h *handler) savedReportList(w http.ResponseWriter, r *http.Request) {
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	query, ok := configurationListQuery(w, r)
	if !ok {
		return
	}
	result, err := h.service.ListSavedReports(r.Context(), actor, query)
	writeResult(w, http.StatusOK, result, err)
}

func (h *handler) savedReportCreate(w http.ResponseWriter, r *http.Request) {
	input := contract.ReportDefinition{}
	if !decodeJSON(w, r, &input) {
		return
	}
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.CreateSavedReport(r.Context(), actor, input)
	writePresentationResult(w, http.StatusCreated, result, err)
}

func (h *handler) savedReportUpdate(w http.ResponseWriter, r *http.Request) {
	version, ok := requiredVersion(w, r)
	if !ok {
		return
	}
	input := contract.ReportDefinition{}
	if !decodeJSON(w, r, &input) {
		return
	}
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.UpdateSavedReport(r.Context(), actor, chi.URLParam(r, "report_id"), version, input)
	writePresentationResult(w, http.StatusOK, result, err)
}

func (h *handler) savedReportShare(w http.ResponseWriter, r *http.Request) {
	version, ok := requiredVersion(w, r)
	if !ok {
		return
	}
	input := contract.ReportShare{}
	if !decodeJSON(w, r, &input) {
		return
	}
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.ShareSavedReport(r.Context(), actor, chi.URLParam(r, "report_id"), version, input)
	writePresentationResult(w, http.StatusOK, result, err)
}

func (h *handler) reportExport(w http.ResponseWriter, r *http.Request) {
	input := contract.ReportExport{}
	if !decodeJSON(w, r, &input) {
		return
	}
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.StartReportExport(r.Context(), actor, chi.URLParam(r, "report_id"), input)
	writeResult(w, http.StatusAccepted, result, err)
}
