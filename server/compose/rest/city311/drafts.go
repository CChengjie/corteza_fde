package city311

import (
	"net/http"
	"strconv"

	city311Service "github.com/cortezaproject/corteza/server/compose/service/city311"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/go-chi/chi/v5"
)

func requireConstituentIdentitySession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		resolved := identitySessionFromContext(r.Context())
		if resolved == nil || resolved.User == nil {
			writeJSON(w, http.StatusUnauthorized, contract.APIError{Error: contract.ErrorUnauthenticated, Message: authenticationRequiredMessage, Retryable: false})
			return
		}
		if resolved.Actor == nil {
			writeJSON(w, http.StatusForbidden, contract.APIError{Error: contract.ErrorForbidden, Message: "A constituent account is required.", Retryable: false})
			return
		}
		for _, role := range resolved.Actor.ApplicationRoles {
			if role == contract.ApplicationRoleConstituent {
				next.ServeHTTP(w, r)
				return
			}
		}
		writeJSON(w, http.StatusForbidden, contract.APIError{Error: contract.ErrorForbidden, Message: "A constituent account is required.", Retryable: false})
	})
}

func (h *handler) draftCreate(w http.ResponseWriter, r *http.Request) {
	input := contract.PortalDraftWrite{}
	if !decodeJSON(w, r, &input) {
		return
	}
	resolved := identitySessionFromContext(r.Context())
	result, err := h.service.CreateDraft(r.Context(), resolved.User.ID, input)
	writeDraftResult(w, http.StatusCreated, result, err)
}

func (h *handler) draftGet(w http.ResponseWriter, r *http.Request) {
	requestID, ok := draftRequestID(w, r)
	if !ok {
		return
	}
	resolved := identitySessionFromContext(r.Context())
	result, err := h.service.GetDraft(r.Context(), resolved.User.ID, requestID)
	writeDraftResult(w, http.StatusOK, result, err)
}

func (h *handler) draftUpdate(w http.ResponseWriter, r *http.Request) {
	requestID, ok := draftRequestID(w, r)
	if !ok {
		return
	}
	expectedVersion, err := parseIfMatch(r.Header.Get(contract.IfMatchHeader))
	if err != nil {
		writeResult(w, 0, nil, &city311Service.ServiceError{Status: http.StatusPreconditionRequired, Payload: contract.APIError{
			Error: contract.ErrorExpectedVersionRequired, Message: "If-Match must identify the expected draft version.", Retryable: false,
		}})
		return
	}
	input := contract.PortalDraftWrite{}
	if !decodeJSON(w, r, &input) {
		return
	}
	resolved := identitySessionFromContext(r.Context())
	result, err := h.service.UpdateDraft(r.Context(), resolved.User.ID, requestID, expectedVersion, input)
	writeDraftResult(w, http.StatusOK, result, err)
}

func (h *handler) draftDelete(w http.ResponseWriter, r *http.Request) {
	requestID, ok := draftRequestID(w, r)
	if !ok {
		return
	}
	expectedVersion, err := parseIfMatch(r.Header.Get(contract.IfMatchHeader))
	if err != nil {
		writeResult(w, 0, nil, &city311Service.ServiceError{Status: http.StatusPreconditionRequired, Payload: contract.APIError{
			Error: contract.ErrorExpectedVersionRequired, Message: "If-Match must identify the expected draft version.", Retryable: false,
		}})
		return
	}
	resolved := identitySessionFromContext(r.Context())
	if err = h.service.DeleteDraft(r.Context(), resolved.User.ID, requestID, expectedVersion); err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) draftSubmit(w http.ResponseWriter, r *http.Request) {
	requestID, ok := draftRequestID(w, r)
	if !ok {
		return
	}
	expectedVersion, err := parseIfMatch(r.Header.Get(contract.IfMatchHeader))
	if err != nil {
		writeResult(w, 0, nil, &city311Service.ServiceError{Status: http.StatusPreconditionRequired, Payload: contract.APIError{
			Error: contract.ErrorExpectedVersionRequired, Message: "If-Match must identify the expected draft version.", Retryable: false,
		}})
		return
	}
	resolved := identitySessionFromContext(r.Context())
	result, err := h.service.SubmitDraft(r.Context(), resolved.User.ID, requestID, expectedVersion)
	if err == nil && result != nil {
		w.Header().Set("ETag", `"`+strconv.FormatUint(result.Version, 10)+`"`)
	}
	writeResult(w, http.StatusOK, result, err)
}

func draftRequestID(w http.ResponseWriter, r *http.Request) (uint64, bool) {
	requestID, err := strconv.ParseUint(chi.URLParam(r, "request_id"), 10, 64)
	if err != nil || requestID == 0 {
		writeValidation(w, "/path/request_id", contract.ValidationInvalidFormat)
		return 0, false
	}
	return requestID, true
}

func writeDraftResult(w http.ResponseWriter, status int, result *contract.ServiceRequest, err error) {
	if err == nil && result != nil {
		w.Header().Set("ETag", `"`+strconv.FormatUint(result.Version, 10)+`"`)
	}
	writeResult(w, status, result, err)
}
