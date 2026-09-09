import stat
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from fe10_matrix import ARTIFACT_DIR_ENV, JOURNEYS, VIEWPORTS, artifact_directory


class Fe10MatrixTests(unittest.TestCase):
    def test_matrix_covers_required_viewports_journeys_and_browser_markers(self):
        source = Path(__file__).with_name("fe10_matrix.py").read_text(encoding="utf-8")
        self.assertEqual(VIEWPORTS, ((360, 844), (390, 844), (768, 900), (1024, 900), (1440, 900), (1920, 900)))
        self.assertEqual(JOURNEYS, ("login_identity", "public_submit", "anonymous_lookup", "staff_update"))
        for marker in ("C311Mode", "C311MockRole", "C311MockScenario", "C311MockSession", "chromium", "firefox", "webkit", "aria-describedby", "console_errors", "page_errors", "failed_requests", "localStorage", "sessionStorage", "horizontal overflow", "workflow-test", "report-export", "mail-preview", "calendar-export", "audit-filter"):
            self.assertIn(marker, source)

    def test_artifact_directory_is_private(self):
        with tempfile.TemporaryDirectory(prefix="c311-fe10-test-") as parent:
            configured = Path(parent) / "artifacts"
            with patch.dict("os.environ", {ARTIFACT_DIR_ENV: str(configured)}):
                directory = artifact_directory()
            self.assertEqual(stat.S_IMODE(directory.stat().st_mode), 0o700)


if __name__ == "__main__":
    unittest.main()
