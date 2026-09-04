package city311

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	service "github.com/cortezaproject/corteza/server/compose/service/city311"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/pkg/auth"
	"github.com/go-chi/chi/v5"
)

func (h *handler) constituentList(w http.ResponseWriter, r *http.Request) { h.recordList(w, r, false) }
func (h *handler) auditList(w http.ResponseWriter, r *http.Request)       { h.recordList(w, r, true) }

func (h *handler) recordList(w http.ResponseWriter, r *http.Request, audit bool) {
	w.Header().Set("Cache-Control", "no-store")
	actor, err := h.service.FindActor(r.Context(), auth.GetIdentityFromContext(r.Context()).Identity())
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	if err = service.AuthorizeRecordRead(actor, audit); err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	name := "staff_constituent_search"
	if audit {
		name = "audit_list"
	}
	query, err := parseRecordReadQuery(r, contract.RecordReadFilterNames(name))
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	var result *service.RecordReadPage
	if audit {
		result, err = h.service.ListAuditEvents(r.Context(), actor, query)
	} else {
		result, err = h.service.ListConstituents(r.Context(), actor, query)
	}
	writeResult(w, http.StatusOK, result, err)
}

func (h *handler) constituentDetail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	actor, err := h.service.FindActor(r.Context(), auth.GetIdentityFromContext(r.Context()).Identity())
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.FindConstituent(r.Context(), actor, chi.URLParam(r, "constituent_id"))
	writeResult(w, http.StatusOK, result, err)
}

func parseRecordReadQuery(r *http.Request, allowed []string) (service.RecordReadQuery, error) {
	result := service.RecordReadQuery{PageSize: 50, Filters: map[string]string{}}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return result, recordQueryError("filters")
	}
	filters, err := recordReadFilters(query, allowed)
	if err != nil {
		return result, err
	}
	result.Filters = filters
	if query.Has("page_size") {
		number, err := strconv.ParseUint(query.Get("page_size"), 10, 16)
		if err != nil || number == 0 || number > 100 {
			return result, recordQueryError("page_size")
		}
		result.PageSize = uint(number)
	}
	result.Sort, result.PageToken = query.Get("sort"), query.Get("page_token")
	return result, nil
}

func recordReadFilters(query url.Values, allowed []string) (map[string]string, error) {
	result := map[string]string{}
	for name, values := range query {
		if len(values) != 1 {
			return result, recordQueryError(recordPointerToken(name))
		}
		switch name {
		case "page_size", "page_token", "sort", "filters":
		default:
			if !recordFilterAllowed(allowed, name) || query.Get("filters") != "" {
				return result, recordQueryError(recordPointerToken(name))
			}
			result[name] = values[0]
		}
	}
	if raw := query.Get("filters"); raw != "" {
		return decodeRecordFilters(raw)
	}
	return result, nil
}

func recordFilterAllowed(allowed []string, name string) bool {
	for _, field := range allowed {
		if name == field {
			return true
		}
	}
	return false
}

func decodeRecordFilters(raw string) (map[string]string, error) {
	var values map[string]*string
	if strings.TrimSpace(raw) == "null" || json.Unmarshal([]byte(raw), &values) != nil {
		return nil, recordQueryError("filters")
	}
	result := map[string]string{}
	for name, value := range values {
		if value == nil {
			return nil, recordQueryError("filters/" + recordPointerToken(name))
		}
		result[name] = *value
	}
	return result, nil
}

func recordQueryError(field string) error {
	return &service.ServiceError{Status: 422, Payload: contract.APIError{Error: contract.ErrorValidation, Message: invalidFieldsMessage, Errors: []contract.FieldError{{Field: "/query/" + field, Code: contract.ValidationInvalidValue}}}}
}

func recordPointerToken(value string) string {
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(value)
}
