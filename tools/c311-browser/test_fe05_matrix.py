from __future__ import annotations

import os
import stat
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from fe05_matrix import ARTIFACT_DIR_ENV, VIEWPORTS, artifact_directory, allowed_failure


class Fe05MatrixTests(unittest.TestCase):
    def test_mock_matrix_uses_required_viewports_and_stable_selectors(self) -> None:
        source = Path(__file__).with_name("fe05_matrix.py").read_text(encoding="utf-8")
        self.assertEqual(VIEWPORTS, ((1440, 900), (390, 844)))
        for selector in ("#c311-status-request-number", "#c311-status-email", '[data-c311-action="lookup-status"]', '[data-c311-action^="view-request-"]', '[data-c311-route="/c311/account"]', '[data-c311-language]'):
            self.assertIn(selector, source)
        self.assertIn("window.C311Mode='mock'", source)
        self.assertIn('unexpected_responses', source)
        self.assertIn('unexpected_writes', source)
        self.assertIn('unexpected_console_errors', source)
        self.assertIn('sensitive_requests', source)
        self.assertIn('retryable-error', source)
        self.assertIn('terminal-error', source)
        self.assertIn('"scenarios": {"compose": {}, "admin": {}}', source)
        self.assertIn('/c311/401', source)
        self.assertIn('/c311/403', source)
        self.assertIn('Mock-only', source)
        self.assertIn('select_option("es")', source)
        self.assertIn('page.reload', source)
        self.assertIn('status request number was not restored after refresh', source)
        self.assertIn('status error summary did not focus the request number field', source)
        self.assertIn('check_account_failure', source)
        self.assertIn('invalid-credentials', source)
        self.assertIn('version-conflict', source)
        self.assertIn('expired account session', source)
        self.assertIn('data-c311-public-relationships', source)
        self.assertIn('data-c311-public-notes', source)
        self.assertIn('data-c311-public-relationship-permission', source)
        self.assertIn('public relationship notification result missing', source)
        self.assertIn('data-c311-public-notification-result', source)
        self.assertIn('data-c311-public-relationship-audit', source)
        self.assertIn('data-c311-relationship-notification-result', source)
        self.assertIn('data-c311-relationship-audit-events', source)
        self.assertIn('staff relationship audit missing', source)
        self.assertIn('data-c311-form="public-note"', source)
        self.assertIn('append-public-note', source)
        self.assertIn('account-disposition-conflict', source)
        self.assertIn('account-disposition-success', source)
        self.assertIn('account anonymization result missing', source)

    def test_configured_artifact_directory_is_private(self) -> None:
        with tempfile.TemporaryDirectory(prefix="c311-fe05-test-") as parent:
            configured = Path(parent) / "artifacts"
            with patch.dict(os.environ, {ARTIFACT_DIR_ENV: str(configured)}):
                directory = artifact_directory()
            self.assertEqual(stat.S_IMODE(directory.stat().st_mode), 0o700)

    def test_external_failure_allowlist_is_exact(self) -> None:
        self.assertTrue(allowed_failure("https://api.cortezaproject.your-domain.tld/system/locale/en-US/corteza-webapp-compose"))
        self.assertFalse(allowed_failure("https://api.cortezaproject.your-domain.tld/api/v1/session"))
        self.assertTrue(allowed_failure("http://127.0.0.1:18086/code-snippets.js"))
        self.assertFalse(allowed_failure("http://127.0.0.1:18086/api/v1/unknown"))

    def test_browser_harness_is_not_counted_as_production_coverage(self) -> None:
        properties = Path(__file__).parents[2] / "sonar-project.properties"
        contents = properties.read_text(encoding="utf-8")
        self.assertIn("tools/c311-browser/fe05_matrix.py", contents)


if __name__ == "__main__":
    unittest.main()
