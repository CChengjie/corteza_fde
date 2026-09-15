#!/usr/bin/env python3
from __future__ import annotations
import json, os, sys
from pathlib import Path
from playwright.sync_api import sync_playwright

FRONTEND_URL = os.environ.get('C311_REAL_FRONTEND_URL', 'http://127.0.0.1:18092').rstrip('/')
ARTIFACT_DIR = Path(os.environ.get('C311_ARTIFACT_DIR', '.c311-real-http'))

def main() -> int:
    ARTIFACT_DIR.mkdir(parents=True, exist_ok=True)
    diagnostics = {'frontend_url': FRONTEND_URL, 'api_responses': [], 'api_requests': [], 'errors': []}
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch()
        context = browser.new_context()
        context.add_init_script("window.C311Mode = 'http';")
        page = context.new_page()
        page.on('pageerror', lambda error: diagnostics['errors'].append(str(error)))
        page.on('requestfailed', lambda request: diagnostics['errors'].append(
            f"request failed: {request.method} {request.url} ({request.failure})"
        ) if '/api/v1/' in request.url and not request.url.endswith(('/api/v1/staff/service-requests', '/api/v1/admin/identity')) else None)
        def record_request(request):
            if '/api/v1/' in request.url:
                diagnostics['api_requests'].append({'method': request.method, 'url': request.url})
                if request.url.startswith('http') and not request.url.startswith(FRONTEND_URL + '/'):
                    diagnostics['errors'].append(f'cross-origin API request: {request.url}')
        page.on('request', record_request)
        def record(response):
            if '/api/v1/' in response.url:
                diagnostics['api_responses'].append({
                    'method': response.request.method,
                    'url': response.url,
                    'status': response.status,
                    'from_service_worker': response.from_service_worker,
                })
        page.on('response', record)
        page.goto(f'{FRONTEND_URL}/c311/submit', wait_until='domcontentloaded')
        page.locator('[data-c311-main]').wait_for(state='visible', timeout=60000)
        if page.evaluate('window.C311Mode') != 'http':
            raise AssertionError('C311 browser mode was changed from http')
        if page.evaluate("Boolean(window.__C311MockProvider)"):
            raise AssertionError('frontend instantiated the Mock C311 provider')
        branding = page.evaluate("""async () => { const r = await fetch('/api/v1/public/branding', {credentials:'include'}); return {status:r.status, contentType:r.headers.get('content-type')} }""")
        if branding['status'] >= 400 or 'json' not in (branding['contentType'] or ''):
            raise AssertionError(f'live branding endpoint failed: {branding}')
        if not any(item['url'].endswith('/api/v1/public/branding') and item['status'] < 400 for item in diagnostics['api_responses']):
            raise AssertionError('no successful browser API response was observed')
        if any(item['from_service_worker'] for item in diagnostics['api_responses']):
            raise AssertionError('browser API response was served by a service worker instead of the backend')
        session = page.evaluate("""async () => {
          const signIn = await fetch('/api/v1/session', {
            method: 'POST', credentials: 'include',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({login_identifier: 'city311-constituent', password: 'City311-Local-Constituent-1!'})
          })
          const body = await signIn.json()
          return {status: signIn.status, authenticated: body.authenticated === true}
        }""")
        if session['status'] != 200 or not session['authenticated']:
            raise AssertionError(f'live session sign-in failed: {session}')
        current = page.evaluate("""async () => {
          const response = await fetch('/api/v1/session', {credentials: 'include'})
          const body = await response.json()
          return {status: response.status, authenticated: body.authenticated === true}
        }""")
        if current != {'status': 200, 'authenticated': True}:
            raise AssertionError(f'identity cookie was not retained by the browser: {current}')
        page.reload(wait_until='domcontentloaded')
        page.locator('[data-c311-main]').first.wait_for(state='visible', timeout=60000)
        refreshed = page.evaluate("""async () => {
          const response = await fetch('/api/v1/session', {credentials: 'include'})
          const body = await response.json()
          return {status: response.status, authenticated: body.authenticated === true}
        }""")
        if refreshed != {'status': 200, 'authenticated': True}:
            raise AssertionError(f'identity cookie was lost after browser refresh: {refreshed}')
        forbidden = page.evaluate("""async () => {
          const response = await fetch('/api/v1/staff/service-requests', {credentials: 'include'})
          return {status: response.status}
        }""")
        if forbidden['status'] != 403:
            raise AssertionError(f'constituent unexpectedly accessed staff queue: {forbidden}')
        page.reload(wait_until='domcontentloaded')
        page.locator('[data-c311-main]').first.wait_for(state='visible', timeout=60000)
        page.locator('#c311-summary').fill('Live HTTP click request')
        page.locator('#c311-description').fill('Created by clicking the real browser form.')
        page.locator('#c311-requester-name').fill('Live HTTP Smoke')
        page.locator('#c311-requester-email').fill('smoke@example.invalid')
        page.locator('#c311-consent').check()
        submit_button = page.locator('[data-c311-action="submit-request"]')
        submit_button.wait_for(state='visible', timeout=10000)
        submit_button.click()
        result = page.locator('[data-c311-submission-result]')
        result.wait_for(state='visible', timeout=60000)
        result_text = result.inner_text().strip()
        if not result_text:
            raise AssertionError('real browser submit returned an empty result')
        request_number = next((line.split(':', 1)[1].strip() for line in result_text.splitlines() if line.startswith('Request number:')), '')
        if not request_number:
            raise AssertionError(f'real browser submit did not render a request number: {result_text}')
        submission_responses = [
            item for item in diagnostics['api_responses']
            if item['method'] == 'POST' and item['url'].endswith('/api/v1/portal/service-requests')
        ]
        if not any(item['status'] == 201 for item in submission_responses):
            raise AssertionError(f'click submit did not produce HTTP 201: {submission_responses}')
        submission = {'status': 201, 'request_number': request_number, 'result_text': result_text}
        diagnostics['live_write'] = submission
        persisted = page.evaluate("""async () => {
          const response = await fetch('/api/v1/portal/service-requests', {credentials: 'include'})
          const body = await response.json()
          const rows = body.items || body.results || []
          return {
            status: response.status,
            found: rows.some(item => (item.summary || item.title) === 'Live HTTP click request'),
          }
        }""")
        if persisted != {'status': 200, 'found': True}:
            raise AssertionError(f'created request was not persisted in portal list: {persisted}')
        diagnostics['persistence'] = persisted
        constituent_admin = page.evaluate("""async () => {
          const response = await fetch('/api/v1/admin/identity', {credentials: 'include'})
          return {status: response.status}
        }""")
        if constituent_admin['status'] not in (401, 403):
            raise AssertionError(f'constituent unexpectedly reached admin API: {constituent_admin}')
        diagnostics['authorization'] = {'constituent_admin_identity_status': constituent_admin['status']}
        page.goto(f'{FRONTEND_URL}/c311/requests', wait_until='domcontentloaded')
        page.get_by_text(request_number, exact=True).first.wait_for(state='visible', timeout=60000)
        page.reload(wait_until='domcontentloaded')
        page.get_by_text(request_number, exact=True).first.wait_for(state='visible', timeout=60000)
        if diagnostics['errors']:
            raise AssertionError(f"browser page errors: {diagnostics['errors']}")

        admin_login = page.evaluate("""async () => {
          const response = await fetch('/api/v1/session', {
            method: 'POST', credentials: 'include',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({login_identifier: 'city311-platform-administrator', password: 'City311-Local-Admin-1!'})
          })
          const body = await response.json()
          return {status: response.status, authenticated: body.authenticated === true, capabilities: body.actor?.capabilities || []}
        }""")
        if admin_login['status'] != 200 or not admin_login['authenticated']:
            raise AssertionError(f'platform administrator sign-in failed: {admin_login}')
        page.goto(f'{FRONTEND_URL}/c311/admin', wait_until='domcontentloaded')
        page.locator('[data-c311-main]').first.wait_for(state='visible', timeout=60000)
        admin_heading_locator = page.get_by_role('heading', name='City 311 administration').first
        admin_heading_locator.wait_for(state='visible', timeout=60000)
        admin_heading = admin_heading_locator.inner_text()
        if admin_heading != 'City 311 administration':
            raise AssertionError(f'admin workspace did not render: {admin_heading}')
        admin_api = [
            item for item in diagnostics['api_responses']
            if item['method'] == 'GET' and item['url'].endswith('/api/v1/admin/identity')
        ]
        if not any(item['status'] == 200 for item in admin_api):
            raise AssertionError(f'admin workspace did not load identity configuration: {admin_api}')
        page.reload(wait_until='domcontentloaded')
        page.get_by_role('heading', name='City 311 administration').first.wait_for(state='visible', timeout=60000)
        diagnostics['admin_workspace'] = {'status': 'rendered-after-login-and-refresh', 'identity_api': admin_api}
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
        ARTIFACT_DIR.mkdir(parents=True, exist_ok=True)
        (ARTIFACT_DIR / 'real-http-failure.txt').write_text(
            f'{type(error).__name__}: {error}\n', encoding='utf-8')
        print(f'real HTTP smoke failed: {error}', file=sys.stderr)
        raise
