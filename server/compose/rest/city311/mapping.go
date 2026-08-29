package city311

import (
	"net/http"

	city311Service "github.com/cortezaproject/corteza/server/compose/service/city311"
	"github.com/go-chi/chi/v5"
)

type mappingHandler struct {
	service            *city311Service.MappingService
	configurationError error
}

func MountMappingRoutes() func(chi.Router) {
	service, err := city311Service.NewMappingFromEnvironment(nil)
	return mountMappingRoutes(service, err)
}

func MountMappingRoutesWithService(service *city311Service.MappingService) func(chi.Router) {
	return mountMappingRoutes(service, nil)
}

func mountMappingRoutes(service *city311Service.MappingService, configurationError error) func(chi.Router) {
	return func(r chi.Router) {
		h := &mappingHandler{service: service, configurationError: configurationError}
		r.Post("/geocode", h.geocode)
	}
}

func (h *mappingHandler) geocode(w http.ResponseWriter, r *http.Request) {
	input := struct {
		Address string `json:"address"`
	}{}
	if !decodeJSON(w, r, &input) {
		return
	}
	if h.configurationError != nil || h.service == nil {
		writeResult(w, 0, nil, &city311Service.ServiceError{
			Status:  http.StatusServiceUnavailable,
			Payload: city311Service.MappingUnavailablePayload(),
		})
		return
	}
	response, err := h.service.Geocode(r.Context(), input.Address)
	writeResult(w, http.StatusOK, response, err)
}
