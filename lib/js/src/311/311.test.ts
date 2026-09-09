import { expect } from 'chai'
import { readFileSync } from 'node:fs'
import { C311ApiError } from './errors'
import { cloneFixtureSet, createDefaultFixtureSet } from './fixtures'
import { MockC311Provider } from './mock-provider'
import { C311FetchTransport, C311HttpProvider, type C311Provider, type C311TransportRequest, type ReportExportOptions } from './provider'
import { C311_TIMEZONE, formatC311DateTime } from './time'
import type { PortalServiceRequestCreate, ReportDefinition, ServiceRequestCreate, StaffServiceRequestCreate } from './types'
import { APPLICATION_ROLES, PORTAL_ATTACHMENT_MAX_BYTES, PORTAL_ATTACHMENT_MAX_COUNT, PORTAL_ATTACHMENT_MEDIA_TYPES } from './enums'
import { validatePortalAttachment } from './provider'

async function expectError (action: () => Promise<unknown>, code: string): Promise<C311ApiError> {
  try {
    await action()
    throw new Error(`expected ${code}`)
  } catch (error) {
    expect(error).to.be.instanceOf(C311ApiError)
    expect((error as C311ApiError).code).to.equal(code)
    return error as C311ApiError
  }
}

describe('City 311 frontend contract', () => {
  it('returns contract-v1 fixture data with wire-compatible fields', async () => {
    const fixtures = cloneFixtureSet(createDefaultFixtureSet())
    const page = await new MockC311Provider().listPortalRequests()

    expect(fixtures.fixture_id).to.equal('contract-v1')
    expect(fixtures.contract_version).to.equal('1.0.0')
    expect(page.items).to.have.length(1)
    expect(page.items[0].request_id).to.equal('request-fixture-001')
    expect(page.items[0].owning_department).to.equal('STREETS')
    expect(page.items[0]).to.not.have.property('primary_requester')
  })

  it('keeps the empty response shape stable', async () => {
    const page = await new MockC311Provider({ role: 'service_agent', scenario: 'empty' }).listStaffRequests()

    expect(page.items).to.deep.equal([])
    expect(page.total_count).to.equal(0)
    expect(page.next_page_token).to.equal(null)
    expect(page.applied_filters).to.deep.equal({})
  })

  it('keeps anonymous lookup privacy and reopen response shapes', async () => {
    const fixtures = cloneFixtureSet(createDefaultFixtureSet())
    fixtures.requests[0].status = 'RESOLVED'
    fixtures.details['request-fixture-001'].request.status = 'RESOLVED'
    fixtures.public_details['SR-2026-00001'].status = 'RESOLVED'
    const provider = new MockC311Provider({ fixtures })
    expect(await provider.getPublicStatus({ request_number: 'SR-2026-99999', email: 'unknown@example.test' })).to.deep.equal({ request_detail: null })
    expect(await provider.getPublicStatus({ request_number: 'SR-2026-00001', email: 'wrong@example.test' })).to.deep.equal({ request_detail: null })
    expect((await provider.getPublicStatus({ request_number: 'SR-2026-00001', email: 'alex@example.test' })).request_detail?.request_number).to.equal('SR-2026-00001')
    expect((await provider.getPublicStatus({ request_number: 'SR-2026-00001', email: ' ALEX@EXAMPLE.TEST ' })).request_detail?.request_number).to.equal('SR-2026-00001')
    expect(await provider.reopenPortalRequest('request-fixture-001', 'fixture reason')).to.deep.equal({
      request_id: 'request-fixture-001',
      status: 'PENDING_APPROVAL',
    })
    expect((await provider.getPublicStatus({ request_number: 'SR-2026-00001', email: 'alex@example.test' })).request_detail?.status).to.equal('REOPENED')
  })

  it('allows an associated constituent to open a request through the portal projection', async () => {
    const fixtures = cloneFixtureSet(createDefaultFixtureSet())
    fixtures.relationships!['request-fixture-001'] = [{ constituent_id: 'constituent-related', relationship_type: 'AFFECTED_RESIDENT', portal_visible: true, notify_status: false }]
    fixtures.public_relationships = fixtures.relationships
    const profile = { ...fixtures.requests[0].primary_requester, constituent_id: 'constituent-related', emails: ['related@example.test'] }
    const provider = new MockC311Provider({ role: 'constituent', fixtures, profile })

    expect((await provider.listPortalRequests()).items).to.have.length(1)
    expect((await provider.getPublicStatus({ request_number: 'SR-2026-00001', email: 'related@example.test' })).request_detail).to.not.equal(null)
    expect((await provider.getPublicStatus({ request_number: 'SR-2026-00001', email: 'other@example.test' })).request_detail).to.equal(null)
  })

  it('rejects reopening requests that are not resolved or closed', async () => {
    const provider = new MockC311Provider({ role: 'constituent' })
    const error = await expectError(() => provider.reopenPortalRequest('request-fixture-001', 'Too early'), 'VALIDATION_ERROR')
    expect(error.status).to.equal(422)
    expect((await provider.getPublicStatus({ request_number: 'SR-2026-00001', email: 'alex@example.test' })).request_detail?.status).to.equal('SUBMITTED')
  })

  it('projects only portal-visible relationships and notes for public status', async () => {
    const fixtures = cloneFixtureSet(createDefaultFixtureSet())
    fixtures.relationships!['request-fixture-001'] = [
      ...(fixtures.relationships?.['request-fixture-001'] || []),
      { constituent_id: 'constituent-hidden', relationship_type: 'AFFECTED_RESIDENT', portal_visible: false, notify_status: true },
    ]
    fixtures.notes!['request-fixture-001'] = [
      { note_id: 'note-public', request_id: 'request-fixture-001', author_constituent_id: 'actor-fixture-agent', body: 'Public update', portal_visible: true, created_at: '2026-01-15T15:00:00.000Z' },
      { note_id: 'note-private', request_id: 'request-fixture-001', author_constituent_id: 'actor-fixture-agent', body: 'Internal update', portal_visible: false, created_at: '2026-01-15T15:00:00.000Z' },
    ]
    fixtures.public_relationships = fixtures.relationships
    fixtures.public_notes = fixtures.notes
    const detail = (await new MockC311Provider({ fixtures }).getPublicStatus({ request_number: 'SR-2026-00001', email: 'alex@example.test' })).request_detail
    expect(detail?.relationships?.find(item => item.constituent_id === 'constituent-fixture-001')).to.deep.include({ constituent_id: 'constituent-fixture-001', relationship_type: 'PRIMARY_REQUESTER', portal_visible: true, notify_status: false })
    expect(detail?.notes).to.deep.equal([{ note_id: 'note-public', request_id: 'request-fixture-001', author_constituent_id: 'actor-fixture-agent', body: 'Public update', portal_visible: true, created_at: '2026-01-15T15:00:00.000Z' }])
  })

  it('appends a public voter note in Mock mode without exposing it to anonymous users', async () => {
    const provider = new MockC311Provider({ role: 'constituent' })
    const note = await provider.createPortalNote('request-fixture-001', { body: 'Resident follow-up', portal_visible: true })
    expect(note).to.deep.include({ request_id: 'request-fixture-001', author_constituent_id: 'actor-fixture-001', body: 'Resident follow-up', portal_visible: true })
    expect(note.created_at).to.equal('2026-01-15T15:00:00.000Z')
    expect(provider.getWriteCount('portal_note_create')).to.equal(1)
    const status = await provider.getPublicStatus({ request_number: 'SR-2026-00001', email: 'alex@example.test' })
    expect(status.request_detail?.notes).to.deep.include(note)
    await expectError(() => new MockC311Provider({ role: 'public_visitor' }).createPortalNote('request-fixture-001', { body: 'Not allowed', portal_visible: true }), 'UNAUTHENTICATED')
  })

  it('supports mock-only account deletion and anonymization with explicit confirmation', async () => {
    const provider = new MockC311Provider({ role: 'constituent' })
    await expectError(() => provider.deleteOrAnonymizeAccount({ mode: 'ANONYMIZE', confirmation: 'wrong' }), 'VALIDATION_ERROR')
    const result = await provider.deleteOrAnonymizeAccount({ mode: 'ANONYMIZE', confirmation: 'ANONYMIZE' })
    expect(result).to.deep.include({ status: 'ANONYMIZED' })
    expect((await provider.getSession()).authenticated).to.equal(false)
    expect(provider.getWriteCount('account_disposition')).to.equal(1)
  })

  it('keeps the mock session and write count unchanged when account disposition fails', async () => {
    const provider = new MockC311Provider({ role: 'constituent', scenario: 'account-disposition-conflict' })
    const before = await provider.getSession()
    await expectError(() => provider.deleteOrAnonymizeAccount({ mode: 'DELETE', confirmation: 'DELETE' }), 'VERSION_CONFLICT')
    expect(await provider.getSession()).to.deep.equal(before)
    expect(provider.getWriteCount('account_disposition')).to.equal(0)
  })

  it('keeps account disposition HTTP side-effect free until a backend operation exists', async () => {
    const requests: C311TransportRequest[] = []
    const provider = new C311HttpProvider({ request: async <T> (request: C311TransportRequest): Promise<T> => { requests.push(request); return {} as T } })
    const error = await expectError(() => provider.deleteOrAnonymizeAccount({ mode: 'DELETE', confirmation: 'DELETE' }), 'OPERATION_FAILED')
    expect(error.status).to.equal(501)
    expect(requests).to.deep.equal([])
  })

  it('provides complete role and session-expiry fixtures for route checks', () => {
    const fixtures = createDefaultFixtureSet()
    const roles = Object.keys(fixtures.role_fixtures)
    expect(roles).to.have.members(APPLICATION_ROLES)

    for (const role of roles) {
      const fixture = fixtures.role_fixtures[role as keyof typeof fixtures.role_fixtures]
      expect(fixture.denied_route).to.be.a('string')
      expect(fixture.denied_capability).to.be.a('string')
      expect(fixture.denied_scope).to.be.a('string')
      expect(fixture.expired_session.expires_at).to.equal('2026-01-15T14:00:00.000Z')
      if (role === 'public_visitor') expect(fixture.session.authenticated).to.equal(false)
      else {
        expect(fixture.session.actor?.available_routes).to.not.include(fixture.denied_route)
        expect(fixture.session.actor?.capabilities).to.not.include(fixture.denied_capability)
        expect(fixture.session.actor?.scopes).to.not.include(fixture.denied_scope)
        expect(fixture.session.actor?.application_roles).to.include(role)
      }
    }
    expect(fixtures.role_fixtures.supervisor.session.actor?.capabilities).to.not.include('audit_list')
    expect(fixtures.role_fixtures.supervisor.session.actor?.available_routes).to.not.include('audit_list')
    expect(fixtures.role_fixtures.department_manager.session.actor?.capabilities).to.include('admin_categories_update')
    expect(fixtures.role_fixtures.department_manager.session.actor?.available_routes).to.include('data_export')
    expect(fixtures.role_fixtures.platform_administrator.session.actor?.capabilities).to.not.include('staff_request_reassign')
    expect(fixtures.role_fixtures.platform_administrator.session.actor?.available_routes).to.not.include('staff_request_reassign')
  })

  it('selects every role and expired session without changing the fixture set', async () => {
    for (const role of APPLICATION_ROLES) {
      const current = await new MockC311Provider({ role }).getSession()
      const expired = await new MockC311Provider({ role, sessionVariant: 'expired' }).getSession()
      expect(current).to.deep.equal(createDefaultFixtureSet().role_fixtures[role].session)
      expect(expired.expires_at).to.equal('2026-01-15T14:00:00.000Z')
    }
  })

  it('enforces draft capabilities and session expiry in the mock provider', async () => {
    const agent = new MockC311Provider({ role: 'service_agent' })
    const draftInput = { summary: 'Fixture draft' }
    const deniedOperations = [
      () => agent.createDraft(draftInput),
      () => agent.getDraft('draft-fixture-001'),
      () => agent.updateDraft('draft-fixture-001', draftInput, { expectedVersion: 1 }),
      () => agent.deleteDraft('draft-fixture-001', { expectedVersion: 1 }),
      () => agent.submitDraft('draft-fixture-001', { expectedVersion: 1 }),
    ]
    for (const operation of deniedOperations) {
      const error = await expectError(operation, 'FORBIDDEN')
      expect(error.status).to.equal(403)
    }

    const expired = new MockC311Provider({ role: 'constituent', sessionVariant: 'expired' })
    const error = await expectError(() => expired.createDraft(draftInput), 'UNAUTHENTICATED')
    expect(error.status).to.equal(401)

    const constituent = new MockC311Provider({ role: 'constituent' })
    expect((await constituent.createDraft(draftInput)).status).to.equal('DRAFT')
  })

  it('enforces staff-assist capability and session state before writing', async () => {
    const input: StaffServiceRequestCreate = {
      constituent: { constituent_id: 'constituent-fixture-001' },
      request: {
        service_type: 'GENERAL_INQUIRY',
        summary: 'Staff-assisted request',
        description: 'A valid staff-assisted request fixture.',
        requester: { display_name: 'Fixture Resident', email: 'resident@example.test' },
      },
    }

    const anonymous = new MockC311Provider({ role: 'public_visitor' })
    const unauthenticated = await expectError(() => anonymous.createStaffServiceRequest(input), 'UNAUTHENTICATED')
    expect(unauthenticated.status).to.equal(401)
    expect(anonymous.getWriteCount('staff_service_request_create')).to.equal(0)

    const constituent = new MockC311Provider({ role: 'constituent' })
    const forbidden = await expectError(() => constituent.createStaffServiceRequest(input), 'FORBIDDEN')
    expect(forbidden.status).to.equal(403)
    expect(constituent.getWriteCount('staff_service_request_create')).to.equal(0)

    const expired = new MockC311Provider({ role: 'service_agent', sessionVariant: 'expired' })
    const expiredSession = await expectError(() => expired.createStaffServiceRequest(input), 'UNAUTHENTICATED')
    expect(expiredSession.status).to.equal(401)
    expect(expired.getWriteCount('staff_service_request_create')).to.equal(0)

    const agent = new MockC311Provider({ role: 'service_agent' })
    const detail = await agent.createStaffServiceRequest(input)
    expect(detail.request.status).to.equal('SUBMITTED')
    expect(agent.getWriteCount('staff_service_request_create')).to.equal(1)
  })

  it('keeps all role fixture values inside the frozen contract vocabularies', () => {
    const contract = JSON.parse(readFileSync(new URL('../../../../server/compose/types/city311/contract.json', import.meta.url), 'utf8')) as {
      enums: Record<string, string[]>
    }
    const fixtures = createDefaultFixtureSet()
    // FE-08 consumes the capability additions from PR #50 before that server
    // contract is merged into the checked-out baseline.
    const provisionalCapabilities = new Set([
      'admin_help_get', 'admin_help_update', 'admin_help_preview',
      'admin_help_publish', 'admin_help_versions', 'admin_help_rollback',
    ])
    const provisionalRoutes = provisionalCapabilities
    for (const role of APPLICATION_ROLES) {
      const fixture = fixtures.role_fixtures[role]
      const actor = fixture.session.actor
      const assertKnown = (kind: string, value: string) => {
        if ((kind === 'capability' && provisionalCapabilities.has(value)) || (kind === 'route' && provisionalRoutes.has(value))) return
        expect(contract.enums[kind]).to.include(value)
      }
      assertKnown('route', fixture.denied_route)
      assertKnown('capability', fixture.denied_capability)
      assertKnown('oauth_scope', fixture.denied_scope)
      if (!actor) continue
      actor.application_roles.forEach(value => assertKnown('application_role', value))
      actor.department_codes.forEach(value => assertKnown('department_code', value))
      actor.district_codes.forEach(value => assertKnown('district_code', value))
      actor.capabilities.forEach(value => assertKnown('capability', value))
      actor.scopes.forEach(value => assertKnown('oauth_scope', value))
      actor.available_routes.forEach(value => assertKnown('route', value))
      expect(actor.available_routes).to.not.include(fixture.denied_route)
      expect(actor.capabilities).to.not.include(fixture.denied_capability)
      expect(actor.scopes).to.not.include(fixture.denied_scope)
    }
  })

  it('exposes only endpoint-declared failures and models terminal operations in-band', async () => {
    const portalInput = {} as PortalServiceRequestCreate
    const scenarios: Array<[string, () => Promise<unknown>, string, boolean]> = [
      ['forbidden', () => new MockC311Provider({ scenario: 'forbidden' }).listStaffRequests(), 'FORBIDDEN', false],
      ['not-found', () => new MockC311Provider({ role: 'service_agent', scenario: 'not-found' }).getStaffRequest('missing'), 'NOT_FOUND', false],
      ['validation', () => new MockC311Provider({ scenario: 'validation' }).submitPortalRequest(portalInput), 'VALIDATION_ERROR', false],
      ['retryable', () => new MockC311Provider({ scenario: 'retryable' }).geocode({ address: 'fixture' }), 'MAP_TEMPORARILY_UNAVAILABLE', true],
      ['version-conflict', () => new MockC311Provider({ scenario: 'version-conflict' }).updateDraft('draft-fixture-001', {}, { expectedVersion: 1 }), 'VERSION_CONFLICT', false],
    ]

    for (const [scenario, action, code, retryable] of scenarios) {
      const error = await expectError(action, code)
      expect(error.retryable).to.equal(retryable)
      if (scenario === 'retryable') expect(error.retryAfter).to.equal('30')
      if (scenario === 'version-conflict') expect(error.currentVersion).to.equal(2)
    }

    expect((await new MockC311Provider({ scenario: 'version-conflict' }).getSession()).authenticated).to.equal(true)
    const terminal = await new MockC311Provider({ scenario: 'terminal', role: 'department_manager' }).getOperation('operation-fixture-terminal')
    expect(terminal.status).to.equal('FAILED')
    expect(terminal.error?.error).to.equal('OPERATION_FAILED')
  })

  it('does not mutate caller fixtures while exercising draft flows', async () => {
    const fixtures = createDefaultFixtureSet()
    const before = JSON.stringify(fixtures)
    const provider = new MockC311Provider({ fixtures })

    await provider.createDraft({ summary: 'temporary draft' })
    await provider.updateDraft('draft-fixture-001', { summary: 'temporary update' })
    await provider.deleteDraft('draft-fixture-001')

    expect(JSON.stringify(fixtures)).to.equal(before)
  })

  it('uses the frozen endpoint paths and concurrency headers', async () => {
    const requests: C311TransportRequest[] = []
    const transport = {
      request: async <T> (request: C311TransportRequest): Promise<T> => {
        requests.push(request)
        return {} as T
      },
    }
    const provider: C311Provider = new C311HttpProvider(transport)
    const input: ServiceRequestCreate = {
      summary: 'Example request',
      description: 'A sufficiently long fixture description.',
      service_type: 'GENERAL_INQUIRY',
      requester: { display_name: 'Example User', email: 'user@example.test' },
    }

    await provider.createServiceRequest(input, { idempotencyKey: 'fixture-key', expectedVersion: 3 })
    const portalInput: PortalServiceRequestCreate = {
      summary: input.summary,
      description: input.description,
      service_type: input.service_type,
      requester: input.requester,
      attachment_tokens: ['attachment-token-fixture-001'],
    }
    await provider.submitPortalRequest(portalInput, { idempotencyKey: 'portal-fixture-key' })
    await provider.getPublicStatus({ request_number: 'SR-2026-00001', email: 'alex@example.test' })
    await provider.reopenPortalRequest('request-fixture-001', 'fixture reason')
    await provider.downloadAttachment('attachment-fixture-001')
    await provider.uploadPortalAttachment({ file: 'ZmFrZQ==', filename: 'fixture.txt', media_type: 'text/plain' })
    await provider.geocode({ address: '100 Example Street' })
    await provider.exportReport('report-fixture-001')
    await provider.updateWorkflow('workflow-fixture-001', { name: 'Updated fixture workflow' }, { expectedVersion: 4 })
    await provider.deleteDraft('draft-fixture-001', { expectedVersion: 2 })

    expect(requests[0]).to.deep.include({
      method: 'POST',
      path: '/api/v1/service-requests',
      body: input,
      headers: { 'Idempotency-Key': 'fixture-key' },
    })
    expect(requests[1]).to.deep.include({
      method: 'POST',
      path: '/api/v1/portal/service-requests',
      body: portalInput,
      headers: { 'Idempotency-Key': 'portal-fixture-key' },
    })
    expect(requests[2]).to.deep.include({ method: 'POST', path: '/api/v1/public/service-request-status', acceptedStatuses: [404] })
    expect(requests[3]).to.deep.include({ method: 'POST', path: '/api/v1/portal/service-requests/request-fixture-001/reopen' })
    expect(requests[4]).to.deep.include({ method: 'GET', path: '/api/v1/attachments/attachment-fixture-001' })
    expect(requests[5].body).to.be.instanceOf(FormData)
    expect(requests[6]).to.deep.include({ method: 'POST', path: '/api/v1/geocode', body: { address: '100 Example Street' } })
    expect(requests[7]).to.deep.include({
      method: 'POST',
      path: '/api/v1/staff/reports/report-fixture-001/export',
      body: { format: 'CSV' },
    })
    expect(requests[8]).to.deep.include({
      method: 'PATCH',
      path: '/api/v1/admin/workflows/workflow-fixture-001',
      headers: { 'If-Match': '"4"' },
    })
    expect(requests[9]).to.deep.include({
      method: 'DELETE',
      path: '/api/v1/portal/service-request-drafts/draft-fixture-001',
      headers: { 'If-Match': '"2"' },
    })
  })

  it('uses the frozen account maintenance endpoints and payloads', async () => {
    const requests: C311TransportRequest[] = []
    const session = createDefaultFixtureSet().session
    const provider = new C311HttpProvider({
      request: async <T> (request: C311TransportRequest): Promise<T> => {
        requests.push(request)
        if (request.path === '/api/v1/account/login-identifier') return session as T
        if (request.path === '/api/v1/account/link/confirm') return session as T
        return undefined as T
      },
    })

    expect(await provider.changeLoginIdentifier({ current_password: 'Current-password-1!', login_identifier: 'alex.new' })).to.deep.equal(session)
    expect(await provider.changePassword({ current_password: 'Current-password-1!', new_password: 'New-password-2!' })).to.equal(undefined)
    expect(await provider.confirmAccountLink()).to.deep.equal(session)
    expect(requests).to.deep.equal([
      { method: 'POST', path: '/api/v1/account/login-identifier', body: { current_password: 'Current-password-1!', login_identifier: 'alex.new' } },
      { method: 'POST', path: '/api/v1/account/password', body: { current_password: 'Current-password-1!', new_password: 'New-password-2!' } },
      { method: 'POST', path: '/api/v1/account/link/confirm', body: {} },
    ])
  })

  it('keeps the current session and submitted values unchanged when account maintenance fails', async () => {
    const conflict = new MockC311Provider({ role: 'constituent', scenario: 'version-conflict' })
    const before = await conflict.getSession()
    await expectError(() => conflict.changeLoginIdentifier({ current_password: 'Current-password-1!', login_identifier: 'alex.conflict' }), 'VERSION_CONFLICT')
    expect(await conflict.getSession()).to.deep.equal(before)

    const invalid = new MockC311Provider({ role: 'constituent', scenario: 'validation' })
    await expectError(() => invalid.changePassword({ current_password: 'wrong', new_password: 'short' }), 'VALIDATION_ERROR')
    expect(await invalid.getSession()).to.deep.equal(await new MockC311Provider({ role: 'constituent' }).getSession())

    const profile = new MockC311Provider({ role: 'constituent' })
    await expectError(() => profile.updateProfile({ display_name: 'Updated' }), 'EXPECTED_VERSION_REQUIRED')
  })

  it('uses one-time opaque reset tokens and identical forgot-password responses', async () => {
    const privacyProvider = new MockC311Provider()
    const known = await privacyProvider.requestPasswordReset({ email: 'alex@example.test' })
    const unknown = await privacyProvider.requestPasswordReset({ email: 'unknown@example.test' })
    expect(unknown).to.deep.equal(known)

    const provider = new MockC311Provider()
    await provider.requestPasswordReset({ email: 'alex@example.test' })
    await provider.requestPasswordReset({ email: 'alex@example.test' })
    await expectError(() => provider.confirmPasswordReset({ token: 'reset-token-fixture-001', password: 'New-password-2!' }), 'INVALID_RESET_TOKEN')
    expect(await provider.confirmPasswordReset({ token: 'reset-token-fixture-002', password: 'New-password-2!' })).to.include({ message: 'Your password has been reset.' })
    await expectError(() => provider.confirmPasswordReset({ token: 'reset-token-fixture-002', password: 'New-password-2!' }), 'INVALID_RESET_TOKEN')
  })

  it('exposes and completes an explicit link confirmation operation', async () => {
    const provider = new MockC311Provider({ scenario: 'link-confirmation-required' })
    await provider.startFederatedSignIn('oidc')
    const pending = await provider.completeFederatedSignIn('oidc', { code: 'fixture-code' })
    expect(pending.outcome).to.equal('link_confirmation_required')
    const confirmed = await provider.confirmAccountLink()
    expect(confirmed.authenticated).to.equal(true)
  })

  it('keeps the account session unchanged when a pending federated link is cancelled', async () => {
    const provider = new MockC311Provider({ scenario: 'account-link-cancelled' })
    const before = await provider.getSession()
    await expectError(() => provider.startFederatedSignIn('saml'), 'FORBIDDEN')
    expect(await provider.getSession()).to.deep.equal(before)
  })

  it('nests request filters under the contract filters parameter', async () => {
    const requests: C311TransportRequest[] = []
    const provider = new C311HttpProvider({
      request: async <T> (request: C311TransportRequest): Promise<T> => {
        requests.push(request)
        return { items: [], next_page_token: null, total_count: 0, applied_filters: {}, sort: [] } as T
      },
    })

    await provider.listPortalRequests({
      page_size: 20,
      status: 'SUBMITTED',
      service_type: 'POTHOLE',
      filters: { category: 'RESIDENT' },
      sort: '-updated_at',
    })

    expect(requests[0].query).to.deep.equal({
      page_size: 20,
      filters: { category: 'RESIDENT', status: 'SUBMITTED', service_type: 'POTHOLE' },
      sort: '-updated_at',
    })
    expect(requests[0].query).to.not.have.property('status')
  })

  it('continues report lookup across opaque result pages', async () => {
    const requests: C311TransportRequest[] = []
    const report: ReportDefinition = {
      report_id: 'report-page-2',
      name: 'Second page report',
      entity: 'service_requests',
      columns: ['request_number'],
      filters: {},
      sort: [],
      version: 1,
      updated_at: '2026-01-15T15:00:00.000Z',
    }
    const provider = new C311HttpProvider({
      request: async <T> (request: C311TransportRequest): Promise<T> => {
        requests.push(request)
        const secondPage = request.query?.page_token === 'page-2'
        return {
          items: secondPage ? [report] : [],
          next_page_token: secondPage ? null : 'page-2',
          total_count: 1,
          applied_filters: {},
          sort: [],
        } as T
      },
    })

    expect(await provider.getReport('report-page-2')).to.deep.equal(report)
    expect(requests).to.have.length(2)
    expect(requests[1].query).to.deep.equal({ page_token: 'page-2' })
  })

  it('maps fetch responses, query arrays and contract errors', async () => {
    const calls: Array<{ url: string; init: RequestInit }> = []
    const okTransport = new C311FetchTransport({
      baseURL: 'https://fixture.example',
      fetch: async (url, init) => {
        calls.push({ url: String(url), init })
        return {
          ok: true,
          status: 204,
          headers: { get: () => null, forEach: () => {} },
        } as unknown as Response
      },
    })

    await okTransport.request({
      method: 'DELETE',
      path: '/api/v1/session',
      query: { sort: ['updated_at', 'desc'], page_size: 20, filters: { status: 'SUBMITTED' } },
    })
    expect(calls[0].url).to.equal('https://fixture.example/api/v1/session?sort=updated_at&sort=desc&page_size=20&filters=%7B%22status%22%3A%22SUBMITTED%22%7D')
    expect(calls[0].init.credentials).to.equal('include')

    const errorTransport = new C311FetchTransport({
      fetch: async () => ({
        ok: false,
        status: 422,
        headers: { get: () => 'application/json', forEach: () => {} },
        json: async () => ({ error: 'VALIDATION_ERROR', message: 'Invalid input.', retryable: false, errors: [{ field: '/summary', code: 'REQUIRED' }] }),
      } as unknown as Response),
    })
    const error = await expectError(() => errorTransport.request({ method: 'POST', path: '/api/v1/portal/service-requests', body: {} }), 'VALIDATION_ERROR')
    expect(error.status).to.equal(422)
    expect(error.error).to.equal('VALIDATION_ERROR')
    expect(error.fieldErrors[0].field).to.equal('/summary')

    const privacyTransport = new C311FetchTransport({
      fetch: async () => ({
        ok: false,
        status: 404,
        headers: { get: () => 'application/json', forEach: () => {} },
        json: async () => ({ request_detail: null }),
      } as unknown as Response),
    })
    const privacyResponse = await privacyTransport.request<{ request_detail: null }>({
      method: 'POST',
      path: '/api/v1/public/service-request-status',
      body: {},
      acceptedStatuses: [404],
    })
    expect(privacyResponse).to.deep.equal({ request_detail: null })

    const networkError = await expectError(() => new C311FetchTransport({ fetch: async () => { throw new Error('offline') } }).request({ method: 'GET', path: '/healthz' }), 'TEMPORARILY_UNAVAILABLE')
    expect(networkError.retryable).to.equal(true)

    const malformedTransport = new C311FetchTransport({
      fetch: async () => ({
        ok: false,
        status: 503,
        headers: { get: () => 'application/json', forEach: () => {} },
        text: async () => '{invalid',
      } as unknown as Response),
    })
    const malformed = await expectError(() => malformedTransport.request({ method: 'GET', path: '/healthz' }), 'OPERATION_FAILED')
    expect(malformed.status).to.equal(503)
    expect(malformed.retryable).to.equal(false)
  })

  it('normalizes anonymous HTTP misses to the same non-disclosing response', async () => {
    const provider = new C311HttpProvider({
      request: async <T> (): Promise<T> => ({ error: 'NOT_FOUND', message: 'not found' } as T),
    })

    expect(await provider.getPublicStatus({ request_number: 'SR-2026-99999', email: 'unknown@example.test' })).to.deep.equal({ request_detail: null })
  })

  it('formats the benchmark instant in the fixed timezone', () => {
    expect(C311_TIMEZONE).to.equal('America/New_York')
    expect(formatC311DateTime('2026-01-15T15:00:00.000Z', 'en-US')).to.equal('01/15/2026 10:00 AM EST')
    expect(formatC311DateTime('2026-07-15T15:00:00.000Z', 'en-US')).to.equal('07/15/2026 11:00 AM EDT')
    expect(formatC311DateTime('2026-07-15T15:00:00.000Z', 'es')).to.equal('07/15/2026 11:00 AM EDT')
    expect(formatC311DateTime('not-a-date')).to.equal('')
  })

  it('deep clones custom fixture sets', () => {
    const fixtures = createDefaultFixtureSet()
    const clone = cloneFixtureSet(fixtures)
    clone.requests[0].summary = 'changed in test'

    expect(fixtures.requests[0].summary).to.equal('Pothole on Example Street')
  })

  it('maps public identity operations to the frozen contract paths', async () => {
    const requests: C311TransportRequest[] = []
    const provider: any = new C311HttpProvider({
      request: async <T> (request: C311TransportRequest): Promise<T> => {
        requests.push(request)
        if (request.path === '/api/v1/public/branding') return { organisation_name: 'City 311' } as T
        if (request.path.includes('/content/')) return { content_key: 'HOME', body: '<p>Welcome</p>' } as T
        if (request.path.includes('/help/')) return { help_key: 'public.request.submit', language: 'EN', body: '<p>Help</p>', version: 1, updated_at: '2026-01-15T15:00:00.000Z' } as T
        if (request.path === '/api/v1/account/profile') return createDefaultFixtureSet().requests[0].primary_requester as T
        if (request.method === 'POST' && request.path === '/api/v1/session') return createDefaultFixtureSet().session as T
        return { accepted: true, message: 'accepted' } as T
      },
    })

    await provider.registerAccount({ display_name: 'Example', email: 'example@example.test', login_identifier: 'example', password: 'ValidPassword1!', preferred_language: 'EN' })
    await provider.requestPasswordReset({ email: 'example@example.test' })
    await provider.confirmPasswordReset({ token: 'ephemeral-token', password: 'ValidPassword1!' })
    await provider.startFederatedSignIn('oidc')
    await provider.completeFederatedSignIn('saml', { code: 'ephemeral-code', state: 'ephemeral-state' })
    await provider.getBranding()
    await provider.getAdminBranding()
    await provider.getPublicContent('HOME')
    await provider.getPublicHelp('public.request.submit', 'EN')
    await provider.getProfile()
    await provider.updateProfile({ display_name: 'Updated' }, { expectedVersion: 1 })
    await provider.updateLanguage('ES')
    await provider.changeLoginIdentifier({ current_password: 'Current-password-1!', login_identifier: 'updated.login' })
    await provider.changePassword({ current_password: 'Current-password-1!', new_password: 'New-password-2!' })

    expect(requests.map(request => `${request.method} ${request.path}`)).to.include.members([
      'POST /api/v1/accounts',
      'POST /api/v1/auth/password-reset/request',
      'POST /api/v1/auth/password-reset/confirm',
      'GET /api/v1/auth/oidc/start',
      'GET /api/v1/auth/saml/callback',
      'GET /api/v1/public/branding',
      'GET /api/v1/admin/branding',
      'GET /api/v1/public/content/HOME',
      'GET /api/v1/public/help/public.request.submit',
      'GET /api/v1/account/profile',
      'PATCH /api/v1/account/profile',
      'PATCH /api/v1/preferences/language',
      'POST /api/v1/account/login-identifier',
      'POST /api/v1/account/password',
    ])
    expect(requests.find(request => request.path === '/api/v1/account/login-identifier')?.body).to.deep.equal({ current_password: 'Current-password-1!', login_identifier: 'updated.login' })
    expect(requests.find(request => request.path === '/api/v1/account/password')?.body).to.deep.equal({ current_password: 'Current-password-1!', new_password: 'New-password-2!' })
    expect(requests.find(request => request.path === '/api/v1/auth/saml/callback')?.query).to.deep.equal({ code: 'ephemeral-code', state: 'ephemeral-state' })
  })

  it('maps FE-08 administration operations to contract paths and concurrency headers', async () => {
    const requests: C311TransportRequest[] = []
    const provider = new C311HttpProvider({
      request: async <T> (request: C311TransportRequest): Promise<T> => {
        requests.push(request)
        return { version: 1, updated_at: '2026-01-15T15:00:00.000Z', content_key: 'HOME', body: '<p>safe</p>', state: 'DRAFT', published: false } as T
      },
    })
    const options = { expectedVersion: 1 }
    await provider.updateBranding({ organisation_name: 'Fixture City' }, options)
    await provider.previewBranding({ organisation_name: 'Preview City' })
    await provider.publishBranding(options)
    await provider.listBrandingVersions()
    await provider.rollbackBranding({ target_version: 1 }, options)
    await provider.getAdminContent('HOME')
    await provider.listAdminContent()
    await provider.updateAdminContent('HOME', { body: '<p>draft</p>' }, options)
    await provider.previewAdminContent('HOME', { body: '<p>preview</p>' })
    await provider.publishAdminContent('HOME', options)
    await provider.listAdminContentVersions('HOME')
    await provider.rollbackAdminContent('HOME', { target_version: 1 }, options)
    await provider.getAdminHelp('public.request.submit', 'EN')
    await provider.updateAdminHelp('public.request.submit', { language: 'EN', body: '<p>help</p>' }, options)
    await provider.previewAdminHelp('public.request.submit', { language: 'EN', body: '<p>preview help</p>' })
    await provider.publishAdminHelp('public.request.submit', 'EN', options)
    await provider.listAdminHelpVersions('public.request.submit', { language: 'EN' })
    await provider.rollbackAdminHelp('public.request.submit', { target_version: 1 }, 'EN', { expectedVersion: 2 })
    await provider.listAdminCategories()
    await provider.createAdminCategory({ code: 'NEW', active: true, labels: { EN: 'New' } })
    await provider.updateAdminCategory('NEW', { code: 'NEW', active: false, labels: { EN: 'Disabled' } }, options)
    await provider.listAdminCustomFields()
    await provider.createAdminCustomField({ key: 'field', labels: { EN: 'Field' }, entity: 'service_request', field_type: 'TEXT', required: false, active: true, version: 1, updated_at: '2026-01-15T15:00:00.000Z' })
    await provider.updateAdminCustomField('field', { key: 'field', labels: { EN: 'Updated' }, entity: 'service_request', field_type: 'TEXT', required: false, active: true, version: 1, updated_at: '2026-01-15T15:00:00.000Z' }, options)

    expect(requests.map(request => `${request.method} ${request.path}`)).to.deep.equal([
      'PATCH /api/v1/admin/branding', 'POST /api/v1/admin/branding/preview', 'POST /api/v1/admin/branding/publish', 'GET /api/v1/admin/branding/versions', 'POST /api/v1/admin/branding/rollback',
      'GET /api/v1/admin/content/HOME', 'GET /api/v1/admin/content', 'PATCH /api/v1/admin/content/HOME', 'POST /api/v1/admin/content/HOME/preview', 'POST /api/v1/admin/content/HOME/publish', 'GET /api/v1/admin/content/HOME/versions', 'POST /api/v1/admin/content/HOME/rollback',
      'GET /api/v1/admin/help/public.request.submit', 'PATCH /api/v1/admin/help/public.request.submit', 'POST /api/v1/admin/help/public.request.submit/preview', 'POST /api/v1/admin/help/public.request.submit/publish', 'GET /api/v1/admin/help/public.request.submit/versions', 'POST /api/v1/admin/help/public.request.submit/rollback', 'GET /api/v1/admin/contact-categories', 'POST /api/v1/admin/contact-categories', 'PATCH /api/v1/admin/contact-categories/NEW', 'GET /api/v1/admin/custom-fields', 'POST /api/v1/admin/custom-fields', 'PATCH /api/v1/admin/custom-fields/field',
    ])
    const versionedPaths = requests.filter(request => ['PATCH /api/v1/admin/branding', 'POST /api/v1/admin/branding/publish', 'POST /api/v1/admin/branding/rollback', 'PATCH /api/v1/admin/content/HOME', 'POST /api/v1/admin/content/HOME/publish', 'POST /api/v1/admin/content/HOME/rollback', 'PATCH /api/v1/admin/help/public.request.submit', 'POST /api/v1/admin/help/public.request.submit/publish', 'POST /api/v1/admin/help/public.request.submit/rollback', 'PATCH /api/v1/admin/contact-categories/NEW', 'PATCH /api/v1/admin/custom-fields/field'].includes(`${request.method} ${request.path}`))
    expect(versionedPaths.filter(request => `${request.method} ${request.path}`.endsWith('/help/public.request.submit/rollback')).every(request => request.headers?.['If-Match'] === '"2"')).to.equal(true)
    expect(versionedPaths.filter(request => !`${request.method} ${request.path}`.endsWith('/help/public.request.submit/rollback')).every(request => request.headers?.['If-Match'] === '"1"')).to.equal(true)
    expect(requests[0].body).to.deep.equal({ organisation_name: 'Fixture City' })
  })

  it('enforces FE-08 administration capabilities in Mock mode', async () => {
    expect((await new MockC311Provider({ role: 'public_visitor' }).getBranding()).organisation_name).to.equal('City 311')
    expect((await new MockC311Provider({ role: 'constituent' }).getBranding()).organisation_name).to.equal('City 311')
    await expectError(() => new MockC311Provider({ role: 'service_agent' }).getAdminBranding(), 'FORBIDDEN')
    const admin = new MockC311Provider({ role: 'platform_administrator' })
    const branding = await admin.getAdminBranding()
    expect(branding.version).to.equal(1)
    await admin.updateBranding({ organisation_name: 'Draft city' }, { expectedVersion: branding.version })
    expect((await admin.getBranding()).organisation_name).to.equal('City 311')
    await admin.publishBranding({ expectedVersion: 2 })
    expect((await admin.getBranding()).organisation_name).to.equal('Draft city')
    const before = await admin.getAdminContent('HOME')
    const updated = await admin.updateAdminContent('HOME', { body: '<p>draft</p>' }, { expectedVersion: before.version })
    expect(updated.published).to.equal(false)
    await expectError(() => admin.updateAdminContent('HOME', { body: '<p>stale</p>' }, { expectedVersion: before.version }), 'VERSION_CONFLICT')
    await expectError(() => admin.previewAdminContent('HOME', { body: '<img src=x onerror=alert(1)>' }), 'VALIDATION_ERROR')
    await expectError(() => admin.updateAdminHelp('public.request.submit', { language: 'EN', body: '<script>alert(1)</script>' }, { expectedVersion: 1 }), 'VALIDATION_ERROR')
    const publicBeforeDraft = await admin.getPublicHelp('public.request.submit', 'EN')
    await admin.updateAdminHelp('public.request.submit', { language: 'EN', body: '<p>Draft only.</p>' }, { expectedVersion: 1 })
    expect((await admin.getPublicHelp('public.request.submit', 'EN')).body).to.equal(publicBeforeDraft.body)
    await admin.previewAdminHelp('public.request.submit', { language: 'EN', body: '<p>Preview only.</p>' })
    await admin.publishAdminHelp('public.request.submit', 'EN', { expectedVersion: 2 })
    expect((await admin.getPublicHelp('public.request.submit', 'EN')).body).to.equal('<p>Draft only.</p>')
    const help = await admin.updateAdminHelp('public.request.submit', { language: 'ES', body: '<p>Ayuda segura.</p>' }, { expectedVersion: 1 })
    expect(help.language).to.equal('ES')
    await admin.publishAdminHelp('public.request.submit', 'ES', { expectedVersion: 2 })
    expect((await admin.getPublicHelp('public.request.submit', 'ES')).body).to.equal('<p>Ayuda segura.</p>')
  })

  it('protects categories in use and validates custom-field defaults', async () => {
    const admin = new MockC311Provider({ role: 'platform_administrator' })
    await expectError(() => admin.updateAdminCategory('RESIDENT', { code: 'RESIDENT', active: false, labels: { EN: 'Resident' } }, { expectedVersion: 1 }), 'VALIDATION_ERROR')
    await expectError(() => admin.updateAdminCategory('LEGACY', { code: 'LEGACY', active: true, labels: { EN: 'Legacy' } }), 'EXPECTED_VERSION_REQUIRED')
    const legacy = await admin.updateAdminCategory('LEGACY', { code: 'LEGACY', active: true, labels: { EN: 'Legacy' } }, { expectedVersion: 2 })
    expect(legacy.active).to.equal(true)

    const fields = await admin.listAdminCustomFields()
    expect(fields.items[0].default).to.equal('EMAIL')
    await expectError(() => admin.updateAdminCustomField('contact_preference', { ...fields.items[0], default: 'POST' }, { expectedVersion: fields.items[0].version }), 'VALIDATION_ERROR')
    const updated = await admin.updateAdminCustomField('contact_preference', { ...fields.items[0], active: false, default: 'PHONE' }, { expectedVersion: fields.items[0].version })
    expect(updated.default).to.equal('PHONE')
    expect((await admin.getStaffRequest('request-fixture-001')).request.custom_fields).to.deep.equal({ contact_preference: 'EMAIL' })
  })

  it('validates password policy without persisting credentials', () => {
    const fixture = createDefaultFixtureSet()
    expect(fixture).to.not.have.property('password')
    expect(fixture).to.not.have.property('reset_token')
    const provider: any = new MockC311Provider({ scenario: 'registration-validation' })
    return expectError(() => provider.registerAccount({ display_name: '', email: 'bad', login_identifier: 'x', password: 'short', preferred_language: 'EN' }), 'VALIDATION_ERROR')
  })

  it('models identity and public-content fixture scenarios with contract errors', async () => {
    const cases: Array<[string, string, number]> = [
      ['invalid-credentials', 'UNAUTHENTICATED', 401],
      ['expired-reset-token', 'EXPIRED_RESET_TOKEN', 422],
      ['invalid-reset-token', 'INVALID_RESET_TOKEN', 422],
      ['oidc-failure', 'TEMPORARILY_UNAVAILABLE', 503],
      ['saml-failure', 'FORBIDDEN', 403],
      ['identity-claims-failure', 'UNAUTHENTICATED', 401],
      ['branding-failure', 'TEMPORARILY_UNAVAILABLE', 503],
      ['content-loading-failure', 'TEMPORARILY_UNAVAILABLE', 503],
      ['help-loading-failure', 'TEMPORARILY_UNAVAILABLE', 503],
      ['account-loading', 'TEMPORARILY_UNAVAILABLE', 503],
    ]
    for (const [scenario, code, status] of cases) {
      const provider: any = new MockC311Provider({ scenario: scenario as any })
      const action = scenario === 'invalid-credentials'
        ? () => provider.signIn({ login_identifier: 'fixture', password: 'not-a-secret' })
        : scenario === 'expired-reset-token' || scenario === 'invalid-reset-token'
          ? () => provider.confirmPasswordReset({ token: 'ephemeral-token', password: 'ValidPassword1!' })
          : scenario === 'oidc-failure' || scenario === 'saml-failure'
            ? () => provider.startFederatedSignIn(scenario === 'oidc-failure' ? 'oidc' : 'saml')
            : scenario === 'identity-claims-failure'
              ? () => provider.completeFederatedSignIn('oidc', { code: 'fixture-code' })
            : scenario === 'branding-failure'
              ? () => provider.getBranding()
              : scenario === 'help-loading-failure'
                ? () => provider.getPublicHelp('public.request.submit', 'EN')
                : scenario === 'account-loading'
                  ? () => provider.getProfile()
                  : () => provider.getPublicContent('HOME')
      const error = await expectError(action, code)
      expect(error.status).to.equal(status)
    }
  })

  it('rejects public SAML callbacks in the mock provider', async () => {
    await expectError(() => new MockC311Provider().completeFederatedSignIn('saml'), 'FORBIDDEN')
  })

  it('keeps accounts and the current session unchanged when identity claims fail', async () => {
    const provider: any = new MockC311Provider({ scenario: 'identity-claims-failure', role: 'constituent' })
    const accountsBefore = JSON.parse(JSON.stringify(provider.fixtures.role_fixtures))
    const sessionBefore = await provider.getSession()

    await expectError(() => provider.completeFederatedSignIn('oidc', { code: 'fixture-code' }), 'UNAUTHENTICATED')

    expect(provider.fixtures.role_fixtures).to.deep.equal(accountsBefore)
    expect(await provider.getSession()).to.deep.equal(sessionBefore)
  })

  it('maps a cancelled federated callback to the generic unauthenticated result', async () => {
    const provider = new MockC311Provider()
    const error = await expectError(() => provider.completeFederatedSignIn('oidc', { error: 'access_denied' }), 'UNAUTHENTICATED')
    expect(error.status).to.equal(401)
    expect(error.message).to.equal('Federated sign-in was cancelled.')
  })

  it('supports an empty authenticated request catalogue fixture', async () => {
    const page = await new MockC311Provider({ scenario: 'empty-my-requests', role: 'constituent' }).listPortalRequests()
    expect(page.items).to.deep.equal([])
    expect(page.total_count).to.equal(0)
  })

  it('maps FE-03 submit, draft, and staff-assist operations to contract paths and headers', async () => {
    const requests: C311TransportRequest[] = []
    const provider = new C311HttpProvider({
      request: async <T> (request: C311TransportRequest): Promise<T> => {
        requests.push(request)
        if (request.path === '/api/v1/staff/service-requests') return { request: { request_id: 'staff-request' } } as T
        if (request.path.includes('/submit')) return { request_id: 'draft-request', request_number: 'SR-2026-00004', status: 'SUBMITTED', version: 2, created_at: '2026-01-15T15:00:00.000Z', links: { self: '/api/v1/service-requests/draft-request' } } as T
        if (request.method === 'DELETE') return undefined as T
        return { request_id: 'draft-request', status: 'DRAFT', version: 2 } as T
      },
    })
    const portalInput = {
      summary: 'Pothole near library',
      description: 'The road surface is damaged near the library entrance.',
      service_type: 'POTHOLE' as const,
      requester: { display_name: 'Alex Example', email: 'alex@example.test' },
      location: { address: '100 Example Street', latitude: 42.9001, longitude: -88.8801 },
      attachment_tokens: ['upload-00031'],
      custom_fields: { ward: 'NORTH' },
    }
    await provider.submitPortalRequest(portalInput, { idempotencyKey: 'fe03-submit-1' })
    await provider.createDraft(portalInput)
    await provider.getDraft('draft-request')
    await provider.updateDraft('draft-request', { summary: 'Updated summary' }, { expectedVersion: 2 })
    await provider.deleteDraft('draft-request', { expectedVersion: 2 })
    await provider.submitDraft('draft-request', { expectedVersion: 2 })
    await provider.createStaffServiceRequest({
      constituent: { constituent_id: 'constituent-fixture-001' },
      request: portalInput,
    })

    expect(requests.find(request => request.path === '/api/v1/portal/service-requests')?.headers).to.deep.equal({ 'Idempotency-Key': 'fe03-submit-1' })
    expect(requests.find(request => request.path === '/api/v1/portal/service-request-drafts/draft-request' && request.method === 'PATCH')?.headers).to.deep.equal({ 'If-Match': '"2"' })
    expect(requests.find(request => request.path === '/api/v1/portal/service-request-drafts/draft-request' && request.method === 'DELETE')?.headers).to.deep.equal({ 'If-Match': '"2"' })
    expect(requests.find(request => request.path.endsWith('/draft-request/submit'))?.headers).to.deep.equal({ 'If-Match': '"2"' })
    expect(requests.find(request => request.path === '/api/v1/staff/service-requests')?.body).to.deep.equal({ constituent: { constituent_id: 'constituent-fixture-001' }, request: portalInput })
  })

  it('persists mock drafts, increments versions, and deduplicates logical portal submissions', async () => {
    const provider = new MockC311Provider({ role: 'constituent' })
    const input = {
      summary: 'Pothole near library',
      description: 'The road surface is damaged near the library entrance.',
      service_type: 'POTHOLE' as const,
      requester: { display_name: 'Alex Example', email: 'alex@example.test' },
      location: { address: '100 Example Street', latitude: 42.9001, longitude: -88.8801 },
    }
    const first = await provider.submitPortalRequest(input, { idempotencyKey: 'fe03-submit-1' })
    const replay = await provider.submitPortalRequest(input, { idempotencyKey: 'fe03-submit-1' })
    expect(replay).to.deep.equal(first)
    expect(provider.getWriteCount('portal_service_request_submit')).to.equal(1)
    await expectError(() => provider.submitPortalRequest({ ...input, summary: 'Different summary' }, { idempotencyKey: 'fe03-submit-1' }), 'IDEMPOTENCY_CONFLICT')

    const created = await provider.createDraft(input)
    expect(created.status).to.equal('DRAFT')
    const loaded = await provider.getDraft(created.request_id)
    expect(loaded.summary).to.equal(input.summary)
    const updated = await provider.updateDraft(created.request_id, { summary: 'Changed draft' }, { expectedVersion: created.version })
    expect(updated.summary).to.equal('Changed draft')
    expect(updated.version).to.equal(created.version + 1)
    await expectError(() => provider.updateDraft(created.request_id, { summary: 'Stale update' }, { expectedVersion: created.version }), 'VERSION_CONFLICT')
    await provider.deleteDraft(created.request_id, { expectedVersion: updated.version })
    await expectError(() => provider.getDraft(created.request_id), 'NOT_FOUND')
  })

  it('models attachment staging, one-time token use, and expected-version failures', async () => {
    const provider = new MockC311Provider({ role: 'public_visitor' })
    const attachment = await provider.uploadPortalAttachment({ file: 'opaque-fixture-bytes', filename: 'fixture.txt', media_type: 'text/plain' })
    expect(attachment.attachment_token).to.match(/^attachment-token-fixture-/)
    expect(provider.getWriteCount('portal_attachment_upload')).to.equal(1)
    const input: PortalServiceRequestCreate = {
      summary: 'Fixture attachment request',
      description: 'This request validates one-time attachment staging.',
      service_type: 'GENERAL_INQUIRY',
      requester: { display_name: 'Fixture Resident', email: 'resident@example.test' },
      attachment_tokens: [attachment.attachment_token],
    }
    const first = await provider.submitPortalRequest(input, { idempotencyKey: 'attachment-submit-1' })
    expect(first.status).to.equal('SUBMITTED')
    expect(await provider.submitPortalRequest(input, { idempotencyKey: 'attachment-submit-1' })).to.deep.equal(first)
    await expectError(() => provider.submitPortalRequest(input, { idempotencyKey: 'attachment-submit-2' }), 'IDEMPOTENCY_CONFLICT')
    const missingVersion = await expectError(() => new MockC311Provider({ scenario: 'expected-version-required', role: 'constituent' }).updateDraft('draft-fixture-001', { summary: 'x' }), 'EXPECTED_VERSION_REQUIRED')
    expect(missingVersion.status).to.equal(428)
  })

  it('enforces attachment download errors and the five-file staging limit', async () => {
    await expectError(() => new MockC311Provider({ scenario: 'not-found' }).downloadAttachment('missing-attachment'), 'NOT_FOUND')
    await expectError(() => new MockC311Provider({ scenario: 'forbidden' }).downloadAttachment('attachment-fixture-001'), 'FORBIDDEN')
    await expectError(() => new MockC311Provider({ role: 'public_visitor' }).downloadAttachment('attachment-fixture-001'), 'UNAUTHENTICATED')
    await expectError(() => new MockC311Provider({ role: 'service_agent' }).downloadAttachment('attachment-fixture-001'), 'FORBIDDEN')
    const authorized = await new MockC311Provider({ role: 'constituent' }).downloadAttachment('attachment-fixture-001')
    expect(authorized.body).to.equal('fixture attachment')

    const provider = new MockC311Provider({ role: 'public_visitor' })
    const uploaded: Array<{ attachment_token: string }> = []
    for (let index = 0; index < PORTAL_ATTACHMENT_MAX_COUNT; index += 1) {
      uploaded.push(await provider.uploadPortalAttachment({ file: 'fixture', filename: `fixture-${index}.txt`, media_type: 'text/plain' }))
    }
    const tooMany = await expectError(() => provider.uploadPortalAttachment({ file: 'fixture', filename: 'fixture-six.txt', media_type: 'text/plain' }), 'VALIDATION_ERROR')
    expect(tooMany.status).to.equal(422)
    expect(provider.getWriteCount('portal_attachment_upload')).to.equal(PORTAL_ATTACHMENT_MAX_COUNT)
    provider.removePortalAttachment(uploaded[0].attachment_token)
    const replacement = await provider.uploadPortalAttachment({ file: 'fixture', filename: 'replacement.txt', media_type: 'text/plain' })
    expect(replacement.filename).to.equal('replacement.txt')
    expect(provider.getWriteCount('portal_attachment_upload')).to.equal(PORTAL_ATTACHMENT_MAX_COUNT + 1)
    await provider.submitPortalRequest({
      summary: 'Consume staged attachments',
      description: 'This fixture consumes the current staged attachment batch.',
      service_type: 'GENERAL_INQUIRY',
      requester: { display_name: 'Fixture Resident', email: 'resident@example.test' },
      attachment_tokens: [...uploaded.slice(1).map(item => item.attachment_token), replacement.attachment_token],
    }, { idempotencyKey: 'attachment-batch-submit' })
    await provider.uploadPortalAttachment({ file: 'fixture', filename: 'next-request.txt', media_type: 'text/plain' })
    expect(provider.getWriteCount('portal_attachment_upload')).to.equal(PORTAL_ATTACHMENT_MAX_COUNT + 2)
  })

  it('exposes explicit idempotency and retryable attachment scenarios', async () => {
    const conflict = await expectError(() => new MockC311Provider({ scenario: 'idempotency-conflict' }).submitPortalRequest({
      summary: 'Fixture request',
      description: 'This request is long enough for the fixture.',
      service_type: 'GENERAL_INQUIRY',
      requester: { display_name: 'Fixture Resident', email: 'resident@example.test' },
    }, { idempotencyKey: 'fixture-key' }), 'IDEMPOTENCY_CONFLICT')
    expect(conflict.status).to.equal(409)
    const retryable = await expectError(() => new MockC311Provider({ scenario: 'retryable' }).uploadPortalAttachment({ file: 'fixture', filename: 'fixture.txt', media_type: 'text/plain' }), 'TEMPORARILY_UNAVAILABLE')
    expect(retryable.status).to.equal(503)
    expect(retryable.retryable).to.equal(true)
  })

  it('centralizes attachment contract validation and map failure fixtures', async () => {
    expect(PORTAL_ATTACHMENT_MAX_COUNT).to.equal(5)
    expect(PORTAL_ATTACHMENT_MAX_BYTES).to.equal(10485760)
    expect(PORTAL_ATTACHMENT_MEDIA_TYPES).to.have.members(['image/jpeg', 'image/png', 'application/pdf', 'text/plain', 'application/vnd.openxmlformats-officedocument.wordprocessingml.document'])
    expect(validatePortalAttachment({ filename: 'fixture.txt', media_type: 'text/plain', size: PORTAL_ATTACHMENT_MAX_BYTES })).to.deep.equal([])
    expect(validatePortalAttachment({ filename: '', media_type: 'text/plain', size: 1 }).map(error => error.code)).to.include('REQUIRED')
    expect(validatePortalAttachment({ filename: 'x'.repeat(121), media_type: 'application/zip', size: PORTAL_ATTACHMENT_MAX_BYTES + 1 }).map(error => error.code)).to.have.members(['TOO_LONG', 'INVALID_FORMAT', 'OUT_OF_RANGE'])

    const success = await new MockC311Provider().geocode({ address: '100 Example Street, Buffalo, NY 14201' })
    expect(success.address).to.equal('100 Example Street, Buffalo, NY 14201')
    expect(success.latitude.toFixed(4)).to.equal('42.9001')
    const normalized = await new MockC311Provider().geocode({ address: '  100   example street,   buffalo, ny 14201  ' })
    expect(normalized.address).to.equal(success.address)
    expect(normalized.latitude).to.equal(success.latitude)
    expect(normalized.longitude).to.equal(success.longitude)
    await expectError(() => new MockC311Provider({ scenario: 'not-found' }).geocode({ address: 'missing' }), 'ADDRESS_NOT_FOUND')
    const unavailable = await expectError(() => new MockC311Provider({ scenario: 'map-retryable' }).geocode({ address: 'fixture' }), 'MAP_TEMPORARILY_UNAVAILABLE')
    expect(unavailable.status).to.equal(503)
    const unauthenticated = await expectError(() => new MockC311Provider({ scenario: 'map-auth-failure' }).geocode({ address: 'fixture' }), 'MAP_UNAUTHENTICATED')
    expect(unauthenticated.status).to.equal(401)
  })

  it('rejects invalid attachment uploads before creating a staging write', async () => {
    const provider = new MockC311Provider({ role: 'public_visitor' })
    const boundary = await provider.uploadPortalAttachment({ file: new Blob([new Uint8Array(PORTAL_ATTACHMENT_MAX_BYTES)]), filename: 'boundary.txt', media_type: 'text/plain' })
    expect(boundary.size).to.equal(PORTAL_ATTACHMENT_MAX_BYTES)
    await expectError(() => provider.uploadPortalAttachment({ file: new Blob([new Uint8Array(1)]), filename: 'x'.repeat(121), media_type: 'application/zip' as 'text/plain' }), 'VALIDATION_ERROR')
    expect(provider.getWriteCount('portal_attachment_upload')).to.equal(1)
  })

  it('rejects invalid HTTP attachments before transport', async () => {
    const requests: C311TransportRequest[] = []
    const provider = new C311HttpProvider({
      request: async <T> (request: C311TransportRequest): Promise<T> => {
        requests.push(request)
        return {} as T
      },
    })
    const error = await expectError(() => provider.uploadPortalAttachment({ file: 'fixture', filename: '', media_type: 'text/plain' }), 'VALIDATION_ERROR')
    expect(error.status).to.equal(422)
    expect(requests).to.have.length(0)
  })

  it('exposes retryable and terminal portal submission failures', async () => {
    const input = {
      service_type: 'GENERAL_INQUIRY' as const,
      summary: 'A valid summary',
      description: 'A valid description for a portal request.',
      requester: { display_name: 'Fixture Resident', email: 'resident@example.test' },
    }
    const retryable = await expectError(() => new MockC311Provider({ scenario: 'retryable' }).submitPortalRequest(input), 'TEMPORARILY_UNAVAILABLE')
    expect(retryable.status).to.equal(503)
    expect(retryable.retryable).to.equal(true)
    const terminal = await expectError(() => new MockC311Provider({ scenario: 'terminal' }).submitPortalRequest(input), 'OPERATION_FAILED')
    expect(terminal.status).to.equal(500)
    expect(terminal.retryable).to.equal(false)
  })

  it('enforces portal query and account capabilities while keeping anonymous lookup public', async () => {
    await expectError(() => new MockC311Provider({ role: 'public_visitor' }).listPortalRequests(), 'UNAUTHENTICATED')
    await expectError(() => new MockC311Provider({ role: 'service_agent' }).listPortalRequests(), 'FORBIDDEN')
    await expectError(() => new MockC311Provider({ role: 'public_visitor' }).listStaffRequests(), 'UNAUTHENTICATED')
    await expectError(() => new MockC311Provider({ role: 'constituent' }).listStaffRequests(), 'FORBIDDEN')
    await expectError(() => new MockC311Provider({ role: 'public_visitor' }).getStaffRequest('request-fixture-001'), 'UNAUTHENTICATED')
    await expectError(() => new MockC311Provider({ role: 'constituent' }).getStaffRequest('request-fixture-001'), 'FORBIDDEN')
    await expectError(() => new MockC311Provider({ role: 'service_agent' }).getProfile(), 'FORBIDDEN')
    await expectError(() => new MockC311Provider({ role: 'public_visitor' }).changeLoginIdentifier({ current_password: 'fixture', login_identifier: 'alex.new' }), 'UNAUTHENTICATED')
    await expectError(() => new MockC311Provider({ role: 'service_agent' }).changePassword({ current_password: 'fixture', new_password: 'Fixture-password-2!' }), 'FORBIDDEN')
    await expectError(() => new MockC311Provider({ role: 'constituent', sessionVariant: 'expired' }).getProfile(), 'UNAUTHENTICATED')
    const anonymous = await new MockC311Provider({ role: 'public_visitor' }).getPublicStatus({ request_number: 'SR-2026-00001', email: 'wrong@example.test' })
    expect(anonymous).to.deep.equal({ request_detail: null })
    const page = await new MockC311Provider({ role: 'constituent' }).listPortalRequests({ page_size: 1, page_token: 'opaque-page' })
    expect(page.items).to.have.length(1)
    expect(page.applied_filters).to.deep.equal({})
  })

  it('requires a visible constituent relationship for portal request operations', async () => {
    const noRelationFixtures = cloneFixtureSet(createDefaultFixtureSet())
    noRelationFixtures.relationships!['request-fixture-001'] = [{ constituent_id: 'constituent-other', relationship_type: 'REPORTER', portal_visible: true, notify_status: false }]
    noRelationFixtures.public_relationships = noRelationFixtures.relationships
    const noRelation = new MockC311Provider({ role: 'constituent', fixtures: noRelationFixtures })
    await expectError(() => noRelation.listPortalRequests(), 'FORBIDDEN')
    await expectError(() => noRelation.createPortalNote('request-fixture-001', { body: 'Not allowed', portal_visible: true }), 'FORBIDDEN')
    await expectError(() => noRelation.reopenPortalRequest('request-fixture-001', 'Not allowed'), 'FORBIDDEN')

    const hiddenFixtures = cloneFixtureSet(createDefaultFixtureSet())
    hiddenFixtures.relationships!['request-fixture-001']![0].portal_visible = false
    hiddenFixtures.public_relationships = hiddenFixtures.relationships
    await expectError(() => new MockC311Provider({ role: 'constituent', fixtures: hiddenFixtures }).listPortalRequests(), 'FORBIDDEN')

    const visibleFixtures = cloneFixtureSet(createDefaultFixtureSet())
    visibleFixtures.requests[0].status = 'RESOLVED'
    visibleFixtures.details['request-fixture-001'].request.status = 'RESOLVED'
    visibleFixtures.public_details['SR-2026-00001'].status = 'RESOLVED'
    visibleFixtures.relationships!['request-fixture-001'] = [{ constituent_id: 'constituent-fixture-001', relationship_type: 'REPORTER', portal_visible: true, notify_status: true }]
    visibleFixtures.public_relationships = visibleFixtures.relationships
    const visible = new MockC311Provider({ role: 'constituent', fixtures: visibleFixtures })
    expect((await visible.listPortalRequests()).items).to.have.length(1)
    await visible.createPortalNote('request-fixture-001', { body: 'Visible follow-up', portal_visible: true })
    expect((await visible.reopenPortalRequest('request-fixture-001', 'Visible follow-up')).status).to.equal('PENDING_APPROVAL')
  })

  it('maps public status retryable and terminal failures without exposing a record', async () => {
    const retryable = await expectError(() => new MockC311Provider({ scenario: 'retryable' }).getPublicStatus({ request_number: 'SR-2026-00001', email: 'alex@example.test' }), 'TEMPORARILY_UNAVAILABLE')
    expect(retryable.status).to.equal(503)
    const terminal = await expectError(() => new MockC311Provider({ scenario: 'terminal' }).getPublicStatus({ request_number: 'SR-2026-00001', email: 'alex@example.test' }), 'OPERATION_FAILED')
    expect(terminal.status).to.equal(500)
  })

  it('maps constituent relationship and note operations to the frozen staff contract', async () => {
    const requests: C311TransportRequest[] = []
    const provider = new C311HttpProvider({
      request: async <T> (request: C311TransportRequest): Promise<T> => {
        requests.push(request)
        if (request.path.endsWith('/notes')) return { note_id: 'note-1', body: 'Fixture note', portal_visible: true } as T
        return { request: { request_id: 'request-fixture-001', version: 1 }, relationships: [] } as T
      },
    })
    await provider.linkStaffConstituent('request-fixture-001', { constituent_id: 'constituent-2', relationship_type: 'AFFECTED_RESIDENT', portal_visible: true, notify_status: false }, { expectedVersion: 1 })
    await provider.unlinkStaffConstituent('request-fixture-001', 'constituent-2', { reason: 'No longer affected.' }, { expectedVersion: 1 })
    await provider.createStaffNote('request-fixture-001', { body: 'Fixture note', portal_visible: true })
    expect(requests[0]).to.deep.include({ method: 'POST', path: '/api/v1/staff/service-requests/request-fixture-001/constituents', headers: { 'If-Match': '"1"' } })
    expect(requests[0].body).to.deep.equal({ constituent_id: 'constituent-2', relationship_type: 'AFFECTED_RESIDENT', portal_visible: true, notify_status: false })
    expect(requests[1]).to.deep.include({ method: 'DELETE', path: '/api/v1/staff/service-requests/request-fixture-001/constituents/constituent-2', headers: { 'If-Match': '"1"' } })
    expect(requests[1].body).to.deep.equal({ reason: 'No longer affected.' })
    expect(requests[2]).to.deep.include({ method: 'POST', path: '/api/v1/staff/service-requests/request-fixture-001/notes' })
  })

  it('enforces relationship capabilities, primary uniqueness, and append-only notes in the mock', async () => {
    const input = { constituent_id: 'constituent-2', relationship_type: 'AFFECTED_RESIDENT' as const, portal_visible: true, notify_status: true }
    await expectError(() => new MockC311Provider({ role: 'public_visitor' }).linkStaffConstituent('request-fixture-001', input, { expectedVersion: 1 }), 'UNAUTHENTICATED')
    await expectError(() => new MockC311Provider({ role: 'constituent' }).linkStaffConstituent('request-fixture-001', input, { expectedVersion: 1 }), 'FORBIDDEN')

    const agent = new MockC311Provider({ role: 'service_agent' })
    const initial = await agent.getStaffRequest('request-fixture-001')
    expect(initial.relationships?.find(item => item.constituent_id === 'constituent-fixture-001')).to.deep.include({ constituent_id: 'constituent-fixture-001', relationship_type: 'PRIMARY_REQUESTER', portal_visible: true, notify_status: false })
    const linked = await agent.linkStaffConstituent('request-fixture-001', input, { expectedVersion: initial.request.version })
    expect(linked.relationships?.find(item => item.constituent_id === input.constituent_id)).to.deep.include(input)
    expect(linked.request.version).to.equal(initial.request.version + 1)
    expect(linked.relationships?.find(item => item.constituent_id === 'constituent-2')).to.deep.include({ notification_target: 'constituent-2', notification_result: 'SENT' })
    expect(linked.relationships?.find(item => item.constituent_id === 'constituent-2')?.audit?.[0]).to.deep.include({ action: 'LINKED', actor_id: 'actor-fixture-agent' })
    expect(linked.relationships?.find(item => item.constituent_id === 'constituent-2')?.audit?.[0].audit_id).to.not.equal(initial.relationships?.find(item => item.constituent_id === 'constituent-fixture-001')?.audit?.[0].audit_id)
    expect(agent.getWriteCount('staff_constituent_link')).to.equal(1)
    await expectError(() => agent.linkStaffConstituent('request-fixture-001', input, { expectedVersion: initial.request.version }), 'VERSION_CONFLICT')
    await expectError(() => agent.linkStaffConstituent('request-fixture-001', { ...input, relationship_type: 'PRIMARY_REQUESTER' }, { expectedVersion: initial.request.version + 1 }), 'VALIDATION_ERROR')
    const unlinked = await agent.unlinkStaffConstituent('request-fixture-001', 'constituent-2', { reason: 'Fixture cleanup.' }, { expectedVersion: linked.request.version })
    expect(unlinked.relationships).to.have.length(1)
    expect(unlinked.request.version).to.equal(linked.request.version + 1)
    expect(unlinked.audit.some(event => event.action === 'UNLINKED' && event.actor_id === 'actor-fixture-agent')).to.equal(true)
    await expectError(() => agent.unlinkStaffConstituent('request-fixture-001', 'constituent-fixture-001', { reason: 'Cannot remove primary.' }, { expectedVersion: unlinked.request.version }), 'VALIDATION_ERROR')

    const firstNote = await agent.createStaffNote('request-fixture-001', { body: 'First fixture note', portal_visible: true })
    const secondNote = await agent.createStaffNote('request-fixture-001', { body: 'Second fixture note', portal_visible: false })
    expect(firstNote.note_id).not.to.equal(secondNote.note_id)
    expect((await agent.getStaffRequest('request-fixture-001')).notes).to.have.length(2)
    expect(agent.getWriteCount('staff_note_create')).to.equal(2)
  })

  it('refreshes the public relationship and note projection after staff writes', async () => {
    const agent = new MockC311Provider({ role: 'service_agent' })
    const initial = await agent.getStaffRequest('request-fixture-001')
    await agent.linkStaffConstituent('request-fixture-001', { constituent_id: 'constituent-2', relationship_type: 'AFFECTED_RESIDENT', portal_visible: true, notify_status: true }, { expectedVersion: initial.request.version })
    await agent.createStaffNote('request-fixture-001', { body: 'Public staff update', portal_visible: true })
    const detail = (await agent.getPublicStatus({ request_number: 'SR-2026-00001', email: 'alex@example.test' })).request_detail
    expect(detail?.relationships?.find(item => item.constituent_id === 'constituent-2')).to.deep.include({ constituent_id: 'constituent-2', relationship_type: 'AFFECTED_RESIDENT', portal_visible: true, notify_status: true, notification_target: 'constituent-2', notification_result: 'SENT' })
    expect(detail?.notes?.some(note => note.body === 'Public staff update')).to.equal(true)
  })
})

describe('FE-06 staff queue and detail contract', () => {
  it('filters the queue, preserves opaque pagination, and enforces role scope', async () => {
    const fixtures = createDefaultFixtureSet()
    const second = JSON.parse(JSON.stringify(fixtures.queue[0]))
    second.request_id = 'request-fixture-002'
    second.request_number = 'SR-2026-00002'
    second.owning_department = 'GENERAL_SERVICES'
    second.council_district = 'SOUTH'
    second.service_type = 'GENERAL_INQUIRY'
    fixtures.queue.push(second)
    const secondDetail = JSON.parse(JSON.stringify(fixtures.details['request-fixture-001']))
    secondDetail.request.request_id = second.request_id
    secondDetail.request.request_number = second.request_number
    secondDetail.request.owning_department = second.owning_department
    secondDetail.request.council_district = second.council_district
    secondDetail.request.service_type = second.service_type
    fixtures.details[second.request_id] = secondDetail

    const manager = new MockC311Provider({ role: 'department_manager', fixtures })
    const page = await manager.listStaffRequests({ department: 'STREETS', page_size: 1, sort: '-updated_at' })
    expect(page.items).to.have.length(1)
    expect(page.applied_filters).to.include({ department: 'STREETS' })
    expect(page.sort).to.deep.equal(['-updated_at'])
    expect(page.next_page_token).to.equal(null)

    const workflow = new MockC311Provider({ role: 'workflow_designer', fixtures })
    await expectError(() => workflow.listStaffRequests(), 'FORBIDDEN')
    await expectError(() => manager.getStaffRequest(second.request_id), 'FORBIDDEN')
    await expectError(() => manager.listStaffRequests({ filters: { unsupported: 'value' } }), 'INVALID_FILTER')
    await expectError(() => new MockC311Provider({ role: 'service_agent', sessionVariant: 'expired' }).listStaffRequests(), 'UNAUTHENTICATED')
    await expectError(() => new MockC311Provider().listStaffRequests(), 'FORBIDDEN')
  })

  it('filters out-of-scope records while protecting direct detail access', async () => {
    const agent = new MockC311Provider({ role: 'service_agent', scenario: 'scope-filter' })
    const agentPage = await agent.listStaffRequests()
    expect(agentPage.items).to.have.length(1)
    expect(agentPage.items[0].owning_department).to.equal('STREETS')
    await expectError(() => agent.getStaffRequest('request-fixture-foreign'), 'FORBIDDEN')

    const administrator = new MockC311Provider({ role: 'platform_administrator', scenario: 'scope-filter' })
    const administratorPage = await administrator.listStaffRequests()
    expect(administratorPage.items).to.have.length(2)
    expect(administratorPage.items.some(item => item.request_id === 'request-fixture-foreign')).to.equal(true)
    const foreignDetail = await administrator.getStaffRequest('request-fixture-foreign')
    expect(foreignDetail.request.request_number).to.equal('SR-2026-00099')
  })

  it('requires If-Match and increments the detail version for staff writes', async () => {
    const provider = new MockC311Provider({ role: 'supervisor' })
    const before = await provider.getStaffRequest('request-fixture-001')
    await expectError(() => provider.transitionStaffRequest('request-fixture-001', { to_status: 'TRIAGED' }), 'EXPECTED_VERSION_REQUIRED')
    const transitioned = await provider.transitionStaffRequest('request-fixture-001', { to_status: 'TRIAGED' }, { expectedVersion: before.request.version })
    expect(transitioned.request.status).to.equal('TRIAGED')
    expect(transitioned.request.version).to.equal(before.request.version + 1)
    const reassigned = await provider.reassignStaffRequest('request-fixture-001', { assignee_id: 'actor-fixture-agent', reason: 'fixture' }, { expectedVersion: transitioned.request.version })
    expect(reassigned.primary_assignee_id).to.equal('actor-fixture-agent')
    expect(reassigned.request.version).to.equal(transitioned.request.version + 1)
    const staffAgent = new MockC311Provider({ role: 'service_agent' })
    const reminder = await staffAgent.createStaffReminder('request-fixture-001', { title: 'Fixture reminder', due_at: '2026-01-16T15:00:00.000Z', timezone: 'America/New_York', recipient_staff_id: 'actor-fixture-agent', channel: 'IN_APP' })
    expect(reminder.status).to.equal('SCHEDULED')
    const note = await staffAgent.createStaffNote('request-fixture-001', { body: 'Fixture note', portal_visible: false })
    expect(note.request_id).to.equal('request-fixture-001')
    expect(provider.getWriteCount('staff_request_transition')).to.equal(1)
    expect(provider.getWriteCount('staff_request_reassign')).to.equal(1)
    expect(staffAgent.getWriteCount('staff_reminder_create')).to.equal(1)
    expect(staffAgent.getWriteCount('staff_note_create')).to.equal(1)
  })

  it('maps all FE-06 HTTP operations to contract paths and headers', async () => {
    const requests: C311TransportRequest[] = []
    const transport = { request: async <T>(request: C311TransportRequest) => { requests.push(request); return {} as T } }
    const provider = new C311HttpProvider(transport)
    await provider.reassignStaffRequest('r1', { assignee_id: 'a1', reason: 'fixture' }, { expectedVersion: 2 })
    await provider.addStaffCollaborator('r1', 'a2', { reason: 'fixture' }, { expectedVersion: 3 })
    await provider.removeStaffCollaborator('r1', 'a2', { reason: 'fixture' }, { expectedVersion: 4 })
    await provider.createStaffReminder('r1', { title: 'Reminder', due_at: '2026-01-16T15:00:00.000Z', timezone: 'America/New_York', recipient_staff_id: 'a1', channel: 'IN_APP' })
    await provider.actionStaffReminder('rem1', 'COMPLETE')
    await provider.overrideStaffOrigin('r1', { origin_class: 'INTERNAL', reason: 'fixture' }, { expectedVersion: 5 })
    await provider.overrideStaffScope('r1', { department_code: 'STREETS', district_codes: ['NORTH'], reason: 'fixture' }, { expectedVersion: 6 })
    await provider.confirmStaffDuplicateGroup('r1', { duplicate_group_id: 'dg1', reason: 'fixture' }, { expectedVersion: 7 })
    await provider.removeStaffDuplicateGroup('r1', { reason: 'fixture' }, { expectedVersion: 8 })
    await provider.approveStaffReopen('r1', { reason: 'fixture' }, { expectedVersion: 9 })
    expect(requests.map(request => `${request.method} ${request.path}`)).to.deep.equal([
      'POST /api/v1/staff/service-requests/r1/assignment',
      'PUT /api/v1/staff/service-requests/r1/collaborators/a2',
      'DELETE /api/v1/staff/service-requests/r1/collaborators/a2',
      'POST /api/v1/staff/service-requests/r1/reminders',
      'POST /api/v1/staff/reminders/rem1/COMPLETE',
      'POST /api/v1/staff/service-requests/r1/origin-class',
      'POST /api/v1/staff/service-requests/r1/scope-override',
      'POST /api/v1/staff/service-requests/r1/duplicate-group',
      'DELETE /api/v1/staff/service-requests/r1/duplicate-group',
      'POST /api/v1/staff/service-requests/r1/reopen/approve',
    ])
    expect(requests[0].headers).to.deep.equal({ 'If-Match': '"2"' })
    expect(requests[1].headers).to.deep.equal({ 'If-Match': '"3"' })
  })

  it('applies fixture failures and reminder scope checks to staff mutations', async () => {
    const supervisorInput = { assignee_id: 'actor-fixture-agent', reason: 'fixture' }
    await expectError(() => new MockC311Provider({ role: 'supervisor', scenario: 'forbidden' }).reassignStaffRequest('request-fixture-001', supervisorInput, { expectedVersion: 1 }), 'FORBIDDEN')
    await expectError(() => new MockC311Provider({ role: 'supervisor', scenario: 'version-conflict' }).reassignStaffRequest('request-fixture-001', supervisorInput, { expectedVersion: 1 }), 'VERSION_CONFLICT')

    const supervisor = new MockC311Provider({ role: 'supervisor' })
    await expectError(() => supervisor.actionStaffReminder('missing-reminder', 'COMPLETE'), 'NOT_FOUND')
    await expectError(() => new MockC311Provider({ role: 'service_agent', scenario: 'forbidden' }).createStaffNote('request-fixture-001', { body: 'fixture', portal_visible: false }), 'FORBIDDEN')
  })

  it('enforces the FE-07 status machine and keeps invalid transitions side-effect free', async () => {
    const provider = new MockC311Provider({ role: 'supervisor' })
    const before = await provider.getStaffRequest('request-fixture-001')
    const triaged = await provider.transitionStaffRequest('request-fixture-001', { to_status: 'TRIAGED', reason: 'reviewed' }, { expectedVersion: before.request.version })
    expect(triaged.request.version).to.equal(before.request.version + 1)
    expect(triaged.available_actions).to.deep.equal(['ASSIGN'])
    await expectError(() => provider.transitionStaffRequest('request-fixture-001', { to_status: 'CLOSED' }, { expectedVersion: triaged.request.version }), 'INVALID_STATUS_TRANSITION')
    const unchanged = await provider.getStaffRequest('request-fixture-001')
    expect(unchanged.request.status).to.equal('TRIAGED')
    expect(unchanged.request.version).to.equal(triaged.request.version)
    expect(provider.getWriteCount('staff_request_transition')).to.equal(1)
  })

  it('applies scope and duplicate controls with versioned staff writes', async () => {
    const manager = new MockC311Provider({ role: 'department_manager' })
    const before = await manager.getStaffRequest('request-fixture-001')
    const scoped = await manager.overrideStaffScope('request-fixture-001', { department_code: 'STREETS', district_codes: ['NORTH'], reason: 'fixture scope' }, { expectedVersion: before.request.version })
    expect(scoped.request.version).to.equal(before.request.version + 1)
    expect(scoped.request.owning_department).to.equal('STREETS')
    const supervisor = new MockC311Provider({ role: 'supervisor' })
    const duplicateBefore = await supervisor.getStaffRequest('request-fixture-001')
    const grouped = await supervisor.confirmStaffDuplicateGroup('request-fixture-001', { duplicate_group_id: 'duplicate-fixture-001', reason: 'fixture duplicate' }, { expectedVersion: duplicateBefore.request.version })
    expect(grouped.request.duplicate_group_id).to.equal('duplicate-fixture-001')
    expect(grouped.request.version).to.equal(duplicateBefore.request.version + 1)
    const removed = await supervisor.removeStaffDuplicateGroup('request-fixture-001', { reason: 'fixture remove' }, { expectedVersion: grouped.request.version })
    expect(removed.request.duplicate_group_id).to.equal(undefined)
    expect(removed.request.version).to.equal(grouped.request.version + 1)
    expect(manager.getWriteCount('staff_scope_override')).to.equal(1)
    expect(supervisor.getWriteCount('staff_duplicate_group_confirm')).to.equal(1)
    expect(supervisor.getWriteCount('staff_duplicate_group_remove')).to.equal(1)
    await expectError(() => new MockC311Provider({ role: 'service_agent' }).overrideStaffScope('request-fixture-001', { department_code: 'STREETS', district_codes: ['NORTH'], reason: 'forbidden' }, { expectedVersion: 1 }), 'FORBIDDEN')
  })

  it('performs atomic, idempotent bulk updates with expected versions', async () => {
    const provider = new MockC311Provider({ role: 'supervisor', scenario: 'pagination' })
    const page = await provider.listStaffRequests({ page_size: 2 })
    const input = { action: 'UPDATE' as const, changes: { primary_assignee_id: 'actor-fixture-agent', staff_note: 'bulk fixture' }, request_items: page.items.map(item => ({ request_id: item.request_id, expected_version: item.version })) }
    const result = await provider.bulkStaffRequests(input, { idempotencyKey: 'bulk-fixture-001' })
    expect(result.updated_count).to.equal(2)
    expect(await provider.bulkStaffRequests(input, { idempotencyKey: 'bulk-fixture-001' })).to.deep.equal(result)
    expect(provider.getWriteCount('staff_request_bulk')).to.equal(1)
    const currentItems = await provider.listStaffRequests({ page_size: 2 })
    await expectError(() => provider.bulkStaffRequests({ ...input, changes: { status: 'CLOSED' }, request_items: currentItems.items.map(item => ({ request_id: item.request_id, expected_version: item.version })) }, { idempotencyKey: 'bulk-fixture-002' }), 'INVALID_STATUS_TRANSITION')
    expect((await provider.getStaffRequest(page.items[0].request_id)).primary_assignee_id).to.equal('actor-fixture-agent')
  })

  it('rejects a CLOSE batch that also requests a second status transition', async () => {
    const fixtures = cloneFixtureSet(createDefaultFixtureSet())
    fixtures.requests[0].status = 'RESOLVED'
    fixtures.queue[0].status = 'RESOLVED'
    fixtures.details['request-fixture-001'].request.status = 'RESOLVED'
    const provider = new MockC311Provider({ role: 'supervisor', fixtures })

    await expectError(() => provider.bulkStaffRequests({
      action: 'CLOSE',
      changes: { status: 'REOPENED' },
      request_items: [{ request_id: 'request-fixture-001', expected_version: 1 }],
    }, { idempotencyKey: 'bulk-close-status-fixture' }), 'VALIDATION_ERROR')

    const unchanged = await provider.getStaffRequest('request-fixture-001')
    expect(unchanged.request.status).to.equal('RESOLVED')
    expect(unchanged.request.version).to.equal(1)
    expect(provider.getWriteCount('staff_request_bulk')).to.equal(0)
  })

  it('applies bulk priority and appends staff notes to the detail note collection', async () => {
    const provider = new MockC311Provider({ role: 'supervisor' })
    const result = await provider.bulkStaffRequests({
      action: 'UPDATE',
      changes: { priority: 'HIGH', staff_note: 'Reviewed by the bulk desk.' },
      request_items: [{ request_id: 'request-fixture-001', expected_version: 1 }],
    }, { idempotencyKey: 'bulk-priority-note-fixture' })

    expect(result.updated_count).to.equal(1)
    const detail = await provider.getStaffRequest('request-fixture-001')
    expect((detail.request as typeof detail.request & { priority?: string }).priority).to.equal('HIGH')
    expect(detail.notes?.map(note => note.body)).to.include('Reviewed by the bulk desk.')
    expect(detail.audit.some(event => String(event.action).startsWith('BULK_NOTE:'))).to.equal(false)
  })

  it('rolls back every bulk record when a later selected record fails', async () => {
    const fixtures = cloneFixtureSet(createDefaultFixtureSet())
    fixtures.requests[0].status = 'RESOLVED'
    fixtures.queue[0].status = 'RESOLVED'
    fixtures.details['request-fixture-001'].request.status = 'RESOLVED'
    fixtures.queue.push({ ...fixtures.queue[0], request_id: 'request-fixture-002', request_number: 'SR-2026-00002' })
    fixtures.details['request-fixture-002'] = { ...cloneFixtureSet(fixtures).details['request-fixture-001'], request: { ...fixtures.details['request-fixture-001'].request, request_id: 'request-fixture-002', request_number: 'SR-2026-00002' } }
    const provider = new MockC311Provider({ role: 'supervisor', fixtures })
    const page = await provider.listStaffRequests({ page_size: 2 })

    const error = await expectError(() => provider.bulkStaffRequests({
      action: 'CLOSE',
      changes: {},
      request_items: page.items.map(item => ({ request_id: item.request_id, expected_version: item.version })),
    }, { idempotencyKey: 'bulk-rollback-fixture' }), 'NOT_FOUND')

    expect(error.failingRequestID).to.equal('request-fixture-002')
    const unchanged = await provider.getStaffRequest('request-fixture-001')
    expect(unchanged.request.status).to.equal('RESOLVED')
    expect(unchanged.request.version).to.equal(1)
    expect(provider.getWriteCount('staff_request_bulk')).to.equal(0)
  })

  it('rejects bulk records from different departments or duplicate groups before changing data', async () => {
    for (const mismatch of ['department', 'duplicate-group'] as const) {
      const fixtures = cloneFixtureSet(createDefaultFixtureSet())
      const first = fixtures.details['request-fixture-001']
      first.request.duplicate_group_id = 'duplicate-fixture-001'
      fixtures.requests[0].duplicate_group_id = 'duplicate-fixture-001'
      fixtures.queue[0].duplicate_group_id = 'duplicate-fixture-001'
      const secondRequest = {
        ...fixtures.requests[0],
        request_id: 'request-fixture-002',
        request_number: 'SR-2026-00002',
        owning_department: mismatch === 'department' ? 'PUBLIC_WORKS' : 'STREETS',
        duplicate_group_id: mismatch === 'duplicate-group' ? 'duplicate-fixture-002' : 'duplicate-fixture-001',
      } as typeof fixtures.requests[number]
      fixtures.requests.push(secondRequest)
      fixtures.queue.push({ ...fixtures.queue[0], request_id: secondRequest.request_id, request_number: secondRequest.request_number || '', owning_department: secondRequest.owning_department, duplicate_group_id: secondRequest.duplicate_group_id })
      fixtures.details[secondRequest.request_id] = { ...cloneFixtureSet(fixtures).details['request-fixture-001'], request: secondRequest }
      const provider = new MockC311Provider({ role: 'department_manager', fixtures })
      const items = fixtures.queue.map(item => ({ request_id: item.request_id, expected_version: item.version }))

      const error = await expectError(() => provider.bulkStaffRequests({ action: 'UPDATE', changes: { priority: 'HIGH' }, request_items: items }, { idempotencyKey: `bulk-${mismatch}-fixture` }), 'VALIDATION_ERROR')
      expect(error.failingRequestID).to.equal('request-fixture-002')
      expect((await provider.getStaffRequest('request-fixture-001')).request.version).to.equal(1)
      expect((await provider.getStaffRequest('request-fixture-002')).request.version).to.equal(1)
      expect(provider.getWriteCount('staff_request_bulk')).to.equal(0)
    }
  })

  it('keeps the frozen bulk role matrix limited to supervisors and department managers', async () => {
    const fixtures = createDefaultFixtureSet()
    expect(fixtures.role_fixtures.platform_administrator.session.actor?.capabilities).to.not.include('staff_request_bulk')
    await expectError(() => new MockC311Provider({ role: 'platform_administrator' }).bulkStaffRequests({
      action: 'UPDATE',
      changes: { priority: 'HIGH' },
      request_items: [{ request_id: 'request-fixture-001', expected_version: 1 }],
    }, { idempotencyKey: 'bulk-admin-forbidden' }), 'FORBIDDEN')
  })

  it('models reminder lifecycle and CivicWorks event idempotency', async () => {
    const fixtures = createDefaultFixtureSet()
    fixtures.details['request-fixture-001'].reminders = [{ reminder_id: 'reminder-fixture-001', request_id: 'request-fixture-001', title: 'Existing', due_at: '2026-01-16T15:00:00.000Z', timezone: 'America/New_York', recipient_staff_id: 'actor-fixture-supervisor', channel: 'IN_APP', status: 'SCHEDULED', completed_at: null }]
    const supervisor = new MockC311Provider({ role: 'supervisor', fixtures })
    const snoozed = await supervisor.actionStaffReminder('reminder-fixture-001', 'SNOOZE', { due_at: '2026-01-17T15:00:00.000Z' })
    expect(snoozed.status).to.equal('SNOOZED')
    expect((snoozed as typeof snoozed & { history?: Array<Record<string, unknown>> }).history).to.deep.equal([{
      action: 'SNOOZE',
      previous_due_at: '2026-01-16T15:00:00.000Z',
      due_at: '2026-01-17T15:00:00.000Z',
      occurred_at: '2026-01-15T15:00:00.000Z',
    }])
    const completed = await supervisor.actionStaffReminder('reminder-fixture-001', 'COMPLETE')
    expect(completed.status).to.equal('COMPLETED')
    expect(await supervisor.actionStaffReminder('reminder-fixture-001', 'COMPLETE')).to.deep.equal(completed)

    const event = { event_id: 'cw-event-001', event_type: 'work_order.status_changed' as const, work_order_id: 'cw-001', source_case_id: 'request-fixture-001', previous_status: 'ASSIGNED' as const, status: 'COMPLETED' as const, version: 2, occurred_at: '2026-01-15T15:00:00.000Z' }
    const result = await supervisor.processCivicWorksEvent(event, event.event_id, 'fixture-signature')
    expect(result.acknowledged).to.equal(true)
    expect((await supervisor.processCivicWorksEvent(event, event.event_id, 'fixture-signature')).duplicate).to.equal(true)
    expect(supervisor.getWriteCount('civicworks_event_callback')).to.equal(1)
    await expectError(() => new MockC311Provider({ scenario: 'civicworks-invalid-signature' }).processCivicWorksEvent(event, event.event_id, 'bad'), 'INVALID_SIGNATURE')
  })

  it('keeps reassignment available after assignment and records complete audit context', async () => {
    const fixtures = cloneFixtureSet(createDefaultFixtureSet())
    fixtures.details['request-fixture-001'].primary_assignee_id = 'staff-fixture-former'
    const provider = new MockC311Provider({ role: 'supervisor', fixtures })
    const submitted = await provider.getStaffRequest('request-fixture-001')
    const triaged = await provider.transitionStaffRequest('request-fixture-001', { to_status: 'TRIAGED', reason: 'triaged' }, { expectedVersion: submitted.request.version })
    const assigned = await provider.transitionStaffRequest('request-fixture-001', { to_status: 'ASSIGNED', reason: 'assigned' }, { expectedVersion: triaged.request.version })
    const reassigned = await provider.reassignStaffRequest('request-fixture-001', { assignee_id: 'staff-fixture-new', reason: 'Balance the workload' }, { expectedVersion: assigned.request.version })

    expect(reassigned.request.status).to.equal('ASSIGNED')
    expect(reassigned.audit[reassigned.audit.length - 1]).to.include({ action: 'ASSIGN', reason: 'Balance the workload', previous_assignee_id: 'staff-fixture-former', assignee_id: 'staff-fixture-new' })
    expect(reassigned.assignment_notifications).to.deep.include.members([
      { notification_id: 'assignment-notification-fixture-001', request_id: 'request-fixture-001', recipient_staff_id: 'staff-fixture-former', recipient_role: 'FORMER_PRIMARY_ASSIGNEE', result: 'SENT', occurred_at: '2026-01-15T15:00:00.000Z' },
      { notification_id: 'assignment-notification-fixture-002', request_id: 'request-fixture-001', recipient_staff_id: 'staff-fixture-new', recipient_role: 'NEW_PRIMARY_ASSIGNEE', result: 'SENT', occurred_at: '2026-01-15T15:00:00.000Z' },
    ])
    expect(reassigned.external_work_order).to.deep.include({
      source_case_id: 'request-fixture-001',
      service_request_number: 'SR-2026-00001',
      status: 'ASSIGNED',
      version: 1,
      created_at: '2026-01-15T15:00:00.000Z',
      updated_at: '2026-01-15T15:00:00.000Z',
    })
    expect(reassigned.external_work_order?.external_status_url).to.match(/^https?:\/\//)
    const started = await provider.transitionStaffRequest('request-fixture-001', { to_status: 'IN_PROGRESS', reason: 'started' }, { expectedVersion: reassigned.request.version })
    const movedAgain = await provider.reassignStaffRequest('request-fixture-001', { assignee_id: 'staff-fixture-other', reason: 'Specialist required' }, { expectedVersion: started.request.version })
    expect(movedAgain.request.status).to.equal('IN_PROGRESS')
    expect(movedAgain.audit[movedAgain.audit.length - 1]).to.include({ previous_assignee_id: 'staff-fixture-new', assignee_id: 'staff-fixture-other', reason: 'Specialist required' })
  })

  it('rejects unsupported reminder channels and malformed timestamps', async () => {
    const agent = new MockC311Provider({ role: 'service_agent' })
    const base = { title: 'Fixture reminder', timezone: 'America/New_York', recipient_staff_id: 'actor-fixture-agent' }
    await expectError(() => agent.createStaffReminder('request-fixture-001', { ...base, due_at: '2026-01-16T15:00:00.000Z', channel: 'SMS' as never }), 'VALIDATION_ERROR')
    await expectError(() => agent.createStaffReminder('request-fixture-001', { ...base, due_at: 'not-a-date', channel: 'IN_APP' }), 'VALIDATION_ERROR')

    const supervisor = new MockC311Provider({ role: 'supervisor' })
    await expectError(() => supervisor.actionStaffReminder('reminder-fixture-001', 'SNOOZE', { due_at: 'not-a-date' }), 'VALIDATION_ERROR')
  })

  it('allows a real retry after single and bulk version conflicts are reloaded', async () => {
    const single = new MockC311Provider({ role: 'supervisor', scenario: 'version-conflict' })
    await expectError(() => single.reassignStaffRequest('request-fixture-001', { assignee_id: 'staff-fixture-new', reason: 'Keep this input' }, { expectedVersion: 1 }), 'VERSION_CONFLICT')
    const current = await single.getStaffRequest('request-fixture-001')
    expect(current.request.version).to.equal(2)
    const reapplied = await single.reassignStaffRequest('request-fixture-001', { assignee_id: 'staff-fixture-new', reason: 'Keep this input' }, { expectedVersion: current.request.version })
    expect(reapplied.primary_assignee_id).to.equal('staff-fixture-new')
    expect(reapplied.request.version).to.equal(3)

    const bulk = new MockC311Provider({ role: 'supervisor', scenario: 'bulk-version-conflict' })
    const input = { action: 'UPDATE' as const, changes: { priority: 'HIGH' }, request_items: [{ request_id: 'request-fixture-001', expected_version: 1 }] }
    await expectError(() => bulk.bulkStaffRequests(input, { idempotencyKey: 'bulk-retry-fixture' }), 'VERSION_CONFLICT')
    const bulkCurrent = await bulk.getStaffRequest('request-fixture-001')
    expect(bulkCurrent.request.version).to.equal(2)
    const bulkResult = await bulk.bulkStaffRequests({ ...input, request_items: [{ request_id: 'request-fixture-001', expected_version: bulkCurrent.request.version }] }, { idempotencyKey: 'bulk-retry-fixture' })
    expect(bulkResult.updated_count).to.equal(1)
    expect((await bulk.getStaffRequest('request-fixture-001')).request.version).to.equal(3)
  })

  it('normalizes direct CivicWorks completion through the legal CRM lifecycle', async () => {
    const fixtures = cloneFixtureSet(createDefaultFixtureSet())
    fixtures.requests[0].status = 'ASSIGNED'
    fixtures.queue[0].status = 'ASSIGNED'
    fixtures.details['request-fixture-001'].request.status = 'ASSIGNED'
    const provider = new MockC311Provider({ role: 'supervisor', fixtures })
    const event = { event_id: 'cw-direct-completion', event_type: 'work_order.status_changed' as const, work_order_id: 'cw-001', source_case_id: 'request-fixture-001', previous_status: 'ASSIGNED' as const, status: 'COMPLETED' as const, version: 2, occurred_at: '2026-01-15T15:00:00.000Z' }

    await provider.processCivicWorksEvent(event, event.event_id, 'fixture-signature')

    const detail = await provider.getStaffRequest('request-fixture-001')
    expect(detail.request.status).to.equal('RESOLVED')
    expect(detail.request.version).to.equal(3)
    expect(detail.history.slice(-2).map(item => item.action)).to.deep.equal(['IN_PROGRESS', 'RESOLVED'])
    expect(detail.external_work_order).to.deep.equal({
      work_order_id: 'cw-001',
      source_case_id: 'request-fixture-001',
      service_request_number: 'SR-2026-00001',
      status: 'COMPLETED',
      external_status_url: 'https://civicworks.fixture.invalid/ui/work-orders/cw-001',
      version: 2,
      created_at: '2026-01-15T15:00:00.000Z',
      updated_at: '2026-01-15T15:00:00.000Z',
    })
  })

  it('rejects malformed CivicWorks events and applies new versions exactly once', async () => {
    const fixtures = cloneFixtureSet(createDefaultFixtureSet())
    fixtures.requests[0].status = 'ASSIGNED'
    fixtures.queue[0].status = 'ASSIGNED'
    fixtures.details['request-fixture-001'].request.status = 'ASSIGNED'
    fixtures.details['request-fixture-001'].external_work_order = {
      work_order_id: 'cw-001', source_case_id: 'request-fixture-001', service_request_number: 'SR-2026-00001', status: 'ASSIGNED', external_status_url: 'https://civicworks.fixture.invalid/ui/work-orders/cw-001', version: 2, created_at: '2026-01-15T14:00:00.000Z', updated_at: '2026-01-15T14:00:00.000Z',
    }
    const provider = new MockC311Provider({ role: 'supervisor', fixtures })
    const base = { event_type: 'work_order.status_changed' as const, work_order_id: 'cw-001', source_case_id: 'request-fixture-001', previous_status: 'ASSIGNED' as const, version: 3, occurred_at: '2026-01-15T15:00:00.000Z' }

    await expectError(() => provider.processCivicWorksEvent({ ...base, event_id: 'cw-invalid-signature', status: 'IN_PROGRESS' }, 'cw-invalid-signature', 'not-the-fixture-signature'), 'INVALID_SIGNATURE')
    await expectError(() => provider.processCivicWorksEvent({ ...base, event_id: 'cw-invalid-status', status: 'BOGUS' as never }, 'cw-invalid-status', 'fixture-signature'), 'VALIDATION_ERROR')
    await expectError(() => provider.processCivicWorksEvent({ ...base, event_id: 'cw-invalid-date', status: 'IN_PROGRESS', occurred_at: 'not-a-date' }, 'cw-invalid-date', 'fixture-signature'), 'VALIDATION_ERROR')
    expect((await provider.getStaffRequest('request-fixture-001')).request.status).to.equal('ASSIGNED')

    const old = { ...base, event_id: 'cw-old', status: 'IN_PROGRESS' as const, version: 2 }
    expect(await provider.processCivicWorksEvent(old, old.event_id, 'fixture-signature')).to.deep.equal({ acknowledged: true })
    expect(await provider.processCivicWorksEvent(old, old.event_id, 'fixture-signature')).to.deep.equal({ acknowledged: true, duplicate: true })
    expect((await provider.getStaffRequest('request-fixture-001')).request.status).to.equal('ASSIGNED')

    const current = { ...base, event_id: 'cw-current', status: 'IN_PROGRESS' as const }
    expect(await provider.processCivicWorksEvent(current, current.event_id, 'fixture-signature')).to.deep.equal({ acknowledged: true })
    expect(await provider.processCivicWorksEvent(current, current.event_id, 'fixture-signature')).to.deep.equal({ acknowledged: true, duplicate: true })
    const detail = await provider.getStaffRequest('request-fixture-001')
    expect(detail.request.status).to.equal('IN_PROGRESS')
    expect(detail.history.filter(item => item.action === 'IN_PROGRESS')).to.have.length(1)
    expect(provider.getWriteCount('civicworks_event_callback')).to.equal(1)
  })

  it('maps FE-07 bulk and CivicWorks HTTP contracts', async () => {
    const requests: C311TransportRequest[] = []
    const transport = { request: async <T>(request: C311TransportRequest) => { requests.push(request); return {} as T } }
    const provider = new C311HttpProvider(transport)
    await provider.bulkStaffRequests({ action: 'CLOSE', changes: {}, request_items: [{ request_id: 'r1', expected_version: 2 }] }, { idempotencyKey: 'bulk-key' })
    await provider.processCivicWorksEvent({ event_id: 'e1', event_type: 'work_order.status_changed', work_order_id: 'w1', source_case_id: 'r1', previous_status: 'ASSIGNED', status: 'IN_PROGRESS', version: 2, occurred_at: '2026-01-15T15:00:00.000Z' }, 'e1', 'sig')
    expect(requests[0]).to.include({ method: 'POST', path: '/api/v1/staff/service-requests/bulk' })
    expect(requests[0].headers).to.deep.equal({ 'Idempotency-Key': 'bulk-key' })
    expect(requests[1].path).to.equal('/integrations/civicworks/events')
    expect(requests[1].headers).to.deep.equal({ 'Content-Type': 'application/json', 'X-CivicWorks-Event-Id': 'e1', 'X-CivicWorks-Signature': 'sig' })
  })
})

describe('FE-09 workflow and extension provider', () => {
  it('maps FE-09 workflow action, operation, calendar, mail, report, audit and export contracts', async () => {
    const requests: C311TransportRequest[] = []
    const transport = { request: async <T> (request: C311TransportRequest): Promise<T> => { requests.push(request); return {} as T } }
    const provider = new C311HttpProvider(transport)
    await provider.executeWorkflowAction({ action: 'notify_department', request_id: 'request-fixture-001', payload: { channel: 'EMAIL' } }, { idempotencyKey: 'action-key' })
    await provider.getOperation('operation/1')
    await provider.importCalendar({ ics: 'BEGIN:VCALENDAR\r\nEND:VCALENDAR' })
    await provider.exportCalendar()
    await provider.previewMail({ to: ['fixture@example.test'], subject: 'Fixture', text: 'Hello', html: '<p>Hello</p>' })
    await provider.sendMail({ to: ['fixture@example.test'], subject: 'Fixture', text: 'Hello', html: '<p>Hello</p>' }, { idempotencyKey: 'mail-key' })
    await provider.getMailDelivery('delivery/1')
    await provider.listReportCatalogue({ page_size: 25, page_token: 'opaque' })
    await provider.shareReport('report/1', { roles: ['supervisor'] }, { expectedVersion: 3 })
    await provider.exportReport('report/1', { format: 'CSV' })
    await provider.listAuditEvents({ page_size: 10, filters: { event_type: ['REQUEST_CREATED'] } })
    await provider.exportAuditEvents({ event_type: ['REQUEST_CREATED'] })
    await provider.exportContactEmails({ filters: { primary_category: 'RESIDENT' } })
    await provider.exportData('constituents', { page_size: 5, filters: { email: 'fixture@example.test' }, updated_since: '2026-01-01T00:00:00.000Z' })
    expect(requests.map(request => `${request.method} ${request.path}`)).to.deep.equal([
      'POST /api/v1/actions',
      'GET /api/v1/operations/operation%2F1',
      'POST /api/v1/staff/calendar/import',
      'GET /api/v1/staff/calendar/export',
      'POST /api/v1/staff/mail/preview',
      'POST /api/v1/staff/mail',
      'GET /api/v1/staff/mail/delivery%2F1',
      'GET /api/v1/staff/reports/catalogue',
      'POST /api/v1/staff/reports/report%2F1/share',
      'POST /api/v1/staff/reports/report%2F1/export',
      'GET /api/v1/staff/audit-events',
      'POST /api/v1/staff/audit-events/export',
      'POST /api/v1/staff/contact-email-export',
      'GET /api/v1/export/constituents',
    ])
    expect(requests[0]).to.deep.include({ body: { action: 'notify_department', request_id: 'request-fixture-001', payload: { channel: 'EMAIL' } } })
    expect(requests[0].headers).to.deep.equal({ 'Idempotency-Key': 'action-key' })
    expect(requests[5].headers).to.deep.equal({ 'Idempotency-Key': 'mail-key' })
    expect(requests[8].body).to.deep.equal({ roles: ['supervisor'] })
    expect(requests[8].headers).to.deep.equal({ 'If-Match': '"3"' })
    expect(requests[10].query).to.deep.include({ page_size: 10, filters: { event_type: ['REQUEST_CREATED'] } })
    expect(requests[11].body).to.deep.equal({ filters: { event_type: ['REQUEST_CREATED'] } })
    expect(requests[12].body).to.deep.equal({ filters: { primary_category: 'RESIDENT' } })
    expect(requests[13].query).to.deep.include({ page_size: 5, filters: { email: 'fixture@example.test' }, updated_since: '2026-01-01T00:00:00.000Z' })
  })

  it('executes a workflow OAuth2 action and exposes its execution result without credentials', async () => {
    const provider = new MockC311Provider({ role: 'workflow_designer' })
    const input = { action: 'notify_department', request_id: 'request-fixture-001', payload: { channel: 'EMAIL' } }
    const accepted = await provider.executeWorkflowAction(input, { idempotencyKey: 'workflow-action-1' })
    expect(accepted).to.deep.equal({ execution_id: 'execution-fixture-action', accepted_at: '2026-01-15T15:00:00.000Z' })
    expect((await provider.getWorkflowExecution(accepted.execution_id))).to.include({ outcome: 'SUCCEEDED', succeeded: true })
    expect(provider.getWriteCount('workflow_action_execute')).to.equal(1)
    expect(await provider.executeWorkflowAction(input, { idempotencyKey: 'workflow-action-1' })).to.deep.equal(accepted)
    expect(provider.getWriteCount('workflow_action_execute')).to.equal(1)
  })

  it('uses repeated ICS imports for updates and cancellation and resolves the import operation', async () => {
    const provider = new MockC311Provider({ role: 'department_manager' })
    const update = await provider.importCalendar({ ics: 'BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:fixture-calendar-001\r\nSUMMARY:Updated fixture event\r\nDESCRIPTION:Fixture description\r\nDTSTART;TZID=America/New_York:20260115T100000\r\nDTEND;TZID=America/New_York:20260115T110000\r\nSTATUS:CONFIRMED\r\nLAST-MODIFIED:20260115T090000Z\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n' })
    expect(update.status).to.equal('PENDING')
    expect(await provider.getOperation(update.operation_id)).to.deep.include({ status: 'SUCCEEDED', result: { summary: { imported: 0, updated: 1, cancelled: 0, ignored: 0 } } })
    const cancel = await provider.importCalendar({ ics: 'BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:fixture-calendar-001\r\nSUMMARY:Updated fixture event\r\nDESCRIPTION:Fixture description\r\nDTSTART;TZID=America/New_York:20260115T100000\r\nDTEND;TZID=America/New_York:20260115T110000\r\nSTATUS:CANCELLED\r\nLAST-MODIFIED:20260115T090000Z\r\nEND:VEVENT\r\nBEGIN:VEVENT\r\nUID:unknown-event\r\nSUMMARY:Unknown\r\nDESCRIPTION:Unknown event\r\nDTSTART;TZID=America/New_York:20260115T100000\r\nDTEND;TZID=America/New_York:20260115T110000\r\nSTATUS:CANCELLED\r\nLAST-MODIFIED:20260115T090000Z\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n' })
    expect((await provider.getOperation(cancel.operation_id)).result).to.deep.equal({ summary: { imported: 0, updated: 0, cancelled: 1, ignored: 1 } })
    expect((await provider.exportCalendar()).body).to.contain('STATUS:CANCELLED')
    expect(provider.getWriteCount('calendar_import')).to.equal(2)
  })

  it('observes mail delivery through the contract query without creating a retry write', async () => {
    const provider = new MockC311Provider({ role: 'department_manager' })
    const input = { to: ['fixture@example.test'], subject: 'Lifecycle', text: 'Hello', html: '<p>Hello</p>' }
    const pending = await provider.sendMail(input, { idempotencyKey: 'fe09-mail-lifecycle' })
    expect(pending).to.include({ status: 'PENDING', attempts: 1 })
    expect(await provider.getMailDelivery(pending.delivery_id)).to.include({ status: 'DELIVERED', attempts: 2 })
    expect(provider.getWriteCount('mail_send')).to.equal(1)

    const terminalProvider = new MockC311Provider({ role: 'department_manager', scenario: 'terminal' })
    const terminalPending = await terminalProvider.sendMail(input, { idempotencyKey: 'fe09-mail-terminal' })
    expect(await terminalProvider.getMailDelivery(terminalPending.delivery_id)).to.include({ status: 'TERMINAL_FAILURE', attempts: 2 })
  })

  it('returns contract report catalogue/share fields and provider-produced UTF-8 CSV', async () => {
    const provider = new MockC311Provider({ role: 'department_manager' })
    const catalogueItems = (await provider.listReportCatalogue()).items
    expect(catalogueItems).to.have.length(5)
    expect(catalogueItems.map(item => item.name)).to.deep.equal(['Request volume', 'Request status and age', 'Assignment workload', 'Resolution performance', 'Follow-up activity'])
    const catalogue = catalogueItems[0]
    expect(catalogue).to.have.keys('report_key', 'name', 'supported_filters', 'supported_grouping', 'supported_sort')
    const report = await provider.getReport('report-fixture-001')
    expect((await provider.shareReport(report.report_id, { roles: ['supervisor'] }, { expectedVersion: report.version })).report_id).to.equal(report.report_id)
    const pending = await provider.exportReport(report.report_id, { format: 'CSV' })
    const complete = await provider.getOperation(pending.operation_id)
    expect(complete.status).to.equal('SUCCEEDED')
    expect(String(complete.result?.body)).to.contain('Pothole on Example Street').and.contain('request_number')
  })

  it('filters audit data and validates scoped paginated exports', async () => {
    const provider = new MockC311Provider({ role: 'department_manager' })
    expect((await provider.listAuditEvents({ filters: { event_type: ['REQUEST_CREATED'] } })).items).to.have.length(1)
    expect((await provider.listAuditEvents({ filters: { event_type: ['OTHER'] } })).items).to.have.length(0)
    const auditOperation = await provider.exportAuditEvents({ actor_id: ['actor-fixture-manager'] })
    expect(await provider.getOperation(auditOperation.operation_id)).to.deep.include({ status: 'SUCCEEDED' })
    expect((await provider.exportData('constituents', { filters: { constituent_id: 'constituent-fixture-001' }, page_size: 50 })).items[0]).to.include({ constituent_id: 'constituent-fixture-001' })
    expect((await provider.exportData('constituents', { filters: { email: 'alex@example.test' } })).items[0]).to.include({ constituent_id: 'constituent-fixture-001' })
    await expectError(() => provider.exportData('constituents', { page_token: 'invalid' }), 'INVALID_PAGE_TOKEN')
    await expectError(() => provider.exportData('constituents', { page_size: 101 }), 'INVALID_FILTER')
    await expectError(() => provider.exportData('constituents', { filters: { secret_filter: true } }), 'INVALID_FILTER')
    await expectError(() => new MockC311Provider({ role: 'service_agent' }).exportData('constituents'), 'FORBIDDEN')
    await expectError(() => new MockC311Provider({ role: 'department_manager', scenario: 'rate-limited' }).exportData('constituents'), 'RATE_LIMITED')
    expect((await new MockC311Provider({ role: 'department_manager', scenario: 'retryable' }).exportData('constituents')).items).to.be.an('array')
    expect((await new MockC311Provider({ role: 'department_manager', scenario: 'terminal' }).exportData('constituents')).items).to.be.an('array')
  })

  it('rejects invalid report definitions before any write', async () => {
    const provider = new MockC311Provider({ role: 'department_manager' })
    const invalid = { report_id: 'invalid', name: 'Too many', entity: 'service_requests' as const, columns: Array.from({ length: 21 }, (_, index) => `column_${index}`), filters: {}, grouping: null, sort: ['a', 'b', 'c', 'd'], version: 1, updated_at: '2026-01-15T15:00:00.000Z' }
    await expectError(() => provider.createReport(invalid), 'VALIDATION_ERROR')
    expect(provider.getWriteCount('saved_report_create')).to.equal(0)
  })

  it('allows FE-09 version conflicts to recover after reloading the server version', async () => {
    const workflowProvider = new MockC311Provider({ role: 'workflow_designer', scenario: 'version-conflict' })
    const workflow = await workflowProvider.getWorkflow('workflow-fixture-001')
    const workflowError = await expectError(() => workflowProvider.updateWorkflow(workflow.workflow_id, { ...workflow, name: 'Reapplied workflow' }, { expectedVersion: workflow.version }), 'VERSION_CONFLICT')
    expect(workflowError.currentVersion).to.equal(workflow.version)
    const workflowReloaded = await workflowProvider.getWorkflow(workflow.workflow_id)
    const workflowUpdated = await workflowProvider.updateWorkflow(workflowReloaded.workflow_id, { ...workflowReloaded, name: 'Reapplied workflow' }, { expectedVersion: workflowReloaded.version })
    expect(workflowUpdated.name).to.equal('Reapplied workflow')

    const reportProvider = new MockC311Provider({ role: 'department_manager', scenario: 'version-conflict' })
    const report = await reportProvider.getReport('report-fixture-001')
    await expectError(() => reportProvider.shareReport(report.report_id, { roles: ['supervisor'] }, { expectedVersion: report.version }), 'VERSION_CONFLICT')
    const reportReloaded = await reportProvider.getReport(report.report_id)
    const shared = await reportProvider.shareReport(reportReloaded.report_id, { roles: ['supervisor'] }, { expectedVersion: reportReloaded.version })
    expect(shared.version).to.equal(report.version + 1)
  })

  it('enforces workflow OAuth error codes and role scopes', async () => {
    await expectError(() => new MockC311Provider({ role: 'workflow_designer', scenario: 'invalid-client' }).executeWorkflowAction({ action: 'notify', request_id: 'request-fixture-001', payload: {} }), 'INVALID_CLIENT')
    await expectError(() => new MockC311Provider({ role: 'workflow_designer', scenario: 'invalid-token' }).executeWorkflowAction({ action: 'notify', request_id: 'request-fixture-001', payload: {} }), 'INVALID_TOKEN')
    await expectError(() => new MockC311Provider({ role: 'workflow_designer', scenario: 'insufficient-scope' }).executeWorkflowAction({ action: 'notify', request_id: 'request-fixture-001', payload: {} }), 'INSUFFICIENT_SCOPE')
    await expectError(() => new MockC311Provider({ role: 'constituent' }).executeWorkflowAction({ action: 'notify', request_id: 'request-fixture-001', payload: {} }), 'INSUFFICIENT_SCOPE')
  })

  it('round-trips ICS metadata and rejects duplicate UIDs', async () => {
    const provider = new MockC311Provider({ role: 'department_manager' })
    const ics = 'BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:metadata-001\r\nSUMMARY:Fixture\r\nDESCRIPTION:Line one\\nLine two\r\nDTSTART;TZID=America/New_York:20260115T100000\r\nDTEND;TZID=America/New_York:20260115T110000\r\nRRULE:FREQ=DAILY;COUNT=2\r\nSTATUS:CONFIRMED\r\nLAST-MODIFIED:20260115T090000Z\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n'
    await provider.importCalendar({ ics })
    const exported = (await provider.exportCalendar()).body
    expect(exported).to.contain('DTSTART;TZID=America/New_York:20260115T100000')
    expect(exported).to.contain('DTEND;TZID=America/New_York:20260115T110000')
    expect(exported).to.contain('DESCRIPTION:Line one\\nLine two')
    expect(exported).to.contain('RRULE:FREQ=DAILY;COUNT=2')
    expect(exported).to.contain('LAST-MODIFIED:20260115T090000Z')
    await expectError(() => provider.importCalendar({ ics: 'BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:duplicate\r\nSUMMARY:One\r\nEND:VEVENT\r\nBEGIN:VEVENT\r\nUID:duplicate\r\nSUMMARY:Two\r\nEND:VEVENT\r\nEND:VCALENDAR' }), 'VALIDATION_ERROR')
    await expectError(() => provider.importCalendar({ ics: 'BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:mixed-timezone\r\nSUMMARY:Mixed timezone\r\nDESCRIPTION:Fixture\r\nDTSTART;TZID=America/New_York:20260115T100000\r\nDTEND:20260115T160000Z\r\nSTATUS:CONFIRMED\r\nLAST-MODIFIED:20260115T090000Z\r\nEND:VEVENT\r\nEND:VCALENDAR' }), 'VALIDATION_ERROR')
    await expectError(() => provider.importCalendar({ ics: 'BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:invalid-status\r\nSUMMARY:Invalid status\r\nDESCRIPTION:Fixture\r\nDTSTART;TZID=America/New_York:20260115T100000\r\nDTEND;TZID=America/New_York:20260115T110000\r\nSTATUS:COMPLETED\r\nLAST-MODIFIED:20260115T090000Z\r\nEND:VEVENT\r\nEND:VCALENDAR' }), 'VALIDATION_ERROR')
  })

  it('loads and edits Mock-only mail templates without changing HTTP contract', async () => {
    const provider = new MockC311Provider({ role: 'department_manager' })
    const templates = await provider.listMailTemplates!()
    expect(templates).to.have.length(2)
    const updated = await provider.updateMailTemplate!('service-update', { name: templates[0].name, subject: 'Updated subject', text: 'Updated body', html: '<p>Updated body</p>' })
    expect(updated).to.include({ subject: 'Updated subject', version: 2 })
    expect((await provider.listMailTemplates!()).find(item => item.template_id === 'service-update')?.subject).to.equal('Updated subject')
  })

  it('exports contact emails only for authorized roles and resolves filtered operations', async () => {
    const provider = new MockC311Provider({ role: 'platform_administrator' })
    const pending = await provider.exportContactEmails({ filters: { primary_category: 'RESIDENT' } })
    expect((await provider.getOperation(pending.operation_id)).result).to.deep.include({ exported_count: 1, filters: { primary_category: 'RESIDENT' } })
    expect(provider.getWriteCount('contact_email_export')).to.equal(1)
    await expectError(() => new MockC311Provider({ role: 'service_agent' }).exportContactEmails({ filters: {} }), 'FORBIDDEN')
  })

  it('keeps every export entity inside the actor scope and preserves contract projections', async () => {
    const fixtures = cloneFixtureSet(createDefaultFixtureSet())
    const foreign = JSON.parse(JSON.stringify(fixtures.requests[0])) as typeof fixtures.requests[number]
    foreign.request_id = 'request-fixture-foreign'
    foreign.request_number = 'SR-2026-00099'
    foreign.owning_department = 'GENERAL_SERVICES'
    foreign.council_district = 'SOUTH'
    foreign.primary_requester = { ...foreign.primary_requester, constituent_id: 'constituent-fixture-foreign', emails: ['foreign@example.test'] }
    fixtures.requests.push(foreign)
    fixtures.verified_emails = { ...(fixtures.verified_emails || {}), 'constituent-fixture-foreign': ['foreign@example.test'] }
    const optedOut = { ...foreign, request_id: 'request-fixture-opted-out', request_number: 'SR-2026-00098', owning_department: 'STREETS' as const, council_district: 'NORTH' as const, primary_requester: { ...foreign.primary_requester, constituent_id: 'constituent-fixture-opted-out', emails: ['opted-out@example.test'], email_opt_out: true } }
    const unverified = { ...foreign, request_id: 'request-fixture-unverified', request_number: 'SR-2026-00097', owning_department: 'STREETS' as const, council_district: 'NORTH' as const, primary_requester: { ...foreign.primary_requester, constituent_id: 'constituent-fixture-unverified', emails: ['unverified@example.test'] } }
    fixtures.requests.push(optedOut, unverified)
    fixtures.verified_emails = { ...fixtures.verified_emails, 'constituent-fixture-opted-out': ['opted-out@example.test'] }
    fixtures.audit_events = [...(fixtures.audit_events || []), { audit_id: 'audit-fixture-foreign', actor_id: 'actor-fixture-agent', actor_type: 'staff', entity_type: 'service_request', entity_id: foreign.request_id, event_type: 'REQUEST_CREATED', occurred_at: '2026-01-15T15:00:00.000Z', source_channel: 'STAFF_IN_PERSON', before: {}, after: { request_id: foreign.request_id } }]
    fixtures.follow_up_actions = [...(fixtures.follow_up_actions || []), { action_type: 'CALL_REQUESTER', actor: 'actor-fixture-agent', occurred_at: '2026-01-15T15:00:00.000Z', local_display_time: '2026-01-15 10:00 America/New_York', request_id: foreign.request_id, visibility: 'STAFF', payload: {} }]
    const manager = new MockC311Provider({ role: 'department_manager', fixtures })
    expect((await manager.exportData('service-requests')).items.map(item => item.request_id)).to.deep.equal([
      'request-fixture-001',
      'request-fixture-opted-out',
      'request-fixture-unverified',
    ])
    expect((await manager.exportData('follow-up-actions')).items).to.have.length(1)
    expect((await manager.listAuditEvents()).items).to.have.length(1)
    const audit = await manager.exportAuditEvents({})
    const auditResult = await manager.getOperation(audit.operation_id)
    expect(String(auditResult.result?.body)).to.contain('actor_type').and.contain('occurred_at').and.contain('before').and.contain('after')
    const contact = await manager.exportContactEmails({ filters: { email: 'alex@example.test', department: 'STREETS', district: 'NORTH' } })
    const contactResult = await manager.getOperation(contact.operation_id)
    expect(String(contactResult.result?.body).split('\r\n')[0]).to.equal('"email","display_name","primary_category","preferred_language","opt_out"')
    expect(String(contactResult.result?.body)).to.not.contain('foreign@example.test')
    expect(String(contactResult.result?.body)).to.not.contain('opted-out@example.test').and.not.contain('unverified@example.test')
    const administrator = new MockC311Provider({ role: 'platform_administrator', fixtures })
    expect((await administrator.exportData('service-requests')).items).to.have.length(4)
    expect((await administrator.exportData('follow-up-actions')).items).to.have.length(2)
  })

  it('validates workflow definitions, preserves active state and records test executions', async () => {
    const provider = new MockC311Provider({ role: 'workflow_designer' })
    await expectError(() => provider.createWorkflow({ workflow_id: 'workflow-invalid', name: 'Invalid', trigger: 'SERVICE_REQUEST_CREATED', active: false, conditions: [], actions: [], version: 1, updated_at: '2026-01-15T15:00:00.000Z' }), 'VALIDATION_ERROR')
    const current = await provider.getWorkflow('workflow-fixture-001')
    const updated = await provider.updateWorkflow(current.workflow_id, { ...current, active: false }, { expectedVersion: current.version })
    expect(updated.active).to.equal(true)
    const operation = await provider.testWorkflow(updated.workflow_id, { request_id: 'request-fixture-001' })
    const complete = await provider.getOperation(operation.operation_id)
    expect(complete.result).to.have.property('execution_id')
    expect((await provider.listWorkflowExecutions()).items.some(item => item.execution_id === complete.result?.execution_id)).to.equal(true)
    await expectError(() => provider.executeWorkflowAction({ action: 'notify', request_id: 'request-fixture-001', payload: {} }), 'VALIDATION_ERROR')
  })

  it('rejects incomplete or unsafe extension payloads without undeclared HTTP failures', async () => {
    const provider = new MockC311Provider({ role: 'department_manager' })
    await expectError(() => provider.importCalendar({ ics: 'BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:missing\r\nSUMMARY:Missing fields\r\nEND:VEVENT\r\nEND:VCALENDAR' }), 'VALIDATION_ERROR')
    await expectError(() => provider.importCalendar({ ics: 'BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:bad-tz\r\nSUMMARY:Bad timezone\r\nDESCRIPTION:Fixture\r\nDTSTART;TZID=Europe/London:20260115T100000\r\nDTEND;TZID=Europe/London:20260115T110000\r\nSTATUS:CONFIRMED\r\nLAST-MODIFIED:20260115T090000Z\r\nEND:VEVENT\r\nEND:VCALENDAR' }), 'VALIDATION_ERROR')
    const preview = await provider.previewMail({ to: ['alex@example.test'], subject: 'Unsafe', text: 'Body', html: '<script>alert(1)</script><p>Safe</p>' })
    expect(preview.html).to.equal('<p>Safe</p>')
    expect((await new MockC311Provider({ role: 'department_manager', scenario: 'retryable' }).importCalendar({ ics: 'BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:retryable\r\nSUMMARY:Retryable\r\nDESCRIPTION:Fixture\r\nDTSTART;TZID=America/New_York:20260115T100000\r\nDTEND;TZID=America/New_York:20260115T110000\r\nSTATUS:CONFIRMED\r\nLAST-MODIFIED:20260115T090000Z\r\nEND:VEVENT\r\nEND:VCALENDAR' })).status).to.equal('PENDING')
  })

  it('binds CRM export page tokens to the complete query context', async () => {
    const fixtures = cloneFixtureSet(createDefaultFixtureSet())
    const second = JSON.parse(JSON.stringify(fixtures.requests[0])) as typeof fixtures.requests[number]
    second.request_id = 'request-fixture-002'
    second.request_number = 'SR-2026-00002'
    fixtures.requests.push(second)
    const provider = new MockC311Provider({ role: 'department_manager', fixtures })
    const first = await provider.exportData('service-requests', { page_size: 1 })
    expect(first.items).to.have.length(1)
    expect(first.next_page_token).to.be.a('string')
    const token = first.next_page_token!
    await expectError(() => provider.exportData('service-requests', { page_size: 2, page_token: token }), 'INVALID_PAGE_TOKEN')
    await expectError(() => provider.exportData('service-requests', { page_size: 1, page_token: token, filters: { status: 'SUBMITTED' } }), 'INVALID_PAGE_TOKEN')
    const secondPage = await provider.exportData('service-requests', { page_size: 1, page_token: token })
    expect(secondPage.items.map(item => item.request_id)).to.deep.equal(['request-fixture-002'])
  })

  it('rejects non-CSV report exports and enforces report ownership', async () => {
    const owner = new MockC311Provider({ role: 'department_manager' })
    await expectError(() => owner.exportReport('report-fixture-001', { format: 'JSON' } as unknown as ReportExportOptions), 'VALIDATION_ERROR')
    const report = await owner.getReport('report-fixture-001')
    const nonOwner = new MockC311Provider({ role: 'supervisor' })
    await expectError(() => nonOwner.updateReport(report.report_id, report, { expectedVersion: report.version }), 'FORBIDDEN')
    await expectError(() => nonOwner.shareReport(report.report_id, { roles: ['supervisor'] }, { expectedVersion: report.version }), 'FORBIDDEN')
  })

  it('binds audit page tokens to filters, sort and page size', async () => {
    const fixtures = cloneFixtureSet(createDefaultFixtureSet())
    fixtures.audit_events = [...(fixtures.audit_events || []), { ...(fixtures.audit_events || [])[0], audit_id: 'audit-fixture-002', entity_id: 'request-fixture-001' }]
    const provider = new MockC311Provider({ role: 'department_manager', fixtures })
    const query = { page_size: 1, sort: '-occurred_at', filters: { actor_id: ['actor-fixture-manager'] } }
    const first = await provider.listAuditEvents(query)
    expect(first.next_page_token).to.be.a('string')
    const token = first.next_page_token!
    await expectError(() => provider.listAuditEvents({ ...query, page_size: 2, page_token: token }), 'INVALID_PAGE_TOKEN')
    await expectError(() => provider.listAuditEvents({ ...query, page_token: token, filters: { event_type: ['OTHER'] } }), 'INVALID_PAGE_TOKEN')
    const second = await provider.listAuditEvents({ ...query, page_token: token })
    expect(second.items).to.have.length(1)
  })

  it('uses the contract retry delay for CRM export rate limits', async () => {
    const provider = new MockC311Provider({ role: 'department_manager', scenario: 'rate-limited' })
    const error = await expectError(() => provider.exportData('constituents'), 'RATE_LIMITED')
    expect(error.retryAfter).to.equal('60')
  })

  it('projects the five report catalogue views and applies created date filters', async () => {
    const provider = new MockC311Provider({ role: 'platform_administrator' })
    const catalogue = (await provider.listReportCatalogue()).items
    expect(catalogue).to.have.length(5)
    const definitions: ReportDefinition[] = [
      { report_id: 'report-service', name: 'Requests', entity: 'service_requests', columns: ['request_number', 'status', 'created_at'], filters: { created_from: '2026-01-01T00:00:00.000Z', created_to: '2026-01-31T23:59:59.000Z' }, grouping: null, sort: ['-created_at'], version: 1, updated_at: '2026-01-15T15:00:00.000Z' },
      { report_id: 'report-age', name: 'Status age', entity: 'service_requests', columns: ['status', 'age_days', 'created_at', 'updated_at'], filters: {}, grouping: 'status', sort: ['status'], version: 1, updated_at: '2026-01-15T15:00:00.000Z' },
      { report_id: 'report-assignment', name: 'Assignment workload', entity: 'service_requests', columns: ['primary_assignee_id', 'owning_department', 'collaborator_count', 'assignment_count'], filters: {}, grouping: null, sort: ['primary_assignee_id'], version: 1, updated_at: '2026-01-15T15:00:00.000Z' },
      { report_id: 'report-resolution', name: 'Resolution', entity: 'service_requests', columns: ['resolved_at', 'resolution_days', 'reopened', 'owning_department'], filters: {}, grouping: null, sort: ['-updated_at'], version: 1, updated_at: '2026-01-15T15:00:00.000Z' },
      { report_id: 'report-constituents', name: 'Constituents', entity: 'constituents', columns: ['constituent_id', 'email', 'department', 'district'], filters: { created_from: '2026-01-01T00:00:00.000Z', created_to: '2026-01-31T23:59:59.000Z' }, grouping: null, sort: ['constituent_id'], version: 1, updated_at: '2026-01-15T15:00:00.000Z' },
      { report_id: 'report-actions', name: 'Actions', entity: 'follow_up_actions', columns: ['request_id', 'request_number', 'action_type', 'occurred_at', 'owning_department'], filters: { created_from: '2026-01-01T00:00:00.000Z', created_to: '2026-01-31T23:59:59.000Z' }, grouping: null, sort: ['occurred_at'], version: 1, updated_at: '2026-01-15T15:00:00.000Z' },
    ]
    for (const definition of definitions) {
      const operation = await provider.runReport({ definition })
      const result = await provider.getOperation(operation.operation_id)
      expect(result.result?.row_count).to.be.greaterThan(0)
      expect(result.result?.rows?.[0]).to.include.keys(definition.columns)
    }
  })

  it('accepts date-only report bounds and includes the entire end date', async () => {
    const provider = new MockC311Provider({ role: 'platform_administrator' })
    const definition: ReportDefinition = {
      report_id: 'report-date-only',
      name: 'Date-only filter',
      entity: 'service_requests',
      columns: ['request_id', 'created_at'],
      filters: { created_from: '2026-01-15', created_to: '2026-01-15' },
      grouping: null,
      sort: ['created_at'],
      version: 1,
      updated_at: '2026-01-15T15:00:00.000Z',
    }
    const operation = await provider.runReport({ definition })
    const result = await provider.getOperation(operation.operation_id)
    expect(result.result?.row_count).to.equal(1)
  })

  it('sanitizes mail HTML, enforces the 5 MiB attachment limit and returns unknown operations as 404', async () => {
    const provider = new MockC311Provider({ role: 'department_manager' })
    const preview = await provider.previewMail({ to: ['alex@example.test'], subject: 'Fixture', text: 'Body', html: '<p>Safe</p><script>alert(1)</script><a href="javascript:bad">bad</a>' })
    expect(preview.html).to.contain('<p>Safe</p>')
    expect(preview.html).to.not.contain('<script>')
    expect(preview.html).to.not.contain('javascript:')
    await expectError(() => provider.previewMail({ to: ['alex@example.test'], subject: 'Fixture', text: 'Body', attachments: [{ attachment_token: 'large', filename: 'large.txt', media_type: 'text/plain', size: 6 * 1024 * 1024, expires_at: '2026-01-16T15:00:00.000Z' }] }), 'VALIDATION_ERROR')
    await expectError(() => provider.getOperation('operation-unknown'), 'NOT_FOUND')
  })

  it('keeps terminal workflow executions consistent with their operation result', async () => {
    const provider = new MockC311Provider({ role: 'workflow_designer', scenario: 'terminal' })
    const operation = await provider.testWorkflow('workflow-fixture-001', { request_id: 'request-fixture-001' })
    const result = await provider.getOperation(operation.operation_id)
    expect(result.status).to.equal('FAILED')
    const execution = (await provider.listWorkflowExecutions()).items.find(item => item.execution_id === result.result?.execution_id)
    expect(execution?.outcome).to.equal('FAILED')
    expect(execution?.succeeded).to.equal(false)
  })

  it('models SMTP retry and terminal delivery lifecycles with bounded attempts', async () => {
    for (const scenario of ['smtp-421', 'smtp-451'] as const) {
      const provider = new MockC311Provider({ role: 'department_manager', scenario })
      const delivery = await provider.sendMail({ to: ['alex@example.test'], subject: 'Fixture', text: 'Body' }, { idempotencyKey: scenario })
      const first = await provider.getMailDelivery(delivery.delivery_id)
      const second = await provider.getMailDelivery(delivery.delivery_id)
      expect(first.status).to.equal('PENDING')
      expect(second.status).to.equal('DELIVERED')
      expect(second.attempts).to.equal(3)
    }
    for (const scenario of ['smtp-550', 'smtp-553'] as const) {
      const provider = new MockC311Provider({ role: 'department_manager', scenario })
      const delivery = await provider.sendMail({ to: ['alex@example.test'], subject: 'Fixture', text: 'Body' }, { idempotencyKey: scenario })
      const result = await provider.getMailDelivery(delivery.delivery_id)
      expect(result.status).to.equal('TERMINAL_FAILURE')
      expect(result.error?.message).to.contain(scenario.replace('-', ' ').toUpperCase())
    }
    await expectError(() => new MockC311Provider({ role: 'department_manager', scenario: 'terminal' }).getOperation('operation-unknown'), 'NOT_FOUND')
  })
})
