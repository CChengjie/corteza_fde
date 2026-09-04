package city311

import (
	"io"
	"net/http"
	"strconv"

	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/pkg/auth"
	"github.com/go-chi/chi/v5"
)

func (h *handler) attachmentUpload(w http.ResponseWriter, r *http.Request) {
	const maximumFileBytes = 10 << 20
	r.Body = http.MaxBytesReader(w, r.Body, maximumFileBytes+(64<<10))
	err := r.ParseMultipartForm(1 << 20)
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	if err != nil || r.MultipartForm == nil {
		writeValidation(w, "/file", contract.ValidationInvalidFormat)
		return
	}
	form := r.MultipartForm
	if len(form.File) != 1 || len(form.File["file"]) != 1 || len(form.Value) != 2 || len(form.Value["filename"]) != 1 || len(form.Value["media_type"]) != 1 {
		writeValidation(w, "/file", contract.ValidationInvalidValue)
		return
	}
	file, err := form.File["file"][0].Open()
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maximumFileBytes+1))
	if err != nil {
		writeValidation(w, "/file", contract.ValidationInvalidFormat)
		return
	}
	actor := auth.GetIdentityFromContext(r.Context())
	var ownerID uint64
	if actor.Valid() {
		ownerID = actor.Identity()
	}
	result, err := h.service.StageAttachment(r.Context(), ownerID, form.Value["filename"][0], form.Value["media_type"][0], content)
	w.Header().Set("Cache-Control", "no-store")
	writeResult(w, http.StatusCreated, result, err)
}

func (h *handler) attachmentDownload(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	actor, err := h.service.FindActor(r.Context(), auth.GetIdentityFromContext(r.Context()).Identity())
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	attachmentID, err := strconv.ParseUint(chi.URLParam(r, "attachment_id"), 10, 64)
	if err != nil || attachmentID == 0 {
		writeJSON(w, http.StatusNotFound, contract.APIError{Error: contract.ErrorNotFound, Message: "The attachment was not found.", Retryable: false})
		return
	}
	result, err := h.service.DownloadAttachment(r.Context(), actor, attachmentID)
	writeResult(w, http.StatusOK, result, err)
}
