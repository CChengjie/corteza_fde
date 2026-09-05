package city311

import (
	"net/http"

	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
)

func (h *handler) emailReplacementRequest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	input := contract.EmailReplacementRequest{}
	if !decodeJSON(w, r, &input) {
		return
	}
	response, err := h.identity.RequestEmailReplacement(r.Context(), identitySessionFromContext(r.Context()), input)
	writeResult(w, http.StatusAccepted, response, err)
}

func (h *handler) emailReplacementConfirm(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	input := contract.EmailReplacementConfirm{}
	if !decodeJSON(w, r, &input) {
		return
	}
	response, err := h.identity.ConfirmEmailReplacement(r.Context(), input)
	writeResult(w, http.StatusOK, response, err)
}
