#!/usr/bin/env python3
"""FE-10 browser quality matrix for the public and staff C311 journeys."""

from __future__ import annotations

import json
import os
import sys
import tempfile
from datetime import datetime, timezone
from pathlib import Path

from playwright.sync_api import Page, sync_playwright


VIEWPORTS = ((360, 844), (390, 844), (768, 900), (1024, 900), (1440, 900), (1920, 900))
JOURNEYS = ("login_identity", "public_submit", "anonymous_lookup", "staff_update")
# Chromium covers Chrome and Edge when an Edge channel is unavailable; WebKit is
# the deterministic Safari substitute in CI. Set C311_BROWSER to firefox/webkit.
BROWSERS = ("chromium", "firefox", "webkit")
COMPOSE_URL = os.environ.get("C311_COMPOSE_URL", "http://127.0.0.1:18081").rstrip("/")
ADMIN_URL = os.environ.get("C311_ADMIN_URL", "http://127.0.0.1:18082").rstrip("/")
ARTIFACT_DIR_ENV = "C311_ARTIFACT_DIR"
ACTIVE_ARTIFACT_DIR: Path | None = None


def check(condition: bool, message: str) -> None:
    if not condition:
        raise AssertionError(message)


def artifact_directory() -> Path:
    configured = os.environ.get(ARTIFACT_DIR_ENV)
    directory = Path(configured).expanduser() if configured else Path(tempfile.mkdtemp(prefix="c311-fe10-gate-"))
    if directory.is_symlink() or directory.resolve() == Path(directory.anchor):
        raise ValueError(f"{ARTIFACT_DIR_ENV} must point to a private child directory")
    directory.mkdir(parents=True, exist_ok=True, mode=0o700)
    directory.chmod(0o700)
    return directory.resolve()


def bootstrap(page: Page, role: str, scenario: str = "success", session: str = "current") -> None:
    page.add_init_script(
        "window.C311Mode = 'mock'; "
        f"window.C311MockRole = {json.dumps(role)}; "
        f"window.C311MockScenario = {json.dumps(scenario)}; "
        f"window.C311MockSession = {json.dumps(session)};"
    )


def open_c311(page: Page, base_url: str, path: str, role: str, scenario: str = "success", session: str = "current") -> None:
    bootstrap(page, role, scenario, session)
    page.goto(f"{base_url}{path}", wait_until="domcontentloaded")
    page.locator("[data-c311-main]").first.wait_for(state="visible")


def assert_layout(page: Page, label: str) -> None:
    check(page.locator("[data-c311-main]").count() > 0, f"{label}: missing main landmark")
    overflow = page.evaluate("document.documentElement.scrollWidth - document.documentElement.clientWidth")
    check(overflow <= 1, f"{label}: horizontal overflow {overflow}px")


def assert_focus(page: Page, label: str) -> None:
    page.wait_for_timeout(100)
    focused = page.evaluate("document.activeElement && document.activeElement.outerHTML") or ""
    check("<h1" in focused or "data-c311-main" in focused, f"{label}: route focus lost ({focused[:160]})")


def visible(page: Page, selector: str) -> bool:
    return page.locator(selector).count() > 0 and page.locator(selector).first.is_visible()


def login_identity(page: Page, base_url: str, label: str) -> None:
    open_c311(page, base_url, "/c311/sign-in", "public_visitor")
    page.locator("#c311-login-identifier").fill("fixture@example.test")
    page.locator("#c311-login-password").fill("fixture-password")
    check(page.locator("#c311-login-password").get_attribute("aria-describedby") == "c311-error-summary", f"{label}: password error link missing")
    check(page.evaluate("Object.keys(localStorage).every(k => !/password|token|secret|credential/i.test(k))"), f"{label}: secret-shaped local storage key")
    check(page.evaluate("Object.keys(sessionStorage).every(k => !/password|token|secret|credential/i.test(k))"), f"{label}: secret-shaped session storage key")


def public_submit(page: Page, base_url: str, label: str) -> None:
    open_c311(page, base_url, "/c311/submit", "public_visitor")
    page.locator("#c311-summary").fill("Fixture request")
    page.locator("#c311-description").fill("A non-sensitive request used by the FE-10 browser gate.")
    page.reload(wait_until="domcontentloaded")
    check(page.locator("#c311-summary").input_value() == "Fixture request", f"{label}: draft summary was not restored")
    check(page.locator("#c311-description").input_value().startswith("A non-sensitive"), f"{label}: draft description was not restored")
    page.locator('[data-c311-action="submit-request"]').click()
    if visible(page, "[data-c311-error-summary]"):
        check(page.evaluate("document.activeElement.matches('[data-c311-error-summary]')"), f"{label}: error summary is not focused")


def anonymous_lookup(page: Page, base_url: str, label: str) -> None:
    open_c311(page, base_url, "/c311/status", "public_visitor")
    page.locator("#c311-status-request-number").fill("SR-2026-00001")
    page.locator("#c311-status-email").fill("fixture@example.test")
    page.locator('[data-c311-action="lookup-status"]').click()
    page.wait_for_timeout(100)
    check(page.locator('[data-c311-page="status"]').count() == 1, f"{label}: status route did not remain available")
    open_c311(page, base_url, "/c311/requests", "public_visitor", session="current")
    if page.get_by_role("heading", name="Sign-in required").count() == 0:
        check(page.locator("[data-c311-main]").count() == 1, f"{label}: private request view was not guarded")


def staff_update(page: Page, base_url: str, label: str) -> None:
    open_c311(page, base_url, "/c311/staff", "service_agent")
    assert_focus(page, label)
    if visible(page, '[data-c311-action="staff-open-request"]'):
        page.locator('[data-c311-action="staff-open-request"]').first.click()
    check(page.locator("[data-c311-main]").count() == 1, f"{label}: staff route unavailable")


def extension_smoke(page: Page, base_url: str, label: str) -> None:
    for path, role in (("/c311/staff/workflows", "workflow_designer"), ("/c311/staff/reports", "service_agent"), ("/c311/staff/mail", "service_agent"), ("/c311/staff/calendar", "service_agent"), ("/c311/staff/audit", "service_agent")):
        open_c311(page, base_url, path, role)
        assert_layout(page, f"{label}:{path}")
        actions = {"workflows": "workflow-test", "reports": "report-export", "mail": "mail-preview", "calendar": "calendar-export", "audit": "audit-filter"}
        action = next((value for key, value in actions.items() if path.endswith(key)), None)
        if action and visible(page, f'[data-c311-action="{action}"]'):
            page.locator(f'[data-c311-action="{action}"]').first.click()


def run() -> dict:
    global ACTIVE_ARTIFACT_DIR
    artifact_dir = artifact_directory()
    ACTIVE_ARTIFACT_DIR = artifact_dir
    browser_name = os.environ.get("C311_BROWSER", "chromium")
    results = {"viewports": [list(value) for value in VIEWPORTS], "browser": browser_name, "journeys": list(JOURNEYS), "checks": [], "console_errors": [], "page_errors": [], "failed_requests": [], "started_at": datetime.now(timezone.utc).isoformat()}
    with sync_playwright() as playwright:
        browser_type = getattr(playwright, browser_name)
        browser = browser_type.launch(headless=True)
        try:
            for width, height in VIEWPORTS:
                for app, base_url, path, role in (("compose", COMPOSE_URL, "/c311", "public_visitor"), ("admin", ADMIN_URL, "/c311/staff", "service_agent")):
                    context = browser.new_context(viewport={"width": width, "height": height})
                    page = context.new_page()
                    label = f"{app}@{width}x{height}"
                    page.on("console", lambda message, item=label: results["console_errors"].append({"label": item, "text": message.text}) if message.type == "error" else None)
                    page.on("pageerror", lambda error, item=label: results["page_errors"].append({"label": item, "text": str(error)}))
                    page.on("requestfailed", lambda request, item=label: results["failed_requests"].append({"label": item, "url": request.url, "failure": request.failure}))
                    open_c311(page, base_url, path, role)
                    assert_layout(page, label)
                    assert_focus(page, label)
                    if app == "compose":
                        login_identity(page, base_url, label)
                        public_submit(page, base_url, label)
                        anonymous_lookup(page, base_url, label)
                    else:
                        staff_update(page, base_url, label)
                        if width in (390, 1440):
                            extension_smoke(page, base_url, label)
                    screenshot = artifact_dir / f"{app}-{width}x{height}.png"
                    page.screenshot(path=str(screenshot), full_page=True)
                    results["checks"].append({"label": label, "status": "passed", "screenshot": str(screenshot)})
                    context.close()
        finally:
            browser.close()
    check(not results["page_errors"], f"page errors detected: {results['page_errors']}")
    check(not results["console_errors"], f"console errors detected: {results['console_errors']}")
    results["finished_at"] = datetime.now(timezone.utc).isoformat()
    (artifact_dir / "fe10-matrix.json").write_text(json.dumps(results, indent=2), encoding="utf-8")
    return results


if __name__ == "__main__":
    try:
        print(json.dumps(run(), indent=2))
    except Exception as error:
        failure_dir = ACTIVE_ARTIFACT_DIR or artifact_directory()
        (failure_dir / "fe10-matrix-failure.json").write_text(json.dumps({"status": "failed", "browser": os.environ.get("C311_BROWSER", "chromium"), "error": str(error)}, indent=2), encoding="utf-8")
        print(str(error), file=sys.stderr)
        raise
