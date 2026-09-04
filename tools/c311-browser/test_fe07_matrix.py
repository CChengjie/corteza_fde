from __future__ import annotations

import stat
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from fe03_matrix import ARTIFACT_DIR_ENV
from fe07_matrix import FE07_VIEWPORTS, artifact_directory


class Fe07MatrixTests(unittest.TestCase):
    def test_matrix_covers_roles_viewports_and_stable_operations(self) -> None:
        source = Path(__file__).with_name("fe07_matrix.py").read_text(encoding="utf-8")
        self.assertEqual(FE07_VIEWPORTS, ((360, 900), (767, 900), (768, 900), (1023, 900), (1024, 900), (1440, 900), (1920, 900)))
        for role in ("service_agent", "supervisor", "department_manager", "platform_administrator", "workflow_designer"):
            self.assertIn(f'"{role}"', source)
        for selector in ("[data-c311-request-detail]", "[data-c311-staff-request-detail]", "[data-c311-relationships]", "[data-c311-collaborators]", "[data-c311-reminders]", "[data-c311-attachments]", "[data-c311-duplicate-group]", "[data-c311-action=\"override-scope\"]", "[data-c311-action=\"confirm-duplicate\"]", "[data-c311-action=\"remove-duplicate\"]", "[data-c311-action=\"bulk-open-confirm\"]", "[data-c311-action=\"bulk-cancel\"]"):
            self.assertIn(selector, source)
        for scenario in ("not-found", "retryable", "scope-denied", "expired"):
            self.assertIn(scenario, source)
        self.assertIn('"mode": "mock-only"', source)
        self.assertIn("unexpected_responses", source)
        self.assertIn("network writes", source)

    def test_artifact_directory_is_private(self) -> None:
        with tempfile.TemporaryDirectory(prefix="c311-fe07-test-") as parent:
            configured = Path(parent) / "artifacts"
            with patch.dict("os.environ", {ARTIFACT_DIR_ENV: str(configured)}):
                directory = artifact_directory()
            self.assertEqual(stat.S_IMODE(directory.stat().st_mode), 0o700)


if __name__ == "__main__":
    unittest.main()
