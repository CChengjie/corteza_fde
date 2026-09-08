package main

import (
	"encoding/json"
	"net/http"
	"os"
)

type runtimeConfig struct {
	DatabaseDSN string `json:"database_configured"`
	Build       string `json:"build"`
}

func main() {
	address := os.Getenv("HTTP_ADDR")
	if address == "" {
		address = ":8080"
	}
	config := runtimeConfig{DatabaseDSN: configured(os.Getenv("DATABASE_URL")), Build: valueOr(os.Getenv("BUILD_VERSION"), "p1")}
	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "stage": "platform-core"})
	})
	http.HandleFunc("/api/v1/system/info", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(config)
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
func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
