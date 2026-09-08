package city311

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	city311Service "github.com/cortezaproject/corteza/server/compose/service/city311"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/go-chi/chi/v5"
)

type constituentFilterInput struct {
	ConstituentID     stringList            `json:"constituent_id"`
	Query             stringList            `json:"query"`
	DisplayName       stringList            `json:"display_name"`
	Email             stringList            `json:"email"`
	Phone             stringList            `json:"phone"`
	PrimaryCategory   stringList            `json:"primary_category"`
	PreferredLanguage stringList            `json:"preferred_language"`
	EmailOptOut       stringList            `json:"email_opt_out"`
	Department        stringList            `json:"department"`
	District          stringList            `json:"district"`
	CustomFields      map[string]stringList `json:"custom_fields"`
}

func (h *handler) staffConstituentSearch(w http.ResponseWriter, r *http.Request) {
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	pageSize, ok := workflowPageSize(w, r)
	if !ok {
		return
	}
	filters, err := parseConstituentFilters(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.SearchConstituents(r.Context(), actor, city311Service.ConstituentSearchQuery{
		Filters: filters, PageSize: pageSize, PageToken: strings.TrimSpace(r.URL.Query().Get("page_token")), Sort: strings.TrimSpace(r.URL.Query().Get("sort")),
	})
	writeResult(w, http.StatusOK, result, err)
}

func (h *handler) staffConstituentDetail(w http.ResponseWriter, r *http.Request) {
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.GetStaffConstituent(r.Context(), actor, strings.TrimSpace(chi.URLParam(r, "constituent_id")))
	writeResult(w, http.StatusOK, result, err)
}

func parseConstituentFilters(r *http.Request) (map[string][]string, error) {
	input := constituentFilterInput{}
	raw := r.URL.Query().Get("filters")
	if raw != "" {
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			return nil, constituentQueryError("filters", contract.ValidationInvalidFormat)
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			return nil, constituentQueryError("filters", contract.ValidationInvalidFormat)
		}
	} else {
		query := r.URL.Query()
		input.ConstituentID = queryValues(query, "constituent_id")
		input.Query = queryValues(query, "query")
		input.DisplayName = queryValues(query, "display_name")
		input.Email = queryValues(query, "email")
		input.Phone = queryValues(query, "phone")
		input.PrimaryCategory = queryValues(query, "primary_category")
		input.PreferredLanguage = queryValues(query, "preferred_language")
		input.EmailOptOut = queryValues(query, "email_opt_out")
		input.Department = queryValues(query, "department")
		input.District = queryValues(query, "district")
		input.CustomFields = explodedCustomFieldFilters(query)
	}
	filters := map[string][]string{}
	for key, values := range map[string]stringList{
		"constituent_id": input.ConstituentID, "query": input.Query, "display_name": input.DisplayName,
		"email": input.Email, "phone": input.Phone, "primary_category": input.PrimaryCategory,
		"preferred_language": input.PreferredLanguage, "email_opt_out": input.EmailOptOut,
		"department": input.Department, "district": input.District,
	} {
		if trimmed := trimStringList(values); len(trimmed) > 0 {
			filters[key] = []string(trimmed)
		}
	}
	for key, values := range input.CustomFields {
		filters["custom_fields."+key] = []string(trimStringList(values))
	}
	return filters, nil
}

func constituentQueryError(field string, code contract.ValidationCode) error {
	return &city311Service.ServiceError{Status: http.StatusUnprocessableEntity, Payload: contract.APIError{
		Error: contract.ErrorValidation, Message: invalidFieldsMessage, Retryable: false,
		Errors: []contract.FieldError{{Field: "/query/" + field, Code: code}},
	}}
}
