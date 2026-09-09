import stat
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from fe10_matrix import ARTIFACT_DIR_ENV, JOURNEYS, VIEWPORTS, artifact_directory


class Fe10MatrixTests(unittest.TestCase):
    def test_matrix_covers_required_viewports_journeys_and_browser_markers(self):
        source = Path(__file__).with_name("fe10_matrix.py").read_text(encoding="utf-8")
        self.assertEqual(VIEWPORTS, ((360, 844), (767, 900), (768, 900), (1023, 900), (1024, 900), (1440, 900), (1920, 900)))
        self.assertEqual(JOURNEYS, ("login_identity", "public_submit", "anonymous_lookup", "staff_update"))
        for marker in ("C311Mode", "C311MockRole", "C311MockScenario", "C311MockSession", "BROWSERS", "chromium", "firefox", "webkit", "aria-describedby", "data-c311-error-summary", "data-c311-submission-result", "data-c311-status-result", "data-c311-request-detail", "console_errors", "page_errors", "failed_requests", "write_requests", "mocked_locale_requests", "cancelled_locale_requests", "localStorage", "sessionStorage", "horizontal overflow", "workflow-test", "report-export", "mail-preview", "calendar-export", "audit-filter", "assert_language_fallback", "assert_help_drawer_focus", "assert_modal_focus", "data-c311-status-announcer", "data-c311-operation-status", "full_page=True", "service_page", "workflow_page", "/c311/staff/reports", "/c311/staff/mail", "/c311/staff/calendar", "/c311/staff/audit", "/c311/staff/workflows"):
            self.assertIn(marker, source)

    def test_extension_pages_run_at_every_matrix_viewport(self):
        source = Path(__file__).with_name("fe10_matrix.py").read_text(encoding="utf-8")
        self.assertNotIn("if width in (767, 1440)", source)
        self.assertIn("extension_smoke(page, base_url, label, results)", source)

    def test_artifact_directory_is_private(self):
        with tempfile.TemporaryDirectory(prefix="c311-fe10-test-") as parent:
            configured = Path(parent) / "artifacts"
            with patch.dict("os.environ", {ARTIFACT_DIR_ENV: str(configured)}):
                directory = artifact_directory()
            self.assertEqual(stat.S_IMODE(directory.stat().st_mode), 0o700)


if __name__ == "__main__":
    unittest.main()
