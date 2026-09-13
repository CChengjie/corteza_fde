package city311

import (
	"net/http"
	"strconv"

	service "github.com/cortezaproject/corteza/server/compose/service/city311"

	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
)

func (h *handler) profileGet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	profile, err := h.identity.GetProfileSnapshot(r.Context(), identitySessionFromContext(r.Context()))
	writeProfile(w, profile, err)
}

func (h *handler) accountDelete(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if err := h.identity.DeleteAccount(r.Context(), identitySessionFromContext(r.Context())); err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	h.expireIdentityCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func requireProfileConstituent(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resolved := identitySessionFromContext(r.Context())
		if resolved != nil && resolved.Actor != nil {
			for _, role := range resolved.Actor.ApplicationRoles {
				if role == contract.ApplicationRoleConstituent {
					next.ServeHTTP(w, r)
					return
				}
			}
		}
		writeJSON(w, http.StatusForbidden, contract.APIError{Error: contract.ErrorForbidden, Message: "A constituent account is required."})
	})
}

func (h *handler) profileUpdate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var input contract.ProfileUpdate
	if !decodeJSON(w, r, &input) {
		return
	}
	version, err := parseIfMatch(r.Header.Get(contract.IfMatchHeader))
	if err != nil {
		writeResult(w, 0, nil, &service.ServiceError{Status: 428, Payload: contract.APIError{Error: contract.ErrorExpectedVersionRequired, Message: "A quoted current profile version is required."}})
		return
	}
	profile, err := h.identity.UpdateProfile(r.Context(), identitySessionFromContext(r.Context()), version, input)
	writeProfile(w, profile, err)
}

func writeProfile(w http.ResponseWriter, profile *service.ProfileSnapshot, err error) {
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	w.Header().Set("ETag", `"`+strconv.FormatUint(profile.Version, 10)+`"`)
	writeJSON(w, http.StatusOK, profile.Constituent)
}

func (h *handler) languageUpdate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var input contract.LanguagePreference
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Language != contract.LanguageEN && input.Language != contract.LanguageES && input.Language != contract.LanguageVI {
		writeValidation(w, "/language", contract.ValidationInvalidValue)
		return
	}
	resolved := identitySessionFromContext(r.Context())
	if resolved != nil && resolved.Actor != nil {
		for _, role := range resolved.Actor.ApplicationRoles {
			if role != contract.ApplicationRoleConstituent {
				continue
			}
			err := h.identity.SetPreferredLanguage(r.Context(), resolved, input.Language)
			if err != nil {
				writeResult(w, 0, nil, err)
				return
			}
			break
		}
	}
	// The browser owns anonymous session preferences; no anonymous account is
	// created. Authenticated constituents persist the preference above.
	writeJSON(w, http.StatusOK, input)
}
