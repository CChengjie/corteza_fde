package city311

import (
	"net/http"
	"strings"

	city311Service "github.com/cortezaproject/corteza/server/compose/service/city311"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/go-chi/chi/v5"
)

func (h *handler) adminContactCategoryList(w http.ResponseWriter, r *http.Request) {
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	query, ok := configurationListQuery(w, r)
	if !ok {
		return
	}
	result, err := h.service.ListContactCategories(r.Context(), actor, query)
	writeResult(w, http.StatusOK, result, err)
}

func (h *handler) adminContactCategoryCreate(w http.ResponseWriter, r *http.Request) {
	input := contract.CategoryWrite{}
	if !decodeJSON(w, r, &input) {
		return
	}
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.CreateContactCategory(r.Context(), actor, input)
	writePresentationResult(w, http.StatusCreated, result, err)
}

func (h *handler) adminContactCategoryUpdate(w http.ResponseWriter, r *http.Request) {
	input := contract.CategoryWrite{}
	if !decodeJSON(w, r, &input) {
		return
	}
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.UpdateContactCategory(r.Context(), actor, chi.URLParam(r, "category_code"), input)
	writePresentationResult(w, http.StatusOK, result, err)
}

func (h *handler) adminCustomFieldList(w http.ResponseWriter, r *http.Request) {
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	query, ok := configurationListQuery(w, r)
	if !ok {
		return
	}
	result, err := h.service.ListCustomFields(r.Context(), actor, query)
	writeResult(w, http.StatusOK, result, err)
}

func (h *handler) adminCustomFieldCreate(w http.ResponseWriter, r *http.Request) {
	input := contract.CustomFieldDefinition{}
	if !decodeJSON(w, r, &input) {
		return
	}
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.CreateCustomField(r.Context(), actor, input)
	writePresentationResult(w, http.StatusCreated, result, err)
}

func (h *handler) adminCustomFieldUpdate(w http.ResponseWriter, r *http.Request) {
	version, ok := requiredVersion(w, r)
	if !ok {
		return
	}
	input := contract.CustomFieldDefinition{}
	if !decodeJSON(w, r, &input) {
		return
	}
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.UpdateCustomField(r.Context(), actor, chi.URLParam(r, "field_key"), version, input)
	writePresentationResult(w, http.StatusOK, result, err)
}

func configurationListQuery(w http.ResponseWriter, r *http.Request) (city311Service.ConfigurationListQuery, bool) {
	pageSize, ok := workflowPageSize(w, r)
	return city311Service.ConfigurationListQuery{PageSize: pageSize, PageToken: strings.TrimSpace(r.URL.Query().Get("page_token"))}, ok
}
