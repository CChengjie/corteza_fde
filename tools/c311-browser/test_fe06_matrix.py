from __future__ import annotations

import stat
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from fe03_matrix import ARTIFACT_DIR_ENV
from fe06_matrix import FE06_VIEWPORTS, artifact_directory


class Fe06MatrixTests(unittest.TestCase):
    def test_matrix_uses_mock_roles_and_stable_employee_selectors(self) -> None:
        source = Path(__file__).with_name("fe06_matrix.py").read_text(encoding="utf-8")
        self.assertEqual(FE06_VIEWPORTS, ((360, 900), (767, 900), (768, 900), (1023, 900), (1024, 900), (1440, 900), (1920, 900)))
        for role in ("service_agent", "supervisor", "department_manager", "platform_administrator", "workflow_designer"):
            self.assertIn(f'"{role}"', source)
        for selector in ("[data-c311-staff-filters]", "[data-c311-request-detail]", "[data-c311-action^=\"view-request-\"]", "[data-c311-action=\"reassign-request\"]", "[data-c311-action=\"transition-request\"]", "[data-c311-relationships]", "[data-c311-attachments]", "[data-c311-action=\"manage-relationships\"]"):
            self.assertIn(selector, source)
        self.assertIn('"mode": "mock-only"', source)
        self.assertIn('scenario="forbidden"', source)
        self.assertIn('scenario="scope-denied"', source)
        self.assertIn('scenario="scope-filter"', source)
        self.assertIn('request-fixture-foreign', source)
        self.assertIn('scenario="pagination"', source)
        self.assertIn('session="expired"', source)
        self.assertIn('page.reload', source)
        self.assertIn('GENERAL_SERVICES', source)
        self.assertIn('actor-fixture-agent', source)
        self.assertIn('retryable-error', source)
        self.assertIn('terminal-error', source)
        self.assertIn('not-found', source)
        self.assertIn("unexpected_responses", source)
        self.assertIn("unexpected network writes", source)

    def test_artifact_directory_is_private(self) -> None:
        with tempfile.TemporaryDirectory(prefix="c311-fe06-test-") as parent:
            configured = Path(parent) / "artifacts"
            with patch.dict("os.environ", {ARTIFACT_DIR_ENV: str(configured)}):
                directory = artifact_directory()
            self.assertEqual(stat.S_IMODE(directory.stat().st_mode), 0o700)


if __name__ == "__main__":
    unittest.main()
