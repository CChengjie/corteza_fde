from __future__ import annotations

import stat
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from fe04_matrix import VIEWPORTS, artifact_directory
from fe03_matrix import ARTIFACT_DIR_ENV


class Fe04MatrixTests(unittest.TestCase):
    def test_matrix_covers_desktop_and_mobile_and_mock_only_selectors(self) -> None:
        source = Path(__file__).with_name("fe04_matrix.py").read_text(encoding="utf-8")
        self.assertEqual(VIEWPORTS, ((1440, 900), (768, 900), (390, 844)))
        self.assertIn("C311Mode = 'mock'", Path(__file__).with_name("fe03_matrix.py").read_text(encoding="utf-8"))
        for selector in ("#c311-attachment-file", '[data-c311-action="geocode-address"]', '[data-c311-action="confirm-location"]', '[data-c311-map-error]', '[data-c311-attachment-error]', '[data-c311-action="remove-attachment"]', '[data-c311-action="download-attachment"]', '[data-c311-action="preview-attachment"]'):
            self.assertIn(selector, source)
        self.assertIn("replacement.txt", source)
        self.assertIn("100   example street", source)
        self.assertIn("check_download_capability", source)
        self.assertIn("check_route_attachment_lifecycle", source)
        self.assertIn("scenario=\"not-found\"", source)
        self.assertIn("staged tokens leaked across request routes", source)
        self.assertIn("sensitive_names", source)
        self.assertIn('"mode": "mock-only"', source)

    def test_artifact_directory_is_private(self) -> None:
        with tempfile.TemporaryDirectory(prefix="c311-fe04-test-") as parent:
            configured = Path(parent) / "artifacts"
            with patch.dict("os.environ", {ARTIFACT_DIR_ENV: str(configured)}):
                directory = artifact_directory()
            self.assertEqual(stat.S_IMODE(directory.stat().st_mode), 0o700)


if __name__ == "__main__":
    unittest.main()
