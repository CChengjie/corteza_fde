package main

import (
	"encoding/json"
	"net/http"
	"os"
)

type city311Contract struct {
	APIVersion      string   `json:"api_version"`
	ServiceTypes    []string `json:"service_types"`
	RequestStatuses []string `json:"request_statuses"`
	ScopeFields     []string `json:"scope_fields"`
}

func main() {
	address := os.Getenv("HTTP_ADDR")
	if address == "" {
		address = ":8080"
	}
	contract := city311Contract{
		APIVersion: "v1", ServiceTypes: []string{"POTHOLE"},
		RequestStatuses: []string{"DRAFT", "SUBMITTED", "TRIAGED", "CLOSED"},
		ScopeFields:     []string{"department", "district"},
	}
	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "stage": "domain-contract"})
	})
	http.HandleFunc("/api/v1/system/info", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"database_configured": configured(os.Getenv("DATABASE_URL")), "build": "d1"})
	})
	http.HandleFunc("/api/v1/contracts/city311", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(contract)
	})
	if err := http.ListenAndServe(address, nil); err != nil {
		panic(err)
	}
}

func configured(value string) string {
	if value == "" {
		return "false"
	}
	return "true"
}
