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


if __name__ == "__main__":
    unittest.main()
