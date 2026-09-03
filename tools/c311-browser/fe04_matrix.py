#!/usr/bin/env python3
"""Repeatable FE-04 attachment and location checks using the C311 mock provider."""

from __future__ import annotations

import json
import os
import sys
from datetime import datetime, timezone
from pathlib import Path

from playwright.sync_api import sync_playwright

from fe03_matrix import (
    ADMIN_URL,
    COMPOSE_URL,
    ERROR_SUMMARY,
    MAIN,
    VIEWPORTS,
    artifact_directory,
    assert_page,
    check,
    diagnostics,
    finalize_diagnostics,
    open_page,
)

SUBMIT_PATH = "/c311/submit"
STAFF_SUBMIT_PATH = "/c311/staff/submit"


def check_request_secrets(result: dict[str, list], request) -> None:
    sensitive_names = ("authorization", "map_api_token", "access_token", "client_secret")
    headers = {name.lower() for name in request.headers}
    if any(name in headers for name in sensitive_names):
        result["unexpected_console_errors"].append(f"sensitive request header on {request.url}")
    if any(name in request.url.lower() for name in sensitive_names):
        result["unexpected_console_errors"].append(f"sensitive request URL: {request.url}")


def fill_form(page) -> None:
    page.locator("#c311-service-type").select_option("POTHOLE")
    page.locator("#c311-summary").fill("Pothole near the library")
    page.locator("#c311-description").fill("The road surface is damaged near the library entrance.")
    page.locator("#c311-requester-name").fill("Fixture Resident")
    page.locator("#c311-requester-email").fill("resident@example.test")
    page.locator("#c311-consent").check()


def check_location(page, base_url: str) -> None:
    page.locator("#c311-location-address").fill("  100   example street,   buffalo, ny 14201  ")
    page.locator('[data-c311-action="geocode-address"]').click()
    page.locator('[data-c311-normalized-address]').wait_for(state="visible")
    check(page.locator('[data-c311-normalized-address]').inner_text() == "100 Example Street, Buffalo, NY 14201", "normalized address missing")
    check(page.locator('[data-c311-map]').count() == 1, "deterministic map panel missing")
    page.locator('[data-c311-action="move-marker-east"]').click()
    page.locator('[data-c311-action="confirm-location"]').click()
    check(page.locator('[data-c311-location-confirmed]').count() == 1, "location confirmation missing")


def check_attachments(page, can_download: bool = True) -> None:
    page.set_input_files("#c311-attachment-file", {"name": "fixture.txt", "mimeType": "text/plain", "buffer": b"fixture"})
    page.locator('[data-c311-attachment-list]').wait_for(state="visible")
    check(page.locator('[data-c311-attachment-token-status]').count() == 1, "attachment token status missing")
    if can_download:
        page.locator('[data-c311-action="preview-attachment"]').click()
        page.locator('[data-c311-attachment-preview]').wait_for(state="visible")
        check(page.locator('[data-c311-attachment-preview-content]').inner_text() == "fixture attachment", "attachment preview did not consume the response body")
        page.locator('[data-c311-action="close-attachment-preview"]').click()
    else:
        check(page.locator('[data-c311-action="download-attachment"]').count() == 0, "download action exposed without capability")
        check(page.locator('[data-c311-action="preview-attachment"]').count() == 0, "preview action exposed without capability")
    page.locator('[data-c311-action="remove-attachment"]').click()
    check(page.locator('[data-c311-attachment-list]').count() == 0, "attachment was not removed")
    page.set_input_files("#c311-attachment-file", {"name": "replacement.txt", "mimeType": "text/plain", "buffer": b"replacement"})
    page.locator('[data-c311-attachment-list]').wait_for(state="visible")
    check(page.locator('[data-c311-attachment-name]').inner_text() == "replacement.txt", "removed attachment could not be replaced")


def check_download_capability(page, base_url: str) -> None:
    open_page(page, base_url, SUBMIT_PATH, role="public_visitor")
    page.set_input_files("#c311-attachment-file", {"name": "anonymous.txt", "mimeType": "text/plain", "buffer": b"fixture"})
    page.locator('[data-c311-attachment-list]').wait_for(state="visible")
    check(page.locator('[data-c311-action="download-attachment"]').count() == 0, "anonymous download action exposed")
    check(page.locator('[data-c311-action="preview-attachment"]').count() == 0, "anonymous preview action exposed")

    open_page(page, base_url, SUBMIT_PATH, role="constituent")
    page.set_input_files("#c311-attachment-file", {"name": "authorized.txt", "mimeType": "text/plain", "buffer": b"fixture"})
    page.locator('[data-c311-attachment-list]').wait_for(state="visible")
    check(page.locator('[data-c311-action="download-attachment"]').count() == 1, "authorized download action missing")
    check(page.locator('[data-c311-action="preview-attachment"]').count() == 1, "authorized preview action missing")


def check_route_attachment_lifecycle(page, base_url: str) -> None:
    open_page(page, base_url, SUBMIT_PATH, role="public_visitor")
    for index in range(5):
        page.set_input_files("#c311-attachment-file", {"name": f"route-{index}.txt", "mimeType": "text/plain", "buffer": b"fixture"})
        page.wait_for_function("count => document.querySelectorAll('[data-c311-attachment-name]').length >= count", arg=index + 1)
    page.once("dialog", lambda dialog: dialog.accept())
    page.locator('[data-c311-route="/c311"]').click()
    page.wait_for_url("**/c311")
    page.locator('[data-c311-route="/c311/submit"]').click()
    page.wait_for_url(f"**{SUBMIT_PATH}")
    page.set_input_files("#c311-attachment-file", {"name": "after-route.txt", "mimeType": "text/plain", "buffer": b"fixture"})
    page.locator('[data-c311-attachment-list]').wait_for(state="visible")
    check(page.locator('[data-c311-attachment-name]').inner_text() == "after-route.txt", "staged tokens leaked across request routes")


def check_invalid_attachment(page, base_url: str) -> None:
    open_page(page, base_url, SUBMIT_PATH)
    page.locator("#c311-summary").fill("Keep this summary after invalid attachment")
    page.set_input_files("#c311-attachment-file", {"name": "blocked.zip", "mimeType": "application/zip", "buffer": b"fixture"})
    page.locator('[data-c311-attachment-error]').wait_for(state="visible")
    check(page.locator('[data-c311-attachment-list]').count() == 0, "invalid attachment was added")
    check(page.locator("#c311-summary").input_value() == "Keep this summary after invalid attachment", "invalid attachment cleared request input")
    check(page.locator('[data-c311-submission-result]').count() == 0, "invalid attachment created a request")


def check_address_not_found(page, base_url: str) -> None:
    open_page(page, base_url, SUBMIT_PATH, scenario="not-found")
    page.locator("#c311-summary").fill("Keep this summary after missing address")
    page.locator("#c311-location-address").fill("Unknown fixture address")
    page.locator('[data-c311-action="geocode-address"]').click()
    page.locator('[data-c311-map-error]').wait_for(state="visible")
    check("could not be found" in page.locator('[data-c311-map-error]').inner_text().lower(), "address-not-found error was not shown")
    check(page.locator("#c311-location-address").input_value() == "Unknown fixture address", "address-not-found cleared address input")
    check(page.locator("#c311-summary").input_value() == "Keep this summary after missing address", "address-not-found cleared request input")


def check_map_failures(page, base_url: str) -> None:
    open_page(page, base_url, SUBMIT_PATH, scenario="map-retryable")
    page.locator("#c311-summary").fill("Keep this summary while the map retries")
    page.locator("#c311-location-address").fill("100 Example Street, Buffalo, NY 14201")
    page.locator('[data-c311-action="geocode-address"]').click()
    page.locator('[data-c311-map-error]').wait_for(state="visible")
    check(page.locator('[data-c311-action="retry-geocode"]').count() == 1, "map retry action missing")
    check(page.locator("#c311-summary").input_value() == "Keep this summary while the map retries", "map failure cleared request input")
    open_page(page, base_url, SUBMIT_PATH, scenario="map-auth-failure")
    page.locator("#c311-summary").fill("Keep this summary after auth failure")
    page.locator("#c311-location-address").fill("100 Example Street, Buffalo, NY 14201")
    page.locator('[data-c311-action="geocode-address"]').click()
    page.locator('[data-c311-map-error]').wait_for(state="visible")
    check("credentials" in page.locator('[data-c311-map-error]').inner_text().lower(), "map auth failure was not explained")
    check(page.locator("#c311-summary").input_value() == "Keep this summary after auth failure", "map auth failure cleared request input")


def check_upload_retry(page, base_url: str) -> None:
    open_page(page, base_url, SUBMIT_PATH, scenario="attachment-retryable")
    page.locator("#c311-summary").fill("Keep this summary while the attachment retries")
    page.set_input_files("#c311-attachment-file", {"name": "fixture.txt", "mimeType": "text/plain", "buffer": b"fixture"})
    page.locator('[data-c311-attachment-error]').wait_for(state="visible")
    page.locator('[data-c311-action="retry-attachment"]').click()
    page.locator('[data-c311-attachment-list]').wait_for(state="visible")
    check(page.locator("#c311-summary").input_value() == "Keep this summary while the attachment retries", "attachment failure cleared request input")


def check_form(page, base_url: str, path: str) -> None:
    can_download = path != STAFF_SUBMIT_PATH
    open_page(page, base_url, path, role="service_agent" if path == STAFF_SUBMIT_PATH else "constituent")
    fill_form(page)
    check_location(page, base_url)
    check_attachments(page, can_download=can_download)
    page.locator('[data-c311-action="submit-request"], [data-c311-action="submit-staff-request"]').first.click()
    page.locator('[data-c311-submission-result]').wait_for(state="visible")
    check("SUBMITTED" in page.locator('[data-c311-submission-result]').inner_text(), "submission did not complete")


def run() -> dict:
    artifacts = artifact_directory()
    results = {"mode": "mock-only", "viewports": [list(viewport) for viewport in VIEWPORTS], "checks": [], "started_at": datetime.now(timezone.utc).isoformat()}
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(headless=True)
        try:
            for width, height in VIEWPORTS:
                for app, base_url in (("compose", COMPOSE_URL), ("admin", ADMIN_URL)):
                    context = browser.new_context(viewport={"width": width, "height": height})
                    page = context.new_page()
                    result = diagnostics(page, (base_url, COMPOSE_URL) if app == "admin" else base_url)
                    page.on("request", lambda request: check_request_secrets(result, request))
                    label = f"{app}@{width}x{height}"
                    open_page(page, base_url, STAFF_SUBMIT_PATH if app == "admin" else "/c311")
                    assert_page(page, label, width)
                    if app == "compose":
                        check_form(page, base_url, SUBMIT_PATH)
                        check_download_capability(page, base_url)
                        check_route_attachment_lifecycle(page, base_url)
                        check_upload_retry(page, base_url)
                        check_invalid_attachment(page, base_url)
                        check_address_not_found(page, base_url)
                        check_map_failures(page, base_url)
                    else:
                        check_form(page, base_url, STAFF_SUBMIT_PATH)
                    screenshot = artifacts / f"fe04-{app}-{width}x{height}.png"
                    page.screenshot(path=str(screenshot), full_page=True)
                    finalize_diagnostics(result)
                    check(not result["page_errors"], f"{label} page errors: {result['page_errors']}")
                    check(not result["unexpected_console_errors"], f"{label} console errors: {result['unexpected_console_errors']}")
                    check(not result["unexpected_responses"], f"{label} HTTP failures: {result['unexpected_responses']}")
                    check(not result["writes"], f"{label} issued network writes: {result['writes']}")
                    results["checks"].append({"label": label, "status": "passed", "screenshot": str(screenshot), "diagnostics": result})
                    context.close()
        finally:
            browser.close()
    results["finished_at"] = datetime.now(timezone.utc).isoformat()
    (artifacts / "fe04-matrix.json").write_text(json.dumps(results, indent=2), encoding="utf-8")
    return results


if __name__ == "__main__":
    try:
        print(json.dumps(run(), indent=2))
    except Exception as error:
        print(json.dumps({"status": "failed", "error": str(error)}), file=sys.stderr)
        raise
