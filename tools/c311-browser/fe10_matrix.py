#!/usr/bin/env python3
"""FE-10 browser quality matrix for the public and staff C311 journeys."""

from __future__ import annotations

import json
import os
import sys
import tempfile
from datetime import datetime, timezone
from pathlib import Path
from urllib.parse import urlparse

from playwright.sync_api import Page, sync_playwright


VIEWPORTS = (
    (360, 844),
    (767, 900),
    (768, 900),
    (1023, 900),
    (1024, 900),
    (1440, 900),
    (1920, 900),
)
JOURNEYS = ("login_identity", "public_submit", "anonymous_lookup", "staff_update")
# Chromium covers Chrome and Edge-compatible Chromium in CI; WebKit is the
# deterministic Safari substitute when a native Safari runner is unavailable.
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


def assert_accessibility_markers(page: Page, label: str) -> None:
    check(page.locator(".c311-skip-link").count() > 0, f"{label}: skip navigation link missing")
    check(page.locator("[data-c311-status-announcer][aria-live]").count() > 0, f"{label}: status announcer missing")
    check(page.locator("[data-c311-main]").get_attribute("tabindex") == "-1", f"{label}: main is not focusable")
    for control in page.locator("[data-c311-main] input, [data-c311-main] select, [data-c311-main] textarea").all():
        control_id = control.get_attribute("id")
        if control_id:
            check(page.locator(f"label[for='{control_id}']").count() > 0 or control.get_attribute("aria-label"), f"{label}: unlabeled control {control_id}")
    for control in page.locator("[data-c311-main] button, [data-c311-main] a[href]").all():
        if not control.is_visible():
            continue
        name = " ".join((control.inner_text() or "").split())
        check(
            bool(name or control.get_attribute("aria-label") or control.get_attribute("aria-labelledby")),
            f"{label}: interactive control has no accessible name ({(control.evaluate('(element) => element.outerHTML') or '')[:160]})",
        )


def assert_focus(page: Page, label: str) -> None:
    page.wait_for_timeout(100)
    focused = page.evaluate("document.activeElement && document.activeElement.outerHTML") or ""
    check("<h1" in focused or "data-c311-main" in focused, f"{label}: route focus lost ({focused[:160]})")


def wait_for_any(page: Page, selectors: tuple[str, ...], label: str, timeout_ms: int = 4000) -> str:
    elapsed = 0
    while elapsed < timeout_ms:
        for selector in selectors:
            if visible(page, selector):
                return selector
        page.wait_for_timeout(100)
        elapsed += 100
    raise AssertionError(f"{label}: none of {selectors} became visible")


def assert_language_fallback(page: Page, label: str) -> None:
    selector = page.locator("[data-c311-language]").first
    check(selector.count() == 1 and selector.is_visible(), f"{label}: language selector missing")
    for locale in ("es", "vi", "en"):
        selector.select_option(locale)
        page.wait_for_timeout(150)
        check(selector.input_value() == locale, f"{label}: language did not switch to {locale}")
        visible_text = page.locator("body").inner_text()
        check("c311:" not in visible_text, f"{label}: untranslated i18n key exposed for {locale}")
    selector.select_option("vi")
    page.wait_for_timeout(150)
    route = page.url
    page.reload(wait_until="domcontentloaded")
    page.locator("[data-c311-main]").first.wait_for(state="visible")
    check(page.url == route, f"{label}: language refresh changed route")
    check(page.locator("[data-c311-language]").first.input_value() == "vi", f"{label}: language was not restored after refresh")
    check("c311:" not in page.locator("body").inner_text(), f"{label}: fallback exposed an i18n key after refresh")
    page.locator("[data-c311-language]").first.select_option("en")
    page.wait_for_timeout(100)


def assert_help_drawer_focus(page: Page, label: str) -> None:
    trigger = page.locator("button[aria-controls^='c311-help-']").first
    check(trigger.count() == 1 and trigger.is_visible(), f"{label}: help trigger missing")
    trigger.focus()
    page.keyboard.press("Enter")
    drawer = page.locator("[data-c311-help-drawer]").first
    drawer.wait_for(state="visible")
    check(drawer.get_attribute("role") == "dialog", f"{label}: help drawer is not a dialog")
    check(drawer.get_attribute("aria-modal") == "true", f"{label}: help drawer is not modal")
    check(page.evaluate("document.activeElement && document.activeElement.closest('[data-c311-help-drawer]') !== null"), f"{label}: help drawer did not receive focus")
    page.keyboard.press("Escape")
    drawer.wait_for(state="hidden")
    page.wait_for_timeout(100)
    check(page.locator("button[aria-controls^='c311-help-']:focus").count() == 1, f"{label}: help focus did not return to trigger")


def assert_modal_focus(page: Page, base_url: str, label: str) -> None:
    open_c311(page, base_url, "/c311/test/modal", "public_visitor")
    assert_layout(page, f"{label}:modal")
    assert_accessibility_markers(page, f"{label}:modal")
    opener = page.locator("[data-c311-modal-opener]").first
    check(opener.count() == 1 and opener.is_visible(), f"{label}: modal opener missing")
    opener.focus()
    page.keyboard.press("Enter")
    modal = page.locator("[data-c311-focus-modal]").first
    modal.wait_for(state="visible")
    check(modal.get_attribute("role") == "dialog", f"{label}: modal role missing")
    check(modal.get_attribute("aria-modal") == "true", f"{label}: modal aria-modal missing")
    check(page.evaluate("document.activeElement && document.activeElement.closest('[data-c311-focus-modal]') !== null"), f"{label}: modal did not receive focus")
    page.keyboard.press("Escape")
    modal.wait_for(state="hidden")
    page.wait_for_timeout(100)
    active = page.evaluate("document.activeElement && document.activeElement.outerHTML") or ""
    check(page.locator("[data-c311-modal-opener]:focus").count() == 1, f"{label}: modal focus did not return to opener ({active[:160]})")


def visible(page: Page, selector: str) -> bool:
    return page.locator(selector).count() > 0 and page.locator(selector).first.is_visible()


def install_mock_network_boundary(page: Page, results: dict, label: str) -> None:
    """Keep the browser gate self-contained while allowing locale fallback loads."""
    def handle(route) -> None:
        parsed = urlparse(route.request.url)
        if parsed.hostname == "api.cortezaproject.your-domain.tld" and parsed.path.startswith("/system/locale/"):
            results.setdefault("mocked_locale_requests", []).append({"label": label, "url": route.request.url})
            route.fulfill(status=200, content_type="application/json", body="{}")
            return
        if parsed.hostname in {"127.0.0.1", "localhost"}:
            route.continue_()
            return
        route.abort()

    page.route("**/*", handle)


def observe_page(page: Page, results: dict, label: str) -> None:
    install_mock_network_boundary(page, results, label)
    page.on("console", lambda message, item=label: results["console_errors"].append({"label": item, "text": message.text}) if message.type == "error" else None)
    page.on("pageerror", lambda error, item=label: results["page_errors"].append({"label": item, "text": str(error)}))

    def record_failed_request(request, item=label):
        entry = {"label": item, "url": request.url, "failure": request.failure}
        if is_mock_locale_url(request.url):
            results["cancelled_locale_requests"].append(entry)
        else:
            results["failed_requests"].append(entry)

    page.on("requestfailed", record_failed_request)
    page.on("request", lambda request, item=label: results["write_requests"].append({"label": item, "method": request.method, "url": request.url}) if request.method in {"POST", "PUT", "PATCH", "DELETE"} else None)


def is_mock_locale_url(url: str) -> bool:
    parsed = urlparse(url)
    return parsed.hostname == "api.cortezaproject.your-domain.tld" and parsed.path.startswith("/system/locale/")


def login_identity(page: Page, base_url: str, label: str) -> None:
    open_c311(page, base_url, "/c311/sign-in", "public_visitor")
    assert_accessibility_markers(page, label)
    assert_help_drawer_focus(page, f"{label}:login")
    page.locator('[data-c311-action="sign-in"]').click()
    page.locator("[data-c311-error-summary]").wait_for(state="visible")
    check(page.evaluate("document.activeElement && document.activeElement.id === 'c311-error-summary'"), f"{label}: sign-in error summary is not focused")
    check(page.locator("#c311-login-password").get_attribute("aria-required") == "true", f"{label}: password required state missing")
    page.locator('a[href="/c311/forgot-password"]').click()
    page.locator("[data-c311-page='forgot-password']").wait_for(state="visible")
    page.locator("#c311-forgot-email").fill("alex@example.test")
    page.locator('[data-c311-action="forgot-password"]').click()
    page.locator("[data-c311-page='forgot-password'] [role='status']").wait_for(state="visible")
    page.locator('a[href="/c311/sign-in"]').click()
    page.locator("[data-c311-page='sign-in']").wait_for(state="visible")
    page.locator("#c311-login-identifier").fill("fixture@example.test")
    page.locator("#c311-login-password").fill("fixture-password")
    check(page.locator("#c311-login-password").get_attribute("aria-describedby") == "c311-error-summary", f"{label}: password error link missing")
    page.locator('[data-c311-action="sign-in"]').click()
    page.locator("[data-c311-route='/c311']").wait_for(state="visible")
    check(page.locator("[data-c311-route='/c311/requests']").count() == 1, f"{label}: authenticated navigation was not restored")
    check(page.evaluate("Object.keys(localStorage).every(k => !/password|token|secret|credential/i.test(k))"), f"{label}: secret-shaped local storage key")
    check(page.evaluate("Object.keys(sessionStorage).every(k => !/password|token|secret|credential/i.test(k))"), f"{label}: secret-shaped session storage key")


def public_submit(page: Page, base_url: str, label: str) -> None:
    open_c311(page, base_url, "/c311/submit", "public_visitor")
    assert_accessibility_markers(page, label)
    assert_language_fallback(page, f"{label}:submit")
    assert_help_drawer_focus(page, f"{label}:submit")
    page.locator("#c311-summary").fill("Fixture request")
    page.locator("#c311-description").fill("A non-sensitive request used by the FE-10 browser gate.")
    page.locator("#c311-requester-name").fill("Alex Example")
    page.locator("#c311-requester-email").fill("alex@example.test")
    page.locator("#c311-consent").check()
    page.once("dialog", lambda dialog: dialog.dismiss())
    page.locator('[data-c311-route="/c311/status"]').click()
    page.locator('[data-c311-route="/c311/submit"]').click()
    check(page.locator("#c311-summary").input_value() == "Fixture request", f"{label}: cancelled navigation lost draft")
    page.once("dialog", lambda dialog: dialog.accept())
    page.reload(wait_until="commit", timeout=10000)
    page.locator("#c311-summary").wait_for(state="visible")
    check(page.locator("#c311-summary").input_value() == "Fixture request", f"{label}: draft summary was not restored")
    check(page.locator("#c311-description").input_value().startswith("A non-sensitive"), f"{label}: draft description was not restored")
    page.locator('[data-c311-action="submit-request"]').click()
    page.locator("[data-c311-submission-result]").wait_for(state="visible")
    check(page.locator("[data-c311-submission-result]").get_by_text("SR-2026-00002").count() == 1, f"{label}: submitted request number missing")
    check(page.locator("[data-c311-status-announcer]").get_attribute("aria-live") in ("polite", "assertive"), f"{label}: submission status was not announced")


def anonymous_lookup(page: Page, base_url: str, label: str) -> None:
    open_c311(page, base_url, "/c311/status", "public_visitor")
    assert_accessibility_markers(page, label)
    page.locator("#c311-status-request-number").fill("bad")
    page.locator("#c311-status-email").fill("alex@example.test")
    page.locator('[data-c311-action="lookup-status"]').click()
    page.locator("[data-c311-error-summary]").wait_for(state="visible")
    check(page.evaluate("document.activeElement && document.activeElement.id === 'c311-error-summary'"), f"{label}: lookup error summary is not focused")
    page.locator("#c311-status-request-number").fill("SR-2026-00001")
    page.locator("#c311-status-email").fill("alex@example.test")
    page.locator('[data-c311-action="lookup-status"]').click()
    page.locator("[data-c311-status-result]").wait_for(state="visible")
    check(page.locator("[data-c311-status-result] [data-c311-status-value]").inner_text() == "SUBMITTED", f"{label}: anonymous status result mismatch")
    page.locator('[data-c311-route="/c311/sign-in"], a[href="/c311/sign-in"]').first.click()
    page.locator("[data-c311-page='sign-in']").wait_for(state="visible")
    page.locator("#c311-login-identifier").fill("fixture@example.test")
    page.locator("#c311-login-password").fill("fixture-password")
    page.locator('[data-c311-action="sign-in"]').click()
    page.locator('[data-c311-route="/c311/requests"]').wait_for(state="visible")
    page.locator('[data-c311-route="/c311/requests"]').click()
    page.locator('[data-c311-page="requests"]').wait_for(state="visible")
    page.locator('[data-c311-action="view-request-request-fixture-001"]:visible').first.click()
    page.locator("[data-c311-request-detail]").wait_for(state="visible")
    check(page.locator("[data-c311-request-detail]").get_by_text("SR-2026-00001").count() > 0, f"{label}: my request detail missing")


def staff_update(page: Page, base_url: str, label: str) -> None:
    open_c311(page, base_url, "/c311/staff", "service_agent")
    assert_accessibility_markers(page, label)
    assert_focus(page, label)
    assert_language_fallback(page, f"{label}:staff")
    assert_help_drawer_focus(page, f"{label}:staff")
    page.locator('[data-c311-action="view-request-request-fixture-001"]:visible').first.click()
    page.locator("[data-c311-request-detail]").wait_for(state="visible")
    page.locator('[data-c311-action="transition-request"]').click()
    page.locator("#c311-triage-reason").fill("Fixture triage review")
    page.locator('[data-c311-action="confirm-triage-details"]').check()
    page.locator('[data-c311-action="submit-triage"]').click()
    page.locator('[data-c311-status-value]').wait_for(state="visible")
    check(page.locator('[data-c311-status-value]').inner_text() == "TRIAGED", f"{label}: staff update did not transition request")
    check(page.locator("[data-c311-status-announcer]").count() > 0, f"{label}: staff update was not announced")


def extension_page_checks(page: Page, base_url: str, path: str, role: str, label: str) -> None:
    open_c311(page, base_url, path, role)
    page_label = f"{label}:{path}"
    assert_layout(page, page_label)
    assert_accessibility_markers(page, page_label)
    assert_focus(page, page_label)
    check(page.locator("h1").first.is_visible(), f"{page_label}: page heading missing")
    actions = {"workflows": "workflow-test", "reports": "report-export", "mail": "mail-preview", "calendar": "calendar-export", "audit": "audit-filter"}
    action = next((value for key, value in actions.items() if path.endswith(key)), None)
    if action and visible(page, f'[data-c311-action="{action}"]'):
        if action == "mail-preview":
            page.locator("#c311-mail-to").fill("alex@example.test")
            page.locator("#c311-mail-subject").fill("FE-10 browser check")
            page.locator("#c311-mail-text").fill("A non-sensitive preview used by the browser gate.")
            page.locator("#c311-mail-html").fill("<p>FE-10 browser check</p>")
        page.locator(f'[data-c311-action="{action}"]').first.click()
        if action in {"workflow-test", "report-export", "calendar-export"}:
            wait_for_any(page, ("[data-c311-operation-status]", "[data-c311-operation-result]", "[data-c311-calendar-content]"), page_label)
            status = page.locator("[data-c311-operation-status]").first
            if status.count() and status.is_visible():
                check(bool(status.inner_text().strip()), f"{page_label}: async operation status is empty")
        elif action == "mail-preview":
            wait_for_any(page, ("[data-c311-mail-preview]", "[data-c311-mail-preview-error]"), page_label)
            check(page.locator("[data-c311-mail-preview], [data-c311-mail-preview-error]").count() > 0, f"{page_label}: mail preview result missing")
        else:
            page.wait_for_timeout(150)
            check(page.locator("[data-c311-audit-filters]").count() == 1, f"{page_label}: audit filter state missing after apply")
    announcer = page.locator("[data-c311-status-announcer][aria-live]").first
    check(announcer.count() == 1, f"{page_label}: async status announcer missing")
    if ACTIVE_ARTIFACT_DIR is not None:
        safe_path = path.strip("/").replace("/", "-")
        screenshot = ACTIVE_ARTIFACT_DIR / f"{label.replace(':', '-')}-{safe_path}.png"
        page.screenshot(path=str(screenshot), full_page=True)


def extension_smoke(page: Page, base_url: str, label: str, results: dict) -> None:
    service_page = page.context.new_page()
    service_label = f"{label}:service"
    observe_page(service_page, results, service_label)
    try:
        for path in ("/c311/staff/reports", "/c311/staff/mail", "/c311/staff/calendar", "/c311/staff/audit"):
            extension_page_checks(service_page, base_url, path, "department_manager", service_label)
    finally:
        service_page.close()

    workflow_page = page.context.new_page()
    workflow_label = f"{label}:workflow"
    observe_page(workflow_page, results, workflow_label)
    try:
        extension_page_checks(workflow_page, base_url, "/c311/staff/workflows", "workflow_designer", workflow_label)
    finally:
        workflow_page.close()


def run() -> dict:
    global ACTIVE_ARTIFACT_DIR
    artifact_dir = artifact_directory()
    ACTIVE_ARTIFACT_DIR = artifact_dir
    requested_browser = os.environ.get("C311_BROWSER", "all").lower()
    browser_names = BROWSERS if requested_browser == "all" else (requested_browser,)
    unknown_browsers = set(browser_names) - set(BROWSERS)
    if unknown_browsers:
        raise ValueError(f"Unsupported C311_BROWSER value: {', '.join(sorted(unknown_browsers))}")
    results = {"viewports": [list(value) for value in VIEWPORTS], "browsers": list(browser_names), "journeys": list(JOURNEYS), "checks": [], "console_errors": [], "page_errors": [], "failed_requests": [], "write_requests": [], "mocked_locale_requests": [], "cancelled_locale_requests": [], "started_at": datetime.now(timezone.utc).isoformat()}
    with sync_playwright() as playwright:
        for browser_name in browser_names:
            browser_type = getattr(playwright, browser_name)
            browser = browser_type.launch(headless=True)
            try:
                for width, height in VIEWPORTS:
                    for app, base_url, path, role in (("compose", COMPOSE_URL, "/c311", "public_visitor"), ("admin", ADMIN_URL, "/c311/staff", "service_agent")):
                        context = browser.new_context(viewport={"width": width, "height": height})
                        page = context.new_page()
                        label = f"{browser_name}:{app}@{width}x{height}"
                        observe_page(page, results, label)
                        try:
                            open_c311(page, base_url, path, role)
                            assert_layout(page, label)
                            assert_focus(page, label)
                            assert_modal_focus(page, base_url, label)
                            if app == "compose":
                                login_identity(page, base_url, label)
                                public_submit(page, base_url, label)
                                anonymous_lookup(page, base_url, label)
                            else:
                                staff_update(page, base_url, label)
                                extension_smoke(page, base_url, label, results)
                            screenshot = artifact_dir / f"{browser_name}-{app}-{width}x{height}.png"
                            page.screenshot(path=str(screenshot), full_page=True)
                            results["checks"].append({"label": label, "status": "passed", "screenshot": str(screenshot)})
                        except Exception:
                            failure_screenshot = artifact_dir / f"{browser_name}-{app}-{width}x{height}-failure.png"
                            try:
                                page.screenshot(path=str(failure_screenshot), full_page=True)
                            except Exception:
                                pass
                            raise
                        finally:
                            context.close()
            finally:
                browser.close()
    check(not results["page_errors"], f"page errors detected: {results['page_errors']}")
    check(not results["console_errors"], f"console errors detected: {results['console_errors']}")
    check(not results["failed_requests"], f"failed requests detected: {results['failed_requests']}")
    check(not results["write_requests"], f"unexpected business write requests detected: {results['write_requests']}")
    results["finished_at"] = datetime.now(timezone.utc).isoformat()
    (artifact_dir / "fe10-matrix.json").write_text(json.dumps(results, indent=2), encoding="utf-8")
    return results


if __name__ == "__main__":
    try:
        print(json.dumps(run(), indent=2))
    except Exception as error:
        failure_dir = ACTIVE_ARTIFACT_DIR or artifact_directory()
        (failure_dir / "fe10-matrix-failure.json").write_text(json.dumps({"status": "failed", "browsers": list(BROWSERS) if os.environ.get("C311_BROWSER", "all") == "all" else [os.environ.get("C311_BROWSER")], "viewports": [list(value) for value in VIEWPORTS], "error": str(error)}, indent=2), encoding="utf-8")
        print(str(error), file=sys.stderr)
        raise
