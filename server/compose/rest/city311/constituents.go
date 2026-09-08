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
	ConstituentID     constituentStringList         `json:"constituent_id"`
	Query             constituentStringList         `json:"query"`
	DisplayName       constituentStringList         `json:"display_name"`
	Email             constituentStringList         `json:"email"`
	Phone             constituentStringList         `json:"phone"`
	PrimaryCategory   constituentStringList         `json:"primary_category"`
	PreferredLanguage constituentStringList         `json:"preferred_language"`
	EmailOptOut       constituentStringList         `json:"email_opt_out"`
	Department        constituentStringList         `json:"department"`
	District          constituentStringList         `json:"district"`
	CustomFields      constituentCustomFieldFilters `json:"custom_fields"`
}

type constituentStringList []string
type constituentCustomFieldFilters map[string]constituentStringList

func (values *constituentStringList) UnmarshalJSON(data []byte) error {
	var list []string
	if err := decodeNonNullConstituentFilter(data, &list, "constituent filter values must be arrays"); err != nil {
		return err
	}
	*values = list
	return nil
}

func (filters *constituentCustomFieldFilters) UnmarshalJSON(data []byte) error {
	var values map[string]constituentStringList
	if err := decodeNonNullConstituentFilter(data, &values, "constituent custom-field filters must be an object"); err != nil {
		return err
	}
	*filters = values
	return nil
}

func decodeNonNullConstituentFilter(data []byte, target interface{}, message string) error {
	if strings.TrimSpace(string(data)) == "null" {
		return errors.New(message)
	}
	return json.Unmarshal(data, target)
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
		if strings.TrimSpace(raw) == "null" {
			return nil, constituentQueryError("filters", contract.ValidationInvalidFormat)
		}
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
		input.ConstituentID = constituentStringList(queryValuesPreservingEmpty(query, "constituent_id"))
		input.Query = constituentStringList(queryValuesPreservingEmpty(query, "query"))
		input.DisplayName = constituentStringList(queryValuesPreservingEmpty(query, "display_name"))
		input.Email = constituentStringList(queryValuesPreservingEmpty(query, "email"))
		input.Phone = constituentStringList(queryValuesPreservingEmpty(query, "phone"))
		input.PrimaryCategory = constituentStringList(queryValuesPreservingEmpty(query, "primary_category"))
		input.PreferredLanguage = constituentStringList(queryValuesPreservingEmpty(query, "preferred_language"))
		input.EmailOptOut = constituentStringList(queryValuesPreservingEmpty(query, "email_opt_out"))
		input.Department = constituentStringList(queryValuesPreservingEmpty(query, "department"))
		input.District = constituentStringList(queryValuesPreservingEmpty(query, "district"))
		input.CustomFields = constituentCustomFieldFilters{}
		for key, values := range explodedCustomFieldFiltersPreservingEmpty(query) {
			input.CustomFields[key] = constituentStringList(values)
		}
	}
	filters := map[string][]string{}
	for key, values := range map[string]constituentStringList{
		"constituent_id": input.ConstituentID, "query": input.Query, "display_name": input.DisplayName,
		"email": input.Email, "phone": input.Phone, "primary_category": input.PrimaryCategory,
		"preferred_language": input.PreferredLanguage, "email_opt_out": input.EmailOptOut,
		"department": input.Department, "district": input.District,
	} {
		if values != nil {
			filters[key] = trimConstituentStringList(values)
		}
	}
	for key, values := range input.CustomFields {
		filters["custom_fields."+key] = trimConstituentStringList(values)
	}
	return filters, nil
}

func trimConstituentStringList(values constituentStringList) []string {
	out := make([]string, len(values))
	for index, value := range values {
		out[index] = strings.TrimSpace(value)
	}
	return out
}

func constituentQueryError(field string, code contract.ValidationCode) error {
	return &city311Service.ServiceError{Status: http.StatusUnprocessableEntity, Payload: contract.APIError{
		Error: contract.ErrorValidation, Message: invalidFieldsMessage, Retryable: false,
		Errors: []contract.FieldError{{Field: "/query/" + field, Code: code}},
	}}
}
