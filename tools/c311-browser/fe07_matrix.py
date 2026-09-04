#!/usr/bin/env python3
"""Repeatable FE-07 employee operation checks using the C311 mock provider."""

from __future__ import annotations

import json
import sys
from datetime import datetime, timezone

from playwright.sync_api import Page, sync_playwright

from fe03_matrix import (
    ADMIN_URL,
    MAIN,
    artifact_directory,
    assert_page,
    check,
    diagnostics,
    finalize_diagnostics,
    open_page,
)


FE07_VIEWPORTS = ((360, 900), (767, 900), (768, 900), (1023, 900), (1024, 900), (1440, 900), (1920, 900))
STAFF_ROLES = ("service_agent", "supervisor", "department_manager", "platform_administrator", "workflow_designer")
DETAIL_PATH = "/c311/staff/requests/request-fixture-001"


def assert_diagnostics(result: dict, label: str) -> None:
    finalize_diagnostics(result)
    check(not result["page_errors"], f"{label} has uncaught page errors: {result['page_errors']}")
    check(not result["unexpected_console_errors"], f"{label} has unexpected console errors: {result['unexpected_console_errors']}")
    check(not result["unexpected_responses"], f"{label} has unexpected HTTP failures: {result['unexpected_responses']}")
    check(not result["writes"], f"{label} issued network writes in mock mode: {result['writes']}")


def responsive_check(page: Page, width: int, label: str) -> None:
    if width < 768:
        check(page.locator("[data-c311-responsive-cards]").is_visible(), f"{label} did not use cards")
        check(not page.locator("[data-c311-responsive-table]").is_visible(), f"{label} exposed a table on a narrow viewport")
    else:
        check(page.locator("[data-c311-responsive-table]").is_visible(), f"{label} did not expose the queue table")
    check(page.evaluate("document.documentElement.scrollWidth - document.documentElement.clientWidth") <= 1, f"{label} overflows")


def queue_and_bulk(page: Page, width: int, role: str) -> None:
    label = f"{role}@{width}px"
    open_page(page, ADMIN_URL, "/c311/staff", role=role)
    if role == "workflow_designer":
        check("403" in page.url or "access" in page.locator(MAIN).inner_text().lower(), f"{label} did not reject queue access")
        return
    page.locator("[data-c311-staff-filters]").wait_for(state="visible")
    responsive_check(page, width, label)
    for selector in ("#c311-filter-status", "#c311-filter-department", "#c311-filter-assignee", "#c311-filter-sort", "#c311-filter-page-size"):
        check(page.locator(selector).count() == 1, f"{label} missing {selector}")
    page.locator("#c311-filter-department").select_option("STREETS")
    page.locator('[data-c311-action="apply-staff-filters"]').click()
    check(page.locator('[data-c311-department="STREETS"]:visible').count() == 1, f"{label} filter did not change results")
    page.locator("#c311-filter-department").select_option("GENERAL_SERVICES")
    page.locator('[data-c311-action="apply-staff-filters"]').click()
    page.locator('[data-c311-data-state][data-state="empty"]').wait_for(state="visible")
    check(page.locator("#c311-filter-department").input_value() == "GENERAL_SERVICES", f"{label} filter was not retained")
    if role == "supervisor":
        page.locator("#c311-filter-department").select_option("STREETS")
        page.locator('[data-c311-action="apply-staff-filters"]').click()
        page.locator('[data-c311-selection="request-fixture-001"]:visible').first.check()
        page.locator('[data-c311-action="bulk-open-confirm"]').click()
        page.locator('[data-c311-bulk-confirmation]').wait_for(state="visible")
        page.locator('[data-c311-action="bulk-cancel"]').click()
        check(not page.locator('[data-c311-bulk-confirmation]').is_visible(), f"{label} bulk cancel failed")


def detail(page: Page, width: int, role: str) -> None:
    label = f"detail-{role}@{width}px"
    open_page(page, ADMIN_URL, DETAIL_PATH, role=role)
    if role == "workflow_designer":
        check("403" in page.url or "access" in page.locator(MAIN).inner_text().lower(), f"{label} did not reject detail access")
        return
    state = page.locator("[data-c311-data-state]").first
    state.wait_for(state="visible")
    page.locator('[data-c311-request-detail]').wait_for(state="visible")
    check(page.locator('[data-c311-staff-request-detail]').count() == 1, f"{label} FE-05 detail marker missing")
    for marker in ("[data-c311-relationships]", "[data-c311-collaborators]", "[data-c311-reminders]", "[data-c311-attachments]", "[data-c311-history]"):
        check(page.locator(marker).count() == 1, f"{label} missing {marker}")
    check(page.locator('[data-c311-status-label]').inner_text() == "(Submitted)", f"{label} status label mismatch")
    if role in ("service_agent", "supervisor"):
        check(page.locator('[data-c311-audit]').count() == 0, f"{label} exposed audit")
    else:
        check(page.locator('[data-c311-audit]').count() == 1, f"{label} audit capability mismatch")
    if role in ("supervisor", "department_manager"):
        check(page.locator('[data-c311-action="reassign-request"]').count() == 1, f"{label} reassign missing")
        check(page.locator('[data-c311-action="manage-collaborators"]').count() == 1, f"{label} collaborator entry missing")
    else:
        check(page.locator('[data-c311-action="reassign-request"]').count() == 0, f"{label} unauthorized reassign visible")
    if role == "department_manager":
        page.locator('[data-c311-action="override-scope"]').click()
        page.locator('[data-c311-form="scope"]').wait_for(state="visible")
        page.locator("#c311-scope-reason").fill("fixture scope review")
        page.locator('[data-c311-action="submit-scope"]').click()
        page.locator('[data-c311-action-message]').wait_for(state="visible")
        check("Scope updated" in page.locator('[data-c311-action-message]').inner_text(), f"{label} scope override did not complete")
    if role == "supervisor":
        page.locator('[data-c311-action="confirm-duplicate"]').click()
        page.locator('[data-c311-form="duplicate"]').wait_for(state="visible")
        page.locator("#c311-duplicate-group").fill("duplicate-fixture-001")
        page.locator("#c311-duplicate-reason").fill("fixture duplicate review")
        page.locator('[data-c311-action="submit-duplicate"]').click()
        page.locator('[data-c311-duplicate-group]').wait_for(state="visible")
        check(page.locator('[data-c311-duplicate-group]').inner_text() == "duplicate-fixture-001", f"{label} duplicate group confirmation did not complete")
        page.locator('[data-c311-action="remove-duplicate"]').click()
        page.locator('[data-c311-duplicate-group]').wait_for(state="visible")
        check(page.locator('[data-c311-duplicate-group]').inner_text() == "-", f"{label} duplicate group removal did not complete")
    check(page.locator('[data-c311-action="approve-reopen"]').count() == 0, f"{label} reopen exposed for SUBMITTED")
    triage = page.locator('[data-c311-action="transition-request"]')
    if triage.count():
        triage.click()
        page.locator('[data-c311-status-label]').wait_for(state="visible")
        check("Triaged" in page.locator('[data-c311-status-label]').inner_text(), f"{label} transition failed")
    page.reload(wait_until="domcontentloaded")
    page.locator('[data-c311-request-detail]').wait_for(state="visible")
    check(page.url.endswith(DETAIL_PATH), f"{label} detail route was not restored")


def errors(page: Page) -> None:
    open_page(page, ADMIN_URL, "/c311/staff", role="service_agent", session="expired")
    check("401" in page.url or "sign in" in page.locator(MAIN).inner_text().lower(), "expired session did not produce 401")
    open_page(page, ADMIN_URL, "/c311/staff", role="workflow_designer")
    check("403" in page.url or "access" in page.locator(MAIN).inner_text().lower(), "missing capability did not produce 403")
    open_page(page, ADMIN_URL, DETAIL_PATH, scenario="not-found", role="service_agent")
    page.locator('[data-c311-data-state][data-state="not-found"]').wait_for(state="visible")
    open_page(page, ADMIN_URL, DETAIL_PATH, scenario="scope-denied", role="service_agent")
    page.locator('[data-c311-data-state][data-state="forbidden"]').wait_for(state="visible")
    open_page(page, ADMIN_URL, DETAIL_PATH, scenario="retryable", role="service_agent")
    page.locator('[data-c311-data-state][data-state="retryable-error"]').wait_for(state="visible")
    page.locator('[data-c311-data-state][data-state="retryable-error"] button').click()


def run() -> dict:
    artifacts = artifact_directory()
    results = {"mode": "mock-only", "viewports": [list(item) for item in FE07_VIEWPORTS], "checks": [], "started_at": datetime.now(timezone.utc).isoformat()}
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(headless=True)
        try:
            for width, height in FE07_VIEWPORTS:
                for role in STAFF_ROLES:
                    context = browser.new_context(viewport={"width": width, "height": height})
                    page = context.new_page()
                    result = diagnostics(page, ADMIN_URL)
                    label = f"admin-{role}@{width}x{height}"
                    try:
                        queue_and_bulk(page, width, role)
                        detail(page, width, role)
                        assert_page(page, label, width)
                        screenshot = artifacts / f"fe07-{role}-{width}x{height}.png"
                        page.screenshot(path=str(screenshot), full_page=True)
                        assert_diagnostics(result, label)
                        results["checks"].append({"label": label, "status": "passed", "screenshot": str(screenshot), "diagnostics": result})
                    finally:
                        context.close()
                context = browser.new_context(viewport={"width": width, "height": height})
                page = context.new_page()
                result = diagnostics(page, ADMIN_URL)
                try:
                    errors(page)
                    assert_diagnostics(result, f"errors@{width}px")
                finally:
                    context.close()
        finally:
            browser.close()
    results["finished_at"] = datetime.now(timezone.utc).isoformat()
    (artifacts / "fe07-matrix.json").write_text(json.dumps(results, indent=2), encoding="utf-8")
    return results


if __name__ == "__main__":
    try:
        print(json.dumps(run(), indent=2))
    except Exception as error:
        print(json.dumps({"status": "failed", "error": str(error)}), file=sys.stderr)
        raise
