#!/usr/bin/env python3
"""Regression checks for assets served by the E2E development servers."""

import unittest
from pathlib import Path


REPOSITORY_ROOT = Path(__file__).parents[2]


class E2eAssetTests(unittest.TestCase):
    def test_client_config_is_not_a_vue_template_path(self) -> None:
        for client in ("admin", "compose"):
            index = (REPOSITORY_ROOT / "client" / "web" / client / "public" / "index.html").read_text(encoding="utf-8")
            self.assertIn('href="config.js"', index)
            self.assertIn('src="config.js"', index)
            self.assertNotIn('BASE_URL %>config.js', index)

    def test_e2e_uses_the_loaded_config_and_proxies_authentication(self) -> None:
        workflow = (REPOSITORY_ROOT / ".github" / "workflows" / "test-e2e.yml").read_text(encoding="utf-8")
        builder = (REPOSITORY_ROOT / ".github" / "workflows" / "assets" / "client" / "vue.config-builder.js").read_text(encoding="utf-8")
        self.assertIn('client/web/${CLIENT_NAME}/vue.config-builder.js', workflow)
        self.assertNotIn('client/web/${CLIENT_NAME}/public/vue.config-builder.js', workflow)
        self.assertIn("'^/api'", builder)
        self.assertIn("'^/auth'", builder)

    def test_e2e_login_waits_for_callback_session(self) -> None:
        workflow = (REPOSITORY_ROOT / ".github" / "workflows" / "test-e2e.yml").read_text(encoding="utf-8")
        self.assertIn("- 'codex/fix-e2e-login-ci'", workflow)
        self.assertIn("Patch E2E same-origin auth bootstrap", workflow)
        self.assertIn(".nav-sidebar", workflow)


if __name__ == "__main__":
    unittest.main()
