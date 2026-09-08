package main

import (
	"encoding/json"
	"net/http"
	"os"
)

func main() {
	address := os.Getenv("HTTP_ADDR")
	if address == "" {
		address = ":8080"
	}
	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "stage": "bootstrap-harness"})
	})
	if err := http.ListenAndServe(address, nil); err != nil {
		panic(err)
	}
}
