#!/usr/bin/env python3
"""Repeatable FE-08 administration configuration checks in C311 mock mode."""
from __future__ import annotations
import json, os, tempfile
from pathlib import Path
from urllib.parse import urlparse
from playwright.sync_api import Page, sync_playwright

VIEWPORTS = ((1440, 900), (390, 844))
ARTIFACT_DIR_ENV = "C311_ARTIFACT_DIR"
ADMIN_URL = os.environ.get("C311_ADMIN_URL", "http://127.0.0.1:18091").rstrip("/")
MAIN = "[data-c311-main]"
KNOWN_DEV_FAILURES = {"/code-snippets.js", "/custom.css"}

def artifact_directory() -> Path:
    configured = os.environ.get(ARTIFACT_DIR_ENV)
    directory = Path(configured).expanduser() if configured else Path(tempfile.mkdtemp(prefix="c311-fe08-gate-"))
    if directory.is_symlink(): raise ValueError(f"{ARTIFACT_DIR_ENV} must not point to a symbolic link")
    directory.mkdir(parents=True, exist_ok=True, mode=0o700)
    directory = directory.resolve()
    if directory == Path(directory.anchor): raise ValueError(f"{ARTIFACT_DIR_ENV} must point to a child directory")
    directory.chmod(0o700)
    return directory

def check(condition: bool, message: str) -> None:
    if not condition: raise AssertionError(message)

def run_view(page: Page, width: int, artifact_dir: Path) -> dict:
    errors, responses = [], []
    page.on("pageerror", lambda error: errors.append(str(error)))
    page.on("response", lambda response: responses.append({"url": response.url, "status": response.status}))
    page.add_init_script("window.C311Mode='mock'; window.C311MockRole='platform_administrator'; window.C311MockScenario='success'; window.C311MockSession='current';")
    page.set_viewport_size({"width": width, "height": 900 if width > 500 else 844})
    page.goto(f"{ADMIN_URL}/c311/admin", wait_until="domcontentloaded")
    page.locator(MAIN).wait_for(state="visible")
    check(page.locator(f"{MAIN} h1:visible").count() == 1, f"missing unique h1 at {width}px")
    check(page.evaluate("document.documentElement.scrollWidth - document.documentElement.clientWidth") <= 1, f"overflow at {width}px")
    check(page.locator('[data-c311-field="organisation_name"]').count() == 1, f"branding fixture missing at {page.url}: {page.locator('body').inner_text()[:200]}")
    check(page.locator('[data-c311-history="branding"]').count() == 1, "branding history missing")
    if width == 1440:
        page.locator('[data-c311-field="organisation_name"]').fill("Draft Fixture City")
        page.evaluate("""async () => {
          const shell = document.querySelector('[data-c311-app-shell]')
          const provider = shell.__vue__.$C311.provider
          await provider.updateBranding({ organisation_name: 'Concurrent Fixture City' }, { expectedVersion: 1 })
        }""")
        page.locator('[data-c311-action="save-branding"]').click()
        page.locator('[data-c311-conflict]').wait_for(state="visible")
        check(page.locator('[data-c311-current-version]').inner_text() == "2", "branding conflict omitted current version")
        page.locator('[data-c311-action="reload-current-version"]').click()
        page.locator('[data-c311-action="reapply-changes"]').click()
        page.locator('[data-c311-message]').wait_for(state="visible")
        check(page.locator('[data-c311-field="organisation_name"]').input_value() == "Draft Fixture City", "branding reapply lost edits")
    page.locator('[data-c311-tab="content"]').click(); page.locator('[data-c311-content]').wait_for(state="visible")
    check(page.locator('[data-c311-history="content"]').count() == 1, "content history missing")
    if width == 1440:
        page.locator('[data-c311-field="body"]').fill('<img src="x" onerror="alert(1)">')
        page.locator('[data-c311-action="preview-content"]').click()
        page.locator('[data-c311-error]').wait_for(state="visible")
        check(page.locator('[data-c311-content-preview]').count() == 0, "unsafe preview was rendered")
        page.locator('[data-c311-action="edit-help"]').click()
        page.locator('[data-c311-field="help_body"]').wait_for(state="visible")
        public_before = page.evaluate("""async () => {
          const shell = document.querySelector('[data-c311-app-shell]')
          return (await shell.__vue__.$C311.provider.getPublicHelp('public.request.submit', 'ES')).body
        }""")
        page.locator('[data-c311-field="help_language"]').select_option("ES")
        page.wait_for_function("document.querySelector('[data-c311-field=help_body]')?.value.length > 0")
        page.locator('[data-c311-field="help_body"]').fill("<p>Ayuda segura.</p>")
        page.locator('[data-c311-action="save-help"]').click()
        page.wait_for_function("document.querySelector('[data-c311-message]')?.textContent.includes('Help saved.')")
        check(page.locator('[data-c311-help-version]').inner_text() == "2", "help draft version was not saved")
        public_after_draft = page.evaluate("""async () => {
          const shell = document.querySelector('[data-c311-app-shell]')
          return (await shell.__vue__.$C311.provider.getPublicHelp('public.request.submit', 'ES')).body
        }""")
        check(public_after_draft == public_before, "help draft changed the public projection")
        page.locator('[data-c311-action="preview-help"]').click()
        page.locator('[data-c311-help-preview]').wait_for(state="visible")
        check("Ayuda segura" in page.locator('[data-c311-help-preview]').inner_text(), "help preview was not rendered")
        page.locator('[data-c311-action="publish-help"]').click()
        page.wait_for_function("document.querySelector('[data-c311-help-version]')?.textContent.trim() === '3'")
        help_after_publish = page.evaluate("""() => {
          const shell = document.querySelector('[data-c311-app-shell]').__vue__
          return { body: shell.$parent?.help?.body || '', version: shell.$parent?.help?.version || 0 }
        }""")
        check(help_after_publish["body"] == "<p>Ayuda segura.</p>", f"published help did not update the public projection: {help_after_publish}")
        check(page.locator('[data-c311-history="help"]').count() == 1, "help history missing")
        page.locator('[data-c311-action="rollback-help-1"]').click()
        page.wait_for_function("document.querySelector('[data-c311-help-version]')?.textContent.trim() === '4'")
    page.locator('[data-c311-tab="categories"]').click(); page.locator('[data-c311-categories]').wait_for(state="visible")
    check(page.locator('[data-c311-category="RESIDENT"]').count() == 1, "category fixture missing")
    check(page.locator('[data-c311-action="save-category-RESIDENT"]').count() == 1, "category edit action missing")
    if width == 1440:
        resident = page.locator('[data-c311-category="RESIDENT"]')
        resident.locator('input[type="checkbox"]').uncheck()
        page.locator('[data-c311-action="save-category-RESIDENT"]').click()
        page.locator('[data-c311-error]').wait_for(state="visible")
        check("invalid" in page.locator('[data-c311-error]').inner_text().lower(), "in-use category deactivation was not rejected")
        category_errors = page.evaluate("""async () => {
          const provider = document.querySelector('[data-c311-app-shell]').__vue__.$C311.provider
          const result = {}
          try { await provider.updateAdminCategory('LEGACY', { code: 'LEGACY', active: true, labels: { EN: 'Legacy' } }) } catch (error) { result.missing = error.code }
          try { await provider.updateAdminCategory('LEGACY', { code: 'LEGACY', active: true, labels: { EN: 'Legacy' } }, { expectedVersion: 1 }) } catch (error) { result.stale = error.code }
          return result
        }""")
        check(category_errors.get("missing") == "EXPECTED_VERSION_REQUIRED", "missing category version was accepted")
        check(category_errors.get("stale") == "VERSION_CONFLICT", "stale category version was accepted")
    page.locator('[data-c311-tab="fields"]').click(); page.locator('[data-c311-fields]').wait_for(state="visible")
    check(page.locator('[data-c311-field-key="contact_preference"]').count() == 1, "custom field fixture missing")
    page.locator('[data-c311-action="edit-field-contact_preference"]').click()
    check(page.locator('[data-c311-field="field_choices"]').input_value() == "EMAIL\nPHONE", "ordered choices were not loaded")
    check(page.locator('[data-c311-field="field_default"]').input_value() == '"EMAIL"', "custom field default was not loaded")
    check(page.locator('[data-c311-field="field_validation"]').count() == 1, "custom field validation editor missing")
    page.screenshot(path=str(artifact_dir / f"admin-{width}.png"), full_page=True)
    unexpected = [item for item in responses if 400 <= item["status"] < 600 and urlparse(item["url"]).path not in KNOWN_DEV_FAILURES]
    check(not errors, f"page errors at {width}px: {errors}")
    check(not unexpected, f"unexpected HTTP failures at {width}px: {unexpected}")
    return {"width": width, "passed": True, "responses": responses}

def main() -> int:
    artifact_dir, results = artifact_directory(), []
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(headless=True)
        for width, _ in VIEWPORTS:
            page = browser.new_page(); results.append(run_view(page, width, artifact_dir)); page.close()
        browser.close()
    (artifact_dir / "fe08-matrix.json").write_text(json.dumps({"mode": "mock-only", "viewports": results}, indent=2), encoding="utf-8")
    return 0

if __name__ == "__main__": raise SystemExit(main())
