from __future__ import annotations

import json
import os
import tempfile
from pathlib import Path

ARTIFACT_DIR_ENV = "C311_ARTIFACT_DIR"
FE09_VIEWPORTS = ((1440, 900), (390, 844))
ADMIN_URL_ENV = "C311_ADMIN_URL"


def artifact_directory() -> Path:
    configured = os.environ.get(ARTIFACT_DIR_ENV)
    path = Path(configured) if configured else Path(tempfile.mkdtemp(prefix="c311-fe09-gate-"))
    if path.is_symlink():
        raise RuntimeError("artifact directory must not be a symbolic link")
    path.mkdir(parents=True, exist_ok=True)
    path.chmod(0o700)
    return path


def run_matrix() -> dict:
    from playwright.sync_api import Page, sync_playwright

    out = artifact_directory()
    admin_url = os.environ.get(ADMIN_URL_ENV, "http://127.0.0.1:18091")
    results = []

    def open_page(browser, role: str, scenario: str, route: str, width: int, height: int):
        page = browser.new_page(viewport={"width": width, "height": height})
        page.set_default_navigation_timeout(60000)
        page.add_init_script(
            "window.C311Mode='mock';"
            f"window.C311MockRole='{role}';"
            f"window.C311MockScenario='{scenario}';"
            "window.C311MockSession='current';"
        )
        errors, responses = [], []
        allowed_dev_resources = ("/code-snippets.js", "/custom.css")
        known_dev_console_errors = (
            "Failed to load resource: the server responded with a status of 500 (Internal Server Error)",
            "wss://api.cortezaproject.your-domain.tld/websocket",
            "Failed to load resource: net::ERR_CONNECTION_CLOSED",
        )
        page.on("pageerror", lambda error: errors.append(str(error)))
        page.on("console", lambda message: errors.append(message.text) if message.type == "error" and not any(item in message.text for item in allowed_dev_resources + known_dev_console_errors) else None)
        page.on("response", lambda response: responses.append({"url": response.url, "status": response.status}) if response.status >= 400 and not any(item in response.url for item in allowed_dev_resources) else None)
        page.goto(f"{admin_url}{route}", wait_until="commit")
        page.locator("[data-c311-main]").first.wait_for(timeout=30000)
        page.wait_for_timeout(250)
        return page, errors, responses

    def click(page: Page, selector: str) -> None:
        target = page.locator(selector).first
        if not target.count() or not target.is_visible() or not target.is_enabled():
            raise AssertionError(f"required control unavailable: {selector}")
        target.click()
        page.wait_for_timeout(150)

    def writes(page: Page, operation: str) -> int:
        return page.evaluate("operation => window.__C311MockProvider.getWriteCount(operation)", operation)

    def operation_status(page: Page) -> str:
        return page.locator("[data-c311-operation-status]").inner_text()

    def flow_workflow(page: Page, _out: Path) -> str:
        click(page, '[data-c311-action="workflow-edit"]')
        page.locator("#c311-workflow-name").fill("Fixture workflow edited")
        page.locator("#c311-workflow-conditions").fill('[{"status":"SUBMITTED"}]')
        page.locator("#c311-workflow-actions").fill('[{"action":"notify"}]')
        click(page, '[data-c311-action="workflow-save"]')
        click(page, '[data-c311-action="workflow-test"]')
        assert operation_status(page) == "SUCCEEDED"
        # The fixture starts active. Exercise both dedicated lifecycle operations
        # while preserving the update operation's active state semantics.
        click(page, '[data-c311-action="workflow-deactivate"]')
        click(page, '[data-c311-action="workflow-activate"]')
        assert writes(page, "workflow_update") == 1
        assert writes(page, "workflow_deactivate") == 1
        assert "ACTIVE" in page.locator("[data-c311-workflow]").first.inner_text()
        return "workflow save/test operation/activation results verified"

    def flow_workflow_terminal(page: Page, _out: Path) -> str:
        click(page, '[data-c311-action="workflow-test"]')
        assert operation_status(page) == "FAILED"
        assert page.locator('[data-c311-operation-error]').count() == 1
        return "workflow test terminal operation is observable"

    def flow_workflow_action(page: Page, _out: Path) -> str:
        page.locator("#c311-oauth-request").fill("request-fixture-001")
        page.locator("#c311-oauth-action").fill("notify_department")
        page.locator("#c311-oauth-payload").fill('{"channel":"EMAIL"}')
        click(page, '[data-c311-action="workflow-action-execute"]')
        status = page.locator("[data-c311-oauth-status]").inner_text()
        assert "execution-fixture-action" in status and "SUCCEEDED" in status, status
        assert '"succeeded": true' in page.locator("[data-c311-workflow-action-result]").inner_text()
        assert writes(page, "workflow_action_execute") == 1
        return "OAuth2 client-credentials workflow action and execution log verified"

    def flow_workflow_action_failure(page: Page, _out: Path) -> str:
        page.locator("#c311-oauth-request").fill("request-fixture-001")
        page.locator("#c311-oauth-action").fill("notify_department")
        page.locator("#c311-oauth-payload").fill('{"channel":"EMAIL"}')
        click(page, '[data-c311-action="workflow-action-execute"]')
        status = page.locator("[data-c311-oauth-status]").inner_text().lower()
        assert "temporarily unavailable" in status, status
        assert writes(page, "workflow_action_execute") == 0
        return "OAuth2 client-credentials failure is displayed without creating an execution"

    def flow_workflow_action_error(page: Page, expected: str) -> str:
        click(page, '[data-c311-action="workflow-action-execute"]')
        status = page.locator("[data-c311-oauth-status]").inner_text()
        assert expected in status, status
        assert writes(page, "workflow_action_execute") == 0
        return f"OAuth2 {expected} is displayed without creating an execution"

    def flow_workflow_invalid_client(page: Page, _out: Path) -> str:
        return flow_workflow_action_error(page, "INVALID_CLIENT")

    def flow_workflow_invalid_token(page: Page, _out: Path) -> str:
        return flow_workflow_action_error(page, "INVALID_TOKEN")

    def flow_workflow_scope_denied(page: Page, _out: Path) -> str:
        denied = page.locator("main").inner_text().lower()
        assert "403" in denied or "not available" in denied or "access denied" in denied or "permission" in denied
        return "workflow OAuth route is denied without workflow.execute scope"

    def flow_reports(page: Page, _out: Path) -> str:
        catalogue = page.locator("[data-c311-report-catalogue]").inner_text()
        assert page.locator("[data-c311-report-catalogue] li").count() >= 5 and "service_requests" in catalogue and "Request volume" in catalogue and "Resolution performance" in catalogue, catalogue
        page.locator("#c311-report-entity").select_option("service_requests")
        page.locator("#c311-report-name").fill("Too many columns")
        page.locator("#c311-report-columns").fill(",".join(f"column_{index}" for index in range(21)))
        assert page.locator('[data-c311-action="report-save"]').is_disabled()
        assert writes(page, "saved_report_create") == 0
        page.locator("#c311-report-name").fill("Fixture report")
        page.locator("#c311-report-columns").fill("request_number,summary,status")
        page.locator("#c311-report-sort").fill("-created_at,status")
        page.locator("#c311-report-filters").fill('{"status":"SUBMITTED"}')
        click(page, '[data-c311-action="report-save"]')
        report = page.locator('[data-c311-report-id="report-local-001"]')
        report.locator('[data-c311-action="report-run"]').click()
        page.wait_for_timeout(150)
        assert operation_status(page) == "SUCCEEDED"
        report.locator('[data-c311-action="report-share"]').click()
        report.locator('[data-c311-action="report-export"]').click()
        page.wait_for_timeout(150)
        assert writes(page, "saved_report_create") == 1
        assert writes(page, "saved_report_share") == 1
        csv = page.locator("[data-c311-report-csv]").inner_text()
        assert '"request_number","summary","status"' in csv and '"SR-2026-00001","Pothole on Example Street","SUBMITTED"' in csv and "\r\n" in csv, csv
        assert operation_status(page) == "SUCCEEDED"
        return "catalogue schema, report limits, share and provider CSV result verified"

    def flow_reports_terminal(page: Page, _out: Path) -> str:
        page.locator("#c311-report-name").fill("Terminal report")
        page.locator("#c311-report-columns").fill("request_number,status")
        click(page, '[data-c311-action="report-save"]')
        page.locator('[data-c311-report-id="report-local-001"] [data-c311-action="report-run"]').click()
        page.wait_for_timeout(150)
        assert operation_status(page) == "FAILED"
        assert page.locator('[data-c311-operation-error]').count() == 1
        return "report run terminal operation is observable"

    def fill_mail(page: Page) -> None:
        assert page.locator("#c311-mail-template option").count() >= 3
        page.locator("#c311-mail-template").select_option("service-update")
        page.locator("#c311-mail-to").fill("fixture@example.test")
        page.locator("#c311-mail-subject").fill("Fixture update")
        page.locator("#c311-mail-text").fill("Fixture message")

    def flow_mail_success(page: Page, _out: Path) -> str:
        fill_mail(page)
        page.locator("#c311-mail-subject").fill("Fixture update edited")
        page.locator("#c311-mail-html").fill("<p>Safe HTML</p><script>alert(1)</script>")
        click(page, '[data-c311-action="mail-template-save"]')
        assert writes(page, "mail_template_update") == 1
        click(page, '[data-c311-action="mail-preview"]')
        preview = page.locator("[data-c311-mail-preview]")
        assert "Safe HTML" in preview.inner_text() and "script" not in preview.inner_text().lower()
        click(page, '[data-c311-action="mail-send"]')
        assert writes(page, "mail_send") == 1
        assert "PENDING" in page.locator("[data-c311-mail-delivery]").inner_text()
        click(page, '[data-c311-action="mail-refresh"]')
        delivery = page.locator("[data-c311-mail-delivery]").inner_text()
        assert "DELIVERED" in delivery and page.locator("[data-c311-mail-attempts]").inner_text() == "2", delivery
        assert writes(page, "mail_send") == 1
        return "mail send idempotency and PENDING-to-DELIVERED polling verified"

    def flow_mail_retryable(page: Page, _out: Path) -> str:
        fill_mail(page)
        click(page, '[data-c311-action="mail-send"]')
        click(page, '[data-c311-action="mail-refresh"]')
        assert page.locator("[data-c311-mail-error]").count() == 1
        click(page, '[data-c311-action="mail-refresh"]')
        assert "DELIVERED" in page.locator("[data-c311-mail-delivery]").inner_text()
        assert writes(page, "mail_send") == 1
        return "retryable delivery polling preserves input and recovers without resend"

    def flow_mail_smtp_retry(page: Page, _out: Path) -> str:
        fill_mail(page)
        click(page, '[data-c311-action="mail-send"]')
        click(page, '[data-c311-action="mail-refresh"]')
        first = page.locator("[data-c311-mail-delivery]").inner_text()
        assert "PENDING" in first and page.locator("[data-c311-mail-attempts]").inner_text() == "2", first
        click(page, '[data-c311-action="mail-refresh"]')
        final = page.locator("[data-c311-mail-delivery]").inner_text()
        assert "DELIVERED" in final and page.locator("[data-c311-mail-attempts]").inner_text() == "3", final
        assert writes(page, "mail_send") == 1
        return "SMTP retry delivery reaches DELIVERED after at most three attempts"

    def flow_mail_terminal(page: Page, _out: Path) -> str:
        fill_mail(page)
        click(page, '[data-c311-action="mail-send"]')
        click(page, '[data-c311-action="mail-refresh"]')
        assert "TERMINAL_FAILURE" in page.locator("[data-c311-mail-delivery]").inner_text()
        assert page.locator('[data-c311-action="mail-refresh"]').count() == 0
        assert writes(page, "mail_send") == 1
        return "terminal delivery is final and cannot be resent by polling"

    def flow_mail_smtp_terminal(page: Page, _out: Path) -> str:
        fill_mail(page)
        click(page, '[data-c311-action="mail-send"]')
        click(page, '[data-c311-action="mail-refresh"]')
        delivery = page.locator("[data-c311-mail-delivery]").inner_text()
        assert "TERMINAL_FAILURE" in delivery and "SMTP" in delivery, delivery
        assert page.locator('[data-c311-action="mail-refresh"]').count() == 0
        assert writes(page, "mail_send") == 1
        return "SMTP terminal failure preserves the final error and stops polling"

    def flow_calendar(page: Page, out: Path) -> str:
        fixture_ics = out / "fixture-calendar.ics"
        fixture_ics.write_text("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:fe09-browser-001\r\nSUMMARY:Fixture event\r\nDESCRIPTION:Fixture description\r\nDTSTART;TZID=America/New_York:20260115T100000\r\nDTEND;TZID=America/New_York:20260115T110000\r\nRRULE:FREQ=DAILY;COUNT=2\r\nSTATUS:CONFIRMED\r\nLAST-MODIFIED:20260115T090000Z\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n", encoding="utf-8")
        page.locator('[data-c311-action="calendar-import"]').set_input_files(str(fixture_ics))
        page.wait_for_timeout(150)
        assert operation_status(page) == "SUCCEEDED"
        click(page, '[data-c311-action="calendar-update"]')
        click(page, '[data-c311-action="calendar-cancel"]')
        assert writes(page, "calendar_import") == 3
        assert "CANCELLED" in page.locator("[data-c311-calendar-events]").inner_text()
        calendar_content = page.locator("[data-c311-calendar-content]").inner_text()
        assert "DTSTART;TZID=America/New_York" in calendar_content and "DTEND;TZID=America/New_York" in calendar_content and "DESCRIPTION:Fixture description" in calendar_content and "RRULE:FREQ=DAILY;COUNT=2" in calendar_content and "LAST-MODIFIED:20260115T090000Z" in calendar_content
        assert '"cancelled": 1' in page.locator("[data-c311-operation-result]").inner_text()
        page.reload(wait_until="commit")
        page.locator("[data-c311-calendar-events]").wait_for(timeout=30000)
        assert "CANCELLED" in page.locator("[data-c311-calendar-events]").inner_text()
        return "same-UID update and STATUS:CANCELLED import survive refresh"

    def flow_calendar_invalid(page: Page, out: Path) -> str:
        fixture_ics = out / "invalid-calendar.ics"
        fixture_ics.write_text("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:fe09-invalid\r\nSUMMARY:Missing date\r\nDTSTART:not-a-date\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n", encoding="utf-8")
        page.locator('[data-c311-action="calendar-import"]').set_input_files(str(fixture_ics))
        page.wait_for_timeout(150)
        assert "calendar" in page.locator("[data-c311-error]").inner_text().lower() or "invalid" in page.locator("[data-c311-error]").inner_text().lower()
        assert writes(page, "calendar_import") == 0
        return "invalid ICS is rejected without a calendar write"

    def flow_calendar_retryable(page: Page, out: Path) -> str:
        fixture_ics = out / "retryable-calendar.ics"
        fixture_ics.write_text("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:fe09-retryable\r\nSUMMARY:Retryable event\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n", encoding="utf-8")
        page.locator('[data-c311-action="calendar-import"]').set_input_files(str(fixture_ics))
        page.wait_for_timeout(150)
        assert "calendar" in page.locator("[data-c311-error]").inner_text().lower() or "invalid" in page.locator("[data-c311-error]").inner_text().lower()
        assert writes(page, "calendar_import") == 0
        return "retryable ICS failure is displayed without a write"

    def flow_audit(page: Page, _out: Path) -> str:
        page.locator("#c311-audit-event").fill("REQUEST_CREATED")
        page.locator("#c311-audit-actor").fill("actor-fixture-manager")
        click(page, '[data-c311-action="audit-filter"]')
        assert page.locator("[data-c311-audit] li").count() == 1
        click(page, '[data-c311-action="audit-export"]')
        assert operation_status(page) == "SUCCEEDED"
        audit_result = page.locator("[data-c311-operation-result]").inner_text()
        assert "download_url" in audit_result and '"exported": 1' in audit_result and '"event_type"' in audit_result
        page.locator("#c311-contact-email-filters").fill('{"primary_category":"RESIDENT"}')
        click(page, '[data-c311-action="contact-email-export"]')
        assert operation_status(page) == "SUCCEEDED"
        assert '"exported_count": 1' in page.locator("[data-c311-operation-result]").inner_text()
        assert writes(page, "contact_email_export") == 1
        page.locator("#c311-export-entity").select_option("constituents")
        page.locator("#c311-export-filters").fill('{"email":"alex@example.test"}')
        click(page, '[data-c311-action="data-export"]')
        assert "alex@example.test" in page.locator("[data-c311-data-export-result]").inner_text()
        page.reload(wait_until="commit")
        page.locator("#c311-audit-event").wait_for(timeout=30000)
        assert page.locator("#c311-audit-event").input_value() == "REQUEST_CREATED"
        return "audit filters persist and audit/constituent export results are observable"

    def flow_audit_terminal(page: Page, _out: Path) -> str:
        click(page, '[data-c311-action="audit-export"]')
        assert operation_status(page) == "FAILED"
        assert page.locator('[data-c311-operation-error]').count() == 1
        return "audit export terminal operation is observable"

    def flow_contact_terminal(page: Page, _out: Path) -> str:
        click(page, '[data-c311-action="contact-email-export"]')
        assert operation_status(page) == "FAILED"
        assert page.locator('[data-c311-operation-error]').count() == 1
        return "contact email export terminal operation is observable"

    def flow_export_invalid_token(page: Page, _out: Path) -> str:
        page.locator("#c311-export-token").fill("invalid")
        click(page, '[data-c311-action="data-export"]')
        assert "page token is invalid" in page.locator("[data-c311-error]").inner_text().lower()
        return "invalid export page token is rejected"

    def flow_export_rate_limited(page: Page, _out: Path) -> str:
        click(page, '[data-c311-action="data-export"]')
        assert "too many export requests" in page.locator("[data-c311-error]").inner_text().lower()
        return "rate-limited export is displayed as retryable"

    def flow_export_invalid_filter(page: Page, _out: Path) -> str:
        page.locator("#c311-export-filters").fill('{"not_a_filter":true}')
        click(page, '[data-c311-action="data-export"]')
        assert "not supported" in page.locator("[data-c311-error]").inner_text().lower()
        return "entity export rejects unsupported filters"

    def flow_export_terminal(page: Page, _out: Path) -> str:
        click(page, '[data-c311-action="data-export"]')
        assert page.locator("[data-c311-data-export-result]").count() == 1
        assert page.locator("[data-c311-error]").count() == 0
        return "data export remains a synchronous contract response without undeclared terminal HTTP failure"

    flows = (
        ("workflow_designer", "success", "/c311/staff/workflows", flow_workflow),
        ("workflow_designer", "terminal", "/c311/staff/workflows", flow_workflow_terminal),
        ("workflow_designer", "success", "/c311/staff/oauth", flow_workflow_action),
        ("workflow_designer", "retryable", "/c311/staff/oauth", flow_workflow_action_failure),
        ("workflow_designer", "invalid-client", "/c311/staff/oauth", flow_workflow_invalid_client),
        ("workflow_designer", "invalid-token", "/c311/staff/oauth", flow_workflow_invalid_token),
        ("constituent", "success", "/c311/staff/oauth", flow_workflow_scope_denied),
        ("department_manager", "success", "/c311/staff/reports", flow_reports),
        ("department_manager", "terminal", "/c311/staff/reports", flow_reports_terminal),
        ("department_manager", "success", "/c311/staff/mail", flow_mail_success),
        ("department_manager", "retryable", "/c311/staff/mail", flow_mail_retryable),
        ("department_manager", "terminal", "/c311/staff/mail", flow_mail_terminal),
        ("department_manager", "smtp-421", "/c311/staff/mail", flow_mail_smtp_retry),
        ("department_manager", "smtp-451", "/c311/staff/mail", flow_mail_smtp_retry),
        ("department_manager", "smtp-550", "/c311/staff/mail", flow_mail_smtp_terminal),
        ("department_manager", "smtp-553", "/c311/staff/mail", flow_mail_smtp_terminal),
        ("department_manager", "success", "/c311/staff/calendar", flow_calendar),
        ("department_manager", "success", "/c311/staff/calendar", flow_calendar_invalid),
        ("department_manager", "retryable", "/c311/staff/calendar", flow_calendar_retryable),
        ("platform_administrator", "success", "/c311/staff/audit", flow_audit),
        ("platform_administrator", "terminal", "/c311/staff/audit", flow_audit_terminal),
        ("platform_administrator", "terminal", "/c311/staff/audit", flow_contact_terminal),
        ("platform_administrator", "success", "/c311/staff/audit", flow_export_invalid_token),
        ("platform_administrator", "rate-limited", "/c311/staff/audit", flow_export_rate_limited),
        ("platform_administrator", "success", "/c311/staff/audit", flow_export_invalid_filter),
        ("platform_administrator", "terminal", "/c311/staff/audit", flow_export_terminal),
    )

    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(headless=True)
        for role, scenario, route, flow in flows:
            for width, height in FE09_VIEWPORTS:
                page, errors, responses = open_page(browser, role, scenario, route, width, height)
                assertion = ""
                try:
                    assertion = flow(page, out)
                except Exception as error:
                    errors.append(f"assertion: {error}")
                overflow = page.evaluate("document.documentElement.scrollWidth > window.innerWidth")
                heading_count = page.locator("main h1").count()
                passed = heading_count == 1 and not overflow and not errors and not responses and bool(assertion)
                results.append({"role": role, "scenario": scenario, "route": route, "viewport": [width, height], "passed": passed, "assertion": assertion, "heading_count": heading_count, "overflow": overflow, "errors": errors, "unexpected_responses": responses})
                page.screenshot(path=str(out / f"{role}-{scenario}-{route.rsplit('/', 1)[-1]}-{width}.png"), full_page=True)
                page.close()
        browser.close()

    payload = {"mode": "mock-only", "viewports": [list(viewport) for viewport in FE09_VIEWPORTS], "results": results}
    (out / "fe09-matrix.json").write_text(json.dumps(payload, indent=2), encoding="utf-8")
    if not all(item["passed"] for item in results):
        raise SystemExit(1)
    return payload


if __name__ == "__main__":
    run_matrix()
