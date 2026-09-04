#!/usr/bin/env python3
"""Repeatable FE-06 employee queue/detail checks using the C311 mock provider."""

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


# Keep every responsive breakpoint in the release gate, including both sides
# of the tablet/table and card/table boundaries.
FE06_VIEWPORTS = ((360, 900), (767, 900), (768, 900), (1023, 900), (1024, 900), (1440, 900), (1920, 900))
STAFF_PATH = "/c311/staff"
DETAIL_PATH = "/c311/staff/requests/request-fixture-001"
STAFF_ROLES = ("service_agent", "supervisor", "department_manager", "platform_administrator")
FILTER_IDS = (
    "#c311-filter-status",
    "#c311-filter-service-type",
    "#c311-filter-department",
    "#c311-filter-district",
    "#c311-filter-origin",
    "#c311-filter-source",
    "#c311-filter-assignee",
    "#c311-filter-collaborator",
    "#c311-filter-category",
    "#c311-filter-created-from",
    "#c311-filter-created-to",
    "#c311-filter-duplicate",
    "#c311-filter-sort",
    "#c311-filter-page-size",
)


def assert_no_network_failures(result: dict, label: str) -> None:
    finalize_diagnostics(result)
    check(not result["page_errors"], f"{label} has uncaught page errors: {result['page_errors']}")
    check(not result["unexpected_console_errors"], f"{label} has unexpected console errors: {result['unexpected_console_errors']}")
    check(not result["unexpected_responses"], f"{label} has unexpected HTTP failures: {result['unexpected_responses']}")
    check(not result["writes"], f"{label} issued unexpected network writes in mock mode: {result['writes']}")


def wait_state(page: Page, state: str) -> None:
    page.locator(f'[data-c311-data-state][data-state="{state}"]').wait_for(state="visible")


def apply_filter(page: Page) -> None:
    page.locator('[data-c311-action="apply-staff-filters"]').click()
    page.locator('[data-c311-data-state]').first.wait_for(state="visible")


def check_responsive_layout(page: Page, width: int, label: str) -> None:
    cards = page.locator('[data-c311-responsive-cards]')
    table = page.locator('[data-c311-responsive-table]')
    if width < 768:
        check(cards.is_visible(), f"{label} did not use cards below 768px")
        check(not table.is_visible(), f"{label} exposed the full table below 768px")
    else:
        check(table.is_visible(), f"{label} did not expose the table at {width}px")
    overflow = page.evaluate("document.documentElement.scrollWidth - document.documentElement.clientWidth")
    check(overflow <= 1, f"{label} overflows at {width}px: {overflow}")


def check_queue_filters_and_recovery(page: Page, width: int, role: str) -> None:
    label = f"admin-{role}@{width}px"
    open_page(page, ADMIN_URL, STAFF_PATH, role=role)
    check(page.locator('[data-c311-staff-filters]').count() == 1, "staff filters missing")
    for selector in FILTER_IDS:
        check(page.locator(selector).count() == 1, f"filter missing: {selector}")
    check_responsive_layout(page, width, label)

    # Scope filtering must remove a foreign record from a normal queue response;
    # the same record must remain protected when addressed directly.
    if role in ("service_agent", "platform_administrator"):
        page.evaluate("sessionStorage.removeItem('c311.staff.queue')")
        open_page(page, ADMIN_URL, STAFF_PATH, scenario="scope-filter", role=role)
        wait_state(page, "populated")
        visible_rows = page.locator('[data-c311-department]:visible')
        expected_rows = 1 if role == "service_agent" else 2
        check(visible_rows.count() == expected_rows, f"{label}: scope filter returned {visible_rows.count()} rows, expected {expected_rows}")
        if role == "service_agent":
            check(page.locator('[data-c311-department="GENERAL_SERVICES"]:visible').count() == 0, f"{label}: foreign department leaked into queue")
            page.evaluate("sessionStorage.removeItem('c311.staff.queue')")
            open_page(page, ADMIN_URL, "/c311/staff/requests/request-fixture-foreign", scenario="scope-filter", role=role)
            wait_state(page, "forbidden")
        else:
            page.evaluate("sessionStorage.removeItem('c311.staff.queue')")
            open_page(page, ADMIN_URL, "/c311/staff/requests/request-fixture-foreign", scenario="scope-filter", role=role)
            page.locator('[data-c311-request-detail]').wait_for(state="visible")
            check(page.locator('[data-c311-request-detail] h2').inner_text() == "SR-2026-00099", f"{label}: unrestricted scope detail fixture missing")
        page.evaluate("sessionStorage.removeItem('c311.staff.queue')")
        open_page(page, ADMIN_URL, STAFF_PATH, role=role)

    # A department filter must change the actual result set, not merely retain
    # the selected value in the form.
    page.locator("#c311-filter-department").select_option("STREETS")
    apply_filter(page)
    row_actions = page.locator('[data-c311-action^="view-request-"]:visible')
    row_actions.first.wait_for(state="visible")
    check(page.locator('[data-c311-department="STREETS"]:visible').count() == 1, f"{label}: department filter did not constrain the queue")

    page.locator("#c311-filter-department").select_option("GENERAL_SERVICES")
    apply_filter(page)
    wait_state(page, "empty")
    check(page.locator("#c311-filter-department").input_value() == "GENERAL_SERVICES", "empty filter was not retained")

    # Assignee is a distinct filter and should produce an empty result for the
    # unassigned fixture rather than being ignored.
    page.locator("#c311-filter-department").select_option("")
    page.locator("#c311-filter-assignee").fill("actor-fixture-agent")
    apply_filter(page)
    wait_state(page, "empty")
    check(page.locator("#c311-filter-assignee").input_value() == "actor-fixture-agent", "assignee filter was not retained")

    page.locator("#c311-filter-assignee").fill("")
    page.locator("#c311-filter-sort").fill("-updated_at")
    page.locator("#c311-filter-page-size").fill("1")
    apply_filter(page)
    row_actions = page.locator('[data-c311-action^="view-request-"]:visible')
    row_actions.first.wait_for(state="visible")
    check(page.locator("#c311-filter-sort").input_value() == "-updated_at", "sort was not applied")
    check(page.locator("#c311-filter-page-size").input_value() == "1", "page size was not applied")

    # The pagination fixture has an opaque next token and a second result.
    open_page(page, ADMIN_URL, STAFF_PATH, scenario="pagination", role=role)
    page.locator("#c311-filter-page-size").fill("1")
    apply_filter(page)
    page.locator('[data-c311-action="next-page"]').wait_for(state="visible")
    check(not page.locator('[data-c311-action="next-page"]').is_disabled(), "pagination next page was not enabled")
    first_request = page.locator('[data-c311-action^="view-request-"]:visible').first.get_attribute("data-c311-action")
    page.locator('[data-c311-action="next-page"]').click()
    page.locator('[data-c311-action^="view-request-"]:visible').first.wait_for(state="visible")
    check(page.locator('[data-c311-action^="view-request-"]:visible').first.get_attribute("data-c311-action") != first_request, "next page did not change the result")
    check(page.locator('[data-c311-action="previous-page"]').is_enabled(), "previous page was not enabled after advancing")

    # Queue state is persisted and restored after a real browser reload.
    open_page(page, ADMIN_URL, STAFF_PATH, role=role)
    page.locator("#c311-filter-department").select_option("STREETS")
    page.locator("#c311-filter-sort").fill("-updated_at")
    apply_filter(page)
    page.reload(wait_until="domcontentloaded")
    page.locator('[data-c311-staff-filters]').wait_for(state="visible")
    check(page.locator("#c311-filter-department").input_value() == "STREETS", "queue department was not restored after refresh")
    check(page.locator("#c311-filter-sort").input_value() == "-updated_at", "queue sort was not restored after refresh")


def check_detail(page: Page, width: int, role: str) -> None:
    label = f"detail-{role}@{width}px"
    open_page(page, ADMIN_URL, STAFF_PATH, role=role)
    page.locator('[data-c311-action^="view-request-"]:visible').first.click()
    page.wait_for_url(f"**{DETAIL_PATH}", wait_until="commit")
    page.locator('[data-c311-request-detail]').wait_for(state="visible")
    check(page.locator('[data-c311-staff-request-detail]').count() == 1, "FE-05 staff detail marker missing")
    check(page.locator('[data-c311-request-detail] code').first.inner_text() == "SUBMITTED", "stable API status missing")
    for marker in ("[data-c311-relationships]", "[data-c311-collaborators]", "[data-c311-attachments]", "[data-c311-history]"):
        check(page.locator(marker).count() == 1, f"detail section missing: {marker}")
    check(page.locator('[data-c311-attachment-entry]').count() >= 1, "attachment entries missing")
    check(page.locator('[data-c311-action="manage-relationships"]').count() == 1, "relationship entry missing")
    if role in ("service_agent", "supervisor"):
        check(page.locator('[data-c311-audit]').count() == 0, f"{role} saw audit details without audit_list")
        check(page.locator('[data-c311-audit-unavailable]').count() == 1, "audit denial message missing")
    else:
        check(page.locator('[data-c311-audit]').count() == 1, f"{role} audit section missing")
    if role in ("supervisor", "department_manager"):
        check(page.locator('[data-c311-action="reassign-request"]').count() == 1, f"{role} reassignment entry missing")
    else:
        check(page.locator('[data-c311-action="reassign-request"]').count() == 0, f"{role} saw unauthorized reassignment")
    check(page.locator('[data-c311-action="approve-reopen"]').count() == 0, "reopen was exposed for SUBMITTED record")
    if role in ("supervisor", "department_manager"):
        check(page.locator('[data-c311-action="manage-collaborators"]').count() == 1, f"{role} collaborator entry missing")
    else:
        check(page.locator('[data-c311-action="manage-collaborators"]').count() == 0, f"{role} saw unauthorized collaborator entry")
    if role in ("department_manager", "platform_administrator"):
        check(page.locator('[data-c311-action="override-origin"]').count() == 1, f"{role} origin override entry missing")
    else:
        check(page.locator('[data-c311-action="override-origin"]').count() == 0, f"{role} saw unauthorized origin override entry")
    check(page.locator('[data-c311-status-label]').inner_text() == "(Submitted)", "localized status value label missing")

    # FE-07 executes the transition through the mock provider with If-Match.
    transition = page.locator('[data-c311-action="transition-request"]')
    if transition.count():
        transition.click()
        page.locator('[data-c311-status-label]').wait_for(state="visible")
        check("Triaged" in page.locator('[data-c311-status-label]').inner_text(), "triage transition did not update status")

    # Detail reload must remain on the detail route and load the same record.
    page.reload(wait_until="domcontentloaded")
    page.locator('[data-c311-request-detail]').wait_for(state="visible")
    check(page.url.endswith(DETAIL_PATH), f"detail route was not restored after reload: {page.url}")
    assert_page(page, label, width)


def check_error_routes(page: Page) -> None:
    open_page(page, ADMIN_URL, STAFF_PATH, role="service_agent", session="expired")
    check("401" in page.url or "sign in" in page.locator(MAIN).inner_text().lower(), "expired session did not render 401")

    open_page(page, ADMIN_URL, STAFF_PATH, scenario="forbidden", role="service_agent")
    check("403" in page.url or "access denied" in page.locator(MAIN).inner_text().lower(), "forbidden queue did not render 403")

    open_page(page, ADMIN_URL, STAFF_PATH, role="workflow_designer")
    check("403" in page.url or "access denied" in page.locator(MAIN).inner_text().lower(), "workflow designer did not render 403")

    open_page(page, ADMIN_URL, DETAIL_PATH, scenario="scope-denied", role="service_agent")
    wait_state(page, "forbidden")
    check("scope" in page.locator('[data-c311-data-state]').inner_text().lower() or "access" in page.locator('[data-c311-data-state]').inner_text().lower(), "scope denial did not explain access")

    open_page(page, ADMIN_URL, DETAIL_PATH, scenario="not-found", role="service_agent")
    wait_state(page, "not-found")

    open_page(page, ADMIN_URL, DETAIL_PATH, scenario="retryable", role="service_agent")
    wait_state(page, "retryable-error")
    retry = page.locator('[data-c311-data-state][data-state="retryable-error"] button')
    retry.wait_for(state="visible")
    retry.click()
    wait_state(page, "retryable-error")

    open_page(page, ADMIN_URL, DETAIL_PATH, scenario="terminal", role="service_agent")
    wait_state(page, "terminal-error")


def run() -> dict:
    artifacts = artifact_directory()
    results = {
        "mode": "mock-only",
        "viewports": [list(viewport) for viewport in FE06_VIEWPORTS],
        "checks": [],
        "started_at": datetime.now(timezone.utc).isoformat(),
    }
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(headless=True)
        try:
            for width, height in FE06_VIEWPORTS:
                for role in STAFF_ROLES:
                    context = browser.new_context(viewport={"width": width, "height": height})
                    page = context.new_page()
                    result = diagnostics(page, ADMIN_URL)
                    label = f"admin-{role}@{width}x{height}"
                    try:
                        check_queue_filters_and_recovery(page, width, role)
                        check_detail(page, width, role)
                        assert_page(page, label, width)
                        screenshot = artifacts / f"fe06-admin-{role}-{width}x{height}.png"
                        page.screenshot(path=str(screenshot), full_page=True)
                        assert_no_network_failures(result, label)
                        results["checks"].append({"label": label, "status": "passed", "screenshot": str(screenshot), "diagnostics": result})
                    finally:
                        context.close()

                context = browser.new_context(viewport={"width": width, "height": height})
                page = context.new_page()
                result = diagnostics(page, ADMIN_URL)
                label = f"admin-errors@{width}x{height}"
                try:
                    check_error_routes(page)
                    assert_page(page, label, width)
                    screenshot = artifacts / f"fe06-admin-errors-{width}x{height}.png"
                    page.screenshot(path=str(screenshot), full_page=True)
                    assert_no_network_failures(result, label)
                    results["checks"].append({"label": label, "status": "passed", "screenshot": str(screenshot), "diagnostics": result})
                finally:
                    context.close()
        finally:
            browser.close()
    results["finished_at"] = datetime.now(timezone.utc).isoformat()
    (artifacts / "fe06-matrix.json").write_text(json.dumps(results, indent=2), encoding="utf-8")
    return results


if __name__ == "__main__":
    try:
        print(json.dumps(run(), indent=2))
    except Exception as error:
        print(json.dumps({"status": "failed", "error": str(error)}), file=sys.stderr)
        raise
