package city311

import (
	"net/http"
	"strings"

	city311Service "github.com/cortezaproject/corteza/server/compose/service/city311"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/go-chi/chi/v5"
)

func (h *handler) adminIdentityConfigurationGet(w http.ResponseWriter, r *http.Request) {
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.identity.IdentityConfiguration(r.Context(), actor)
	writePresentationResult(w, http.StatusOK, result, err)
}

func (h *handler) adminIdentityConfigurationUpdate(w http.ResponseWriter, r *http.Request) {
	version, ok := requiredVersion(w, r)
	if !ok {
		return
	}
	input := contract.IdentityConfigurationWrite{}
	if !decodeJSON(w, r, &input) {
		return
	}
	actor, err := h.workflowActor(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.identity.UpdateIdentityConfiguration(r.Context(), actor, version, input)
	writePresentationResult(w, http.StatusOK, result, err)
}

func (h *handler) federatedSignInStart(w http.ResponseWriter, r *http.Request) {
	result, flow, err := h.identity.StartFederatedSignIn(
		r.Context(), chi.URLParam(r, "provider"), r.URL.Query().Get("client"), identitySessionFromContext(r.Context()),
	)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	h.setFederationCookie(w, flow)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, result)
}

func (h *handler) federatedSignInCallback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	cookie, err := r.Cookie(city311Service.FederationFlowCookie)
	if err != nil || strings.TrimSpace(r.URL.Query().Get("error")) != "" {
		h.expireFederationCookie(w)
		writeJSON(w, http.StatusUnauthorized, contract.APIError{
			Error: contract.ErrorUnauthenticated, Message: "Federated authentication failed. Please return to sign in and try again.", Retryable: false,
		})
		return
	}
	provider := chi.URLParam(r, "provider")
	state := r.URL.Query().Get("state")
	if strings.EqualFold(provider, "saml") {
		state = r.URL.Query().Get("RelayState")
	}
	token, resolved, completionErr := h.identity.CompleteFederatedSignIn(
		r.Context(), provider, state, r.URL.Query().Get("code"), r.URL.Query().Get("SAMLResponse"), cookie.Value,
	)
	h.expireFederationCookie(w)
	if completionErr != nil {
		writeResult(w, 0, nil, completionErr)
		return
	}
	h.setIdentityCookie(w, token)
	writeJSON(w, http.StatusOK, h.identity.Session(resolved))
}

func (h *handler) setFederationCookie(w http.ResponseWriter, value string) {
	http.SetCookie(w, &http.Cookie{
		Name: city311Service.FederationFlowCookie, Value: value, Path: "/api/v1/auth",
		MaxAge: 600, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
	})
}

func (h *handler) expireFederationCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: city311Service.FederationFlowCookie, Value: "", Path: "/api/v1/auth",
		MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
	})
}
