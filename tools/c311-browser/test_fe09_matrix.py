import stat
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch
from fe09_matrix import ARTIFACT_DIR_ENV, FE09_VIEWPORTS, artifact_directory

class Fe09MatrixTests(unittest.TestCase):
    def test_roles_viewports_and_mock_boundary(self):
        source = Path(__file__).with_name("fe09_matrix.py").read_text(encoding="utf-8")
        self.assertEqual(FE09_VIEWPORTS, ((1440, 900), (390, 844)))
        for role in ("workflow_designer", "department_manager", "platform_administrator"):
            self.assertIn(role, source)
        for marker in ("C311Mode", "C311MockRole", "C311MockScenario", "C311MockSession", '"mode": "mock-only"', "getWriteCount", "TERMINAL_FAILURE", "calendar_import", "DTSTART;TZID", "PENDING-to-DELIVERED", "workflow-action-execute", "INVALID_CLIENT", "INVALID_TOKEN", "smtp-421", "smtp-451", "smtp-550", "smtp-553", "flow_mail_smtp_retry", "flow_mail_smtp_terminal", "contact-email-export", "mail-template-save", "Too many columns", "invalid export page token", "rate-limited", "flow_workflow_terminal", "flow_reports_terminal", "flow_calendar_invalid", "flow_calendar_retryable", "flow_audit_terminal", "flow_contact_terminal", "flow_export_terminal"):
            self.assertIn(marker, source)

    def test_artifact_directory_is_private(self):
        with tempfile.TemporaryDirectory(prefix="c311-fe09-test-") as parent:
            configured = Path(parent) / "artifacts"
            with patch.dict("os.environ", {ARTIFACT_DIR_ENV: str(configured)}):
                directory = artifact_directory()
            self.assertEqual(stat.S_IMODE(directory.stat().st_mode), 0o700)

if __name__ == "__main__":
    unittest.main()
