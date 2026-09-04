package city311

import (
	"io"
	"mime"
	"net/http"

	city311Service "github.com/cortezaproject/corteza/server/compose/service/city311"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/go-chi/chi/v5"
)

func MountCivicWorksRoutes() func(chi.Router) {
	return MountCivicWorksRoutesWithService(city311Service.Default)
}

func MountCivicWorksRoutesWithService(service *city311Service.Service) func(chi.Router) {
	return func(r chi.Router) {
		h := &handler{service: service}
		r.Post("/civicworks/events", h.civicWorksEvent)
	}
}

func (h *handler) civicWorksEvent(w http.ResponseWriter, r *http.Request) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeValidation(w, "/headers/Content-Type", contract.ValidationInvalidValue)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maximumJSONBody))
	if err != nil {
		writeValidation(w, "/", contract.ValidationInvalidFormat)
		return
	}
	if h.service == nil {
		writeResult(w, 0, nil, &city311Service.ServiceError{Status: http.StatusServiceUnavailable, Payload: contract.APIError{
			Error: contract.ErrorTemporarilyUnavailable, Message: "CivicWorks integration is temporarily unavailable.", Retryable: true,
		}})
		return
	}
	err = h.service.HandleCivicWorksEvent(r.Context(), body, r.Header.Get("X-CivicWorks-Event-Id"), r.Header.Get("X-CivicWorks-Signature"))
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
