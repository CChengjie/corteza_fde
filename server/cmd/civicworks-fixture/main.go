package main

import (
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cortezaproject/corteza/server/pkg/civicworksfixture"
)

func main() {
	if err := run(func(server *http.Server) error { return server.ListenAndServe() }); err != nil {
		log.Fatal(err)
	}
}

func run(serve func(*http.Server) error) error {
	fixture, err := civicworksfixture.New(civicworksfixture.Config{
		APIToken: os.Getenv("CIVICWORKS_API_TOKEN"), ControlToken: os.Getenv("CIVICWORKS_CONTROL_TOKEN"),
		WebhookSecret: os.Getenv("CIVICWORKS_WEBHOOK_SECRET"), BenchmarkRun: os.Getenv("BENCHMARK_RUN_ID"),
		PublicBaseURL: os.Getenv("CIVICWORKS_PUBLIC_BASE_URL"),
	})
	if err != nil {
		return err
	}
	address := strings.TrimSpace(os.Getenv("CIVICWORKS_FIXTURE_ADDR"))
	if address == "" {
		address = ":8080"
	}
	server := &http.Server{Addr: address, Handler: fixture.Handler(), ReadHeaderTimeout: 5 * time.Second}
	log.Printf("CivicWorks fixture listening on %s", address)
	if err = serve(server); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
