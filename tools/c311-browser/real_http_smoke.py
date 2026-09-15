#!/usr/bin/env python3
from __future__ import annotations
import json, os, sys
from pathlib import Path
from playwright.sync_api import sync_playwright

FRONTEND_URL = os.environ.get('C311_REAL_FRONTEND_URL', 'http://127.0.0.1:18092').rstrip('/')
ARTIFACT_DIR = Path(os.environ.get('C311_ARTIFACT_DIR', '.c311-real-http'))

def main() -> int:
    ARTIFACT_DIR.mkdir(parents=True, exist_ok=True)
    diagnostics = {'frontend_url': FRONTEND_URL, 'api_responses': [], 'errors': []}
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch()
        context = browser.new_context()
        context.add_init_script("window.C311Mode = 'http';")
        page = context.new_page()
        page.on('pageerror', lambda error: diagnostics['errors'].append(str(error)))
        def record(response):
            if '/api/v1/' in response.url:
                diagnostics['api_responses'].append({'method': response.request.method, 'url': response.url, 'status': response.status})
        page.on('response', record)
        page.goto(f'{FRONTEND_URL}/c311/submit', wait_until='domcontentloaded')
        page.locator('[data-c311-main]').wait_for(state='visible', timeout=60000)
        if page.evaluate("Boolean(window.__C311MockProvider)"):
            raise AssertionError('frontend instantiated the Mock C311 provider')
        branding = page.evaluate("""async () => { const r = await fetch('/api/v1/public/branding', {credentials:'include'}); return {status:r.status, contentType:r.headers.get('content-type')} }""")
        if branding['status'] >= 400 or 'json' not in (branding['contentType'] or ''):
            raise AssertionError(f'live branding endpoint failed: {branding}')
        if not any(item['url'].endswith('/api/v1/public/branding') and item['status'] < 400 for item in diagnostics['api_responses']):
            raise AssertionError('no successful browser API response was observed')
        if diagnostics['errors']:
            raise AssertionError(f"browser page errors: {diagnostics['errors']}")
        browser.close()
    (ARTIFACT_DIR / 'real-http-smoke.json').write_text(json.dumps(diagnostics, indent=2), encoding='utf-8')
    print(json.dumps(diagnostics, indent=2))
    return 0

if __name__ == '__main__':
    try:
        raise SystemExit(main())
    except Exception as error:
        print(f'real HTTP smoke failed: {error}', file=sys.stderr)
        raise
