package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"
)

type validateLocation struct {
	Address string `json:"address"`
}
type mappingResult struct {
	Address   string  `json:"address"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Provider  string  `json:"provider"`
}

func main() {
	address := os.Getenv("HTTP_ADDR")
	if address == "" {
		address = ":8080"
	}
	client := &http.Client{Timeout: 3 * time.Second}
	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "stage": "integration-adapters"})
	})
	http.HandleFunc("/api/v1/contracts/city311", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"api_version": "v1", "service_types": []string{"POTHOLE"}, "request_statuses": []string{"DRAFT", "SUBMITTED", "TRIAGED", "CLOSED"}, "scope_fields": []string{"department", "district"}})
	})
	http.HandleFunc("/api/v1/internal/mapping/validate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		input := validateLocation{}
		if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Address) == "" {
			writeError(w, http.StatusUnprocessableEntity, "VALIDATION")
			return
		}
		baseURL, token := os.Getenv("MAP_BASE_URL"), os.Getenv("MAP_API_TOKEN")
		if baseURL == "" || token == "" {
			writeError(w, http.StatusServiceUnavailable, "MAP_TEMPORARILY_UNAVAILABLE")
			return
		}
		body, _ := json.Marshal(input)
		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, strings.TrimRight(baseURL, "/")+"/internal/integrations/mapping/geocode", bytes.NewReader(body))
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "MAP_TEMPORARILY_UNAVAILABLE")
			return
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		response, err := client.Do(req)
		if err != nil || response.StatusCode != http.StatusOK {
			if response != nil {
				response.Body.Close()
			}
			writeError(w, http.StatusServiceUnavailable, "MAP_TEMPORARILY_UNAVAILABLE")
			return
		}
		defer response.Body.Close()
		result := mappingResult{}
		if json.NewDecoder(response.Body).Decode(&result) != nil || result.Provider == "" {
			writeError(w, http.StatusServiceUnavailable, "MAP_TEMPORARILY_UNAVAILABLE")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	})
	if err := http.ListenAndServe(address, nil); err != nil {
		panic(err)
	}
}

func writeError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}
