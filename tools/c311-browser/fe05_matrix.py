#!/usr/bin/env python3
"""Repeatable FE-05 public query, request list, and account checks in Mock mode."""

from __future__ import annotations

import json
import os
import tempfile
from pathlib import Path
from urllib.parse import urlparse

from playwright.sync_api import Page, sync_playwright


VIEWPORTS = ((1440, 900), (390, 844))
ARTIFACT_DIR_ENV = "C311_ARTIFACT_DIR"
COMPOSE_URL = os.environ.get("C311_COMPOSE_URL", "http://127.0.0.1:18086").rstrip("/")
ADMIN_URL = os.environ.get("C311_ADMIN_URL", "http://127.0.0.1:18087").rstrip("/")
MAIN = "[data-c311-main]"
STATUS_ROUTE = '[data-c311-route="/c311/status"]'
REQUESTS_ROUTE = '[data-c311-route="/c311/requests"]'
ACCOUNT_ROUTE = '[data-c311-route="/c311/account"]'
STATUS_LOOKUP = '[data-c311-action="lookup-status"]'
VIEW_REQUEST = '[data-c311-action^="view-request-"]'
LANGUAGE_SELECTOR = '[data-c311-language]'
ACCOUNT_DISPOSITION_ACTION = '[data-c311-action="account-disposition"]'
KNOWN_DEV_FAILURE_PATHS = {"/code-snippets.js", "/custom.css"}
KNOWN_EXTERNAL_HOST = "api.cortezaproject.your-domain.tld"


def artifact_directory() -> Path:
    configured = os.environ.get(ARTIFACT_DIR_ENV)
    directory = Path(configured).expanduser() if configured else Path(tempfile.mkdtemp(prefix="c311-fe05-gate-"))
    if directory.is_symlink():
        raise ValueError(f"{ARTIFACT_DIR_ENV} must not point to a symbolic link")
    directory.mkdir(parents=True, exist_ok=True, mode=0o700)
    directory = directory.resolve()
    if directory == Path(directory.anchor):
        raise ValueError(f"{ARTIFACT_DIR_ENV} must point to a child directory")
    directory.chmod(0o700)
    return directory


def bootstrap(page: Page, role: str, scenario: str = "success", session: str = "current") -> None:
    page.add_init_script(
        "window.C311Mode='mock';"
        f"window.C311MockRole={json.dumps(role)};"
        f"window.C311MockScenario={json.dumps(scenario)};"
        f"window.C311MockSession={json.dumps(session)};"
    )


def allowed_failure(url: str) -> bool:
    parsed = urlparse(url)
    if parsed.hostname == KNOWN_EXTERNAL_HOST and parsed.path.startswith("/system/locale/"):
        return True
    return parsed.path in KNOWN_DEV_FAILURE_PATHS


def diagnostics(page: Page) -> dict[str, list]:
    result = {"console_errors": [], "unexpected_console_errors": [], "page_errors": [], "responses": [], "unexpected_responses": [], "writes": [], "unexpected_writes": [], "sensitive_requests": []}

    def on_console(message) -> None:
        if message.type == "error":
            location = message.location
            entry = {"text": message.text, "url": location.get("url", "")}
            result["console_errors"].append(entry)
            if not allowed_console_error(message.text, entry["url"]):
                result["unexpected_console_errors"].append(entry)

    def on_response(response) -> None:
        if response.status >= 400:
            entry = {"status": response.status, "method": response.request.method, "url": response.url}
            result["responses"].append(entry)
            if not allowed_failure(response.url):
                result["unexpected_responses"].append(entry)

    def on_request(request) -> None:
        parsed = urlparse(request.url)
        query = parsed.query.lower()
        sensitive_query = ("token", "secret", "password", "authorization", "api_key", "apikey")
        if any(name in query for name in sensitive_query):
            result["sensitive_requests"].append({"method": request.method, "url": request.url, "reason": "sensitive query parameter"})
        headers = {key.lower() for key in request.headers}
        if {"authorization", "x-api-key", "x-map-api-token"} & headers:
            result["sensitive_requests"].append({"method": request.method, "url": request.url, "reason": "sensitive request header"})
        if request.method not in {"GET", "HEAD", "OPTIONS"}:
            entry = {"method": request.method, "url": request.url}
            result["writes"].append(entry)
            if "/api/v1/" not in request.url:
                result["unexpected_writes"].append(entry)

    page.on("console", on_console)
    page.on("pageerror", lambda error: result["page_errors"].append(str(error)))
    page.on("response", on_response)
    page.on("request", on_request)
    return result


def allowed_console_error(message: str, url: str) -> bool:
    """Allow only known development-only resources and the configured external locale/websocket."""
    path = urlparse(url).path
    if path in KNOWN_DEV_FAILURE_PATHS:
        return True
    if KNOWN_EXTERNAL_HOST in message or (urlparse(url).hostname == KNOWN_EXTERNAL_HOST and path.startswith("/system/locale/")):
        return True
    return False


def open_page(page: Page, base_url: str, path: str, role: str, scenario: str = "success", session: str = "current") -> None:
    bootstrap(page, role, scenario, session)
    page.goto(f"{base_url}{path}", wait_until="domcontentloaded")
    page.locator(MAIN).first.wait_for(state="visible")


def assert_shell(page: Page, label: str) -> None:
    if page.locator(MAIN).count() != 1:
        raise AssertionError(f"{label}: expected one main landmark")
    if page.locator(f"{MAIN} h1:visible").count() != 1:
        raise AssertionError(f"{label}: expected one visible h1")
    overflow = page.evaluate("document.documentElement.scrollWidth - document.documentElement.clientWidth")
    if overflow > 1:
        raise AssertionError(f"{label}: horizontal overflow {overflow}")


def check_status(page: Page, base_url: str) -> None:
    open_page(page, base_url, "/c311/status", "public_visitor")
    assert_shell(page, "anonymous status")
    page.locator(STATUS_LOOKUP).click()
    page.locator("[data-c311-error-summary]").wait_for(state="visible")
    page.locator("[data-c311-error-summary] a").first.click()
    if page.evaluate("document.activeElement && document.activeElement.id") != "c311-status-request-number":
        raise AssertionError("status error summary did not focus the request number field")
    page.locator("#c311-status-request-number").fill("SR-2026-00001")
    page.locator("#c311-status-email").fill("alex@example.test")
    page.wait_for_timeout(50)
    page.reload(wait_until="domcontentloaded")
    page.locator(MAIN).first.wait_for(state="visible")
    if page.locator("#c311-status-request-number").input_value() != "SR-2026-00001":
        raise AssertionError("status request number was not restored after refresh")
    if page.locator("#c311-status-email").input_value() != "alex@example.test":
        raise AssertionError("status email was not restored after refresh")
    page.locator(STATUS_LOOKUP).click()
    page.locator("[data-c311-status-result]").wait_for(state="visible")
    if page.locator("[data-c311-status-value]").inner_text() != "SUBMITTED":
        raise AssertionError("status value was not preserved")
    if page.locator("[data-c311-status-history] li").count() < 1:
        raise AssertionError("public status history missing")
    page.locator("#c311-status-email").fill("unknown@example.test")
    page.locator(STATUS_LOOKUP).click()
    page.locator("[data-c311-data-state][data-state='not-found']").wait_for(state="visible")
    page.locator('[data-c311-status-result]').wait_for(state="detached")
    if page.locator("[data-c311-status-history]").count() != 0:
        raise AssertionError("anonymous mismatch exposed public history")


def check_requests_and_account(page: Page, base_url: str) -> None:
    open_page(page, base_url, "/c311/requests", "constituent")
    assert_shell(page, "my requests")
    page.locator(f"{VIEW_REQUEST}:visible").first.click()
    page.locator("[data-c311-request-detail]").wait_for(state="visible")
    if page.locator("[data-c311-request-history] li").count() < 1:
        raise AssertionError("public request history missing")
    public_relationships = page.locator("[data-c311-public-relationships]")
    public_notes = page.locator("[data-c311-public-notes]")
    if public_relationships.count() != 1 or public_relationships.locator("[data-c311-public-relationship]").count() < 1:
        raise AssertionError("public relationship projection missing")
    if public_relationships.locator("[data-c311-public-relationship-permission]").count() != 1:
        raise AssertionError("public relationship permission result missing")
    if public_relationships.locator("text=constituent-fixture-hidden").count() != 0:
        raise AssertionError("hidden relationship was exposed in the portal")
    if "do not notify" not in public_relationships.inner_text().lower() and "notify status" not in public_relationships.inner_text().lower():
        raise AssertionError("public relationship notification result missing")
    if public_relationships.locator("[data-c311-public-notification-result]").count() != 1:
        raise AssertionError("public relationship delivery result missing")
    if public_relationships.locator("[data-c311-public-relationship-audit]").count() != 1:
        raise AssertionError("public relationship audit missing")
    if public_notes.count() != 1 or public_notes.locator("[data-c311-public-note]").count() < 1:
        raise AssertionError("public voter note projection missing")
    if public_notes.locator("text=Internal triage note.").count() != 0:
        raise AssertionError("internal note was exposed in the portal")
    if "actor-fixture-agent" not in public_notes.inner_text():
        raise AssertionError("public note actor missing")
    if "01/15/2026 10:00 AM EST" not in public_notes.inner_text():
        raise AssertionError("public note local time missing")
    note_form = page.locator('[data-c311-form="public-note"]')
    if note_form.count() != 1:
        raise AssertionError("public note append form missing for the authenticated constituent")
    note_form.locator("#c311-public-note-body").fill("Resident follow-up")
    note_form.locator('[data-c311-action="append-public-note"]').click()
    page.locator("[data-c311-public-note]", has_text="Resident follow-up").wait_for(state="visible")
    page.locator(ACCOUNT_ROUTE).click()
    page.wait_for_url("**/c311/account")
    page.locator("#c311-account-name").wait_for(state="visible")
    if page.locator("#c311-account-name").input_value() != "Alex Example":
        raise AssertionError("account profile was not loaded")
    language = page.locator(LANGUAGE_SELECTOR)
    language.select_option("es")
    if language.input_value() != "es":
        raise AssertionError("language switch did not select Spanish")
    page.reload(wait_until="domcontentloaded")
    page.locator(MAIN).first.wait_for(state="visible")
    if "/c311/account" not in page.url or page.locator(LANGUAGE_SELECTOR).input_value() != "es":
        raise AssertionError("language or route was not restored after refresh")


def check_account_failure(page: Page, base_url: str, scenario: str, expected_message: str) -> None:
    open_page(page, base_url, "/c311/account", "constituent", scenario)
    assert_shell(page, f"account {scenario}")
    login_identifier = page.locator("#c311-account-login-identifier")
    current_password = page.locator("#c311-account-current-password")
    login_identifier.fill("alex-recovery")
    current_password.fill("CurrentPasswordFixture!")
    page.locator('[data-c311-action="change-login-identifier"]').click()
    page.locator("#c311-error-summary").wait_for(state="visible")
    summary = page.locator("#c311-error-summary").inner_text()
    if expected_message not in summary:
        raise AssertionError(f"account {scenario} did not expose expected failure: {summary}")
    if login_identifier.input_value() != "alex-recovery":
        raise AssertionError(f"account {scenario} discarded the login identifier")
    if current_password.input_value() != "CurrentPasswordFixture!":
        raise AssertionError(f"account {scenario} discarded the re-authentication input")


def check_expired_account_guard(page: Page, base_url: str) -> None:
    open_page(page, base_url, "/c311/account", "constituent", "success", "expired")
    page.wait_for_url("**/c311/401*")
    assert_shell(page, "expired account session")


def check_account_disposition(page: Page, base_url: str) -> None:
    open_page(page, base_url, "/c311/account", "constituent")
    assert_shell(page, "account disposition")
    page.locator("#c311-account-disposition-mode").select_option("ANONYMIZE")
    page.locator("#c311-account-disposition-confirm").fill("ANONYMIZE")
    page.locator(ACCOUNT_DISPOSITION_ACTION).click()
    page.locator("[data-c311-account-disposition-result]").wait_for(state="visible")
    if "ANONYMIZED" not in page.locator("[data-c311-account-disposition-result]").inner_text():
        raise AssertionError("account anonymization result missing")
    if page.locator(ACCOUNT_ROUTE).count() != 0:
        raise AssertionError("authenticated account navigation remained after disposition")


def check_account_disposition_failure(page: Page, base_url: str) -> None:
    open_page(page, base_url, "/c311/account", "constituent", "account-disposition-conflict")
    assert_shell(page, "account disposition conflict")
    page.locator("#c311-account-disposition-mode").select_option("DELETE")
    page.locator("#c311-account-disposition-confirm").fill("DELETE")
    page.locator(ACCOUNT_DISPOSITION_ACTION).click()
    page.locator("[data-c311-error-summary]").wait_for(state="visible")
    if "account changed" not in page.locator("[data-c311-error-summary]").inner_text().lower():
        raise AssertionError("account disposition conflict was not shown")
    if page.locator("#c311-account-disposition-confirm").input_value() != "DELETE":
        raise AssertionError("account disposition input was discarded after conflict")


def check_admin_staff(page: Page, base_url: str) -> None:
    open_page(page, base_url, "/c311/staff", "service_agent")
    assert_shell(page, "staff request queue")
    if page.locator("[data-c311-state='populated']").count() == 0 and page.locator("text=SR-2026-00001").count() == 0:
        raise AssertionError("staff request queue did not load")
    page.locator(f"{VIEW_REQUEST}:visible").first.click()
    page.locator("[data-c311-staff-request-detail]").wait_for(state="visible")
    if page.locator("[data-c311-relationship-notification-result]").count() != 1:
        raise AssertionError("staff relationship notification result missing")
    if page.locator("[data-c311-relationship-audit]").count() < 1:
        raise AssertionError("staff relationship audit missing")
    if page.locator("[data-c311-relationship-audit-events]").count() != 1:
        raise AssertionError("staff relationship audit events missing")


def check_status_scenario(page: Page, base_url: str, scenario: str, expected_state: str) -> None:
    open_page(page, base_url, "/c311/status", "public_visitor", scenario)
    page.locator("#c311-status-request-number").fill("SR-2026-00001")
    page.locator("#c311-status-email").fill("alex@example.test")
    page.locator(STATUS_LOOKUP).click()
    page.locator(f"[data-c311-data-state][data-state='{expected_state}']").wait_for(state="visible")


def check_access_guard(page: Page, base_url: str, path: str, role: str, session: str, expected_path: str) -> None:
    open_page(page, base_url, path, role, "success", session)
    page.wait_for_url(f"**{expected_path}*")


def run() -> Path:
    artifacts = artifact_directory()
    matrix = {
        "mode": "Mock-only",
        "viewports": [{"width": width, "height": height} for width, height in VIEWPORTS],
        "apps": {},
        "scenarios": {"compose": {}, "admin": {}},
    }
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(headless=True)
        try:
            for app, base_url in (("compose", COMPOSE_URL), ("admin", ADMIN_URL)):
                matrix["apps"][app] = {}
                for width, height in VIEWPORTS:
                    context = browser.new_context(viewport={"width": width, "height": height})
                    page = context.new_page()
                    result = diagnostics(page)
                    if app == "compose":
                        check_status(page, base_url)
                        check_requests_and_account(page, base_url)
                    else:
                        check_admin_staff(page, base_url)
                    assert_shell(page, f"{app}-{width}")
                    if result["page_errors"] or result["unexpected_console_errors"] or result["unexpected_responses"] or result["unexpected_writes"] or result["sensitive_requests"]:
                        raise AssertionError(f"{app}-{width} diagnostics: {result}")
                    name = f"{app}-{width}x{height}"
                    page.screenshot(path=str(artifacts / f"{name}.png"), full_page=True)
                    (artifacts / f"{name}.json").write_text(json.dumps(result, indent=2), encoding="utf-8")
                    matrix["apps"][app][name] = {"passed": True, "diagnostics": result}
                    context.close()

                    if app == "compose":
                        for scenario, expected_state in (("retryable", "retryable-error"), ("terminal", "terminal-error")):
                            scenario_context = browser.new_context(viewport={"width": width, "height": height})
                            scenario_page = scenario_context.new_page()
                            scenario_result = diagnostics(scenario_page)
                            check_status_scenario(scenario_page, base_url, scenario, expected_state)
                            if scenario_result["page_errors"] or scenario_result["unexpected_console_errors"] or scenario_result["unexpected_responses"] or scenario_result["unexpected_writes"] or scenario_result["sensitive_requests"]:
                                raise AssertionError(f"compose-{width} {scenario} diagnostics: {scenario_result}")
                            matrix["scenarios"]["compose"][f"{scenario}-{width}x{height}"] = {"passed": True, "diagnostics": scenario_result}
                            scenario_context.close()

                        for path, role, session, expected_path in (("/c311/account", "public_visitor", "current", "/c311/401"), ("/c311/requests", "service_agent", "current", "/c311/403")):
                            guard_context = browser.new_context(viewport={"width": width, "height": height})
                            guard_page = guard_context.new_page()
                            guard_result = diagnostics(guard_page)
                            check_access_guard(guard_page, base_url, path, role, session, expected_path)
                            if guard_result["page_errors"] or guard_result["unexpected_console_errors"] or guard_result["unexpected_responses"] or guard_result["unexpected_writes"] or guard_result["sensitive_requests"]:
                                raise AssertionError(f"compose-{width} {path} diagnostics: {guard_result}")
                            matrix["scenarios"]["compose"][f"guard-{path}-{width}x{height}"] = {"passed": True, "diagnostics": guard_result}
                            guard_context.close()

                        for scenario, expected_message in (("invalid-credentials", "The sign-in details are not valid."), ("version-conflict", "The record changed before your update.")):
                            failure_context = browser.new_context(viewport={"width": width, "height": height})
                            failure_page = failure_context.new_page()
                            failure_result = diagnostics(failure_page)
                            check_account_failure(failure_page, base_url, scenario, expected_message)
                            if failure_result["page_errors"] or failure_result["unexpected_console_errors"] or failure_result["unexpected_responses"] or failure_result["unexpected_writes"] or failure_result["sensitive_requests"]:
                                raise AssertionError(f"compose-{width} account {scenario} diagnostics: {failure_result}")
                            matrix["scenarios"]["compose"][f"account-{scenario}-{width}x{height}"] = {"passed": True, "diagnostics": failure_result}
                            failure_context.close()

                        expired_context = browser.new_context(viewport={"width": width, "height": height})
                        expired_page = expired_context.new_page()
                        expired_result = diagnostics(expired_page)
                        check_expired_account_guard(expired_page, base_url)
                        if expired_result["page_errors"] or expired_result["unexpected_console_errors"] or expired_result["unexpected_responses"] or expired_result["unexpected_writes"] or expired_result["sensitive_requests"]:
                            raise AssertionError(f"compose-{width} expired account diagnostics: {expired_result}")
                        matrix["scenarios"]["compose"][f"account-expired-{width}x{height}"] = {"passed": True, "diagnostics": expired_result}
                        expired_context.close()

                        disposition_context = browser.new_context(viewport={"width": width, "height": height})
                        disposition_page = disposition_context.new_page()
                        disposition_result = diagnostics(disposition_page)
                        check_account_disposition(disposition_page, base_url)
                        if disposition_result["page_errors"] or disposition_result["unexpected_console_errors"] or disposition_result["unexpected_responses"] or disposition_result["unexpected_writes"] or disposition_result["sensitive_requests"]:
                            raise AssertionError(f"compose-{width} account disposition diagnostics: {disposition_result}")
                        matrix["scenarios"]["compose"][f"account-disposition-success-{width}x{height}"] = {"passed": True, "diagnostics": disposition_result}
                        disposition_context.close()

                        conflict_context = browser.new_context(viewport={"width": width, "height": height})
                        conflict_page = conflict_context.new_page()
                        conflict_result = diagnostics(conflict_page)
                        check_account_disposition_failure(conflict_page, base_url)
                        if conflict_result["page_errors"] or conflict_result["unexpected_console_errors"] or conflict_result["unexpected_responses"] or conflict_result["unexpected_writes"] or conflict_result["sensitive_requests"]:
                            raise AssertionError(f"compose-{width} account disposition conflict diagnostics: {conflict_result}")
                        matrix["scenarios"]["compose"][f"account-disposition-conflict-{width}x{height}"] = {"passed": True, "diagnostics": conflict_result}
                        conflict_context.close()
        finally:
            browser.close()
    (artifacts / "fe05-matrix.json").write_text(json.dumps(matrix, indent=2), encoding="utf-8")
    return artifacts


if __name__ == "__main__":
    print(run())
