package civicworksfixture

import (
	"os"
	"strings"
	"testing"
)

func TestPackagedComplementCoversImplementedFixtureBoundary(t *testing.T) {
	document, err := os.ReadFile("../../../docs/civicworks-consumer-system.implementer-input.md")
	if err != nil {
		t.Fatalf("read packaged CivicWorks complement: %v", err)
	}
	contract := string(document)
	for _, required := range []string{
		"CIVICWORKS_BASE_URL",
		"CIVICWORKS_API_TOKEN",
		"CIVICWORKS_WEBHOOK_SECRET",
		"CIVICWORKS_CALLBACK_BASE_URL",
		"CIVICWORKS_CONTROL_TOKEN",
		"BENCHMARK_RUN_ID",
		"Authorization: Bearer <CIVICWORKS_API_TOKEN>",
		"Authorization: Bearer <CIVICWORKS_CONTROL_TOKEN>",
		"X-Benchmark-Run-Id: <BENCHMARK_RUN_ID>",
		"GET /healthz",
		"POST /api/v1/work-orders",
		"GET /api/v1/work-orders/{work_order_id}",
		"GET /api/v1/work-orders?source_case_id={source_case_id}",
		"POST /__fixture/v1/reset",
		"POST /__fixture/v1/work-orders/{work_order_id}/status",
		"POST /__fixture/v1/events/{event_id}/redeliver",
		"GET /__fixture/v1/deliveries",
		"PUT /__fixture/v1/outage",
		"ASSIGNED -> IN_PROGRESS",
		"IN_PROGRESS -> PARTIALLY_COMPLETED",
		"PARTIALLY_COMPLETED -> COMPLETED",
		"IDEMPOTENCY_CONFLICT",
		"SOURCE_CASE_CONFLICT",
		"INVALID_STATUS_TRANSITION",
		"TEMPORARILY_UNAVAILABLE",
		"HMAC-SHA256(CIVICWORKS_WEBHOOK_SECRET, exact_request_body)",
	} {
		if !strings.Contains(contract, required) {
			t.Errorf("packaged CivicWorks complement omits %q", required)
		}
	}
}
