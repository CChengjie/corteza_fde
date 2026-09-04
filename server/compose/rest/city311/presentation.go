package city311

import (
	"net/http"
	"strconv"
	"strings"

	city311Service "github.com/cortezaproject/corteza/server/compose/service/city311"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/go-chi/chi/v5"
)

func (h *handler) publicBrandingGet(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.PublicBranding(r.Context())
	writePresentationResult(w, http.StatusOK, result, err)
}

func (h *handler) publicContentGet(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.PublicContent(r.Context(), chi.URLParam(r, "content_key"))
	writePresentationResult(w, http.StatusOK, result, err)
}

func (h *handler) publicHelpGet(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.PublicHelp(r.Context(), chi.URLParam(r, "help_key"), preferredHelpLanguages(r.Header.Get("Accept-Language")))
	writePresentationResult(w, http.StatusOK, result, err)
}

func (h *handler) adminBrandingGet(w http.ResponseWriter, r *http.Request) {
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.AdminBranding(r.Context(), actor)
	writePresentationResult(w, http.StatusOK, result, err)
}

func (h *handler) adminBrandingPreview(w http.ResponseWriter, r *http.Request) {
	input := contract.BrandingWrite{}
	if !decodeJSON(w, r, &input) {
		return
	}
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.PreviewBranding(r.Context(), actor, input)
	writePresentationResult(w, http.StatusOK, result, err)
}

func (h *handler) adminBrandingUpdate(w http.ResponseWriter, r *http.Request) {
	version, ok := requiredVersion(w, r)
	if !ok {
		return
	}
	input := contract.BrandingWrite{}
	if !decodeJSON(w, r, &input) {
		return
	}
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.UpdateBranding(r.Context(), actor, version, input)
	writePresentationResult(w, http.StatusOK, result, err)
}

func (h *handler) adminBrandingPublish(w http.ResponseWriter, r *http.Request) {
	version, ok := requiredVersion(w, r)
	if !ok || !decodeEmptyJSON(w, r) {
		return
	}
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.PublishBranding(r.Context(), actor, version)
	writePresentationResult(w, http.StatusOK, result, err)
}

func (h *handler) adminBrandingRollback(w http.ResponseWriter, r *http.Request) {
	version, ok := requiredVersion(w, r)
	if !ok {
		return
	}
	input := contract.Rollback{}
	if !decodeJSON(w, r, &input) {
		return
	}
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.RollbackBranding(r.Context(), actor, version, input.TargetVersion)
	writePresentationResult(w, http.StatusOK, result, err)
}

func (h *handler) adminBrandingVersions(w http.ResponseWriter, r *http.Request) {
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	query, ok := presentationListQuery(w, r)
	if !ok {
		return
	}
	result, err := h.service.BrandingVersions(r.Context(), actor, query)
	writeResult(w, http.StatusOK, result, err)
}

func (h *handler) adminContentList(w http.ResponseWriter, r *http.Request) {
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	query, ok := presentationListQuery(w, r)
	if !ok {
		return
	}
	result, err := h.service.ListContent(r.Context(), actor, query)
	writeResult(w, http.StatusOK, result, err)
}

func (h *handler) adminContentGet(w http.ResponseWriter, r *http.Request) {
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.AdminContent(r.Context(), actor, chi.URLParam(r, "content_key"))
	writePresentationResult(w, http.StatusOK, result, err)
}

func (h *handler) adminContentPreview(w http.ResponseWriter, r *http.Request) {
	input := contract.ContentWrite{}
	if !decodeJSON(w, r, &input) {
		return
	}
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.PreviewContent(r.Context(), actor, chi.URLParam(r, "content_key"), input)
	writePresentationResult(w, http.StatusOK, result, err)
}

func (h *handler) adminContentUpdate(w http.ResponseWriter, r *http.Request) {
	version, ok := requiredVersion(w, r)
	if !ok {
		return
	}
	input := contract.ContentWrite{}
	if !decodeJSON(w, r, &input) {
		return
	}
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.UpdateContent(r.Context(), actor, chi.URLParam(r, "content_key"), version, input)
	writePresentationResult(w, http.StatusOK, result, err)
}

func (h *handler) adminContentPublish(w http.ResponseWriter, r *http.Request) {
	version, ok := requiredVersion(w, r)
	if !ok || !decodeEmptyJSON(w, r) {
		return
	}
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.PublishContent(r.Context(), actor, chi.URLParam(r, "content_key"), version)
	writePresentationResult(w, http.StatusOK, result, err)
}

func (h *handler) adminContentRollback(w http.ResponseWriter, r *http.Request) {
	version, ok := requiredVersion(w, r)
	if !ok {
		return
	}
	input := contract.Rollback{}
	if !decodeJSON(w, r, &input) {
		return
	}
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.RollbackContent(r.Context(), actor, chi.URLParam(r, "content_key"), version, input.TargetVersion)
	writePresentationResult(w, http.StatusOK, result, err)
}

func (h *handler) adminContentVersions(w http.ResponseWriter, r *http.Request) {
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	query, ok := presentationListQuery(w, r)
	if !ok {
		return
	}
	result, err := h.service.ContentVersions(r.Context(), actor, chi.URLParam(r, "content_key"), query)
	writeResult(w, http.StatusOK, result, err)
}

func (h *handler) adminHelpUpdate(w http.ResponseWriter, r *http.Request) {
	version, ok := requiredVersion(w, r)
	if !ok {
		return
	}
	input := contract.HelpWrite{}
	if !decodeJSON(w, r, &input) {
		return
	}
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.UpdateHelp(r.Context(), actor, chi.URLParam(r, "help_key"), version, input)
	writePresentationResult(w, http.StatusOK, result, err)
}

func presentationListQuery(w http.ResponseWriter, r *http.Request) (city311Service.PresentationListQuery, bool) {
	pageSize, ok := workflowPageSize(w, r)
	return city311Service.PresentationListQuery{PageSize: pageSize, PageToken: r.URL.Query().Get("page_token")}, ok
}

func preferredHelpLanguages(header string) []contract.Language {
	values := []contract.Language{}
	seen := map[contract.Language]bool{}
	for _, part := range strings.Split(header, ",") {
		code := strings.ToUpper(strings.TrimSpace(strings.SplitN(part, ";", 2)[0]))
		if len(code) >= 2 {
			code = code[:2]
		}
		language := contract.Language(code)
		if !seen[language] {
			seen[language] = true
			values = append(values, language)
		}
	}
	return values
}

func decodeEmptyJSON(w http.ResponseWriter, r *http.Request) bool {
	empty := struct{}{}
	return decodeJSON(w, r, &empty)
}

func writePresentationResult(w http.ResponseWriter, status int, value any, err error) {
	if err == nil {
		var version uint64
		switch typed := value.(type) {
		case *contract.Branding:
			version = typed.Version
		case *contract.ContentObject:
			version = typed.Version
		case *contract.HelpContent:
			version = typed.Version
		case *contract.Category:
			version = typed.Version
		case *contract.CustomFieldDefinition:
			version = typed.Version
		}
		if version > 0 {
			w.Header().Set("ETag", `"`+strconv.FormatUint(version, 10)+`"`)
		}
	}
	writeResult(w, status, value, err)
}
