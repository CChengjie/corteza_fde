import stat
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch
from fe08_matrix import ARTIFACT_DIR_ENV, VIEWPORTS, artifact_directory

class Fe08MatrixTests(unittest.TestCase):
    def test_matrix_is_mock_only_and_covers_required_surfaces(self):
        source = Path(__file__).with_name("fe08_matrix.py").read_text(encoding="utf-8")
        self.assertEqual(VIEWPORTS, ((1440, 900), (390, 844)))
        self.assertIn("window.C311Mode='mock'", source)
        for marker in ('data-c311-tab', 'data-c311-field="organisation_name"', 'data-c311-history="branding"', 'data-c311-history="content"', 'data-c311-history="help"', 'data-c311-conflict', 'data-c311-action="reload-current-version"', 'data-c311-action="reapply-changes"', 'data-c311-field="help_body"', 'data-c311-action="preview-help"', 'data-c311-action="publish-help"', 'data-c311-action="rollback-help-1"', 'published help did not update the public projection', 'data-c311-category="RESIDENT"', 'in-use category deactivation was not rejected', 'EXPECTED_VERSION_REQUIRED', 'VERSION_CONFLICT', 'data-c311-field-key="contact_preference"', 'data-c311-field="field_default"', 'unsafe preview was rendered'):
            self.assertIn(marker, source)

    def test_artifact_directory_is_private(self):
        with tempfile.TemporaryDirectory(prefix="c311-fe08-test-") as parent:
            configured = Path(parent) / "artifacts"
            with patch.dict("os.environ", {ARTIFACT_DIR_ENV: str(configured)}): directory = artifact_directory()
            self.assertEqual(stat.S_IMODE(directory.stat().st_mode), 0o700)

if __name__ == "__main__": unittest.main()
