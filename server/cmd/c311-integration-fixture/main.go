package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

// This process is intentionally deterministic and state-light. It provides the
// local contracts used by City311 integration smoke tests without reaching the
// public internet or requiring operator configuration.
func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { write(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		issuer := "http://integration-fixture:8080"
		if strings.HasPrefix(r.Header.Get("X-Forwarded-Proto"), "https") {
			issuer = "https://integration-fixture:8080"
		}
		write(w, 200, map[string]string{"issuer": issuer, "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/oauth/token", "jwks_uri": issuer + "/.well-known/jwks.json"})
	})
	mux.HandleFunc("GET /saml/metadata", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/samlmetadata+xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<?xml version="1.0"?><md:EntityDescriptor xmlns:md="urn:oasis:names:tc:SAML:2.0:metadata" entityID="http://integration-fixture:8080/saml"><md:IDPSSODescriptor protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol"><md:SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect" Location="http://integration-fixture:8080/saml/sso"/></md:IDPSSODescriptor></md:EntityDescriptor>`))
	})
	mux.HandleFunc("POST /internal/integrations/mapping/geocode", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Address string `json:"address"`
		}
		if json.NewDecoder(r.Body).Decode(&in) != nil || strings.TrimSpace(in.Address) == "" {
			write(w, 422, map[string]any{"error": "VALIDATION_ERROR", "message": "address is required"})
			return
		}
		if strings.EqualFold(strings.TrimSpace(in.Address), "Unknown fixture address") {
			write(w, 404, map[string]any{"error": "NOT_FOUND", "message": "address not found"})
			return
		}
		write(w, 200, map[string]any{"latitude": 40.7128, "longitude": -74.0060, "formatted_address": strings.TrimSpace(in.Address)})
	})
	mux.HandleFunc("POST /oauth/token", func(w http.ResponseWriter, _ *http.Request) {
		write(w, 200, map[string]any{"access_token": "fixture-workflow-token", "token_type": "Bearer", "expires_in": 3600, "scope": "workflow.execute"})
	})
	mux.HandleFunc("POST /api/v1/actions", func(w http.ResponseWriter, r *http.Request) {
		write(w, 202, map[string]any{"execution_id": "fixture-execution-001", "accepted_at": "2026-01-15T12:00:00Z"})
	})
	mux.HandleFunc("POST /api/v1/mail/send", func(w http.ResponseWriter, _ *http.Request) {
		write(w, 202, map[string]any{"status": "DELIVERED", "message_id": "fixture-mail-001"})
	})
	_ = http.ListenAndServe(":8080", mux)
}

func write(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
