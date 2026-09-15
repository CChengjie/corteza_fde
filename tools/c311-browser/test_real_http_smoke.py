"""Regression checks for the live C311 HTTP browser gate.

These checks intentionally inspect the gate itself.  A change that silently
turns the live job back into a mock-only page should fail before Docker starts.
"""

import unittest
from pathlib import Path


ROOT = Path(__file__).parents[2]


class RealHttpSmokeContractTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.smoke = (ROOT / "tools/c311-browser/real_http_smoke.py").read_text(encoding="utf-8")
        cls.runner = (ROOT / "tools/c311-browser/run-real-http.sh").read_text(encoding="utf-8")
        cls.workflow = (ROOT / ".github/workflows/test.yml").read_text(encoding="utf-8")

    def test_browser_is_forced_into_http_mode_and_rejects_mock_provider(self) -> None:
        self.assertIn("context.add_init_script(\"window.C311Mode = 'http';\")", self.smoke)
        self.assertIn("page.evaluate('window.C311Mode') != 'http'", self.smoke)
        self.assertIn("window.__C311MockProvider", self.smoke)

    def test_gate_requires_same_origin_backend_api_and_no_service_worker(self) -> None:
        self.assertIn("cross-origin API request", self.smoke)
        self.assertIn("response.from_service_worker", self.smoke)
        self.assertIn("/api/v1/public/branding", self.smoke)
        self.assertIn("credentials:'include'", self.smoke)

    def test_gate_clicks_real_submit_and_checks_admin_refresh(self) -> None:
        for marker in (
            "submit_button.click()",
            "data-c311-submission-result",
            "city311-platform-administrator",
            "/c311/admin",
            "City 311 administration",
            "/api/v1/admin/identity",
            "rendered-after-login-and-refresh",
        ):
            self.assertIn(marker, self.smoke)

    def test_runner_waits_for_database_and_frontend_before_browser(self) -> None:
        self.assertIn("/healthz", self.runner)
        self.assertIn('"database":"ok"', self.runner)
        self.assertIn("/config.js", self.runner)
        self.assertIn("real_http_smoke.py", self.runner)
        self.assertIn("BENCHMARK_NOW", self.runner)
        self.assertIn("2099-01-01T00:00:00Z", self.runner)
        self.assertIn("c311-real-http:", self.workflow)
        self.assertIn("run: tools/c311-browser/run-real-http.sh", self.workflow)

    def test_runner_uses_compose_health_gate_and_collects_failure_state(self) -> None:
        self.assertIn("--wait --wait-timeout 180", self.runner)
        self.assertIn("compose-ps.txt", self.runner)
        self.assertIn("trap cleanup EXIT", self.runner)
        self.assertIn("trap 'exit 143' INT TERM", self.runner)

    def test_workflow_validates_harness_before_starting_docker(self) -> None:
        validation = self.workflow.index("name: Validate live HTTP harness")
        execution = self.workflow.index("name: Verify browser to Corteza HTTP connectivity")
        self.assertLess(validation, execution)
        self.assertIn("bash -n tools/c311-browser/run-real-http.sh", self.workflow)
        self.assertIn("py_compile tools/c311-browser/real_http_smoke.py", self.workflow)


if __name__ == "__main__":
    unittest.main()
