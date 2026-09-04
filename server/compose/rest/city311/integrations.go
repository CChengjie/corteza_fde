package city311

import (
	"net/http"
	"strings"

	city311Service "github.com/cortezaproject/corteza/server/compose/service/city311"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/go-chi/chi/v5"
)

type integrationFilterInput struct {
	Kind   stringList `json:"kind"`
	Active *bool      `json:"active"`
}

func (h *handler) adminIntegrationList(w http.ResponseWriter, r *http.Request) {
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	pageSize, ok := workflowPageSize(w, r)
	if !ok {
		return
	}
	filters := integrationFilterInput{}
	if !decodeQueryFilters(w, r, &filters) {
		return
	}
	kinds := make([]contract.IntegrationKind, 0, len(filters.Kind))
	seen := map[contract.IntegrationKind]bool{}
	for _, raw := range filters.Kind {
		kind := contract.IntegrationKind(strings.ToUpper(strings.TrimSpace(raw)))
		if !validIntegrationKind(kind) {
			writeValidation(w, "/query/filters/kind", contract.ValidationInvalidValue)
			return
		}
		if !seen[kind] {
			seen[kind] = true
			kinds = append(kinds, kind)
		}
	}
	result, err := h.service.ListIntegrations(r.Context(), actor, city311Service.IntegrationListQuery{
		Kinds: kinds, Active: filters.Active, PageSize: pageSize,
		PageToken: strings.TrimSpace(r.URL.Query().Get("page_token")), Sort: strings.TrimSpace(r.URL.Query().Get("sort")),
	})
	writeResult(w, http.StatusOK, result, err)
}

func (h *handler) adminIntegrationGet(w http.ResponseWriter, r *http.Request) {
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.GetIntegration(r.Context(), actor, chi.URLParam(r, "integration_id"))
	writePresentationResult(w, http.StatusOK, result, err)
}

func (h *handler) adminIntegrationUpdate(w http.ResponseWriter, r *http.Request) {
	version, ok := requiredVersion(w, r)
	if !ok {
		return
	}
	input := contract.IntegrationConnectionWrite{}
	if !decodeJSON(w, r, &input) {
		return
	}
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.UpdateIntegration(r.Context(), actor, chi.URLParam(r, "integration_id"), version, input)
	writePresentationResult(w, http.StatusOK, result, err)
}

func (h *handler) adminIntegrationRotate(w http.ResponseWriter, r *http.Request) {
	version, ok := requiredVersion(w, r)
	if !ok {
		return
	}
	input := contract.SecretRotation{}
	if !decodeJSON(w, r, &input) {
		return
	}
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.RotateIntegrationSecret(r.Context(), actor, chi.URLParam(r, "integration_id"), version, input)
	writePresentationResult(w, http.StatusOK, result, err)
}

func (h *handler) adminIntegrationRevoke(w http.ResponseWriter, r *http.Request) {
	version, ok := requiredVersion(w, r)
	if !ok {
		return
	}
	input := contract.Reason{}
	if !decodeJSON(w, r, &input) {
		return
	}
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.RevokeIntegration(r.Context(), actor, chi.URLParam(r, "integration_id"), version, input)
	writePresentationResult(w, http.StatusOK, result, err)
}

func validIntegrationKind(kind contract.IntegrationKind) bool {
	for _, candidate := range contract.IntegrationKinds {
		if kind == candidate {
			return true
		}
	}
	return false
}
