import { C311ApiError } from './errors'
import { APPLICATION_ROLES, CIVICWORKS_STATUSES, CONTACT_CATEGORIES, C311_SCENARIOS, DEPARTMENT_CODES, DISTRICT_CODES, LANGUAGES, ORIGIN_CLASSES, PHONE_LABELS, RELATIONSHIP_TYPES, REMINDER_CHANNELS, SERVICE_REQUEST_STATUSES, SERVICE_TYPES, SOURCE_CHANNELS, type ApplicationRole, type C311Scenario, type ContractCapability, type HelpKey, type IdentityProvider, type Language, type PublicContentKey } from './enums'
import { cloneFixtureSet, createDefaultFixtureSet } from './fixtures'
import type {
  AccountRegistration,
  AccountRegistrationAcknowledgement,
  AccountDispositionRequest,
  AccountDispositionResult,
  AnonymousStatusLookupRequest,
  AnonymousStatusLookupResponse,
  BinaryAttachment,
  Branding,
  C311FixtureSet,
  ContentObject,
  DraftWrite,
  GeocodeRequest,
  GeocodeResponse,
  FederatedRedirect,
  HelpContent,
  LanguagePreference,
  LoginIdentifierChange,
  ListQuery,
  LocalSignIn,
  Operation,
  PageResponse,
  PortalAttachment,
  PortalServiceRequestCreate,
  PasswordResetConfirm,
  PasswordResetRequest,
  PasswordResetResponse,
  PasswordChange,
  ProfileUpdate,
  ReportDefinition,
  RequestListQuery,
  RequestQueueItem,
  RequestSummary,
  ReopenRequestResponse,
  ServiceRequest,
  ServiceRequestCreate,
  ServiceRequestResponse,
  Session,
  FederatedSignInResult,
  Constituent,
  StaffServiceRequestDetail,
  StaffServiceRequestCreate,
  RequestTransition,
  Reassignment,
  CollaboratorChange,
  ReminderWrite,
  ReminderActionInput,
  OriginOverride,
  ScopeOverride,
  DuplicateGroupChange,
  Reminder,
  ConstituentLink,
  ConstituentUnlink,
  RequestNote,
  RequestRelationship,
  RequestRelationshipAudit,
  WorkflowDefinition,
  BulkRequest,
  BulkResult,
  CivicWorksEvent,
  CivicWorksEventResult,
  CivicWorksWorkOrder,
} from './types'
import { validatePortalAttachment, type C311Provider, type C311RequestOptions, type PortalAttachmentUpload, type ReportExportOptions } from './provider'

export interface MockC311ProviderOptions {
  scenario?: C311Scenario
  fixtures?: C311FixtureSet
  role?: ApplicationRole
  sessionVariant?: 'current' | 'expired'
  /** Optional constituent profile for relationship-aware portal fixtures. */
  profile?: Constituent
}

const statusByScenario: Partial<Record<C311Scenario, number>> = {
  forbidden: 403,
  'not-found': 404,
  validation: 422,
  retryable: 503,
  terminal: 500,
  'version-conflict': 409,
  'idempotency-conflict': 409,
  'expected-version-required': 428,
  'invalid-credentials': 401,
  'registration-validation': 422,
  'expired-reset-token': 422,
  'invalid-reset-token': 422,
  'oidc-failure': 503,
  'saml-failure': 503,
  'branding-failure': 503,
  'content-loading-failure': 503,
  'help-loading-failure': 503,
  'account-loading': 503,
  'identity-claims-failure': 401,
  'account-disposition-conflict': 409,
  'account-disposition-failure': 500,
  'attachment-retryable': 503,
  'attachment-terminal': 500,
  'map-retryable': 503,
  'map-auth-failure': 401,
  'scope-denied': 403,
  'invalid-status-transition': 422,
  'bulk-validation': 422,
  'bulk-version-conflict': 409,
  'civicworks-invalid-signature': 401,
  'civicworks-stale': 422,
  'civicworks-duplicate': 204,
  'reminder-validation': 422,
  'reminder-retryable': 503,
  'reminder-terminal': 500,
}

function copy<T> (value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

function normalizeGeocodeAddress (value: unknown): string {
  return typeof value === 'string' ? value.trim().replace(/\s+/g, ' ').toLowerCase() : ''
}

function validISODateTime (value: unknown): value is string {
  return typeof value === 'string' && /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/.test(value) && Number.isFinite(Date.parse(value))
}

function validMockProfileInput (input: ProfileUpdate): boolean {
  const allowed = ['display_name', 'phone_numbers', 'addresses', 'preferred_language', 'primary_category']
  if (Object.keys(input).some(key => !allowed.includes(key))) return false
  if (input.display_name !== undefined && (!input.display_name.trim() || input.display_name.length > 120)) return false
  if (input.preferred_language !== undefined && !LANGUAGES.includes(input.preferred_language)) return false
  if (input.primary_category !== undefined && !CONTACT_CATEGORIES.includes(input.primary_category)) return false
  if (input.phone_numbers !== undefined) {
    if (input.phone_numbers.length > 3 || input.phone_numbers.some(phone => !PHONE_LABELS.includes(phone.label) || typeof phone.value !== 'string' || !/^\+[1-9]\d{1,14}$/.test(phone.value))) return false
  }
  if (input.addresses !== undefined) {
    if (input.addresses.length > 5 || input.addresses.some(address => {
      const values = address as unknown as Record<string, unknown>
      return ['line1', 'city', 'region', 'postal_code', 'country'].some(field => !String(values[field] || '').trim()) || String(values.line1).length > 200 || String(values.line2 || '').length > 200 || String(values.city).length > 120 || String(values.region).length > 120 || String(values.postal_code).length > 32 || String(values.country).length !== 2
    })) return false
    if (input.addresses.length > 0 && input.addresses.filter(address => address.primary).length !== 1) return false
  }
  return true
}

export class MockC311Provider implements C311Provider {
  private readonly fixtures: C311FixtureSet
  private readonly scenario: C311Scenario
  private readonly role?: ApplicationRole
  private readonly sessionVariant: 'current' | 'expired'
  private currentSession: Session
  private readonly draftRecords: Record<string, ServiceRequest> = {}
  private readonly draftPayloads: Record<string, DraftWrite> = {}
  private readonly idempotentResponses = new Map<string, { fingerprint: string; response: ServiceRequestResponse }>()
  private readonly bulkIdempotentResponses = new Map<string, { fingerprint: string; response: BulkResult }>()
  private readonly civicWorksEvents = new Map<string, string>()
  private readonly uploadedAttachmentTokens = new Set<string>()
  private readonly consumedAttachmentTokens = new Set<string>()
  private attachmentSerial = 0
  private attachmentRetryFailures = 0
  private readonly writeCounts: Record<string, number> = {}
  private resetTokenSerial = 0
  private activeResetToken: string | null = null
  private resetTokenUsed = false
  private pendingAccountLinkProvider: IdentityProvider | null = null
  private pendingAccountLinkExpiresAt: string | null = null
  private pendingAccountLinkConsumed = false
  private profile: Constituent
  private readonly relationships: Record<string, RequestRelationship[]>
  private readonly notes: Record<string, RequestNote[]>
  private readonly publicRelationships: Record<string, RequestRelationship[]>
  private readonly publicNotes: Record<string, RequestNote[]>
  private readonly requestVersions: Record<string, number> = {}
  private readonly relationshipAudits: Record<string, RequestRelationshipAudit[]> = {}
  private readonly consumedScenarioFailures = new Set<string>()
  private noteSerial = 0

  constructor (options: MockC311ProviderOptions = {}) {
    this.fixtures = cloneFixtureSet(options.fixtures || createDefaultFixtureSet())
    this.scenario = options.scenario || 'success'
    this.role = options.role
    this.sessionVariant = options.sessionVariant || 'current'

    if (!C311_SCENARIOS.includes(this.scenario)) {
      throw new Error(`Unsupported City 311 fixture scenario: ${this.scenario}`)
    }
    if (this.role && !APPLICATION_ROLES.includes(this.role)) {
      throw new Error(`Unsupported City 311 fixture role: ${this.role}`)
    }
    this.currentSession = this.role
      ? copy(this.sessionVariant === 'expired' ? this.fixtures.role_fixtures[this.role].expired_session : this.fixtures.role_fixtures[this.role].session)
      : copy(this.fixtures.session)
    this.profile = copy(options.profile || this.fixtures.requests[0].primary_requester)
    this.relationships = copy(this.fixtures.relationships || {})
    this.notes = copy(this.fixtures.notes || {})
    this.publicRelationships = copy(this.fixtures.public_relationships || this.relationships)
    this.publicNotes = copy(this.fixtures.public_notes || this.notes)
    if (this.scenario === 'scope-filter' && this.fixtures.queue[0]) {
      const baseDetail = this.fixtures.details[this.fixtures.queue[0].request_id]
      if (baseDetail) {
        const foreignRequestID = 'request-fixture-foreign'
        const foreignDetail = copy(baseDetail)
        foreignDetail.request = {
          ...foreignDetail.request,
          request_id: foreignRequestID,
          request_number: 'SR-2026-00099',
          summary: 'Out of scope fixture request',
          owning_department: 'GENERAL_SERVICES',
          council_district: 'SOUTH',
        }
        this.fixtures.details[foreignRequestID] = foreignDetail
      }
    }
    if (this.scenario === 'pagination' && this.fixtures.queue[0]) {
      const baseDetail = this.fixtures.details[this.fixtures.queue[0].request_id]
      if (baseDetail) {
        const secondDetail = copy(baseDetail)
        secondDetail.request = { ...secondDetail.request, request_id: 'request-fixture-002', request_number: 'SR-2026-00002', summary: 'Second fixture request' }
        this.fixtures.details['request-fixture-002'] = secondDetail
        this.fixtures.requests.push(copy(secondDetail.request))
      }
    }
    Object.entries(this.relationships).forEach(([requestID, relationships]) => {
      this.relationshipAudits[requestID] = relationships.flatMap(relationship => relationship.audit || []).map(audit => copy(audit))
    })
    Object.entries(this.fixtures.details).forEach(([requestID, detail]) => {
      this.requestVersions[requestID] = detail.request.version
    })
    this.restorePendingAccountLink()
    Object.entries(this.fixtures.drafts).forEach(([requestID, draft]) => {
      const payload = 'primary_requester' in draft ? {
        request_id: requestID,
        summary: draft.summary,
        description: draft.description,
        service_type: draft.service_type,
        requester: {
          display_name: draft.primary_requester.display_name,
          email: draft.primary_requester.emails[0] || '',
          ...(draft.primary_requester.phone_numbers[0]?.value ? { phone: draft.primary_requester.phone_numbers[0].value } : {}),
        },
        ...(draft.location?.address?.line1 ? { location: { address: draft.location.address.line1, latitude: draft.location.latitude, longitude: draft.location.longitude } } : {}),
        custom_fields: draft.custom_fields,
      } : { ...draft, request_id: requestID }
      this.draftPayloads[requestID] = copy(payload)
      this.draftRecords[requestID] = this.makeDraftRecord(requestID, payload, 'version' in draft && typeof draft.version === 'number' ? draft.version : 1)
    })
  }

  private pendingAccountLinkStorageKey (): string {
    return `c311.mock.pending.${this.scenario}.${this.role || 'public_visitor'}`
  }

  private restorePendingAccountLink (): void {
    try {
      const raw = typeof sessionStorage !== 'undefined' ? sessionStorage.getItem(this.pendingAccountLinkStorageKey()) : null
      if (!raw) return
      const pending = JSON.parse(raw) as { provider?: IdentityProvider, expires_at?: string, status?: string }
      if (pending.provider && pending.expires_at && Date.parse(pending.expires_at) > Date.now()) {
        this.pendingAccountLinkProvider = pending.provider
        this.pendingAccountLinkExpiresAt = pending.expires_at
        this.pendingAccountLinkConsumed = pending.status === 'consumed'
      } else {
        sessionStorage.removeItem(this.pendingAccountLinkStorageKey())
      }
    } catch (_error) {
      // Browser storage is optional in non-browser unit tests.
    }
  }

  private persistPendingAccountLink (): void {
    if (!this.pendingAccountLinkProvider || !this.pendingAccountLinkExpiresAt) return
    try {
      if (typeof sessionStorage !== 'undefined') sessionStorage.setItem(this.pendingAccountLinkStorageKey(), JSON.stringify({ provider: this.pendingAccountLinkProvider, expires_at: this.pendingAccountLinkExpiresAt, status: this.pendingAccountLinkConsumed ? 'consumed' : 'pending' }))
    } catch (_error) {
      // Browser storage is optional in non-browser unit tests.
    }
  }

  private clearPendingAccountLink (): void {
    this.pendingAccountLinkProvider = null
    this.pendingAccountLinkExpiresAt = null
    this.pendingAccountLinkConsumed = false
    try {
      if (typeof sessionStorage !== 'undefined') sessionStorage.removeItem(this.pendingAccountLinkStorageKey())
    } catch (_error) {
      // Browser storage is optional in non-browser unit tests.
    }
  }

  getWriteCount (operation: string): number {
    return this.writeCounts[operation] || 0
  }

  private countWrite (operation: string): void {
    this.writeCounts[operation] = (this.writeCounts[operation] || 0) + 1
  }

  private fingerprint (value: unknown): string {
    if (Array.isArray(value)) return `[${value.map(item => this.fingerprint(item)).join(',')}]`
    if (value && typeof value === 'object') return `{${Object.keys(value as Record<string, unknown>).sort((left, right) => left.localeCompare(right)).map(key => `${JSON.stringify(key)}:${this.fingerprint((value as Record<string, unknown>)[key])}`).join(',')}}`
    return JSON.stringify(value)
  }

  private makeDraftRecord (requestID: string, input: DraftWrite, version = 1): ServiceRequest {
    const base = this.fixtures.requests[0]
    const hasRequester = !!input.requester
    const requester = input.requester || {
      display_name: base.primary_requester.display_name,
      email: base.primary_requester.emails[0] || '',
    }
    const primaryRequester = copy({
      ...base.primary_requester,
      display_name: requester.display_name,
      emails: [requester.email],
      phone_numbers: requester.phone ? [{ label: 'MOBILE' as const, value: requester.phone }] : hasRequester ? [] : base.primary_requester.phone_numbers,
    })
    const location = input.location && base.location
      ? { ...base.location, address: { ...base.location.address, line1: input.location.address }, latitude: input.location.latitude, longitude: input.location.longitude }
      : undefined
    return copy({
      ...base,
      request_id: requestID,
      request_number: undefined,
      status: 'DRAFT',
      summary: input.summary || base.summary,
      description: input.description || base.description,
      service_type: input.service_type || base.service_type,
      primary_requester: primaryRequester,
      location,
      custom_fields: input.custom_fields,
      version,
    })
  }

  private failIfNeeded (supported: readonly C311Scenario[] = []): void {
    if (this.scenario === 'success' || this.scenario === 'empty') return
    if (!supported.includes(this.scenario)) return

    const payload = this.fixtures.errors[this.scenario]
    if (payload) {
      const headers = this.scenario === 'retryable' ? { 'Retry-After': '30' } : undefined
      throw new C311ApiError(payload, statusByScenario[this.scenario], headers)
    }
  }

  private failScenario (scenario: C311Scenario): void {
    const payload = this.fixtures.errors[scenario]
    if (!payload) return
    throw new C311ApiError(payload, statusByScenario[scenario], payload.retryable ? { 'Retry-After': '30' } : undefined)
  }

  private requireCapability (capability: ContractCapability): void {
    const expiresAt = this.currentSession.expires_at
    if (!this.currentSession.authenticated || (expiresAt && Date.parse(expiresAt) <= Date.now())) {
      throw new C311ApiError({ error: 'UNAUTHENTICATED', message: 'Authentication is required.', retryable: false }, 401)
    }
    if (!this.currentSession.actor?.capabilities?.includes(capability)) {
      throw new C311ApiError({ error: 'FORBIDDEN', message: 'You are not allowed to perform this operation.', retryable: false }, 403)
    }
  }

  private page<T> (items: T[], query: ListQuery = {}): PageResponse<T> {
    const { page_token: _pageToken, page_size: _pageSize, filters = {}, sort, ...filterFields } = query as RequestListQuery
    const appliedFilters = Object.entries(filterFields).reduce<Record<string, unknown>>((out, [key, value]) => {
      if (value !== undefined) out[key] = value
      return out
    }, { ...filters })

    return {
      items: copy(items),
      next_page_token: null,
      total_count: items.length,
      applied_filters: appliedFilters,
      sort: sort ? [sort] : [],
    }
  }

  private request (requestID: string): ServiceRequest {
    const request = this.fixtures.requests.find(item => item.request_id === requestID)
    if (!request) throw new C311ApiError(this.fixtures.errors['not-found'], 404)
    return request
  }

  private requestVersion (requestID: string): number {
    if (this.requestVersions[requestID] === undefined) {
      const detail = this.fixtures.details[requestID]
      const request = this.fixtures.requests.find(item => item.request_id === requestID)
      this.requestVersions[requestID] = detail?.request.version || request?.version || 1
    }
    return this.requestVersions[requestID]
  }

  private updateRequestStatus (requestID: string, status: ServiceRequest['status']): number {
    const request = this.request(requestID)
    const version = this.requestVersion(requestID) + 1
    const updatedAt = '2026-01-15T15:00:00.000Z'
    request.status = status
    request.version = version
    request.updated_at = updatedAt
    this.requestVersions[requestID] = version

    const detail = this.fixtures.details[requestID]
    if (detail) {
      detail.request = { ...detail.request, status, version, updated_at: updatedAt }
      detail.history = detail.history.concat({ action: status, occurred_at: updatedAt, responsible_department: request.owning_department })
    }
    const publicDetail = request.request_number ? this.fixtures.public_details[request.request_number] : undefined
    if (publicDetail) {
      publicDetail.status = status
      publicDetail.updated_at = updatedAt
      publicDetail.history = publicDetail.history.concat({ action: status, occurred_at: updatedAt, responsible_department: request.owning_department })
    }
    const queueItem = this.fixtures.queue.find(item => item.request_id === requestID)
    if (queueItem) {
      queueItem.status = status
      queueItem.version = version
      queueItem.updated_at = updatedAt
    }
    return version
  }

  private snapshotRequestState (): Pick<C311FixtureSet, 'requests' | 'queue' | 'details' | 'public_details'> & { requestVersions: Record<string, number>, notes: Record<string, RequestNote[]>, noteSerial: number } {
    return {
      requests: copy(this.fixtures.requests),
      queue: copy(this.fixtures.queue),
      details: copy(this.fixtures.details),
      public_details: copy(this.fixtures.public_details),
      requestVersions: copy(this.requestVersions),
      notes: copy(this.notes),
      noteSerial: this.noteSerial,
    }
  }

  private restoreRequestState (snapshot: ReturnType<MockC311Provider['snapshotRequestState']>): void {
    this.fixtures.requests.splice(0, this.fixtures.requests.length, ...snapshot.requests)
    this.fixtures.queue.splice(0, this.fixtures.queue.length, ...snapshot.queue)
    for (const key of Object.keys(this.fixtures.details)) delete this.fixtures.details[key]
    Object.assign(this.fixtures.details, snapshot.details)
    for (const key of Object.keys(this.fixtures.public_details)) delete this.fixtures.public_details[key]
    Object.assign(this.fixtures.public_details, snapshot.public_details)
    for (const key of Object.keys(this.requestVersions)) delete this.requestVersions[key]
    Object.assign(this.requestVersions, snapshot.requestVersions)
    for (const key of Object.keys(this.notes)) delete this.notes[key]
    Object.assign(this.notes, snapshot.notes)
    this.noteSerial = snapshot.noteSerial
  }

  private bulkError (error: unknown, requestID: string): never {
    if (error instanceof C311ApiError) {
      throw new C311ApiError({ ...error.toJSON(), failing_request_id: requestID }, error.status, error.headers)
    }
    throw error
  }

  private consumeScenarioFailure (operation: string): boolean {
    const key = `${this.scenario}:${operation}`
    if (this.consumedScenarioFailures.has(key)) return false
    this.consumedScenarioFailures.add(key)
    return true
  }

  private portalRelationshipIDs (): Set<string> {
    return new Set([this.profile.constituent_id, this.currentSession.actor?.actor_id].filter(Boolean) as string[])
  }

  private hasVisiblePortalRelationship (requestID: string): boolean {
    const identities = this.portalRelationshipIDs()
    return (this.relationships[requestID] || []).some(relationship => relationship.portal_visible && identities.has(relationship.constituent_id))
  }

  private hasValidAuthenticatedSession (): boolean {
    const expiresAt = this.currentSession.expires_at
    return this.currentSession.authenticated && (!expiresAt || Date.parse(expiresAt) > Date.now())
  }

  private requireVisiblePortalRelationship (requestID: string): void {
    if (!this.hasVisiblePortalRelationship(requestID)) {
      throw new C311ApiError({ error: 'FORBIDDEN', message: 'You are not associated with this request.', retryable: false }, 403)
    }
  }

  private syncPublicRelationships (requestID: string): void {
    const current = this.relationships[requestID] || []
    const currentKeys = new Set(current.map(item => `${item.constituent_id}:${item.relationship_type}`))
    const preservedHidden = (this.publicRelationships[requestID] || []).filter(item => !currentKeys.has(`${item.constituent_id}:${item.relationship_type}`) && !item.portal_visible)
    this.publicRelationships[requestID] = current.concat(preservedHidden)
  }

  private syncPublicNotes (requestID: string, note: RequestNote): void {
    if (!note.portal_visible) return
    this.publicNotes[requestID] = (this.publicNotes[requestID] || []).concat(note)
  }

  private staffDetail (requestID: string): StaffServiceRequestDetail {
    const detail = this.fixtures.details[requestID]
    if (!detail) throw new C311ApiError(this.fixtures.errors['not-found'], 404)
    return copy({
      ...detail,
      request: { ...detail.request, version: this.requestVersion(requestID) },
      relationships: this.relationships[requestID] || [],
      notes: this.notes[requestID] || [],
      audit: [...detail.audit, ...(this.relationshipAudits[requestID] || []).map(event => ({ ...event } as Record<string, unknown>))],
    })
  }

  private draft (requestID: string): ServiceRequest {
    const draft = this.draftRecords[requestID]
    if (!draft) throw new C311ApiError(this.fixtures.errors['not-found'], 404)
    return draft
  }

  private requestSummary (request: ServiceRequest): RequestSummary {
    return {
      request_id: request.request_id,
      request_number: request.request_number || '',
      summary: request.summary,
      service_type: request.service_type,
      status: request.status,
      owning_department: request.owning_department,
      updated_at: request.updated_at,
    }
  }

  private staffRequest (requestID: string, capability: ContractCapability, options: C311RequestOptions = {}, requireVersion = true): StaffServiceRequestDetail {
    this.requireStaffCapability(capability)
    const detail = this.fixtures.details[requestID]
    if (!detail) throw new C311ApiError(this.fixtures.errors['not-found'], 404)
    const actor = this.currentSession.actor
    const request = detail.request
    const currentVersion = this.requestVersion(requestID)
    request.version = currentVersion
    const unrestricted = actor?.application_roles.includes('platform_administrator')
    const inDepartment = unrestricted || !!actor?.department_codes.includes(request.owning_department)
    const inDistrict = unrestricted || !request.council_district || !!actor?.district_codes.includes(request.council_district)
    if (!inDepartment || !inDistrict) throw new C311ApiError({ error: 'FORBIDDEN', message: 'The request is outside your assigned scope.', retryable: false }, 403)
    if (options.expectedVersion !== undefined && options.expectedVersion !== currentVersion) {
      throw new C311ApiError({ error: 'VERSION_CONFLICT', message: 'The record changed before your update.', retryable: false, current_version: currentVersion }, 409)
    }
    if (requireVersion && options.expectedVersion === undefined && capability !== 'staff_request_detail') {
      throw new C311ApiError({ error: 'EXPECTED_VERSION_REQUIRED', message: 'An expected version is required for this operation.', retryable: false }, 428)
    }
    return detail
  }

  private requireStaffCapability (capability: ContractCapability): void {
    this.requireCapability(capability)
  }

  private syncQueueItem (detail: StaffServiceRequestDetail): void {
    const index = this.fixtures.queue.findIndex(item => item.request_id === detail.request.request_id)
    if (index < 0) return
    const request = detail.request
    this.fixtures.queue[index] = {
      ...this.fixtures.queue[index],
      request_number: request.request_number || '',
      summary: request.summary,
      service_type: request.service_type,
      status: request.status,
      owning_department: request.owning_department,
      council_district: request.council_district,
      origin_class: request.origin_class,
      version: request.version,
      updated_at: request.updated_at,
      primary_assignee_id: detail.primary_assignee_id,
      duplicate_group_id: request.duplicate_group_id,
      available_actions: detail.available_actions,
    }
  }

  private bumpStaffRequest (detail: StaffServiceRequestDetail): void {
    detail.request.version += 1
    this.requestVersions[detail.request.request_id] = detail.request.version
    detail.request.updated_at = '2026-01-15T15:00:00.000Z'
    this.syncQueueItem(detail)
  }

  private auditStaffRequest (detail: StaffServiceRequestDetail, action: string, context: Record<string, unknown> = {}): void {
    detail.audit = [...detail.audit, { action, actor_id: this.currentSession.actor?.actor_id || 'fixture', occurred_at: '2026-01-15T15:00:00.000Z', ...context }]
  }

  private appendStaffNote (requestID: string, body: string, portalVisible = false): RequestNote {
    const note: RequestNote = {
      note_id: `note-fixture-${String(++this.noteSerial).padStart(3, '0')}`,
      request_id: requestID,
      author_constituent_id: this.currentSession.actor?.actor_id,
      body,
      portal_visible: portalVisible,
      created_at: '2026-01-15T15:00:00.000Z',
    }
    this.notes[requestID] = (this.notes[requestID] || []).concat(note)
    return note
  }

  private assignmentNotifications (detail: StaffServiceRequestDetail, previousAssigneeID: string | null, assigneeID: string): NonNullable<StaffServiceRequestDetail['assignment_notifications']> {
    const occurredAt = '2026-01-15T15:00:00.000Z'
    const recipients = [
      ...(previousAssigneeID && previousAssigneeID !== assigneeID ? [{ recipient_staff_id: previousAssigneeID, recipient_role: 'FORMER_PRIMARY_ASSIGNEE' as const }] : []),
      { recipient_staff_id: assigneeID, recipient_role: 'NEW_PRIMARY_ASSIGNEE' as const },
    ]
    const offset = detail.assignment_notifications?.length || 0
    const notifications = recipients.map((recipient, index) => ({
      notification_id: `assignment-notification-fixture-${String(offset + index + 1).padStart(3, '0')}`,
      request_id: detail.request.request_id,
      ...recipient,
      result: 'SENT' as const,
      occurred_at: occurredAt,
    }))
    detail.assignment_notifications = [...(detail.assignment_notifications || []), ...notifications]
    return notifications
  }

  private ensureCivicWorksWorkOrder (detail: StaffServiceRequestDetail): CivicWorksWorkOrder {
    if (!detail.external_work_order) {
      const workOrderID = `WO-${detail.request.request_id}`
      detail.external_work_order = {
        work_order_id: workOrderID,
        source_case_id: detail.request.request_id,
        service_request_number: detail.request.request_number || detail.request.request_id,
        status: 'ASSIGNED',
        external_status_url: `https://civicworks.fixture.invalid/ui/work-orders/${encodeURIComponent(workOrderID)}`,
        version: 1,
        created_at: '2026-01-15T15:00:00.000Z',
        updated_at: '2026-01-15T15:00:00.000Z',
      }
    }
    return detail.external_work_order
  }

  private transitionTargets (status: ServiceRequest['status']): ServiceRequest['status'][] {
    const targets: Record<ServiceRequest['status'], ServiceRequest['status'][]> = {
      DRAFT: ['SUBMITTED'],
      SUBMITTED: ['TRIAGED'],
      TRIAGED: ['ASSIGNED'],
      ASSIGNED: ['IN_PROGRESS'],
      IN_PROGRESS: ['RESOLVED'],
      RESOLVED: ['CLOSED', 'REOPENED'],
      CLOSED: ['REOPENED'],
      REOPENED: ['ASSIGNED', 'IN_PROGRESS'],
    }
    return targets[status] || []
  }

  private availableActionsFor (status: ServiceRequest['status']): import('./enums').RequestAction[] {
    const actionByTarget: Partial<Record<ServiceRequest['status'], import('./enums').RequestAction[]>> = {
      SUBMITTED: ['TRIAGE'],
      TRIAGED: ['ASSIGN'],
      ASSIGNED: ['START_PROGRESS'],
      IN_PROGRESS: ['RESOLVE'],
      RESOLVED: ['CLOSE', 'REQUEST_REOPEN'],
      CLOSED: ['REQUEST_REOPEN'],
      REOPENED: ['ASSIGN', 'START_PROGRESS'],
    }
    return actionByTarget[status] || []
  }

  private requireReason (reason: unknown): asserts reason is string {
    if (typeof reason !== 'string' || !reason.trim()) this.failScenario('validation')
  }

  async getSession (): Promise<Session> {
    this.failIfNeeded()
    return copy(this.currentSession)
  }

  async signIn (_input: LocalSignIn): Promise<Session> {
    if (this.scenario === 'invalid-credentials') this.failScenario('invalid-credentials')
    this.failIfNeeded(['validation'])
    this.currentSession = copy(this.fixtures.role_fixtures.constituent.session)
    return copy(this.currentSession)
  }

  async signOut (): Promise<void> {
    if (this.scenario === 'federated-logout-failure') this.failScenario('federated-logout-failure')
    this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    this.currentSession = { authenticated: false, actor: null, preferred_language: 'EN', expires_at: null }
  }

  async registerAccount (_input: AccountRegistration): Promise<AccountRegistrationAcknowledgement> {
    if (this.scenario === 'registration-validation') this.failScenario('registration-validation')
    return { accepted: true }
  }

  async requestPasswordReset (_input: PasswordResetRequest): Promise<PasswordResetResponse> {
    this.failIfNeeded(['retryable', 'terminal'])
    this.resetTokenSerial += 1
    this.activeResetToken = `reset-token-fixture-${String(this.resetTokenSerial).padStart(3, '0')}`
    this.resetTokenUsed = false
    return { message: 'If the account exists, instructions have been sent.' }
  }

  async confirmPasswordReset (input: PasswordResetConfirm): Promise<PasswordResetResponse> {
    if (this.scenario === 'expired-reset-token') this.failScenario('expired-reset-token')
    if (this.scenario === 'invalid-reset-token') this.failScenario('invalid-reset-token')
    if (!this.activeResetToken && input.token === 'ephemeral-token' && ['success', 'successful-reset'].includes(this.scenario)) {
      this.activeResetToken = input.token
      this.resetTokenUsed = false
    }
    if (input.token !== this.activeResetToken || this.resetTokenUsed) this.failScenario('invalid-reset-token')
    this.resetTokenUsed = true
    return { message: 'Your password has been reset.' }
  }

  async changeLoginIdentifier (_input: LoginIdentifierChange): Promise<Session> {
    this.requireCapability('login_identifier_change')
    if (this.scenario === 'invalid-credentials') this.failScenario('invalid-credentials')
    this.failIfNeeded(['forbidden', 'not-found', 'validation', 'version-conflict'])
    return copy(this.currentSession)
  }

  async changePassword (_input: PasswordChange): Promise<void> {
    this.requireCapability('password_change')
    if (this.scenario === 'invalid-credentials') this.failScenario('invalid-credentials')
    this.failIfNeeded(['forbidden', 'not-found', 'validation', 'version-conflict'])
  }

  async deleteOrAnonymizeAccount (input: AccountDispositionRequest): Promise<AccountDispositionResult> {
    this.requireCapability('profile_get')
    if (!input || !['DELETE', 'ANONYMIZE'].includes(input.mode) || input.confirmation !== input.mode) this.failScenario('validation')
    if (this.scenario === 'account-disposition-conflict') this.failScenario('account-disposition-conflict')
    if (this.scenario === 'account-disposition-failure') this.failScenario('account-disposition-failure')
    this.countWrite('account_disposition')
    if (input.mode === 'ANONYMIZE') {
      this.profile = {
        ...this.profile,
        display_name: 'Anonymous user',
        login_identifier: undefined,
        emails: [],
        phone_numbers: [],
        addresses: [],
      }
    }
    this.currentSession = { authenticated: false, actor: null, preferred_language: 'EN', expires_at: null }
    return {
      status: input.mode === 'DELETE' ? 'DELETED' : 'ANONYMIZED',
      message: input.mode === 'DELETE' ? 'Account deleted.' : 'Account anonymized.',
    }
  }

  async startFederatedSignIn (provider: IdentityProvider): Promise<FederatedRedirect> {
    if (provider === 'saml') throw new C311ApiError({ error: 'FORBIDDEN', message: 'SAML sign-in is available on the staff surface only.', retryable: false }, 403)
    if (this.scenario === 'oidc-failure' && provider === 'oidc') this.failScenario('oidc-failure')
    return { authorization_url: `https://identity.example.test/${provider}/authorize` }
  }

  async confirmAccountLink (): Promise<Session> {
    if (!this.pendingAccountLinkProvider || !this.pendingAccountLinkExpiresAt || Date.parse(this.pendingAccountLinkExpiresAt) <= Date.now()) {
      this.clearPendingAccountLink()
      throw new C311ApiError({ error: 'VALIDATION_ERROR', message: 'The account-link confirmation is no longer valid.', retryable: false }, 422)
    }
    if (this.pendingAccountLinkConsumed) throw new C311ApiError({ error: 'VERSION_CONFLICT', message: 'The account-link confirmation was already consumed.', retryable: false }, 409)
    this.pendingAccountLinkConsumed = true
    this.persistPendingAccountLink()
    if (this.scenario === 'account-link-conflict') this.failScenario('account-link-conflict')
    if (this.scenario === 'identity-claims-failure') this.failScenario('identity-claims-failure')
    this.currentSession = copy(this.fixtures.role_fixtures.constituent.session)
    return copy(this.currentSession)
  }

  async completeFederatedSignIn (_provider: IdentityProvider, _query: Record<string, string> = {}): Promise<FederatedSignInResult> {
    if (_provider === 'saml') throw new C311ApiError({ error: 'FORBIDDEN', message: 'SAML sign-in is available on the staff surface only.', retryable: false }, 403)
    if (['access_denied', 'cancelled', 'canceled'].includes(String(_query.error || '').toLowerCase())) {
      throw new C311ApiError({ error: 'UNAUTHENTICATED', message: 'Federated sign-in was cancelled.', retryable: false }, 401)
    }
    if (this.scenario === 'oidc-failure') this.failScenario('oidc-failure')
    if (this.scenario === 'saml-failure') this.failScenario('saml-failure')
    if (this.scenario === 'identity-claims-failure') this.failScenario('identity-claims-failure')
    if (['link-confirmation-required', 'account-link-success', 'account-link-cancelled', 'account-link-conflict'].includes(this.scenario)) {
      this.pendingAccountLinkProvider = 'oidc'
      this.pendingAccountLinkExpiresAt = new Date(Date.now() + 10 * 60 * 1000).toISOString()
      this.pendingAccountLinkConsumed = false
      this.persistPendingAccountLink()
      return { outcome: 'link_confirmation_required', pending_link: { expires_at: this.pendingAccountLinkExpiresAt, provider_label: 'OIDC' } }
    }
    this.currentSession = copy(this.fixtures.role_fixtures.constituent.session)
    return { outcome: 'authenticated', session: copy(this.currentSession) }
  }

  async getBranding (): Promise<Branding> {
    if (this.scenario === 'branding-failure') this.failScenario('branding-failure')
    this.failIfNeeded(['terminal'])
    return copy(this.fixtures.branding || createDefaultFixtureSet().branding!)
  }

  async getPublicContent (contentKey: PublicContentKey): Promise<ContentObject> {
    if (this.scenario === 'content-loading-failure') this.failScenario('content-loading-failure')
    this.failIfNeeded(['terminal'])
    const content = this.fixtures.public_content?.[contentKey]
    if (!content) throw new C311ApiError(this.fixtures.errors['not-found'], 404)
    if (this.scenario === 'empty-catalogue' && contentKey === 'SERVICE_CATALOGUE') return { ...copy(content), body: '' }
    return copy(content)
  }

  async getPublicHelp (helpKey: HelpKey, language?: Language): Promise<HelpContent> {
    if (this.scenario === 'help-loading-failure') this.failScenario('help-loading-failure')
    this.failIfNeeded(['terminal'])
    const content = this.fixtures.public_help?.[helpKey]
    if (!content) throw new C311ApiError(this.fixtures.errors['not-found'], 404)
    const result = copy(content)
    if (language && language !== result.language) {
      result.language = language
      result.body = language === 'ES' ? '<p>Describe el problema y envialo a la ciudad.</p>' : language === 'VI' ? '<p>Mo ta van de va gui den thanh pho.</p>' : '<p>Describe the issue and submit it to the city.</p>'
    }
    return result
  }

  async getProfile (): Promise<Constituent> {
    this.requireCapability('profile_get')
    if (this.scenario === 'account-loading') this.failScenario('account-loading')
    return copy(this.profile)
  }

  async updateProfile (input: ProfileUpdate, options: C311RequestOptions = {}): Promise<Constituent> {
    this.requireCapability('profile_update')
    this.failIfNeeded(['forbidden', 'not-found', 'validation', 'version-conflict'])
    if (options.expectedVersion === undefined) throw new C311ApiError({ error: 'EXPECTED_VERSION_REQUIRED', message: 'If-Match is required for this update.', retryable: false }, 428)
    if (options.expectedVersion !== undefined && options.expectedVersion !== this.profile.version) this.failScenario('version-conflict')
    if (!validMockProfileInput(input)) this.failScenario('validation')
    this.profile = { ...this.profile, ...input, version: (this.profile.version || 0) + 1, updated_at: new Date().toISOString() }
    return copy(this.profile)
  }

  async updateLanguage (language: Language): Promise<LanguagePreference> {
    this.currentSession = { ...this.currentSession, preferred_language: language }
    return { language }
  }

  async getOperation (operationID: string): Promise<Operation> {
    this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    if (this.scenario === 'terminal') {
      return {
        operation_id: operationID,
        kind: 'fixture',
        status: 'FAILED',
        progress: 100,
        result: null,
        error: copy(this.fixtures.errors.terminal),
        created_at: '2026-01-15T15:00:00.000Z',
        updated_at: '2026-01-15T15:00:00.000Z',
        completed_at: '2026-01-15T15:00:00.000Z',
      }
    }
    return {
      operation_id: operationID,
      kind: 'fixture',
      status: 'SUCCEEDED',
      progress: 100,
      result: {},
      error: null,
      created_at: '2026-01-15T15:00:00.000Z',
      updated_at: '2026-01-15T15:00:00.000Z',
      completed_at: '2026-01-15T15:00:00.000Z',
    }
  }

  async uploadPortalAttachment (input: PortalAttachmentUpload): Promise<PortalAttachment> {
    if (this.scenario === 'attachment-retryable' && this.attachmentRetryFailures++ === 0) {
      throw new C311ApiError({ error: 'TEMPORARILY_UNAVAILABLE', message: 'The attachment service is temporarily unavailable.', retryable: true }, 503, { 'Retry-After': '30' })
    }
    if (this.scenario === 'attachment-terminal') this.failScenario('attachment-terminal')
    this.failIfNeeded(['validation', 'retryable', 'terminal'])
    const size = typeof input.file === 'string' ? input.file.length : Number((input.file as Blob)?.size)
    const validationErrors = validatePortalAttachment({ filename: input.filename, media_type: input.media_type, size })
    if (validationErrors.length) {
      throw new C311ApiError({ error: 'VALIDATION_ERROR', message: 'The attachment is not valid.', retryable: false, errors: validationErrors }, 422)
    }
    if (this.uploadedAttachmentTokens.size >= 5) {
      throw new C311ApiError({ error: 'VALIDATION_ERROR', message: 'A request can include at most five attachments.', retryable: false, errors: [{ field: 'file', code: 'TOO_MANY_ITEMS' }] }, 422)
    }
    this.attachmentSerial += 1
    const attachmentToken = `attachment-token-fixture-${String(this.attachmentSerial).padStart(3, '0')}`
    this.uploadedAttachmentTokens.add(attachmentToken)
    this.countWrite('portal_attachment_upload')
    return {
      attachment_token: attachmentToken,
      filename: input.filename.split(/[\\/]/).pop() || input.filename,
      media_type: input.media_type,
      size,
      expires_at: '2026-01-15T16:00:00.000Z',
    }
  }

  // Mock-only lifecycle helper used by the attachment picker when a staged file is removed.
  // The real API has no client-side delete operation for an upload token.
  removePortalAttachment (attachmentToken: string): void {
    this.uploadedAttachmentTokens.delete(attachmentToken)
  }

  async downloadAttachment (attachmentID: string): Promise<BinaryAttachment> {
    this.requireCapability('attachment_download')
    this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    const attachment = this.fixtures.attachments[attachmentID]
    if (!attachment && this.uploadedAttachmentTokens.has(attachmentID)) {
      return { content_type: 'text/plain', content_disposition: 'inline; filename="fixture.txt"', body: 'fixture attachment' }
    }
    if (!attachment) throw new C311ApiError(this.fixtures.errors['not-found'], 404)
    return copy(attachment)
  }

  async createServiceRequest (_input: ServiceRequestCreate, _options: C311RequestOptions = {}): Promise<ServiceRequestResponse> {
    this.failIfNeeded(['forbidden', 'validation', 'version-conflict'])
    this.countWrite('service_request_create')
    return {
      request_id: 'request-fixture-created',
      request_number: 'SR-2026-00002',
      status: 'SUBMITTED' as const,
      version: 1,
      created_at: '2026-01-15T15:00:00.000Z',
      links: { self: '/api/v1/service-requests/request-fixture-created' },
    }
  }

  async submitPortalRequest (input: PortalServiceRequestCreate, options: C311RequestOptions = {}): Promise<ServiceRequestResponse> {
    if (this.scenario === 'idempotency-conflict') this.failScenario('idempotency-conflict')
    this.failIfNeeded(['forbidden', 'validation', 'retryable', 'terminal'])
    const key = options.idempotencyKey
    const fingerprint = this.fingerprint(input)
    if (key) {
      const previous = this.idempotentResponses.get(key)
      if (previous && previous.fingerprint !== fingerprint) {
        throw new C311ApiError({ error: 'IDEMPOTENCY_CONFLICT', message: 'The idempotency key has already been used with different content.', retryable: false }, 409)
      }
      if (previous) return copy(previous.response)
    }
    for (const token of input.attachment_tokens || []) {
      if (this.consumedAttachmentTokens.has(token)) this.failScenario('idempotency-conflict')
    }
    this.countWrite('portal_service_request_submit')
    const response = {
      request_id: 'request-fixture-submitted',
      request_number: 'SR-2026-00002',
      status: 'SUBMITTED' as const,
      version: 1,
      created_at: '2026-01-15T15:00:00.000Z',
      links: { self: '/api/v1/portal/service-requests/request-fixture-submitted' },
    }
    if (key) this.idempotentResponses.set(key, { fingerprint, response })
    for (const token of input.attachment_tokens || []) {
      if (this.uploadedAttachmentTokens.has(token)) {
        this.consumedAttachmentTokens.add(token)
        this.uploadedAttachmentTokens.delete(token)
      }
    }
    return copy(response)
  }

  async createStaffServiceRequest (input: StaffServiceRequestCreate): Promise<StaffServiceRequestDetail> {
    this.requireCapability('staff_service_request_create')
    this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    this.countWrite('staff_service_request_create')
    const request = this.makeDraftRecord('staff-request-fixture-created', input.request, 1)
    request.status = 'SUBMITTED'
    request.request_number = 'SR-2026-00003'
    return copy({
      request,
      available_actions: [],
      primary_assignee_id: null,
      collaborator_ids: [],
      reminders: [],
      history: [],
      audit: [],
      external_work_order: null,
    })
  }

  async createDraft (input: DraftWrite, _options: C311RequestOptions = {}): Promise<ServiceRequest> {
    this.requireCapability('portal_draft_create')
    this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    const requestID = input.request_id || `draft-fixture-created-${Object.keys(this.draftRecords).length + 1}`
    const payload = { ...copy(input), request_id: requestID }
    const draft = this.makeDraftRecord(requestID, payload, 1)
    this.draftPayloads[requestID] = payload
    this.draftRecords[requestID] = draft
    this.fixtures.drafts[requestID] = copy(payload)
    this.countWrite('portal_draft_create')
    return copy(draft)
  }

  async getDraft (requestID: string): Promise<ServiceRequest> {
    this.requireCapability('portal_draft_get')
    this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    return copy(this.draft(requestID))
  }

  async updateDraft (requestID: string, input: DraftWrite, options: C311RequestOptions = {}): Promise<ServiceRequest> {
    this.requireCapability('portal_draft_update')
    this.failIfNeeded(['forbidden', 'not-found', 'validation', 'version-conflict'])
    if (this.scenario === 'expected-version-required' && options.expectedVersion === undefined) this.failScenario('expected-version-required')
    const current = this.draft(requestID)
    if (options.expectedVersion !== undefined && options.expectedVersion !== current.version) {
      throw new C311ApiError({ error: 'VERSION_CONFLICT', message: 'The record changed before your update.', retryable: false, current_version: current.version }, 409)
    }
    const payload = { ...this.draftPayloads[requestID], ...copy(input), request_id: requestID }
    const updated = this.makeDraftRecord(requestID, payload, current.version + 1)
    this.draftPayloads[requestID] = payload
    this.draftRecords[requestID] = updated
    this.fixtures.drafts[requestID] = copy(payload)
    this.countWrite('portal_draft_update')
    return copy(updated)
  }

  async deleteDraft (requestID: string, options: C311RequestOptions = {}): Promise<void> {
    this.requireCapability('portal_draft_delete')
    this.failIfNeeded(['forbidden', 'not-found', 'validation', 'version-conflict'])
    if (this.scenario === 'expected-version-required' && options.expectedVersion === undefined) this.failScenario('expected-version-required')
    const current = this.draft(requestID)
    if (options.expectedVersion !== undefined && options.expectedVersion !== current.version) {
      throw new C311ApiError({ error: 'VERSION_CONFLICT', message: 'The record changed before your update.', retryable: false, current_version: current.version }, 409)
    }
    delete this.draftRecords[requestID]
    delete this.draftPayloads[requestID]
    delete this.fixtures.drafts[requestID]
    this.countWrite('portal_draft_delete')
  }

  async submitDraft (requestID: string, options: C311RequestOptions = {}): Promise<ServiceRequestResponse> {
    this.requireCapability('portal_draft_submit')
    this.failIfNeeded(['forbidden', 'not-found', 'validation', 'version-conflict'])
    if (this.scenario === 'expected-version-required' && options.expectedVersion === undefined) this.failScenario('expected-version-required')
    const current = this.draft(requestID)
    if (options.expectedVersion !== undefined && options.expectedVersion !== current.version) {
      throw new C311ApiError({ error: 'VERSION_CONFLICT', message: 'The record changed before your update.', retryable: false, current_version: current.version }, 409)
    }
    this.countWrite('portal_draft_submit')
    delete this.draftRecords[requestID]
    delete this.draftPayloads[requestID]
    delete this.fixtures.drafts[requestID]
    return {
      request_id: requestID,
      request_number: 'SR-2026-00003',
      status: 'SUBMITTED',
      version: 1,
      created_at: '2026-01-15T15:00:00.000Z',
      links: { self: `/api/v1/portal/service-requests/${requestID}` },
    }
  }

  async listPortalRequests (query: RequestListQuery = {}): Promise<PageResponse<RequestSummary>> {
    this.requireCapability('portal_my_requests')
    this.failIfNeeded(['forbidden', 'not-found', 'validation', 'retryable', 'version-conflict', 'terminal'])
    const sourceItems = this.scenario === 'empty' || this.scenario === 'empty-my-requests' ? [] : this.fixtures.requests
    const items = sourceItems.filter(request => this.hasVisiblePortalRelationship(request.request_id))
    if (sourceItems.length && !items.length) {
      throw new C311ApiError({ error: 'FORBIDDEN', message: 'You are not associated with any portal requests.', retryable: false }, 403)
    }
    return this.page(items.map(request => this.requestSummary(request)), query)
  }

  async linkAnonymousRequest (input: AnonymousStatusLookupRequest): Promise<ServiceRequest> {
    this.requireCapability('portal_link_anonymous_request')
    this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    const item = this.fixtures.requests.find(request => request.request_number === input.request_number && request.primary_requester.emails.includes(input.email))
    if (!item) throw new C311ApiError(this.fixtures.errors['not-found'], 404)
    return copy(item)
  }

  async reopenPortalRequest (requestID: string, reason: string, _options: C311RequestOptions = {}): Promise<ReopenRequestResponse> {
    this.requireCapability('portal_reopen_request')
    this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    if (!String(reason || '').trim()) this.failScenario('validation')
    const request = this.request(requestID)
    this.requireVisiblePortalRelationship(requestID)
    if (request.status !== 'RESOLVED' && request.status !== 'CLOSED') {
      throw new C311ApiError({
        error: 'VALIDATION_ERROR',
        message: 'Only resolved or closed requests can be reopened.',
        retryable: false,
      }, 422)
    }
    this.updateRequestStatus(requestID, 'REOPENED')
    return { request_id: requestID, status: 'PENDING_APPROVAL' }
  }

  async getPublicStatus (input: AnonymousStatusLookupRequest): Promise<AnonymousStatusLookupResponse> {
    if (this.scenario === 'not-found') return { request_detail: null }
    this.failIfNeeded(['validation', 'retryable', 'terminal'])
    const normalizedEmail = typeof input.email === 'string' ? input.email.trim().toLowerCase() : ''
    const request = this.fixtures.requests.find(item => item.request_number === input.request_number)
    const detail = request ? this.fixtures.public_details[input.request_number] : undefined
    const primaryEmailMatches = !!request && request.primary_requester.emails.some(email => email.trim().toLowerCase() === normalizedEmail)
    const profileEmailMatches = this.profile.emails.some(email => email.trim().toLowerCase() === normalizedEmail)
    const relationshipMatches = !!request && this.hasValidAuthenticatedSession() && profileEmailMatches && this.hasVisiblePortalRelationship(request.request_id)
    if (!request || !detail || (!primaryEmailMatches && !relationshipMatches)) return { request_detail: null }
    const relationships = (this.publicRelationships[request.request_id] || this.relationships[request.request_id] || []).filter(item => item.portal_visible)
    const notes = (this.publicNotes[request.request_id] || this.notes[request.request_id] || []).filter(item => item.portal_visible)
    return { request_detail: copy({ ...detail, relationships, notes }) }
  }

  async createPortalNote (requestID: string, input: RequestNote): Promise<RequestNote> {
    this.requireCapability('portal_my_requests')
    this.failIfNeeded(['forbidden', 'not-found', 'validation', 'retryable', 'terminal'])
    this.request(requestID)
    this.requireVisiblePortalRelationship(requestID)
    if (!input || typeof input.body !== 'string' || !input.body.trim() || input.body.length > 2000 || input.portal_visible !== true) this.failScenario('validation')

    const note: RequestNote = {
      note_id: `portal-note-fixture-${String(++this.noteSerial).padStart(3, '0')}`,
      request_id: requestID,
      author_constituent_id: this.currentSession.actor?.actor_id,
      body: input.body,
      portal_visible: true,
      created_at: '2026-01-15T15:00:00.000Z',
    }
    this.notes[requestID] = (this.notes[requestID] || []).concat(note)
    this.syncPublicNotes(requestID, note)
    this.countWrite('portal_note_create')
    return copy(note)
  }

  async geocode (input: GeocodeRequest): Promise<GeocodeResponse> {
    if (this.scenario === 'map-auth-failure') {
      throw new C311ApiError({ error: 'MAP_UNAUTHENTICATED', message: 'The mapping service credentials are unavailable.', retryable: false }, 401)
    }
    if (this.scenario === 'map-retryable') {
      throw new C311ApiError({ error: 'MAP_TEMPORARILY_UNAVAILABLE', message: 'The mapping service is temporarily unavailable.', retryable: true }, 503, { 'Retry-After': '30' })
    }
    if (this.scenario === 'retryable') {
      throw new C311ApiError({ error: 'MAP_TEMPORARILY_UNAVAILABLE', message: 'The mapping service is temporarily unavailable.', retryable: true }, 503, { 'Retry-After': '30' })
    }
    if (this.scenario === 'not-found') {
      throw new C311ApiError({ error: 'ADDRESS_NOT_FOUND', message: 'The address could not be found.', retryable: false }, 404)
    }
    this.failIfNeeded(['validation'])
    const normalizedAddress = normalizeGeocodeAddress(input.address)
    const result = Object.entries(this.fixtures.geocodes).find(([address]) => normalizeGeocodeAddress(address) === normalizedAddress)?.[1]
    if (!result) {
      throw new C311ApiError({ error: 'ADDRESS_NOT_FOUND', message: 'The address could not be found.', retryable: false }, 404)
    }
    return copy(result)
  }

  async listStaffRequests (query: RequestListQuery = {}): Promise<PageResponse<RequestQueueItem>> {
    this.requireStaffCapability('staff_request_queue')
    this.failIfNeeded(['forbidden', 'not-found', 'validation', 'retryable', 'version-conflict', 'terminal'])
    const pageSize = query.page_size === undefined ? 50 : Number(query.page_size)
    if (!Number.isInteger(pageSize) || pageSize < 1 || pageSize > 100) {
      throw new C311ApiError({ error: 'VALIDATION_ERROR', message: 'page_size must be between 1 and 100.', retryable: false, errors: [{ field: '/page_size', code: 'OUT_OF_RANGE' }] }, 422)
    }
    const sort = query.sort ? String(query.sort).split(',').map(value => value.trim()).filter(Boolean) : []
    if (sort.length > 3) throw new C311ApiError({ error: 'VALIDATION_ERROR', message: 'At most three sort fields are supported.', retryable: false, errors: [{ field: '/sort', code: 'TOO_MANY_ITEMS' }] }, 422)
    let offset = 0
    if (query.page_token) {
      const match = /^fixture-page-(\d+)$/.exec(query.page_token)
      if (!match) throw new C311ApiError({ error: 'INVALID_PAGE_TOKEN', message: 'The page token is invalid.', retryable: false }, 422)
      offset = Number(match[1])
    }
    const actor = this.currentSession.actor
    const unrestricted = actor?.application_roles.includes('platform_administrator')
    const inScope = (item: RequestQueueItem) => unrestricted || (!!actor?.department_codes.includes(item.owning_department) && (!item.council_district || !!actor?.district_codes.includes(item.council_district)))
    const filters = { ...query.filters, ...Object.fromEntries(Object.entries(query).filter(([key, value]) => !['filters', 'page_token', 'page_size', 'sort'].includes(key) && value !== undefined)) }
    const filterNames = new Set(['status', 'service_type', 'department', 'district', 'origin_class', 'source_channel', 'assignee', 'collaborator', 'category', 'created_from', 'created_to', 'duplicate_group'])
    for (const [key, value] of Object.entries(filters)) {
      if (value === undefined || value === '') continue
      if (!filterNames.has(key)) throw new C311ApiError({ error: 'INVALID_FILTER', message: 'The request filter is not supported.', retryable: false, errors: [{ field: `/filters/${key}`, code: 'INVALID_VALUE' }] }, 422)
      const values: Record<string, readonly string[]> = { status: SERVICE_REQUEST_STATUSES, service_type: SERVICE_TYPES, department: DEPARTMENT_CODES, district: DISTRICT_CODES, origin_class: ORIGIN_CLASSES, source_channel: SOURCE_CHANNELS }
      if (values[key] && !values[key].includes(String(value))) throw new C311ApiError({ error: 'VALIDATION_ERROR', message: `Unsupported ${key} filter.`, retryable: false, errors: [{ field: `/${key}`, code: 'INVALID_VALUE' }] }, 422)
    }
    const paginationItem = this.fixtures.queue[0] ? { ...this.fixtures.queue[0], request_id: 'request-fixture-002', request_number: 'SR-2026-00002', summary: 'Second fixture request' } : null
    const foreignItem: RequestQueueItem | null = this.fixtures.queue[0] ? { ...this.fixtures.queue[0], request_id: 'request-fixture-foreign', request_number: 'SR-2026-00099', summary: 'Out of scope fixture request', owning_department: 'GENERAL_SERVICES', council_district: 'SOUTH' } : null
    const queue = this.scenario === 'scope-denied' || this.scenario === 'empty' ? [] : this.scenario === 'pagination' && paginationItem ? this.fixtures.queue.concat(paginationItem) : this.scenario === 'scope-filter' && foreignItem ? this.fixtures.queue.concat(foreignItem) : this.fixtures.queue
    let filtered = queue.filter(item => {
      if (!inScope(item)) return false
      const detail = this.fixtures.details[item.request_id]
      return Object.entries(filters).every(([key, value]) => {
        if (value === undefined || value === '') return true
        if (key === 'department') return item.owning_department === value
        if (key === 'district') return item.council_district === value
        if (key === 'duplicate_group') return item.duplicate_group_id === value
        if (key === 'assignee') return item.primary_assignee_id === value
        if (key === 'collaborator') return !!detail?.collaborator_ids.includes(String(value))
        if (key === 'category') return detail?.request.primary_requester.primary_category === value
        if (key === 'created_from') return !!detail && detail.request.created_at >= String(value)
        if (key === 'created_to') return !!detail && detail.request.created_at <= String(value)
        return (item as unknown as Record<string, unknown>)[key] === value
      })
    })
    if (sort.length) {
      filtered = [...filtered].sort((left, right) => {
        for (const field of sort) {
          const descending = field.startsWith('-')
          const key = descending ? field.slice(1) : field
          const leftValue = String((left as unknown as Record<string, unknown>)[key] ?? '')
          const rightValue = String((right as unknown as Record<string, unknown>)[key] ?? '')
          if (leftValue === rightValue) continue
          const result = leftValue < rightValue ? -1 : 1
          return descending ? -result : result
        }
        return 0
      })
    }
    const items = filtered.slice(offset, offset + pageSize)
    return {
      items: copy(items),
      next_page_token: offset + pageSize < filtered.length ? `fixture-page-${offset + pageSize}` : null,
      total_count: filtered.length,
      applied_filters: copy(filters),
      sort,
    }
  }

  async getStaffRequest (requestID: string): Promise<StaffServiceRequestDetail> {
    this.requireStaffCapability('staff_request_detail')
    this.failIfNeeded(['forbidden', 'not-found', 'validation', 'retryable', 'terminal'])
    if (this.scenario === 'scope-denied') this.failScenario('scope-denied')
    this.staffRequest(requestID, 'staff_request_detail', {}, false)
    return this.staffDetail(requestID)
  }

  async linkStaffConstituent (requestID: string, input: ConstituentLink, options: C311RequestOptions = {}): Promise<StaffServiceRequestDetail> {
    this.requireCapability('staff_constituent_link')
    this.failIfNeeded(['forbidden', 'not-found', 'validation', 'version-conflict'])
    if (options.expectedVersion === undefined) this.failScenario('expected-version-required')
    const current = this.staffRequest(requestID, 'staff_constituent_link', options)
    if (options.expectedVersion !== current.request.version) this.failScenario('version-conflict')
    const relationships = this.relationships[requestID] || []
    if (!input || !input.constituent_id || !RELATIONSHIP_TYPES.includes(input.relationship_type) || typeof input.portal_visible !== 'boolean' || typeof input.notify_status !== 'boolean') this.failScenario('validation')
    if (relationships.some(item => item.constituent_id === input.constituent_id && item.relationship_type === input.relationship_type)) this.failScenario('validation')
    if (input.relationship_type === 'PRIMARY_REQUESTER' && relationships.some(item => item.relationship_type === 'PRIMARY_REQUESTER')) this.failScenario('validation')
    const auditSerial = (this.relationshipAudits[requestID] || []).length + 1
    const relationship: RequestRelationship = {
      ...copy(input),
      notification_target: input.notification_target ?? (input.notify_status ? input.constituent_id : null),
      notification_result: input.notify_status ? 'SENT' : 'NOT_REQUESTED',
      audit: [{ audit_id: `relationship-audit-fixture-${String(auditSerial).padStart(3, '0')}`, action: 'LINKED', actor_id: this.currentSession.actor?.actor_id || 'unknown', occurred_at: '2026-01-15T15:00:00.000Z' }],
    }
    this.relationshipAudits[requestID] = (this.relationshipAudits[requestID] || []).concat(relationship.audit || [])
    this.relationships[requestID] = relationships.concat(relationship)
    this.requestVersions[requestID] = current.request.version + 1
    this.syncPublicRelationships(requestID)
    this.countWrite('staff_constituent_link')
    return this.staffDetail(requestID)
  }

  async unlinkStaffConstituent (requestID: string, constituentID: string, input: ConstituentUnlink, options: C311RequestOptions = {}): Promise<StaffServiceRequestDetail> {
    this.requireCapability('staff_constituent_unlink')
    this.failIfNeeded(['forbidden', 'not-found', 'validation', 'version-conflict'])
    if (options.expectedVersion === undefined) this.failScenario('expected-version-required')
    const current = this.staffRequest(requestID, 'staff_constituent_unlink', options)
    if (options.expectedVersion !== current.request.version) this.failScenario('version-conflict')
    if (!input?.reason?.trim()) this.failScenario('validation')
    const relationships = this.relationships[requestID] || []
    const target = relationships.find(item => item.constituent_id === constituentID)
    if (!target) throw new C311ApiError(this.fixtures.errors['not-found'], 404)
    if (target.relationship_type === 'PRIMARY_REQUESTER') this.failScenario('validation')
    this.relationships[requestID] = relationships.filter(item => item !== target)
    const auditSerial = (this.relationshipAudits[requestID] || []).length + 1
    this.relationshipAudits[requestID] = (this.relationshipAudits[requestID] || []).concat({
      audit_id: `relationship-audit-fixture-${String(auditSerial).padStart(3, '0')}`,
      action: 'UNLINKED',
      actor_id: this.currentSession.actor?.actor_id || 'unknown',
      occurred_at: '2026-01-15T15:00:00.000Z',
    })
    this.requestVersions[requestID] = current.request.version + 1
    this.syncPublicRelationships(requestID)
    this.countWrite('staff_constituent_unlink')
    return this.staffDetail(requestID)
  }

  async createStaffNote (requestID: string, input: RequestNote): Promise<RequestNote> {
    this.requireCapability('staff_note_create')
    this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    this.staffRequest(requestID, 'staff_note_create', {}, false)
    if (!input || typeof input.body !== 'string' || !input.body.trim() || input.body.length > 2000 || typeof input.portal_visible !== 'boolean') this.failScenario('validation')
    const note = this.appendStaffNote(requestID, input.body, input.portal_visible)
    this.syncPublicNotes(requestID, note)
    this.countWrite('staff_note_create')
    return copy(note)
  }

  async transitionStaffRequest (requestID: string, input: RequestTransition, options: C311RequestOptions = {}): Promise<StaffServiceRequestDetail> {
    this.failIfNeeded(['forbidden', 'not-found', 'validation', 'version-conflict', 'expected-version-required', 'invalid-status-transition'])
    const detail = this.staffRequest(requestID, 'staff_request_transition', options)
    if (!input || !SERVICE_REQUEST_STATUSES.includes(input.to_status) || !this.transitionTargets(detail.request.status).includes(input.to_status)) {
      throw new C311ApiError(this.fixtures.errors['invalid-status-transition'], 422)
    }
    if (input.reason !== undefined) this.requireReason(input.reason)
    this.updateRequestStatus(requestID, input.to_status)
    detail.request.status = input.to_status
    if (input.to_status === 'ASSIGNED') this.ensureCivicWorksWorkOrder(detail)
    detail.available_actions = this.availableActionsFor(input.to_status)
    this.syncQueueItem(detail)
    this.auditStaffRequest(detail, `STATUS_${input.to_status}`, input.reason ? { reason: input.reason } : {})
    this.countWrite('staff_request_transition')
    return copy(detail)
  }

  async reassignStaffRequest (requestID: string, input: Reassignment, options: C311RequestOptions = {}): Promise<StaffServiceRequestDetail> {
    this.failIfNeeded(['forbidden', 'not-found', 'validation', 'expected-version-required'])
    const detail = this.staffRequest(requestID, 'staff_request_reassign', options)
    if (this.scenario === 'version-conflict' && this.consumeScenarioFailure('staff_request_reassign')) {
      this.bumpStaffRequest(detail)
      throw new C311ApiError({ ...this.fixtures.errors['version-conflict'], current_version: detail.request.version }, 409)
    }
    if (!input || typeof input.assignee_id !== 'string' || !input.assignee_id.trim()) this.failScenario('validation')
    this.requireReason(input.reason)
    const previousAssigneeID = detail.primary_assignee_id || null
    detail.primary_assignee_id = input.assignee_id
    this.bumpStaffRequest(detail)
    const notifications = this.assignmentNotifications(detail, previousAssigneeID, input.assignee_id)
    this.auditStaffRequest(detail, 'ASSIGN', { reason: input.reason, previous_assignee_id: previousAssigneeID, assignee_id: input.assignee_id, notification_results: copy(notifications) })
    this.countWrite('staff_request_reassign')
    return copy(detail)
  }

  async addStaffCollaborator (requestID: string, staffID: string, input: CollaboratorChange, options: C311RequestOptions = {}): Promise<StaffServiceRequestDetail> {
    this.failIfNeeded(['forbidden', 'not-found', 'validation', 'version-conflict', 'expected-version-required'])
    const detail = this.staffRequest(requestID, 'staff_collaborator_add', options)
    if (!staffID.trim()) this.failScenario('validation')
    this.requireReason(input?.reason)
    if (!detail.collaborator_ids.includes(staffID)) detail.collaborator_ids.push(staffID)
    this.bumpStaffRequest(detail)
    this.auditStaffRequest(detail, 'COLLABORATOR_ADD')
    this.countWrite('staff_collaborator_add')
    return copy(detail)
  }

  async removeStaffCollaborator (requestID: string, staffID: string, input: CollaboratorChange, options: C311RequestOptions = {}): Promise<StaffServiceRequestDetail> {
    this.failIfNeeded(['forbidden', 'not-found', 'validation', 'version-conflict', 'expected-version-required'])
    const detail = this.staffRequest(requestID, 'staff_collaborator_remove', options)
    this.requireReason(input?.reason)
    detail.collaborator_ids = detail.collaborator_ids.filter(id => id !== staffID)
    this.bumpStaffRequest(detail)
    this.auditStaffRequest(detail, 'COLLABORATOR_REMOVE')
    this.countWrite('staff_collaborator_remove')
    return copy(detail)
  }

  async createStaffReminder (requestID: string, input: ReminderWrite): Promise<Reminder> {
    this.failIfNeeded(['forbidden', 'not-found', 'validation', 'reminder-validation', 'reminder-retryable', 'reminder-terminal'])
    const detail = this.staffRequest(requestID, 'staff_reminder_create', {}, false)
    if (!input || !input.title?.trim() || !validISODateTime(input.due_at) || !input.timezone || !input.recipient_staff_id || !REMINDER_CHANNELS.includes(input.channel)) this.failScenario('reminder-validation')
    const reminder: Reminder = { reminder_id: `reminder-fixture-${detail.reminders.length + 1}`, request_id: requestID, ...input, status: 'SCHEDULED', completed_at: null }
    detail.reminders.push(reminder)
    this.countWrite('staff_reminder_create')
    return copy(reminder)
  }

  async actionStaffReminder (reminderID: string, action: import('./enums').ReminderAction, input: ReminderActionInput = {}): Promise<Reminder> {
    this.failIfNeeded(['forbidden', 'not-found', 'validation', 'reminder-validation', 'reminder-retryable', 'reminder-terminal'])
    this.requireCapability('staff_reminder_action')
    for (const detail of Object.values(this.fixtures.details)) {
      const reminder = detail.reminders.find(item => item.reminder_id === reminderID)
      if (reminder) {
        this.staffRequest(detail.request.request_id, 'staff_reminder_action', {}, false)
        if (!['SNOOZE', 'COMPLETE', 'CANCEL'].includes(action)) this.failScenario('reminder-validation')
        if (reminder.status === 'COMPLETED' || reminder.status === 'CANCELLED') return copy(reminder)
        if (action === 'SNOOZE') {
          const dueAt = input.due_at
          if (!validISODateTime(dueAt) || Date.parse(dueAt) <= Date.parse(reminder.due_at)) throw new C311ApiError(this.fixtures.errors['reminder-validation'], 422)
          const previousDueAt = reminder.due_at
          reminder.due_at = dueAt
          reminder.status = 'SNOOZED'
          reminder.history = [...(reminder.history || []), { action, previous_due_at: previousDueAt, due_at: dueAt, occurred_at: '2026-01-15T15:00:00.000Z' }]
        } else {
          reminder.status = action === 'COMPLETE' ? 'COMPLETED' : 'CANCELLED'
          reminder.completed_at = action === 'COMPLETE' ? '2026-01-15T15:00:00.000Z' : null
          reminder.completed_by = action === 'COMPLETE' ? this.currentSession.actor?.actor_id : undefined
          reminder.history = [...(reminder.history || []), { action, occurred_at: '2026-01-15T15:00:00.000Z' }]
        }
        this.countWrite('staff_reminder_action')
        return copy(reminder)
      }
    }
    throw new C311ApiError(this.fixtures.errors['not-found'], 404)
  }

  async overrideStaffOrigin (requestID: string, input: OriginOverride, options: C311RequestOptions = {}): Promise<StaffServiceRequestDetail> {
    this.failIfNeeded(['forbidden', 'not-found', 'validation', 'version-conflict', 'expected-version-required'])
    const detail = this.staffRequest(requestID, 'staff_origin_override', options)
    if (!input || !ORIGIN_CLASSES.includes(input.origin_class)) this.failScenario('validation')
    this.requireReason(input.reason)
    detail.request.origin_class = input.origin_class
    this.bumpStaffRequest(detail)
    this.auditStaffRequest(detail, 'ORIGIN_OVERRIDE')
    this.countWrite('staff_origin_override')
    return copy(detail)
  }

  async overrideStaffScope (requestID: string, input: ScopeOverride, options: C311RequestOptions = {}): Promise<StaffServiceRequestDetail> {
    this.failIfNeeded(['forbidden', 'not-found', 'validation', 'version-conflict', 'expected-version-required'])
    const detail = this.staffRequest(requestID, 'staff_scope_override', options)
    if (!input || !DEPARTMENT_CODES.includes(input.department_code) || !Array.isArray(input.district_codes) || !input.district_codes.length || input.district_codes.some(district => !DISTRICT_CODES.includes(district))) this.failScenario('validation')
    this.requireReason(input.reason)
    detail.request.owning_department = input.department_code
    detail.request.council_district = input.district_codes[0]
    this.bumpStaffRequest(detail)
    this.auditStaffRequest(detail, 'SCOPE_OVERRIDE')
    this.countWrite('staff_scope_override')
    return copy(detail)
  }

  async confirmStaffDuplicateGroup (requestID: string, input: DuplicateGroupChange, options: C311RequestOptions = {}): Promise<StaffServiceRequestDetail> {
    this.failIfNeeded(['forbidden', 'not-found', 'validation', 'version-conflict', 'expected-version-required'])
    const detail = this.staffRequest(requestID, 'staff_duplicate_group_confirm', options)
    if (!input?.duplicate_group_id?.trim()) this.failScenario('validation')
    this.requireReason(input.reason)
    detail.request.duplicate_group_id = input.duplicate_group_id
    this.bumpStaffRequest(detail)
    this.auditStaffRequest(detail, 'DUPLICATE_GROUP_CONFIRM')
    this.countWrite('staff_duplicate_group_confirm')
    return copy(detail)
  }

  async removeStaffDuplicateGroup (requestID: string, input: CollaboratorChange, options: C311RequestOptions = {}): Promise<StaffServiceRequestDetail> {
    this.failIfNeeded(['forbidden', 'not-found', 'validation', 'version-conflict', 'expected-version-required'])
    const detail = this.staffRequest(requestID, 'staff_duplicate_group_remove', options)
    this.requireReason(input?.reason)
    delete detail.request.duplicate_group_id
    this.bumpStaffRequest(detail)
    this.auditStaffRequest(detail, 'DUPLICATE_GROUP_REMOVE')
    this.countWrite('staff_duplicate_group_remove')
    return copy(detail)
  }

  async approveStaffReopen (requestID: string, input: CollaboratorChange, options: C311RequestOptions = {}): Promise<StaffServiceRequestDetail> {
    this.failIfNeeded(['forbidden', 'not-found', 'validation', 'version-conflict', 'expected-version-required'])
    const detail = this.staffRequest(requestID, 'staff_reopen_approve', options)
    this.requireReason(input?.reason)
    if (!['RESOLVED', 'CLOSED'].includes(detail.request.status)) throw new C311ApiError(this.fixtures.errors['invalid-status-transition'], 422)
    this.updateRequestStatus(requestID, 'REOPENED')
    detail.request.status = 'REOPENED'
    detail.available_actions = this.availableActionsFor('REOPENED')
    this.syncQueueItem(detail)
    this.auditStaffRequest(detail, 'REOPEN_APPROVE')
    this.countWrite('staff_reopen_approve')
    return copy(detail)
  }

  async bulkStaffRequests (input: BulkRequest, options: C311RequestOptions = {}): Promise<BulkResult> {
    this.failIfNeeded(['forbidden', 'validation', 'idempotency-conflict', 'bulk-validation'])
    this.requireStaffCapability('staff_request_bulk')
    const key = options.idempotencyKey
    if (!key) throw new C311ApiError(this.fixtures.errors['bulk-validation'], 422)
    if (!input || !['UPDATE', 'CLOSE'].includes(input.action) || !Array.isArray(input.request_items) || !input.request_items.length || new Set(input.request_items.map(item => item.request_id)).size !== input.request_items.length) {
      throw new C311ApiError(this.fixtures.errors['bulk-validation'], 422)
    }
    const fingerprint = this.fingerprint(input)
    const previous = this.bulkIdempotentResponses.get(key)
    if (previous) {
      if (previous.fingerprint !== fingerprint) throw new C311ApiError({ error: 'IDEMPOTENCY_CONFLICT', message: 'The idempotency key has already been used with different content.', retryable: false }, 409)
      return copy(previous.response)
    }
    const allowedChanges = new Set(['primary_assignee_id', 'priority', 'status', 'staff_note'])
    const changes = input.changes || {}
    if (Object.keys(changes).some(keyName => !allowedChanges.has(keyName)) || (changes.status !== undefined && !SERVICE_REQUEST_STATUSES.includes(changes.status)) || (input.action === 'CLOSE' && changes.status !== undefined) || (changes.staff_note !== undefined && (typeof changes.staff_note !== 'string' || !changes.staff_note.trim() || changes.staff_note.length > 2000))) {
      throw new C311ApiError({ ...this.fixtures.errors['bulk-validation'], failing_request_id: input.request_items[0].request_id }, 422)
    }
    const details = input.request_items.map(item => {
      try {
        const detail = this.staffRequest(item.request_id, 'staff_request_bulk', { expectedVersion: item.expected_version })
        this.request(item.request_id)
        return detail
      } catch (error) {
        return this.bulkError(error, item.request_id)
      }
    })
    const department = details[0].request.owning_department
    const duplicateGroup = details[0].request.duplicate_group_id || null
    const incompatible = details.find(detail => detail.request.owning_department !== department || (detail.request.duplicate_group_id || null) !== duplicateGroup)
    if (incompatible) {
      throw new C311ApiError({ ...this.fixtures.errors['bulk-validation'], failing_request_id: incompatible.request.request_id }, 422)
    }
    if ((this.scenario === 'bulk-version-conflict' || this.scenario === 'version-conflict') && this.consumeScenarioFailure('staff_request_bulk')) {
      const detail = details[0]
      this.bumpStaffRequest(detail)
      throw new C311ApiError({ ...this.fixtures.errors['bulk-version-conflict'], current_version: detail.request.version, failing_request_id: detail.request.request_id }, 409)
    }
    const invalidClose = input.action === 'CLOSE' ? details.find(detail => detail.request.status !== 'RESOLVED') : undefined
    if (invalidClose) {
      throw new C311ApiError({ ...this.fixtures.errors['bulk-validation'], failing_request_id: invalidClose.request.request_id }, 422)
    }
    const invalidTransition = changes.status !== undefined ? details.find(detail => !this.transitionTargets(detail.request.status).includes(changes.status!)) : undefined
    if (invalidTransition) {
      throw new C311ApiError({ ...this.fixtures.errors['invalid-status-transition'], failing_request_id: invalidTransition.request.request_id }, 422)
    }
    const snapshot = this.snapshotRequestState()
    const updatedRequestIds: string[] = []
    try {
      for (const detail of details) {
        const requestID = detail.request.request_id
        if (input.action === 'CLOSE') this.updateRequestStatus(requestID, 'CLOSED')
        if (changes.status !== undefined) this.updateRequestStatus(requestID, changes.status)
        if (changes.primary_assignee_id !== undefined) detail.primary_assignee_id = changes.primary_assignee_id
        if (changes.priority !== undefined) (detail.request as ServiceRequest & { priority?: string }).priority = changes.priority
        if (changes.staff_note !== undefined) this.appendStaffNote(requestID, changes.staff_note)
        detail.available_actions = this.availableActionsFor(detail.request.status)
        if (input.action !== 'CLOSE' && changes.status === undefined) this.bumpStaffRequest(detail)
        this.syncQueueItem(detail)
        this.auditStaffRequest(detail, `BULK_${input.action}`)
        updatedRequestIds.push(requestID)
      }
    } catch (error) {
      this.restoreRequestState(snapshot)
      const requestID = details[updatedRequestIds.length]?.request.request_id || input.request_items[0].request_id
      return this.bulkError(error, requestID)
    }
    const response = { updated_count: updatedRequestIds.length, updated_request_ids: updatedRequestIds }
    this.bulkIdempotentResponses.set(key, { fingerprint, response })
    this.countWrite('staff_request_bulk')
    return copy(response)
  }

  async processCivicWorksEvent (input: CivicWorksEvent, eventId: string, signature: string): Promise<CivicWorksEventResult> {
    if (this.scenario === 'civicworks-invalid-signature' || signature !== 'fixture-signature' || eventId !== input?.event_id) {
      throw new C311ApiError(this.fixtures.errors['civicworks-invalid-signature'], 401)
    }
    if (!input || input.event_type !== 'work_order.status_changed' || !input.work_order_id || !input.source_case_id || !CIVICWORKS_STATUSES.includes(input.previous_status) || !CIVICWORKS_STATUSES.includes(input.status) || !Number.isInteger(input.version) || input.version < 1 || !validISODateTime(input.occurred_at)) {
      throw new C311ApiError(this.fixtures.errors['bulk-validation'], 422)
    }
    const fingerprint = this.fingerprint(input)
    const existingEvent = this.civicWorksEvents.get(eventId)
    if (existingEvent) {
      if (existingEvent !== fingerprint) throw new C311ApiError(this.fixtures.errors['civicworks-duplicate'], 422)
      return { acknowledged: true, duplicate: true }
    }
    const detail = this.fixtures.details[input.source_case_id]
    if (!detail) throw new C311ApiError(this.fixtures.errors['not-found'], 404)
    const currentExternalVersion = (detail.external_work_order as CivicWorksWorkOrder | null)?.version || 0
    if (input.version <= currentExternalVersion) {
      this.civicWorksEvents.set(eventId, fingerprint)
      return { acknowledged: true }
    }
    const plans: Partial<Record<ServiceRequest['status'], Partial<Record<CivicWorksWorkOrder['status'], ServiceRequest['status'][]>>>> = {
      ASSIGNED: { ASSIGNED: [], IN_PROGRESS: ['IN_PROGRESS'], PARTIALLY_COMPLETED: ['IN_PROGRESS'], COMPLETED: ['IN_PROGRESS', 'RESOLVED'] },
      IN_PROGRESS: { ASSIGNED: [], IN_PROGRESS: [], PARTIALLY_COMPLETED: [], COMPLETED: ['RESOLVED'] },
      RESOLVED: { COMPLETED: [] },
      CLOSED: { COMPLETED: [] },
      REOPENED: { COMPLETED: [] },
    }
    const transitions = plans[detail.request.status]?.[input.status] || []
    const snapshot = this.snapshotRequestState()
    try {
      for (const status of transitions) {
        this.updateRequestStatus(input.source_case_id, status)
        this.auditStaffRequest(detail, `STATUS_${status}`)
      }
    } catch (error) {
      this.restoreRequestState(snapshot)
      throw error
    }
    detail.available_actions = this.availableActionsFor(detail.request.status)
    const currentWorkOrder = detail.external_work_order
    detail.external_work_order = {
      work_order_id: input.work_order_id,
      source_case_id: input.source_case_id,
      service_request_number: detail.request.request_number || input.source_case_id,
      status: input.status,
      external_status_url: currentWorkOrder?.external_status_url || `https://civicworks.fixture.invalid/ui/work-orders/${encodeURIComponent(input.work_order_id)}`,
      version: input.version,
      created_at: currentWorkOrder?.created_at || '2026-01-15T15:00:00.000Z',
      updated_at: input.occurred_at,
    }
    this.civicWorksEvents.set(eventId, fingerprint)
    this.auditStaffRequest(detail, 'CIVICWORKS_STATUS_CHANGED')
    this.countWrite('civicworks_event_callback')
    return { acknowledged: true }
  }

  async listReports (query: ListQuery = {}): Promise<PageResponse<ReportDefinition>> {
    this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    return this.page(this.scenario === 'empty' ? [] : this.fixtures.reports, query)
  }

  async getReport (reportID: string): Promise<ReportDefinition> {
    this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    const report = this.fixtures.reports.find(item => item.report_id === reportID)
    if (!report) throw new C311ApiError(this.fixtures.errors['not-found'], 404)
    return copy(report)
  }

  async createReport (input: ReportDefinition): Promise<ReportDefinition> {
    this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    return copy(input)
  }

  async updateReport (reportID: string, input: ReportDefinition, _options: C311RequestOptions = {}): Promise<ReportDefinition> {
    this.failIfNeeded(['forbidden', 'not-found', 'validation', 'version-conflict'])
    const report = await this.getReport(reportID)
    return copy({ ...report, ...input, report_id: reportID })
  }

  async runReport (_input: { definition: ReportDefinition }): Promise<Operation> {
    this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    return {
      operation_id: 'operation-fixture-report',
      kind: 'report_run',
      status: 'PENDING',
      created_at: '2026-01-15T15:00:00.000Z',
      updated_at: '2026-01-15T15:00:00.000Z',
    }
  }

  async exportReport (_reportID: string, _options: ReportExportOptions = {}): Promise<Operation> {
    this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    return {
      operation_id: 'operation-fixture-export',
      kind: 'report_export',
      status: 'PENDING',
      created_at: '2026-01-15T15:00:00.000Z',
      updated_at: '2026-01-15T15:00:00.000Z',
    }
  }

  async listWorkflows (query: ListQuery = {}): Promise<PageResponse<WorkflowDefinition>> {
    this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    return this.page(this.scenario === 'empty' ? [] : this.fixtures.workflows, query)
  }

  async getWorkflow (workflowID: string): Promise<WorkflowDefinition> {
    this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    const workflow = this.fixtures.workflows.find(item => item.workflow_id === workflowID)
    if (!workflow) throw new C311ApiError(this.fixtures.errors['not-found'], 404)
    return copy(workflow)
  }

  async createWorkflow (input: WorkflowDefinition): Promise<WorkflowDefinition> {
    this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    return copy(input)
  }

  async updateWorkflow (workflowID: string, input: WorkflowDefinition, _options: C311RequestOptions = {}): Promise<WorkflowDefinition> {
    this.failIfNeeded(['forbidden', 'not-found', 'validation', 'version-conflict'])
    const workflow = await this.getWorkflow(workflowID)
    return copy({ ...workflow, ...input, workflow_id: workflowID })
  }
}
