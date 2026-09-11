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
        for marker in ("C311Mode", "C311MockRole", "C311MockScenario", "C311MockSession", "BROWSERS", "chromium", "firefox", "webkit", "aria-describedby", "data-c311-error-summary", "data-c311-submission-result", "data-c311-status-result", "data-c311-request-detail", "console_errors", "page_errors", "failed_requests", "write_requests", "mocked_locale_requests", "cancelled_locale_requests", "renderer_retries", "is_renderer_crash", "localStorage", "sessionStorage", "horizontal overflow", "workflow-test", "report-export", "mail-preview", "calendar-export", "audit-filter", "assert_language_fallback", "assert_help_drawer_focus", "assert_modal_focus", "data-c311-status-announcer", "data-c311-operation-status", "full_page=True", "service_page", "workflow_page", "/c311/staff/reports", "/c311/staff/mail", "/c311/staff/calendar", "/c311/staff/audit", "/c311/staff/workflows"):
            self.assertIn(marker, source)

    def test_extension_pages_run_at_every_matrix_viewport(self):
        source = Path(__file__).with_name("fe10_matrix.py").read_text(encoding="utf-8")
        self.assertNotIn("if width in (767, 1440)", source)
        self.assertIn("extension_smoke(page, base_url, label, results)", source)

    def test_mail_controls_have_a_narrow_viewport_constraint(self):
        source = Path(__file__).parents[2].joinpath("client/web/admin/src/views/C311/Extensions.vue").read_text(encoding="utf-8")
        self.assertIn('class="c311-mail"', source)
        self.assertIn('.c311-mail .form-control {', source)
        self.assertIn('box-sizing: border-box;', source)
        self.assertIn('width: 100%;', source)
        self.assertIn('.c311-mail label {', source)

    def test_calendar_surface_has_a_narrow_viewport_constraint(self):
        source = Path(__file__).parents[2].joinpath("client/web/admin/src/views/C311/Extensions.vue").read_text(encoding="utf-8")
        self.assertIn('class="c311-calendar"', source)
        self.assertIn('.c311-calendar-controls {', source)
        self.assertIn("input[type='file']", source)
        self.assertIn('overflow-wrap: anywhere;', source)

    def test_extension_forms_use_a_bounded_responsive_row(self):
        source = Path(__file__).parents[2].joinpath("client/web/admin/src/views/C311/Extensions.vue").read_text(encoding="utf-8")
        self.assertIn('class="c311-extension-row', source)
        self.assertIn('.c311-extension-row {', source)
        self.assertIn('display: flex;', source)
        self.assertIn('flex-wrap: wrap;', source)
        self.assertNotIn('class="form-row', source)
        self.assertIn('margin-left: 0;', source)
        self.assertIn('margin-right: 0;', source)
        self.assertIn('overflow-x: hidden;', source)

    def test_sonar_excludes_browser_harness_and_static_bootstrap_duplicates(self):
        properties = Path(__file__).parents[2].joinpath("sonar-project.properties").read_text(encoding="utf-8")
        self.assertIn("tools/c311-browser/fe10_matrix.py", properties)
        self.assertIn(
            "sonar.cpd.exclusions=**/*.gen.go,client/web/admin/public/index.html,client/web/compose/public/index.html",
            properties,
        )

    def test_runner_uses_isolated_ports_and_verifies_server_ownership(self):
        source = Path(__file__).with_name("run-fe10.sh").read_text(encoding="utf-8")
        self.assertIn('admin_port="${C311_ADMIN_PORT:-18120}"', source)
        self.assertIn('compose_port="${C311_COMPOSE_PORT:-18121}"', source)
        self.assertIn('PORT="$admin_port"', source)
        self.assertIn('PORT="$compose_port"', source)
        self.assertIn('wait_for_server "$admin_pid" "$admin_port" "$admin_log"', source)
        self.assertIn('wait_for_server "$compose_pid" "$compose_port" "$compose_log"', source)
        self.assertIn('grep -F "http://127.0.0.1:$port/"', source)

    def test_artifact_directory_is_private(self):
        with tempfile.TemporaryDirectory(prefix="c311-fe10-test-") as parent:
            configured = Path(parent) / "artifacts"
            with patch.dict("os.environ", {ARTIFACT_DIR_ENV: str(configured)}):
                directory = artifact_directory()
            self.assertEqual(stat.S_IMODE(directory.stat().st_mode), 0o700)

    def test_runner_cleanup_uses_process_tree_helper(self):
        source = Path(__file__).with_name("run-fe10.sh").read_text(encoding="utf-8")
        helper = Path(__file__).with_name("process-tree.sh").read_text(encoding="utf-8")
        self.assertIn('source "$repo_root/tools/c311-browser/process-tree.sh"', source)
        self.assertIn("terminate_process_tree", source)
        self.assertIn("pgrep -P", helper)


if __name__ == "__main__":
    unittest.main()
