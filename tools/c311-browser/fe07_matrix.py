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
    open_page(page, ADMIN_URL, "/c311/staff", scenario="pagination" if role == "supervisor" else "success", role=role)
    if role == "workflow_designer":
        check("403" in page.url or "access" in page.locator(MAIN).inner_text().lower(), f"{label} did not reject queue access")
        return
    page.locator("[data-c311-staff-filters]").wait_for(state="visible")
    responsive_check(page, width, label)
    for selector in ("#c311-filter-status", "#c311-filter-department", "#c311-filter-assignee", "#c311-filter-sort", "#c311-filter-page-size"):
        check(page.locator(selector).count() == 1, f"{label} missing {selector}")
    page.locator("#c311-filter-department").select_option("STREETS")
    page.locator('[data-c311-action="apply-staff-filters"]').click()
    check(page.locator('[data-c311-department="STREETS"]:visible').count() >= 1, f"{label} filter did not change results")
    page.locator("#c311-filter-department").select_option("GENERAL_SERVICES")
    page.locator('[data-c311-action="apply-staff-filters"]').click()
    page.locator('[data-c311-data-state][data-state="empty"]').wait_for(state="visible")
    check(page.locator("#c311-filter-department").input_value() == "GENERAL_SERVICES", f"{label} filter was not retained")
    if role == "supervisor":
        page.locator("#c311-filter-department").select_option("STREETS")
        page.locator('[data-c311-action="apply-staff-filters"]').click()
        page.locator('[data-c311-selection="request-fixture-001"]:visible').first.check()
        page.locator('[data-c311-selection="request-fixture-002"]:visible').first.check()
        page.locator("#c311-bulk-assignee").fill("actor-fixture-agent")
        page.locator("#c311-bulk-priority").fill("HIGH")
        page.locator("#c311-bulk-note").fill("Reviewed as one atomic batch")
        page.locator('[data-c311-action="bulk-open-confirm"]').click()
        page.locator('[data-c311-bulk-confirmation]').wait_for(state="visible")
        check(page.locator('[data-c311-bulk-count]').inner_text() == "2", f"{label} confirmation omitted selected count")
        check("primary_assignee_id" in page.locator('[data-c311-bulk-changes]').inner_text(), f"{label} confirmation omitted changes")
        page.locator('[data-c311-action="bulk-confirm"]').click()
        page.locator('[data-c311-bulk-message]').wait_for(state="visible")
        check(page.locator('[data-c311-bulk-message]').inner_text().startswith("2 "), f"{label} bulk update did not commit")
        bulk_state = page.evaluate("""async () => {
          const shell = document.querySelector('[data-c311-app-shell]')
          const provider = shell && shell.__vue__ && shell.__vue__.$C311 && shell.__vue__.$C311.provider
          const details = await Promise.all(['request-fixture-001', 'request-fixture-002'].map(id => provider.getStaffRequest(id)))
          return details.map(detail => ({ priority: detail.request.priority, notes: (detail.notes || []).map(note => note.body) }))
        }""")
        check(all(item == {"priority": "HIGH", "notes": ["Reviewed as one atomic batch"]} for item in bulk_state), f"{label} bulk priority/note projection mismatch: {bulk_state}")
        atomic_failure = page.evaluate("""async () => {
          const shell = document.querySelector('[data-c311-app-shell]')
          const provider = shell.__vue__.$C311.provider
          const project = detail => ({ version: detail.request.version, priority: detail.request.priority, assignee: detail.primary_assignee_id, notes: (detail.notes || []).map(note => note.body) })
          const before = project(await provider.getStaffRequest('request-fixture-001'))
          const writesBefore = provider.getWriteCount('staff_request_bulk')
          let failure = null
          try {
            await provider.bulkStaffRequests({ action: 'UPDATE', changes: { priority: 'CRITICAL' }, request_items: [
              { request_id: 'request-fixture-001', expected_version: before.version },
              { request_id: 'request-fixture-missing', expected_version: 1 },
            ] }, { idempotencyKey: 'browser-atomic-failure' })
          } catch (error) {
            failure = { code: error.code, status: error.status, failing_request_id: error.failing_request_id }
          }
          const after = project(await provider.getStaffRequest('request-fixture-001'))
          return { before, after, failure, writesBefore, writesAfter: provider.getWriteCount('staff_request_bulk') }
        }""")
        check(atomic_failure["failure"] == {"code": "NOT_FOUND", "status": 404, "failing_request_id": "request-fixture-missing"}, f"{label} did not report the failing bulk record: {atomic_failure}")
        check(atomic_failure["before"] == atomic_failure["after"] and atomic_failure["writesBefore"] == atomic_failure["writesAfter"], f"{label} partially mutated a failed bulk operation: {atomic_failure}")
    elif role == "department_manager":
        page.locator("#c311-filter-department").select_option("STREETS")
        page.locator('[data-c311-action="apply-staff-filters"]').click()
        page.locator('[data-c311-department="STREETS"]:visible').first.wait_for(state="visible")
        check(page.locator('[data-c311-bulk-toolbar]').count() == 1, f"{label} manager bulk action missing")
    else:
        check(page.locator('[data-c311-bulk-toolbar]').count() == 0, f"{label} exposed an unauthorized bulk action")


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
        page.locator('[data-c311-action="reassign-request"]').click()
        page.locator("#c311-assignee").fill("actor-fixture-agent")
        page.locator("#c311-reassign-reason").fill("Assign to the fixture agent")
        page.locator('[data-c311-action="submit-reassign"]').click()
        page.locator('[data-c311-action-message]').wait_for(state="visible")
        check("reassigned" in page.locator('[data-c311-action-message]').inner_text().lower(), f"{label} reassignment did not complete")

        page.locator('[data-c311-action="manage-collaborators"]').click()
        page.locator("#c311-collaborator-id").fill("staff-fixture-collaborator")
        page.locator("#c311-collaborator-reason").fill("Specialist review")
        page.locator('[data-c311-action="add-collaborator"]').click()
        page.locator('[data-c311-action="remove-collaborator-staff-fixture-collaborator"]').wait_for(state="visible")
        page.locator('[data-c311-action="remove-collaborator-staff-fixture-collaborator"]').click()
        page.locator('[data-c311-action="remove-collaborator-staff-fixture-collaborator"]').wait_for(state="detached")

        page.locator('[data-c311-action="snooze-reminder-reminder-fixture-001"]').click()
        page.locator("#c311-reminder-action-due").fill("2026-01-17T15:00")
        page.locator('[data-c311-action="submit-reminder-action"]').click()
        page.locator('[data-c311-reminder-history]').wait_for(state="visible")
        page.locator('[data-c311-action="complete-reminder-reminder-fixture-002"]').click()
        page.locator('[data-c311-action="complete-reminder-reminder-fixture-002"]').wait_for(state="detached")
        page.locator('[data-c311-action="cancel-reminder-reminder-fixture-003"]').click()
        page.locator('[data-c311-action="cancel-reminder-reminder-fixture-003"]').wait_for(state="detached")

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
    if role == "service_agent" and width == 1440:
        page.locator('[data-c311-action="create-reminder"]').click()
        page.locator("#c311-reminder-title").fill("Browser fixture reminder")
        page.locator("#c311-reminder-due").fill("2026-01-18T15:00")
        page.locator("#c311-reminder-recipient").fill("actor-fixture-agent")
        page.locator("#c311-reminder-channel").select_option("EMAIL")
        page.locator('[data-c311-action="submit-reminder"]').click()
        page.locator('[data-c311-action="create-reminder"]').wait_for(state="visible")
        check("Browser fixture reminder" in page.locator('[data-c311-reminders]').inner_text(), f"{label} reminder creation did not complete")
        reminder_item = page.locator('[data-c311-reminder-id]').filter(has_text="Browser fixture reminder")
        check("actor-fixture-agent" in reminder_item.locator('[data-c311-reminder-recipient]').inner_text(), f"{label} reminder recipient is not displayed")
        check("EMAIL" in reminder_item.locator('[data-c311-reminder-channel]').inner_text(), f"{label} reminder channel is not displayed")
        reminder_channel = page.evaluate("""async () => {
          const shell = document.querySelector('[data-c311-app-shell]')
          const detail = await shell.__vue__.$C311.provider.getStaffRequest('request-fixture-001')
          return detail.reminders.find(item => item.title === 'Browser fixture reminder').channel
        }""")
        check(reminder_channel == "EMAIL", f"{label} reminder channel selection was ignored")
    triage = page.locator('[data-c311-action="transition-request"]')
    if triage.count():
        triage.click()
        page.locator('[data-c311-form="triage"]').wait_for(state="visible")
        for marker in ("[data-c311-triage-service-type]", "[data-c311-triage-department]", "[data-c311-triage-origin]", "[data-c311-triage-location]", "[data-c311-triage-duplicate]"):
            check(page.locator(marker).count() == 1, f"{label} triage confirmation omitted {marker}")
        page.locator("#c311-triage-reason").fill("Triage details reviewed")
        page.locator('[data-c311-action="confirm-triage-details"]').check()
        page.locator('[data-c311-action="submit-triage"]').click()
        page.locator('[data-c311-status-value]').filter(has_text="TRIAGED").wait_for(state="visible")
        page.locator('[data-c311-action="transition-status"]').click()
        check(page.locator("#c311-transition-status").input_value() == "ASSIGNED", f"{label} did not offer ASSIGN after triage")
        page.locator("#c311-transition-reason").fill("Ready for assignment")
        page.locator('[data-c311-action="submit-transition"]').click()
        page.locator('[data-c311-status-value]').filter(has_text="ASSIGNED").wait_for(state="visible")
        if role == "supervisor" and width == 1440:
            check(page.locator('[data-c311-action="reassign-request"]').count() == 1, f"{label} reassignment disappeared after ASSIGNED")
            reassign_form = page.locator('[data-c311-form="reassign"]')
            if not reassign_form.is_visible():
                page.locator('[data-c311-action="reassign-request"]').click()
            page.locator("#c311-assignee").fill("staff-fixture-after-assignment")
            page.locator("#c311-reassign-reason").fill("Specialist required after assignment")
            page.locator('[data-c311-action="submit-reassign"]').click()
            page.locator('[data-c311-action-message]').wait_for(state="visible")
            reassignment = page.evaluate("""async () => {
              const shell = document.querySelector('[data-c311-app-shell]')
              const detail = await shell.__vue__.$C311.provider.getStaffRequest('request-fixture-001')
              const assignments = detail.audit.filter(event => event.action === 'ASSIGN')
              return { status: detail.request.status, assignee: detail.primary_assignee_id, audit: assignments[assignments.length - 1], notifications: detail.assignment_notifications.slice(-2), work_order: detail.external_work_order }
            }""")
            check(reassignment["status"] == "ASSIGNED", f"{label} reassignment changed status")
            check(reassignment["assignee"] == "staff-fixture-after-assignment", f"{label} reassignment did not apply")
            check(reassignment["audit"].get("reason") == "Specialist required after assignment" and reassignment["audit"].get("previous_assignee_id") == "actor-fixture-agent", f"{label} reassignment audit context missing: {reassignment}")
            check([(item["recipient_staff_id"], item["recipient_role"], item["result"]) for item in reassignment["notifications"]] == [("actor-fixture-agent", "FORMER_PRIMARY_ASSIGNEE", "SENT"), ("staff-fixture-after-assignment", "NEW_PRIMARY_ASSIGNEE", "SENT")], f"{label} reassignment notification results missing: {reassignment}")
            check(reassignment["work_order"]["service_request_number"] == "SR-2026-00001" and reassignment["work_order"]["external_status_url"].startswith("https://") and reassignment["work_order"]["created_at"], f"{label} CivicWorks projection incomplete: {reassignment}")
            check(page.locator('[data-c311-work-order-number]').inner_text() == "SR-2026-00001", f"{label} work-order number is not rendered")
            check(page.locator('[data-c311-work-order-url]').get_attribute("href").startswith("https://"), f"{label} work-order URL is not rendered")
        if role == "service_agent" and width == 1440:
            civicworks = page.evaluate("""async () => {
              const shell = document.querySelector('[data-c311-app-shell]')
              const runtime = shell && shell.__vue__ && shell.__vue__.$C311
              if (!runtime) throw new Error('C311 mock runtime is unavailable')
              const before = await runtime.provider.getStaffRequest('request-fixture-001')
              const base = { event_type: 'work_order.status_changed', work_order_id: before.external_work_order.work_order_id, source_case_id: 'request-fixture-001', previous_status: 'ASSIGNED', occurred_at: '2026-01-15T15:00:00.000Z' }
              let invalidSignature = null
              try {
                const invalid = { ...base, event_id: 'browser-cw-invalid-signature', status: 'IN_PROGRESS', version: 2 }
                await runtime.provider.processCivicWorksEvent(invalid, invalid.event_id, 'invalid-signature')
              } catch (error) { invalidSignature = { code: error.code, status: error.status } }
              const afterInvalid = await runtime.provider.getStaffRequest('request-fixture-001')
              const old = { ...base, event_id: 'browser-cw-old', status: 'IN_PROGRESS', version: 1 }
              const oldResult = await runtime.provider.processCivicWorksEvent(old, old.event_id, 'fixture-signature')
              const oldRetry = await runtime.provider.processCivicWorksEvent(old, old.event_id, 'fixture-signature')
              const afterOld = await runtime.provider.getStaffRequest('request-fixture-001')
              const event = { ...base, event_id: 'browser-cw-complete', status: 'COMPLETED', version: 2 }
              const result = await runtime.provider.processCivicWorksEvent(event, event.event_id, 'fixture-signature')
              const retry = await runtime.provider.processCivicWorksEvent(event, event.event_id, 'fixture-signature')
              const current = await runtime.provider.getStaffRequest('request-fixture-001')
              return {
                invalidSignature, afterInvalid: afterInvalid.request.status,
                oldResult, oldRetry, afterOld: afterOld.request.status,
                result, retry, status: current.request.status,
                history: current.history.slice(-2).map(item => item.action),
                transitionCounts: { inProgress: current.history.filter(item => item.action === 'IN_PROGRESS').length, resolved: current.history.filter(item => item.action === 'RESOLVED').length },
                writes: runtime.provider.getWriteCount('civicworks_event_callback'), work_order: current.external_work_order,
              }
            }""")
            check(civicworks["invalidSignature"] == {"code": "INVALID_SIGNATURE", "status": 401} and civicworks["afterInvalid"] == "ASSIGNED", f"{label} invalid CivicWorks signature changed CRM state: {civicworks}")
            check(civicworks["oldResult"] == {"acknowledged": True} and civicworks["oldRetry"] == {"acknowledged": True, "duplicate": True} and civicworks["afterOld"] == "ASSIGNED", f"{label} old CivicWorks event changed CRM state: {civicworks}")
            check(civicworks["result"] == {"acknowledged": True} and civicworks["retry"] == {"acknowledged": True, "duplicate": True}, f"{label} CivicWorks retry was not idempotent: {civicworks}")
            check(civicworks["status"] == "RESOLVED" and civicworks["history"] == ["IN_PROGRESS", "RESOLVED"], f"{label} CivicWorks completion was not normalized: {civicworks}")
            check(civicworks["transitionCounts"] == {"inProgress": 1, "resolved": 1} and civicworks["writes"] == 1, f"{label} CivicWorks retry produced duplicate CRM effects: {civicworks}")
            check(civicworks["work_order"]["status"] == "COMPLETED" and civicworks["work_order"]["service_request_number"] == "SR-2026-00001" and civicworks["work_order"]["external_status_url"].startswith("https://"), f"{label} CivicWorks projection did not refresh: {civicworks}")
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

    open_page(page, ADMIN_URL, DETAIL_PATH, scenario="version-conflict", role="supervisor")
    page.locator('[data-c311-action="reassign-request"]').click()
    page.locator("#c311-assignee").fill("actor-fixture-agent")
    page.locator("#c311-reassign-reason").fill("Keep this reassignment")
    page.locator('[data-c311-action="submit-reassign"]').click()
    page.locator('[data-c311-conflict-recovery]').wait_for(state="visible")
    check("2" in page.locator('[data-c311-current-version]').inner_text(), "detail conflict omitted current_version")
    check(page.locator('[data-c311-action="reapply-action"]').count() == 0, "detail conflict allowed reapply before reload")
    page.locator('[data-c311-action="reload-current-version"]').click()
    page.locator('[data-c311-action="reapply-action"]').wait_for(state="visible")
    check(page.locator("#c311-assignee").input_value() == "actor-fixture-agent", "detail conflict reload lost valid input")
    page.locator('[data-c311-action="reapply-action"]').click()
    page.locator('[data-c311-action-message]').wait_for(state="visible")
    check("reassigned" in page.locator('[data-c311-action-message]').inner_text().lower(), "detail conflict reapply did not succeed")
    single_state = page.evaluate("""async () => {
      const shell = document.querySelector('[data-c311-app-shell]')
      const detail = await shell.__vue__.$C311.provider.getStaffRequest('request-fixture-001')
      return { version: detail.request.version, assignee: detail.primary_assignee_id }
    }""")
    check(single_state == {"version": 3, "assignee": "actor-fixture-agent"}, f"detail conflict recovery state mismatch: {single_state}")

    open_page(page, ADMIN_URL, "/c311/staff", scenario="bulk-version-conflict", role="supervisor")
    page.locator('[data-c311-selection="request-fixture-001"]:visible').first.check()
    page.locator("#c311-bulk-note").fill("Keep this batch note")
    page.locator('[data-c311-action="bulk-open-confirm"]').click()
    page.locator('[data-c311-action="bulk-confirm"]').click()
    page.locator('[data-c311-bulk-error]').wait_for(state="visible")
    check("request-fixture-001" in page.locator('[data-c311-bulk-failing-request]').inner_text(), "bulk conflict omitted failing_request_id")
    check("2" in page.locator('[data-c311-bulk-current-version]').inner_text(), "bulk conflict omitted current_version")
    check(page.locator('[data-c311-action="reapply-bulk"]').count() == 0, "bulk conflict allowed reapply before reload")
    page.locator('[data-c311-action="reload-bulk-version"]').click()
    page.locator('[data-c311-action="reapply-bulk"]').wait_for(state="visible")
    check(page.locator("#c311-bulk-note").input_value() == "Keep this batch note", "bulk conflict reload lost valid input")
    page.locator('[data-c311-action="reapply-bulk"]').click()
    page.locator('[data-c311-bulk-message]').wait_for(state="visible")
    check(page.locator('[data-c311-bulk-message]').inner_text().startswith("1 "), "bulk conflict reapply did not succeed")
    bulk_state = page.evaluate("""async () => {
      const shell = document.querySelector('[data-c311-app-shell]')
      const detail = await shell.__vue__.$C311.provider.getStaffRequest('request-fixture-001')
      return { version: detail.request.version, notes: (detail.notes || []).map(note => note.body) }
    }""")
    check(bulk_state == {"version": 3, "notes": ["Keep this batch note"]}, f"bulk conflict recovery state mismatch: {bulk_state}")


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
