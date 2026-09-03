package city311

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
)

func (h *handler) publicStatusLookup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	input := contract.AnonymousStatusLookupRequest{}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusNotFound, contract.AnonymousStatusLookupResponse{})
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusNotFound, contract.AnonymousStatusLookupResponse{})
		return
	}
	result, err := h.service.LookupPublicStatus(r.Context(), input)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	status := http.StatusOK
	if result.RequestDetail == nil {
		status = http.StatusNotFound
	}
	writeJSON(w, status, result)
}
