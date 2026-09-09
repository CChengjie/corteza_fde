import { C311ApiError } from './errors'
import { APPLICATION_ROLES, AUDIT_ACTOR_TYPES, CIVICWORKS_STATUSES, CONTACT_CATEGORIES, CUSTOM_FIELD_TYPES, C311_SCENARIOS, DEPARTMENT_CODES, DISTRICT_CODES, LANGUAGES, ORIGIN_CLASSES, PHONE_LABELS, RELATIONSHIP_TYPES, REMINDER_CHANNELS, SERVICE_REQUEST_STATUSES, SERVICE_TYPES, SOURCE_CHANNELS, type ApplicationRole, type C311Scenario, type ContractCapability, type HelpKey, type IdentityProvider, type Language, type PublicContentKey } from './enums'
import { BENCHMARK_NOW, cloneFixtureSet, createDefaultFixtureSet } from './fixtures'
import type {
  AccountRegistration,
  AccountRegistrationAcknowledgement,
  AccountDispositionRequest,
  AccountDispositionResult,
  AnonymousStatusLookupRequest,
  AnonymousStatusLookupResponse,
  BinaryAttachment,
  Branding,
  BrandingWrite,
  Category,
  CategoryWrite,
  C311FixtureSet,
  ContentObject,
  ContentWrite,
  CustomFieldDefinition,
  DraftWrite,
  GeocodeRequest,
  GeocodeResponse,
  FederatedRedirect,
  HelpContent,
  HelpWrite,
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
  RollbackInput,
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
  WorkflowTestInput,
  WorkflowExecution,
  CalendarExport,
  CalendarImport,
  MailCompose,
  MailPreview,
  MailDelivery,
  MailTemplate,
  ReportCatalogueItem,
  ReportShare,
  AuditEvent,
  FollowUpAction,
  AuditFilters,
  ExportResponse,
  WorkflowActionRequest,
  WorkflowActionAccepted,
  DataExportQuery,
  ContactEmailExportRequest,
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
  'rate-limited': 429,
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
  'workflow-invalid-client': 401,
  'workflow-invalid-token': 401,
  'workflow-insufficient-scope': 403,
  'invalid-client': 401,
  'invalid-token': 401,
  'insufficient-scope': 403,
}

interface MockCalendarEvent {
  uid: string
  summary: string
  description?: string
  dtstart?: string
  dtend?: string
  rrule?: string
  last_modified?: string
  timezone?: string
  cancelled: boolean
  updated_at: string
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

function validReportDateFilter (value: unknown): value is string {
  if (typeof value !== 'string') return false
  if (/^\d{4}-\d{2}-\d{2}$/.test(value)) {
    const parsed = Date.parse(`${value}T00:00:00.000Z`)
    return Number.isFinite(parsed) && new Date(parsed).toISOString().slice(0, 10) === value
  }
  return validISODateTime(value)
}

function reportDateBoundary (value: string, endOfDay: boolean): number {
  const normalized = /^\d{4}-\d{2}-\d{2}$/.test(value)
    ? `${value}T${endOfDay ? '23:59:59.999' : '00:00:00.000'}Z`
    : value
  return Date.parse(normalized)
}

function unfoldCalendarLines (ics: string): string[] {
  const lines = String(ics || '').replace(/\r\n/g, '\n').replace(/\r/g, '\n').split('\n')
  const unfolded: string[] = []
  lines.forEach(line => {
    if (/^[ \t]/.test(line) && unfolded.length) unfolded[unfolded.length - 1] += line.slice(1)
    else if (line) unfolded.push(line)
  })
  return unfolded
}

function unescapeCalendarText (value: string): string {
  return value.replace(/\\n/gi, '\n').replace(/\\([\\;,])/g, '$1')
}

const MAIL_ALLOWED_TAGS = new Set(['p', 'br', 'strong', 'em', 'ul', 'ol', 'li', 'a', 'table', 'thead', 'tbody', 'tr', 'th', 'td'])
const MAIL_BLOCKED_TAGS = new Set(['script', 'style', 'iframe', 'object', 'embed', 'form', 'svg', 'link', 'meta'])

function isHTMLWhitespace (value: string | undefined): boolean {
  return value === ' ' || value === '\t' || value === '\n' || value === '\r' || value === '\f'
}

function readMailHref (content: string, nameEnd: number): string {
  let cursor = nameEnd
  while (cursor < content.length) {
    while (cursor < content.length && (isHTMLWhitespace(content[cursor]) || content[cursor] === '/')) cursor += 1
    if (cursor >= content.length) break

    const attributeStart = cursor
    while (cursor < content.length && !isHTMLWhitespace(content[cursor]) && content[cursor] !== '=' && content[cursor] !== '/') cursor += 1
    const attributeName = content.slice(attributeStart, cursor).toLowerCase()
    while (cursor < content.length && isHTMLWhitespace(content[cursor])) cursor += 1
    if (content[cursor] !== '=') {
      while (cursor < content.length && !isHTMLWhitespace(content[cursor])) cursor += 1
      continue
    }

    cursor += 1
    while (cursor < content.length && isHTMLWhitespace(content[cursor])) cursor += 1
    const quote = content[cursor] === '"' || content[cursor] === "'" ? content[cursor] : ''
    if (quote) cursor += 1
    const valueStart = cursor
    if (quote) {
      const valueEnd = content.indexOf(quote, cursor)
      cursor = valueEnd < 0 ? content.length : valueEnd + 1
      if (attributeName === 'href') return content.slice(valueStart, valueEnd < 0 ? content.length : valueEnd)
    } else {
      while (cursor < content.length && !isHTMLWhitespace(content[cursor])) cursor += 1
      if (attributeName === 'href') return content.slice(valueStart, cursor)
    }
  }
  return ''
}

function sanitizeMailHtml (value: string): string {
  const html = String(value || '')
  const lowerHTML = html.toLowerCase()
  let sanitized = ''
  let cursor = 0
  while (cursor < html.length) {
    if (html.startsWith('<!--', cursor)) {
      const commentEnd = html.indexOf('-->', cursor + 4)
      cursor = commentEnd < 0 ? html.length : commentEnd + 3
      continue
    }
    const start = html.indexOf('<', cursor)
    if (start < 0) {
      sanitized += html.slice(cursor)
      break
    }
    sanitized += html.slice(cursor, start)
    const end = html.indexOf('>', start + 1)
    if (end < 0) break
    const tag = html.slice(start, end + 1)
    const closing = tag.startsWith('</')
    const content = tag.slice(closing ? 2 : 1, -1).trim()
    let nameEnd = 0
    while (nameEnd < content.length && !isHTMLWhitespace(content[nameEnd]) && content[nameEnd] !== '/') nameEnd += 1
    const name = content.slice(0, nameEnd).toLowerCase()
    if (MAIL_BLOCKED_TAGS.has(name)) {
      if (!closing) {
        const marker = `</${name}`
        let closingStart = lowerHTML.indexOf(marker, end + 1)
        while (closingStart >= 0) {
          const boundary = lowerHTML[closingStart + marker.length]
          if (boundary === '>' || boundary === '/' || isHTMLWhitespace(boundary)) {
            const closingEnd = html.indexOf('>', closingStart + marker.length)
            cursor = closingEnd < 0 ? html.length : closingEnd + 1
            break
          }
          closingStart = lowerHTML.indexOf(marker, closingStart + marker.length)
        }
        if (closingStart < 0) cursor = html.length
      } else cursor = end + 1
      continue
    }
    if (MAIL_ALLOWED_TAGS.has(name)) {
      if (closing) sanitized += `</${name}>`
      else if (name === 'br') sanitized += '<br>'
      else if (name !== 'a') sanitized += `<${name}>`
      else {
        const href = readMailHref(content, nameEnd)
        const normalizedHref = href.trim().toLowerCase()
        sanitized += (normalizedHref.startsWith('http:') || normalizedHref.startsWith('https:') || normalizedHref.startsWith('mailto:'))
          ? `<a href="${href.replace(/&/g, '&amp;').replace(/"/g, '&quot;')}">`
          : '<a>'
      }
    }
    cursor = end + 1
  }
  return sanitized
}

const MAIL_ATTACHMENT_MAX_BYTES = 5 * 1024 * 1024

function validMailAddress (value: unknown): boolean {
  if (typeof value !== 'string') return false
  const address = value.trim()
  const at = address.indexOf('@')
  if (at <= 0 || at === address.length - 1 || address.slice(at + 1).includes('@') || /\s/.test(address)) return false
  const domain = address.slice(at + 1)
  const dot = domain.indexOf('.')
  return dot > 0 && dot < domain.length - 1
}

function parseCalendarDate (value: string, timezone?: string, requireUTC = false): string {
  const raw = value.trim()
  const match = raw.match(/^(\d{4})(\d{2})(\d{2})T(\d{2})(\d{2})(\d{2})(Z)?$/)
  if (!match) throw new Error('Invalid calendar date.')
  const [, yearText, monthText, dayText, hourText, minuteText, secondText, utc] = match
  const year = Number(yearText); const month = Number(monthText); const day = Number(dayText)
  const hour = Number(hourText); const minute = Number(minuteText); const second = Number(secondText)
  const instant = new Date(Date.UTC(year, month - 1, day, hour, minute, second))
  if (instant.getUTCFullYear() !== year || instant.getUTCMonth() !== month - 1 || instant.getUTCDate() !== day || instant.getUTCHours() !== hour || instant.getUTCMinutes() !== minute || instant.getUTCSeconds() !== second) throw new Error('Invalid calendar date.')
  const isUTC = !!utc
  if (timezone && isUTC) throw new Error('TZID values must use local calendar times.')
  if (requireUTC && !isUTC) throw new Error('UTC calendar date is required.')
  if (!timezone && !isUTC) throw new Error('Local calendar times require TZID.')
  if (timezone !== undefined && timezone !== 'America/New_York') throw new Error('Unsupported calendar timezone.')
  return raw
}

function parseCalendarEvents (ics: string): Array<MockCalendarEvent> {
  const lines = unfoldCalendarLines(ics)
  if (lines[0] !== 'BEGIN:VCALENDAR' || !lines.includes('END:VCALENDAR')) throw new Error('Calendar envelope is invalid.')
  const events: Array<MockCalendarEvent> = []
  let properties: Record<string, { value: string, params: Record<string, string> }> | null = null
  const parseEvent = (eventProperties: Record<string, { value: string, params: Record<string, string> }>): void => {
    const required = ['UID', 'SUMMARY', 'DTSTART', 'DTEND', 'DESCRIPTION', 'STATUS', 'LAST-MODIFIED']
    if (required.some(name => !eventProperties[name])) throw new Error('Calendar event is missing a required property.')
    const uid = eventProperties.UID?.value.trim() || ''
    const status = eventProperties.STATUS?.value.trim().toUpperCase() || ''
    const startTimezone = eventProperties.DTSTART?.params.TZID
    const endTimezone = eventProperties.DTEND?.params.TZID
    if (!!startTimezone !== !!endTimezone || (startTimezone && endTimezone && startTimezone !== endTimezone)) throw new Error('Calendar event timezones must match.')
    const timezone = startTimezone || endTimezone
    if (!uid || !eventProperties.SUMMARY?.value.trim() || !['CONFIRMED', 'TENTATIVE', 'CANCELLED'].includes(status)) throw new Error('Calendar event identity or status is invalid.')
    if (eventProperties.RRULE && (eventProperties.RRULE.value.length > 1024 || /[\r\n]/.test(eventProperties.RRULE.value))) throw new Error('Calendar recurrence rule is invalid.')
    const dtstart = parseCalendarDate(eventProperties.DTSTART.value, eventProperties.DTSTART.params.TZID)
    const dtend = parseCalendarDate(eventProperties.DTEND.value, eventProperties.DTEND.params.TZID)
    const comparable = (value: string): number => Number(value.replace('T', '').replace('Z', ''))
    if (comparable(dtend) <= comparable(dtstart)) throw new Error('Calendar event end must be after start.')
    events.push({
      uid,
      summary: unescapeCalendarText(eventProperties.SUMMARY?.value.trim() || ''),
      cancelled: status === 'CANCELLED',
      updated_at: '2026-01-15T15:00:00.000Z',
      description: unescapeCalendarText(eventProperties.DESCRIPTION.value),
      dtstart,
      dtend,
      ...(eventProperties.RRULE ? { rrule: eventProperties.RRULE.value.trim() } : {}),
      last_modified: parseCalendarDate(eventProperties['LAST-MODIFIED'].value, undefined, true),
      ...(timezone ? { timezone } : {}),
    })
  }
  for (const line of lines) {
    if (line === 'BEGIN:VEVENT') {
      if (properties) throw new Error('Calendar event envelope is invalid.')
      properties = {}
      continue
    }
    if (line === 'END:VEVENT') {
      if (!properties) throw new Error('Calendar event envelope is invalid.')
      parseEvent(properties)
      properties = null
      continue
    }
    if (!properties) continue
    const separator = line.indexOf(':')
    if (separator < 1) continue
    const nameAndParams = line.slice(0, separator).split(';')
    const name = nameAndParams.shift()!.toUpperCase()
    const params: Record<string, string> = {}
    nameAndParams.forEach(part => {
      const equal = part.indexOf('=')
      if (equal > 0) params[part.slice(0, equal).toUpperCase()] = part.slice(equal + 1)
    })
    properties[name] = { value: line.slice(separator + 1), params }
  }
  if (properties) throw new Error('Calendar event envelope is invalid.')
  return events
}

const REPORT_CATALOGUE: ReportCatalogueItem[] = [
  { report_key: 'service_requests', name: 'Request volume', supported_filters: ['status', 'service_type', 'owning_department', 'updated_at', 'created_from', 'created_to', 'primary_assignee_id', 'resolved_at'], supported_grouping: ['status', 'service_type', 'owning_department', 'primary_assignee_id'], supported_sort: ['request_number', 'created_at', 'updated_at', 'status', 'primary_assignee_id', 'resolved_at'] },
  { report_key: 'request_status_age', name: 'Request status and age', supported_filters: ['status', 'created_at', 'updated_at', 'created_from', 'created_to'], supported_grouping: ['status'], supported_sort: ['created_at', 'updated_at', 'status'] },
  { report_key: 'assignment_workload', name: 'Assignment workload', supported_filters: ['primary_assignee_id', 'owning_department', 'status', 'created_from', 'created_to'], supported_grouping: ['primary_assignee_id', 'owning_department'], supported_sort: ['primary_assignee_id', 'updated_at', 'status'] },
  { report_key: 'resolution_performance', name: 'Resolution performance', supported_filters: ['status', 'resolved_at', 'owning_department', 'created_from', 'created_to'], supported_grouping: ['owning_department'], supported_sort: ['resolved_at', 'updated_at', 'owning_department'] },
  { report_key: 'follow_up_actions', name: 'Follow-up activity', supported_filters: ['request_id', 'action_type', 'occurred_at', 'created_from', 'created_to'], supported_grouping: ['action_type'], supported_sort: ['occurred_at', 'request_id'] },
]

const REPORT_ENTITY_RULES: Record<ReportDefinition['entity'], Pick<ReportCatalogueItem, 'supported_filters' | 'supported_grouping' | 'supported_sort'>> = {
  service_requests: REPORT_CATALOGUE[0],
  constituents: { supported_filters: ['constituent_id', 'email', 'primary_category', 'updated_at', 'created_from', 'created_to'], supported_grouping: ['primary_category'], supported_sort: ['constituent_id', 'updated_at'] },
  follow_up_actions: REPORT_CATALOGUE[4],
}

function workflowActionKind (action: Record<string, unknown>): string {
  const raw = String(action.type || action.kind || action.action || '').trim().toUpperCase().replace(/[- ]/g, '_')
  if (['HTTP', 'OAUTH_HTTP', 'AUTHENTICATED_HTTP_ACTION'].includes(raw)) return 'AUTHENTICATED_HTTP'
  if (['ASSIGN', 'ASSIGNEE', 'ASSIGNMENT'].includes(raw)) return 'ASSIGNMENT'
  if (['NOTIFY', 'EMAIL', 'NOTIFICATION'].includes(raw)) return 'NOTIFICATION'
  if (['FIELD_UPDATE', 'UPDATE_FIELD'].includes(raw)) return 'FIELD_UPDATE'
  return raw
}

function assertWorkflowDefinition (input: WorkflowDefinition, creating: boolean): void {
  const errors: Array<{ field: string, code: 'REQUIRED' | 'INVALID_VALUE' | 'TOO_MANY_ITEMS' }> = []
  if (!input || typeof input !== 'object' || Array.isArray(input)) throw new C311ApiError({ error: 'VALIDATION_ERROR', message: 'The workflow definition is invalid.', retryable: false, errors: [{ field: '/', code: 'INVALID_VALUE' }] }, 422)
  if (!/^[A-Za-z][A-Za-z0-9._-]{0,63}$/.test(String(input.workflow_id || '').trim())) errors.push({ field: '/workflow_id', code: 'INVALID_VALUE' })
  const name = String(input.name || '').trim()
  if (!name || name.length > 120) errors.push({ field: '/name', code: name ? 'INVALID_VALUE' : 'REQUIRED' })
  if (!['SERVICE_REQUEST_CREATED', 'SERVICE_REQUEST_STATUS_CHANGED'].includes(input.trigger)) errors.push({ field: '/trigger', code: 'INVALID_VALUE' })
  if (typeof input.active !== 'boolean') errors.push({ field: '/active', code: 'INVALID_VALUE' })
  if (creating && input.active) errors.push({ field: '/active', code: 'INVALID_VALUE' })
  if (!Number.isInteger(input.version) || input.version < 1) errors.push({ field: '/version', code: 'INVALID_VALUE' })
  if (!validISODateTime(input.updated_at)) errors.push({ field: '/updated_at', code: 'INVALID_VALUE' })
  if (!Array.isArray(input.conditions) || input.conditions.length > 20) errors.push({ field: '/conditions', code: Array.isArray(input.conditions) ? 'TOO_MANY_ITEMS' : 'INVALID_VALUE' })
  if (!Array.isArray(input.actions) || input.actions.length < 1) errors.push({ field: '/actions', code: 'REQUIRED' })
  else if (input.actions.length > 20) errors.push({ field: '/actions', code: 'TOO_MANY_ITEMS' })
  ;(input.conditions || []).forEach((condition, index) => {
    if (!condition || typeof condition !== 'object' || Array.isArray(condition)) {
      errors.push({ field: `/conditions/${index}`, code: 'INVALID_VALUE' })
      return
    }
    const kind = String(condition.type || '').trim().toUpperCase()
    const conditionField = String(condition.field || (condition.status !== undefined ? 'status' : '')).trim()
    const valid = kind === 'ACTOR_ROLE' ? APPLICATION_ROLES.includes(String(condition.actor_role || condition.value) as ApplicationRole) : !!conditionField && ['EQ', 'EQUALS', 'NE', 'NOT_EQUALS', 'IN', 'EXISTS', ''].includes(String(condition.operator || '').toUpperCase())
    if (!valid) errors.push({ field: `/conditions/${index}`, code: 'INVALID_VALUE' })
  })
  ;(input.actions || []).forEach((action, index) => {
    if (!action || typeof action !== 'object' || Array.isArray(action)) {
      errors.push({ field: `/actions/${index}`, code: 'INVALID_VALUE' })
      return
    }
    const kind = workflowActionKind(action)
    const valid = kind === 'FIELD_UPDATE' ? !!String(action.field || '').trim() && action.value !== undefined
      : kind === 'ASSIGNMENT' ? !!String(action.assignee_id || '').trim()
        : ['NOTIFICATION', 'NOTIFY', 'EMAIL'].includes(kind) ? !!String(action.type || action.kind || action.action || '').trim()
          : kind === 'AUTHENTICATED_HTTP' ? !!String(action.action || '').trim() && !!action.payload && typeof action.payload === 'object' && !Array.isArray(action.payload)
            : false
    if (!valid) errors.push({ field: `/actions/${index}`, code: 'INVALID_VALUE' })
  })
  if (errors.length) throw new C311ApiError({ error: 'VALIDATION_ERROR', message: 'The workflow definition is invalid.', retryable: false, errors }, 422)
}

function isSafeSanitizedHTML (value: string): boolean {
  return !/<\s*(script|iframe|object|embed|style|svg|math)\b/i.test(value) &&
    !/\son[a-z]+\s*=/i.test(value) &&
    !/(?:javascript|data)\s*:/i.test(value)
}

function validCustomFieldDefinition (input: CustomFieldDefinition, currentKey?: string): boolean {
  const choices = input.choice_values || []
  const choiceType = input.field_type === 'SINGLE_CHOICE' || input.field_type === 'MULTI_CHOICE'
  const validChoiceDefault = input.default === undefined || (input.field_type === 'SINGLE_CHOICE'
    ? typeof input.default === 'string' && choices.includes(input.default)
    : input.field_type === 'MULTI_CHOICE'
      ? Array.isArray(input.default) && input.default.every(value => typeof value === 'string' && choices.includes(value))
      : true)
  return /^[a-z][a-z0-9_.-]*$/.test(input.key) &&
    (!currentKey || input.key === currentKey) &&
    !!input.labels?.EN?.trim() &&
    CUSTOM_FIELD_TYPES.includes(input.field_type) &&
    (!choiceType || (choices.length > 0 && new Set(choices).size === choices.length)) &&
    validChoiceDefault
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
  private readonly extensionIdempotentResponses = new Map<string, { fingerprint: string; response: MailDelivery | WorkflowActionAccepted }>()
  private readonly bulkIdempotentResponses = new Map<string, { fingerprint: string; response: BulkResult }>()
  private readonly civicWorksEvents = new Map<string, string>()
  private readonly calendarEvents: MockCalendarEvent[]
  private readonly mailDeliveries: Record<string, MailDelivery>
  private readonly mailTemplates: Record<string, MailTemplate>
  private readonly operations = new Map<string, Operation>()
  private readonly reportShares: Record<string, ApplicationRole[]>
  private readonly reportOwners: Record<string, string>
  private readonly exportPageTokens = new Map<string, { fingerprint: string, offset: number }>()
  private readonly auditPageTokens = new Map<string, { fingerprint: string, offset: number }>()
  private exportPageTokenSerial = 0
  private auditPageTokenSerial = 0
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
  private branding: Branding
  private publishedBranding: Branding
  private readonly brandingVersions: Branding[]
  private readonly adminContent: Record<string, ContentObject>
  private readonly adminContentVersions: Record<string, ContentObject[]>
  private readonly adminCategories: Category[]
  private readonly adminCustomFields: CustomFieldDefinition[]
  private readonly adminHelp: Record<string, HelpContent>
  private readonly adminHelpVersions: Record<string, HelpContent[]>

  private extensionStorage (): Storage | undefined {
    if (typeof window === 'undefined' || (window as Window & { C311Mode?: string }).C311Mode !== 'mock') return undefined
    try { return window.sessionStorage } catch (_error) { return undefined }
  }

  private restoreExtensionState (): void {
    const storage = this.extensionStorage()
    if (!storage) return
    try {
      const state = JSON.parse(storage.getItem('c311:mock:fe09-extension-state') || '{}') as { calendarEvents?: MockCalendarEvent[], mailDeliveries?: Record<string, MailDelivery>, mailTemplates?: Record<string, MailTemplate> }
      if (Array.isArray(state.calendarEvents)) this.calendarEvents.splice(0, this.calendarEvents.length, ...copy(state.calendarEvents))
      if (state.mailDeliveries && typeof state.mailDeliveries === 'object') Object.assign(this.mailDeliveries, copy(state.mailDeliveries))
      if (state.mailTemplates && typeof state.mailTemplates === 'object') Object.assign(this.mailTemplates, copy(state.mailTemplates))
    } catch (_error) {
      // Ignore malformed optional browser state and use fixture defaults.
      return
    }
  }

  private persistExtensionState (): void {
    const storage = this.extensionStorage()
    if (!storage) return
    storage.setItem('c311:mock:fe09-extension-state', JSON.stringify({ calendarEvents: this.calendarEvents, mailDeliveries: this.mailDeliveries, mailTemplates: this.mailTemplates }))
  }

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
    this.branding = copy(this.fixtures.branding || createDefaultFixtureSet().branding!)
    this.publishedBranding = copy(this.branding)
    this.brandingVersions = [copy(this.branding)]
    this.adminContent = copy(this.fixtures.public_content || {})
    this.adminContentVersions = Object.fromEntries(Object.entries(this.adminContent).map(([key, value]) => [key, [copy(value)]]))
    this.adminCategories = copy(this.fixtures.categories || [])
    this.adminCustomFields = copy(this.fixtures.custom_fields || [])
    this.adminHelp = {}
    this.adminHelpVersions = {}
    Object.values(this.fixtures.public_help || {}).forEach(item => LANGUAGES.forEach(language => {
      const value = { ...copy(item), language, state: 'PUBLISHED' as const, published: true }
      this.adminHelp[`${item.help_key}:${language}`] = value
      this.adminHelpVersions[`${item.help_key}:${language}`] = [copy(value)]
    }))
    this.relationships = copy(this.fixtures.relationships || {})
    this.notes = copy(this.fixtures.notes || {})
    this.publicRelationships = copy(this.fixtures.public_relationships || this.relationships)
    this.publicNotes = copy(this.fixtures.public_notes || this.notes)
    this.calendarEvents = [{ uid: 'fixture-calendar-001', summary: 'Imported event', description: 'A fixture calendar event.', dtstart: '20260115T100000', dtend: '20260115T110000', rrule: 'FREQ=DAILY;COUNT=2', last_modified: '20260115T090000Z', timezone: 'America/New_York', cancelled: false, updated_at: '2026-01-15T15:00:00.000Z' }]
    const deliveryFixtures: MailDelivery[] = copy(this.fixtures.mail_deliveries || [{ delivery_id: 'delivery-fixture-001', status: 'DELIVERED' as const, attempts: 1, updated_at: '2026-01-15T15:00:00.000Z', error: null }])
    this.mailDeliveries = Object.fromEntries(deliveryFixtures.map(item => [item.delivery_id, item]))
    const templateFixtures = copy(this.fixtures.mail_templates || [])
    this.mailTemplates = Object.fromEntries(templateFixtures.map(item => [item.template_id, item]))
    this.reportShares = copy(this.fixtures.report_shares || {})
    this.reportOwners = copy(this.fixtures.report_owners || {})
    this.restoreExtensionState()
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
        this.fixtures.requests.push(copy(foreignDetail.request))
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

  private createOperation (operationID: string, kind: string, result: Record<string, unknown>): Operation {
    const operation: Operation = {
      operation_id: operationID,
      kind,
      status: 'SUCCEEDED',
      progress: 100,
      result: copy(result),
      error: null,
      created_at: '2026-01-15T15:00:00.000Z',
      updated_at: '2026-01-15T15:00:00.000Z',
      completed_at: '2026-01-15T15:00:00.000Z',
    }
    this.operations.set(operationID, operation)
    return copy(operation)
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

  private failIfNeeded (supported: readonly C311Scenario[] = [], retryAfter = '30'): void {
    if (this.scenario === 'success' || this.scenario === 'empty') return
    if (!supported.includes(this.scenario)) return

    const payload = this.fixtures.errors[this.scenario]
    if (payload) {
      const headers = this.scenario === 'retryable' || this.scenario === 'rate-limited' ? { 'Retry-After': retryAfter } : undefined
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

  private requireVersion (options: C311RequestOptions, current: number): void {
    if (options.expectedVersion === undefined) throw new C311ApiError(this.fixtures.errors['expected-version-required'], 428)
    if (options.expectedVersion !== current) throw new C311ApiError({ ...this.fixtures.errors['version-conflict'], current_version: current }, 409)
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

  private isPlatformAdministrator (): boolean {
    return !!this.currentSession.actor?.application_roles.includes('platform_administrator')
  }

  private requestInScope (request: ServiceRequest | undefined): boolean {
    if (!request) return false
    if (this.isPlatformAdministrator()) return true
    const actor = this.currentSession.actor
    return !!actor && actor.department_codes.includes(request.owning_department) && (!request.council_district || actor.district_codes.includes(request.council_district))
  }

  private requestForAudit (event: AuditEvent): ServiceRequest | undefined {
    const requestID = String(event.after?.request_id || event.before?.request_id || (event.entity_type === 'service_request' ? event.entity_id : '') || '')
    if (!requestID) return undefined
    return this.fixtures.requests.find(request => request.request_id === requestID) || this.fixtures.details[requestID]?.request
  }

  private auditInScope (event: AuditEvent): boolean {
    return this.isPlatformAdministrator() || this.requestInScope(this.requestForAudit(event))
  }

  private verifiedEmails (constituent: Constituent): string[] {
    const configured = this.fixtures.verified_emails?.[constituent.constituent_id]
    if (!configured) return []
    const declared = new Set((constituent.emails || []).map(email => email.trim().toLowerCase()))
    return configured.map(email => email.trim().toLowerCase()).filter(email => declared.has(email))
  }

  private csvCell (value: unknown): string {
    return `"${String(value ?? '').replace(/"/g, '""').replace(/\r\n|\r|\n/g, '\r\n')}"`
  }

  private matchesFilter (candidate: unknown, expected: unknown): boolean {
    if (Array.isArray(expected)) {
      const expectedValues = expected.map(value => String(value))
      if (Array.isArray(candidate)) return candidate.some(value => expectedValues.includes(String(value)))
      return expectedValues.includes(String(candidate ?? ''))
    }
    if (Array.isArray(candidate)) return candidate.map(value => String(value)).includes(String(expected))
    return String(candidate ?? '') === String(expected)
  }

  private validationError (message: string, errors: Array<{ field: string, code: 'REQUIRED' | 'INVALID_FORMAT' | 'INVALID_VALUE' | 'OUT_OF_RANGE' | 'TOO_MANY_ITEMS' | 'DUPLICATE' }>): never {
    throw new C311ApiError({ error: 'VALIDATION_ERROR', message, retryable: false, errors }, 422)
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

  private failVersionConflictOnce (operation: string, currentVersion: number): void {
    if (this.scenario !== 'version-conflict' || !this.consumeScenarioFailure(operation)) return
    throw new C311ApiError({
      ...(this.fixtures.errors['version-conflict'] || { error: 'VERSION_CONFLICT', message: 'The resource changed before your update.', retryable: false }),
      current_version: currentVersion,
    }, 409)
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
    return copy(this.publishedBranding)
  }

  async updateBranding (input: BrandingWrite, options: C311RequestOptions = {}): Promise<Branding> {
    this.requireCapability('admin_branding_update'); this.requireVersion(options, this.branding.version); this.failIfNeeded(['validation', 'version-conflict'])
    this.branding = { ...this.branding, ...input, published: false, version: this.branding.version + 1, updated_at: BENCHMARK_NOW }; this.brandingVersions.push(copy(this.branding)); return copy(this.branding)
  }
  async getAdminBranding (): Promise<Branding> { this.requireCapability('admin_branding_get'); return copy(this.branding) }
  async previewBranding (input: BrandingWrite): Promise<Branding> { this.requireCapability('admin_branding_preview'); return { ...copy(this.branding), ...input, published: false } }
  async publishBranding (options: C311RequestOptions = {}): Promise<Branding> { this.requireCapability('admin_branding_publish'); this.requireVersion(options, this.branding.version); this.branding = { ...this.branding, published: true, version: this.branding.version + 1, updated_at: BENCHMARK_NOW }; this.publishedBranding = copy(this.branding); this.brandingVersions.push(copy(this.branding)); return copy(this.branding) }
  async listBrandingVersions (_query: ListQuery = {}): Promise<PageResponse<Branding>> { this.requireCapability('admin_branding_versions'); return { items: copy(this.brandingVersions), next_page_token: null, total_count: this.brandingVersions.length, applied_filters: {}, sort: [] } }
  async rollbackBranding (input: RollbackInput, options: C311RequestOptions = {}): Promise<Branding> { this.requireCapability('admin_branding_rollback'); this.requireVersion(options, this.branding.version); const target = this.brandingVersions.find(item => item.version === input.target_version); if (!target) throw new C311ApiError(this.fixtures.errors['not-found'], 404); this.branding = { ...copy(target), version: this.branding.version + 1, published: true, updated_at: BENCHMARK_NOW }; this.publishedBranding = copy(this.branding); this.brandingVersions.push(copy(this.branding)); return copy(this.branding) }
  async getAdminContent (contentKey: PublicContentKey): Promise<ContentObject> { this.requireCapability('admin_content_get'); const item = this.adminContent[contentKey]; if (!item) throw new C311ApiError(this.fixtures.errors['not-found'], 404); return copy(item) }
  async listAdminContent (_query: ListQuery = {}): Promise<PageResponse<ContentObject>> { this.requireCapability('admin_content_list'); const items = Object.values(this.adminContent); return { items: copy(items), next_page_token: null, total_count: items.length, applied_filters: {}, sort: [] } }
  async updateAdminContent (contentKey: PublicContentKey, input: ContentWrite, options: C311RequestOptions = {}): Promise<ContentObject> { this.requireCapability('admin_content_update'); const item = this.adminContent[contentKey]; if (!item) throw new C311ApiError(this.fixtures.errors['not-found'], 404); this.requireVersion(options, item.version); if (!isSafeSanitizedHTML(input.body)) throw new C311ApiError(this.fixtures.errors.validation, 422); this.adminContent[contentKey] = { ...item, ...input, sanitized: true, state: 'DRAFT', published: false, version: item.version + 1, updated_at: BENCHMARK_NOW }; this.adminContentVersions[contentKey].push(copy(this.adminContent[contentKey])); return copy(this.adminContent[contentKey]) }
  async previewAdminContent (contentKey: PublicContentKey, input: ContentWrite): Promise<ContentObject> { this.requireCapability('admin_content_preview'); const item = this.adminContent[contentKey]; if (!item) throw new C311ApiError(this.fixtures.errors['not-found'], 404); if (!isSafeSanitizedHTML(input.body)) throw new C311ApiError(this.fixtures.errors.validation, 422); return { ...copy(item), ...input, sanitized: true, state: 'DRAFT', published: false } }
  async publishAdminContent (contentKey: PublicContentKey, options: C311RequestOptions = {}): Promise<ContentObject> { this.requireCapability('admin_content_publish'); const item = this.adminContent[contentKey]; if (!item) throw new C311ApiError(this.fixtures.errors['not-found'], 404); this.requireVersion(options, item.version); this.adminContent[contentKey] = { ...item, sanitized: true, state: 'PUBLISHED', published: true, version: item.version + 1, updated_at: BENCHMARK_NOW }; this.fixtures.public_content![contentKey] = copy(this.adminContent[contentKey]); this.adminContentVersions[contentKey].push(copy(this.adminContent[contentKey])); return copy(this.adminContent[contentKey]) }
  async listAdminContentVersions (contentKey: PublicContentKey, _query: ListQuery = {}): Promise<PageResponse<ContentObject>> { this.requireCapability('admin_content_versions'); return { items: copy(this.adminContentVersions[contentKey] || []), next_page_token: null, total_count: (this.adminContentVersions[contentKey] || []).length, applied_filters: {}, sort: [] } }
  async rollbackAdminContent (contentKey: PublicContentKey, input: RollbackInput, options: C311RequestOptions = {}): Promise<ContentObject> { this.requireCapability('admin_content_rollback'); const item = this.adminContent[contentKey]; if (!item) throw new C311ApiError(this.fixtures.errors['not-found'], 404); this.requireVersion(options, item.version); const target = (this.adminContentVersions[contentKey] || []).find(value => value.version === input.target_version); if (!target) throw new C311ApiError(this.fixtures.errors['not-found'], 404); this.adminContent[contentKey] = { ...copy(target), sanitized: true, version: item.version + 1, published: true, state: 'PUBLISHED', updated_at: BENCHMARK_NOW }; this.fixtures.public_content![contentKey] = copy(this.adminContent[contentKey]); this.adminContentVersions[contentKey].push(copy(this.adminContent[contentKey])); return copy(this.adminContent[contentKey]) }
  async getAdminHelp (helpKey: HelpKey, language: Language = 'EN'): Promise<HelpContent> { this.requireCapability('admin_help_get'); const item = this.adminHelp[`${helpKey}:${language}`]; if (!item) throw new C311ApiError(this.fixtures.errors['not-found'], 404); return copy(item) }
  async updateAdminHelp (helpKey: HelpKey, input: HelpWrite, options: C311RequestOptions = {}): Promise<HelpContent> { this.requireCapability('admin_help_update'); const key = `${helpKey}:${input.language}`; const item = this.adminHelp[key]; if (!item) throw new C311ApiError(this.fixtures.errors['not-found'], 404); this.requireVersion(options, item.version); if (!isSafeSanitizedHTML(input.body)) throw new C311ApiError(this.fixtures.errors.validation, 422); const updated = { ...item, ...input, sanitized: true, state: 'DRAFT' as const, published: false, version: item.version + 1, updated_at: BENCHMARK_NOW }; this.adminHelp[key] = copy(updated); this.adminHelpVersions[key].push(copy(updated)); return copy(updated) }
  async previewAdminHelp (helpKey: HelpKey, input: HelpWrite): Promise<HelpContent> { this.requireCapability('admin_help_preview'); const item = this.adminHelp[`${helpKey}:${input.language}`]; if (!item) throw new C311ApiError(this.fixtures.errors['not-found'], 404); if (!isSafeSanitizedHTML(input.body)) throw new C311ApiError(this.fixtures.errors.validation, 422); return { ...copy(item), ...input, sanitized: true, state: 'DRAFT', published: false } }
  async publishAdminHelp (helpKey: HelpKey, language: Language = 'EN', options: C311RequestOptions = {}): Promise<HelpContent> { this.requireCapability('admin_help_publish'); const key = `${helpKey}:${language}`; const item = this.adminHelp[key]; if (!item) throw new C311ApiError(this.fixtures.errors['not-found'], 404); this.requireVersion(options, item.version); const published = { ...item, sanitized: true, state: 'PUBLISHED' as const, published: true, version: item.version + 1, updated_at: BENCHMARK_NOW }; this.adminHelp[key] = copy(published); this.adminHelpVersions[key].push(copy(published)); this.fixtures.public_help![helpKey] = copy(published); return copy(published) }
  async listAdminHelpVersions (helpKey: HelpKey, query: ListQuery & { language?: Language } = {}): Promise<PageResponse<HelpContent>> { this.requireCapability('admin_help_versions'); const key = `${helpKey}:${query.language || 'EN'}`; return { items: copy(this.adminHelpVersions[key] || []), next_page_token: null, total_count: (this.adminHelpVersions[key] || []).length, applied_filters: {}, sort: [] } }
  async rollbackAdminHelp (helpKey: HelpKey, input: RollbackInput, language: Language = 'EN', options: C311RequestOptions = {}): Promise<HelpContent> { this.requireCapability('admin_help_rollback'); const key = `${helpKey}:${language}`; const item = this.adminHelp[key]; const target = this.adminHelpVersions[key]?.find(version => version.version === input.target_version); if (!item) throw new C311ApiError(this.fixtures.errors['not-found'], 404); if (!target) throw new C311ApiError(this.fixtures.errors['not-found'], 404); this.requireVersion(options, item.version); const rolledBack = { ...copy(target), sanitized: true, state: 'PUBLISHED' as const, published: true, version: item.version + 1, updated_at: BENCHMARK_NOW }; this.adminHelp[key] = copy(rolledBack); this.adminHelpVersions[key].push(copy(rolledBack)); this.fixtures.public_help![helpKey] = copy(rolledBack); return copy(rolledBack) }
  async listAdminCategories (_query: ListQuery = {}): Promise<PageResponse<Category>> { this.requireCapability('admin_categories_list'); return { items: copy(this.adminCategories), next_page_token: null, total_count: this.adminCategories.length, applied_filters: {}, sort: [] } }
  async createAdminCategory (input: CategoryWrite): Promise<Category> { this.requireCapability('admin_categories_create'); if (!input.code.trim() || !input.labels.EN?.trim() || this.adminCategories.some(item => item.code === input.code)) throw new C311ApiError(this.fixtures.errors.validation, 422); const item = { ...input, version: 1, updated_at: BENCHMARK_NOW }; this.adminCategories.push(copy(item)); return copy(item) }
  async updateAdminCategory (categoryCode: string, input: CategoryWrite, options: C311RequestOptions = {}): Promise<Category> { this.requireCapability('admin_categories_update'); const index = this.adminCategories.findIndex(item => item.code === categoryCode); if (index < 0) throw new C311ApiError(this.fixtures.errors['not-found'], 404); const current = this.adminCategories[index]; this.requireVersion(options, current.version); const categoryInUse = this.profile.primary_category === categoryCode || this.fixtures.requests.some(request => request.primary_requester.primary_category === categoryCode); if (input.code !== categoryCode || !input.labels.EN?.trim() || (!input.active && categoryInUse)) throw new C311ApiError(this.fixtures.errors.validation, 422); const item = { ...current, ...input, version: current.version + 1, updated_at: BENCHMARK_NOW }; this.adminCategories[index] = item; return copy(item) }
  async listAdminCustomFields (_query: ListQuery = {}): Promise<PageResponse<CustomFieldDefinition>> { this.requireCapability('admin_custom_fields_list'); return { items: copy(this.adminCustomFields), next_page_token: null, total_count: this.adminCustomFields.length, applied_filters: {}, sort: [] } }
  async createAdminCustomField (input: CustomFieldDefinition): Promise<CustomFieldDefinition> { this.requireCapability('admin_custom_fields_create'); if (this.adminCustomFields.some(item => item.key === input.key) || !validCustomFieldDefinition(input)) throw new C311ApiError(this.fixtures.errors.validation, 422); const item = { ...input, version: 1, updated_at: BENCHMARK_NOW }; this.adminCustomFields.push(copy(item)); return copy(item) }
  async updateAdminCustomField (fieldKey: string, input: CustomFieldDefinition, options: C311RequestOptions = {}): Promise<CustomFieldDefinition> { this.requireCapability('admin_custom_fields_update'); const index = this.adminCustomFields.findIndex(item => item.key === fieldKey); if (index < 0) throw new C311ApiError(this.fixtures.errors['not-found'], 404); this.requireVersion(options, this.adminCustomFields[index].version); if (!validCustomFieldDefinition(input, fieldKey)) throw new C311ApiError(this.fixtures.errors.validation, 422); const item = { ...this.adminCustomFields[index], ...input, version: this.adminCustomFields[index].version + 1, updated_at: BENCHMARK_NOW }; this.adminCustomFields[index] = item; return copy(item) }

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
    const requestedLanguage = language || 'EN'
    // Public reads may use a language-specific published admin projection,
    // falling back to the stable public fixture. Drafts and previews remain
    // private until the explicit publish operation succeeds.
    const languageProjection = this.adminHelp[`${helpKey}:${requestedLanguage}`]
    const content = languageProjection?.published ? languageProjection : this.fixtures.public_help?.[helpKey]
    if (!content) throw new C311ApiError(this.fixtures.errors['not-found'], 404)
    const result = copy(content)
    if (language && language !== result.language) {
      result.language = language
      result.body = language === 'ES' ? '<p>Describe el problema y envialo a la ciudad.</p>' : language === 'VI' ? '<p>Mo ta van de va gui den thanh pho.</p>' : '<p>Describe the issue and submit it to the city.</p>'
      result.sanitized = true
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
    this.requireCapability('operation_get')
    this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    const stored = this.operations.get(operationID)
    if (stored) {
      if (this.scenario === 'terminal' && stored.status !== 'FAILED') {
        const failed = { ...stored, status: 'FAILED' as const, progress: 100, result: null, error: copy(this.fixtures.errors.terminal), completed_at: '2026-01-15T15:00:00.000Z' }
        this.operations.set(operationID, copy(failed))
        return copy(failed)
      }
      return copy(stored)
    }
    if (this.scenario === 'terminal' && operationID === 'operation-fixture-terminal') {
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
    throw new C311ApiError(this.fixtures.errors['not-found'], 404)
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
    this.requireCapability('saved_report_list')
    this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    return this.page(this.scenario === 'empty' ? [] : this.fixtures.reports, query)
  }

  async getReport (reportID: string): Promise<ReportDefinition> {
    this.requireCapability('saved_report_list')
    this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    const report = this.fixtures.reports.find(item => item.report_id === reportID)
    if (!report) throw new C311ApiError(this.fixtures.errors['not-found'], 404)
    return copy(report)
  }

  async createReport (input: ReportDefinition): Promise<ReportDefinition> {
    this.requireCapability('saved_report_create')
    this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    this.validateReportDefinition(input)
    if (this.fixtures.reports.some(item => item.report_id === input.report_id)) this.validationError('A report with this ID already exists.', [{ field: '/report_id', code: 'DUPLICATE' }])
    const report = copy(input)
    this.fixtures.reports.push(report)
    this.reportOwners[report.report_id] = this.currentSession.actor?.actor_id || ''
    this.fixtures.report_owners = copy(this.reportOwners)
    this.countWrite('saved_report_create')
    return copy(report)
  }

  private requireReportOwner (report: ReportDefinition): void {
    const actorID = this.currentSession.actor?.actor_id
    if (!actorID || this.reportOwners[report.report_id] !== actorID) {
      throw new C311ApiError({ error: 'FORBIDDEN', message: 'Only the report owner can change this report.', retryable: false }, 403)
    }
  }

  async updateReport (reportID: string, input: ReportDefinition, options: C311RequestOptions = {}): Promise<ReportDefinition> {
    this.requireCapability('saved_report_update')
    this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    this.validateReportDefinition(input)
    const report = await this.getReport(reportID)
    this.requireReportOwner(report)
    if (options.expectedVersion === undefined) throw new C311ApiError({ error: 'EXPECTED_VERSION_REQUIRED', message: 'If-Match is required for this update.', retryable: false }, 428)
    this.failVersionConflictOnce('report_update', report.version)
    if (options.expectedVersion !== report.version) throw new C311ApiError({ error: 'VERSION_CONFLICT', message: 'The report changed before your update.', retryable: false, current_version: report.version }, 409)
    const updated = { ...report, ...input, report_id: reportID, version: report.version + 1, updated_at: '2026-01-15T15:00:00.000Z' }
    const index = this.fixtures.reports.findIndex(item => item.report_id === reportID)
    if (index >= 0) this.fixtures.reports[index] = copy(updated)
    this.fixtures.report_owners = copy(this.reportOwners)
    this.countWrite('saved_report_update')
    return copy(updated)
  }

  private validateReportDefinition (input: ReportDefinition): void {
    const errors = [] as Array<{ field: string, code: 'REQUIRED' | 'TOO_MANY_ITEMS' | 'INVALID_VALUE' }>
    if (!input || typeof input !== 'object' || Array.isArray(input)) {
      this.validationError('The report definition is invalid.', [{ field: '/', code: 'INVALID_VALUE' }])
    }
    if (!String(input.report_id || '').trim()) errors.push({ field: '/report_id', code: 'REQUIRED' })
    if (!Number.isInteger(input.version) || input.version < 1) errors.push({ field: '/version', code: 'INVALID_VALUE' })
    if (!validISODateTime(input.updated_at)) errors.push({ field: '/updated_at', code: 'INVALID_VALUE' })
    if (!String(input.name || '').trim()) errors.push({ field: '/name', code: 'REQUIRED' })
    if (!Array.isArray(input.columns) || input.columns.length < 1) errors.push({ field: '/columns', code: 'REQUIRED' })
    if (Array.isArray(input.columns) && input.columns.length > 20) errors.push({ field: '/columns', code: 'TOO_MANY_ITEMS' })
    if (Array.isArray(input.columns) && (input.columns.some(column => !String(column).trim()) || new Set(input.columns).size !== input.columns.length)) errors.push({ field: '/columns', code: 'INVALID_VALUE' })
    if (!Array.isArray(input.sort)) errors.push({ field: '/sort', code: 'INVALID_VALUE' })
    else if (input.sort.length > 3) errors.push({ field: '/sort', code: 'TOO_MANY_ITEMS' })
    if (input.grouping !== undefined && input.grouping !== null && typeof input.grouping !== 'string') errors.push({ field: '/grouping', code: 'INVALID_VALUE' })
    const grouping = typeof input.grouping === 'string' ? input.grouping.split(',').map(value => value.trim()).filter(Boolean) : []
    if (grouping.length > 1) errors.push({ field: '/grouping', code: 'TOO_MANY_ITEMS' })
    if (!input.filters || typeof input.filters !== 'object' || Array.isArray(input.filters)) errors.push({ field: '/filters', code: 'INVALID_VALUE' })
    const rules = REPORT_ENTITY_RULES[input.entity]
    if (!rules) errors.push({ field: '/entity', code: 'INVALID_VALUE' })
    if (input.filters && rules && Object.keys(input.filters).some(key => !rules.supported_filters.includes(key))) errors.push({ field: '/filters', code: 'INVALID_VALUE' })
    for (const key of ['created_from', 'created_to']) {
      if (input.filters?.[key] !== undefined && !validReportDateFilter(input.filters[key])) errors.push({ field: `/filters/${key}`, code: 'INVALID_VALUE' })
    }
    if (input.filters?.created_from && input.filters?.created_to && String(input.filters.created_from) > String(input.filters.created_to)) errors.push({ field: '/filters', code: 'INVALID_VALUE' })
    if (grouping.length && rules && !rules.supported_grouping.includes(grouping[0])) errors.push({ field: '/grouping', code: 'INVALID_VALUE' })
    if (Array.isArray(input.sort) && (input.sort.some(value => !String(value).trim()) || new Set(input.sort.map(value => value.replace(/^[+-]/, ''))).size !== input.sort.length)) errors.push({ field: '/sort', code: 'INVALID_VALUE' })
    if (Array.isArray(input.sort) && rules && input.sort.some(value => !rules.supported_sort.includes(value.replace(/^[+-]/, '')))) errors.push({ field: '/sort', code: 'INVALID_VALUE' })
    if (errors.length) throw new C311ApiError({ error: 'VALIDATION_ERROR', message: 'The report definition is invalid.', retryable: false, errors }, 422)
  }

  async runReport (_input: { definition: ReportDefinition }): Promise<Operation> {
    this.requireCapability('report_run')
    this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    this.validateReportDefinition(_input.definition)
    const rows = this.reportRows(_input.definition)
    this.createOperation('operation-fixture-report', 'report_run', { rows, row_count: rows.length, filters: copy(_input.definition.filters), grouping: _input.definition.grouping || null, sort: copy(_input.definition.sort) })
    return { ...copy(this.operations.get('operation-fixture-report')!), status: 'PENDING', progress: 0, result: null, completed_at: null }
  }

  async exportReport (_reportID: string, _options: ReportExportOptions = {}): Promise<Operation> {
    this.requireCapability('report_export')
    this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    if (_options.format !== undefined && _options.format !== 'CSV') this.validationError('Only CSV report exports are supported.', [{ field: '/format', code: 'INVALID_VALUE' }])
    const report = await this.getReport(_reportID)
    const source = this.reportRows(report)
    const valueFor = (row: Record<string, unknown>, column: string): unknown => {
      const value = row[column]
      if (value !== undefined && value !== null) return value
      return ''
    }
    const header = report.columns.map(column => this.csvCell(column)).join(',')
    const rows = source.map(row => report.columns.map(column => this.csvCell(valueFor(row as unknown as Record<string, unknown>, column))).join(','))
    const csv = `${[header, ...rows].join('\r\n')}\r\n`
    this.createOperation('operation-fixture-export', 'report_export', { content_type: 'text/csv;charset=utf-8', body: csv, filename: 'city311-report.csv', row_count: source.length, filters: copy(report.filters), grouping: report.grouping || null, sort: copy(report.sort) })
    return { ...copy(this.operations.get('operation-fixture-export')!), status: 'PENDING', progress: 0, result: null, completed_at: null }
  }

  private reportRows (definition: ReportDefinition): Array<Record<string, unknown>> {
    const rows: Array<Record<string, unknown>> = []
    if (definition.entity === 'service_requests') {
      this.fixtures.requests.filter(request => this.requestInScope(request)).forEach(request => {
        const detail = this.fixtures.details[request.request_id]
        const history = detail?.history || []
        const resolvedEvent = history.find(item => ['RESOLVED', 'CLOSED'].includes(item.action))
        const followUpCount = (this.fixtures.follow_up_actions || []).filter(action => action.request_id === request.request_id).length
        const assignmentCount = (detail?.audit || []).filter(item => ['ASSIGN', 'REASSIGN'].includes(String(item.action))).length
        const reminderCount = detail?.reminders || []
        const ageDays = Math.max(0, Math.floor((Date.parse('2026-01-15T15:00:00.000Z') - Date.parse(request.created_at)) / 86400000))
        const resolutionDays = resolvedEvent ? Math.max(0, Math.floor((Date.parse(resolvedEvent.occurred_at) - Date.parse(request.created_at)) / 86400000)) : null
        rows.push({
          ...copy(request),
          request_number: request.request_number || '',
          department: request.owning_department,
          district: request.council_district || '',
          duplicate_group: request.duplicate_group_id || '',
          primary_assignee_id: detail?.primary_assignee_id || '',
          collaborator_count: detail?.collaborator_ids?.length || 0,
          follow_up_count: followUpCount,
          assignment_count: assignmentCount,
          overdue_reminder_count: reminderCount.filter(reminder => !['COMPLETED', 'CANCELLED'].includes(reminder.status) && Date.parse(reminder.due_at) < Date.parse('2026-01-15T15:00:00.000Z')).length,
          age_days: ageDays,
          resolved_at: resolvedEvent?.occurred_at || '',
          resolution_days: resolutionDays,
          reopened: history.some(item => item.action === 'REOPENED'),
        } as unknown as Record<string, unknown>)
      })
    } else if (definition.entity === 'constituents') {
      const seen = new Set<string>()
      this.fixtures.requests.forEach(request => {
        const constituent = request.primary_requester
        if (!this.requestInScope(request) || seen.has(constituent.constituent_id)) return
        seen.add(constituent.constituent_id)
        const requestCount = this.fixtures.requests.filter(item => item.primary_requester.constituent_id === constituent.constituent_id && this.requestInScope(item)).length
        rows.push({ ...copy(constituent), email: constituent.emails[0] || '', department: request.owning_department, district: request.council_district || '', created_at: request.created_at, request_count: requestCount } as unknown as Record<string, unknown>)
      })
    } else {
      const actions = this.fixtures.follow_up_actions || []
      actions.forEach(action => {
        const request = this.fixtures.requests.find(item => item.request_id === action.request_id)
        if (this.requestInScope(request)) rows.push({ ...copy(action), created_at: action.occurred_at, request_number: request?.request_number || '', owning_department: request?.owning_department || '', district: request?.council_district || '' } as unknown as Record<string, unknown>)
      })
    }

    const filtered = rows.filter(row => Object.entries(definition.filters || {}).every(([key, expected]) => {
      if (key === 'created_from') return Date.parse(String(row.created_at || '')) >= reportDateBoundary(String(expected), false)
      if (key === 'created_to') return Date.parse(String(row.created_at || '')) <= reportDateBoundary(String(expected), true)
      return this.matchesFilter(row[key] ?? (key === 'updated_at' ? row.updated_at || row.occurred_at : undefined), expected)
    }))
    const sorted = [...filtered].sort((left, right) => {
      for (const expression of definition.sort || []) {
        const descending = expression.startsWith('-')
        const field = expression.replace(/^[+-]/, '')
        const leftValue = String(left[field] ?? '')
        const rightValue = String(right[field] ?? '')
        if (leftValue === rightValue) continue
        const result = leftValue.localeCompare(rightValue)
        return descending ? -result : result
      }
      const leftID = String(left.request_id || left.constituent_id || left.action_type || '')
      const rightID = String(right.request_id || right.constituent_id || right.action_type || '')
      return leftID.localeCompare(rightID)
    })
    const grouping = typeof definition.grouping === 'string' ? definition.grouping.trim() : ''
    const selected = (items: Array<Record<string, unknown>>): Array<Record<string, unknown>> => items.map(row => Object.fromEntries(definition.columns.map(column => [column, row[column] ?? ''])))
    if (!grouping) return selected(sorted)
    const grouped = new Map<string, Record<string, unknown>>()
    sorted.forEach(row => {
      const value = String(row[grouping] ?? '')
      const existing = grouped.get(value)
      if (existing) existing.count = Number(existing.count || 0) + 1
      else grouped.set(value, { [grouping]: value, count: 1 })
    })
    return selected([...grouped.values()])
  }

  async listWorkflows (query: ListQuery = {}): Promise<PageResponse<WorkflowDefinition>> {
    this.requireCapability('workflow_list')
    this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    return this.page(this.scenario === 'empty' ? [] : this.fixtures.workflows, query)
  }

  async getWorkflow (workflowID: string): Promise<WorkflowDefinition> {
    this.requireCapability('workflow_get')
    this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    const workflow = this.fixtures.workflows.find(item => item.workflow_id === workflowID)
    if (!workflow) throw new C311ApiError(this.fixtures.errors['not-found'], 404)
    return copy(workflow)
  }

  async createWorkflow (input: WorkflowDefinition): Promise<WorkflowDefinition> {
    this.requireCapability('workflow_create')
    this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    assertWorkflowDefinition(input, true)
    if (this.fixtures.workflows.some(item => item.workflow_id === input.workflow_id)) this.failScenario('validation')
    const workflow = { ...copy(input), active: false, version: input.version || 1, updated_at: '2026-01-15T15:00:00.000Z' }
    this.fixtures.workflows.push(workflow)
    this.countWrite('workflow_create')
    return copy(workflow)
  }

  async updateWorkflow (workflowID: string, input: WorkflowDefinition, options: C311RequestOptions = {}): Promise<WorkflowDefinition> {
    this.requireCapability('workflow_update')
    this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    const workflow = await this.getWorkflow(workflowID)
    if (options.expectedVersion === undefined) this.failScenario('expected-version-required')
    this.failVersionConflictOnce('workflow_update', workflow.version)
    if (options.expectedVersion !== workflow.version) this.failScenario('version-conflict')
    const candidate = { ...workflow, ...input, workflow_id: workflowID, active: workflow.active }
    assertWorkflowDefinition(candidate, false)
    const updated = { ...candidate, version: workflow.version + 1, updated_at: '2026-01-15T15:00:00.000Z' }
    const index = this.fixtures.workflows.findIndex(item => item.workflow_id === workflowID)
    if (index >= 0) this.fixtures.workflows[index] = copy(updated)
    this.countWrite('workflow_update')
    return copy(updated)
  }

  async activateWorkflow (workflowID: string, options: C311RequestOptions = {}): Promise<WorkflowDefinition> {
    this.requireCapability('workflow_activate'); this.failIfNeeded(['forbidden', 'not-found', 'validation', 'expected-version-required'])
    const workflow = await this.getWorkflow(workflowID)
    if (options.expectedVersion === undefined) this.failScenario('expected-version-required')
    this.failVersionConflictOnce('workflow_activate', workflow.version)
    if (options.expectedVersion !== workflow.version) this.failScenario('version-conflict')
    workflow.active = true; workflow.version += 1; workflow.updated_at = '2026-01-15T15:00:00.000Z'; this.fixtures.workflows[this.fixtures.workflows.findIndex(item => item.workflow_id === workflowID)] = copy(workflow); this.countWrite('workflow_activate'); return copy(workflow)
  }

  async deactivateWorkflow (workflowID: string, options: C311RequestOptions = {}): Promise<WorkflowDefinition> {
    this.requireCapability('workflow_deactivate'); this.failIfNeeded(['forbidden', 'not-found', 'validation', 'expected-version-required'])
    const workflow = await this.getWorkflow(workflowID)
    if (options.expectedVersion === undefined) this.failScenario('expected-version-required')
    this.failVersionConflictOnce('workflow_deactivate', workflow.version)
    if (options.expectedVersion !== workflow.version) this.failScenario('version-conflict')
    workflow.active = false; workflow.version += 1; workflow.updated_at = '2026-01-15T15:00:00.000Z'; this.fixtures.workflows[this.fixtures.workflows.findIndex(item => item.workflow_id === workflowID)] = copy(workflow); this.countWrite('workflow_deactivate'); return copy(workflow)
  }

  async testWorkflow (workflowID: string, _input: WorkflowTestInput): Promise<Operation> {
    this.requireCapability('workflow_test'); this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    const workflow = await this.getWorkflow(workflowID)
    if (!String(_input?.request_id || '').trim()) this.failScenario('validation')
    const executionID = `execution-fixture-test-${(this.fixtures.workflow_executions || []).length + 1}`
    const terminal = this.scenario === 'terminal'
    const execution: WorkflowExecution = { execution_id: executionID, workflow_version: workflow.version, trigger: workflow.trigger, outcome: terminal ? 'FAILED' : 'SUCCEEDED', actions_attempted: workflow.actions.map(action => String(action.type || action.kind || action.action || '')), succeeded: !terminal, occurred_at: '2026-01-15T15:00:00.000Z', response_status: terminal ? 500 : 200, ...(terminal ? { error: copy(this.fixtures.errors.terminal) } : {}) }
    const executions = this.fixtures.workflow_executions || (this.fixtures.workflow_executions = [])
    executions.push(copy(execution))
    const operation = this.createOperation('operation-fixture-workflow-test', 'workflow_test', { execution_id: executionID, outcome: terminal ? 'FAILED' : 'SUCCEEDED', ...(terminal ? { error: copy(this.fixtures.errors.terminal) } : {}) })
    if (terminal) {
      operation.status = 'FAILED'
      operation.progress = 100
      operation.result = { execution_id: executionID, outcome: 'FAILED' }
      operation.error = copy(this.fixtures.errors.terminal)
      this.operations.set(operation.operation_id, copy(operation))
    }
    this.countWrite('workflow_test')
    return { ...copy(this.operations.get('operation-fixture-workflow-test')!), status: 'PENDING', progress: 0, result: null, completed_at: null }
  }

  async listWorkflowExecutions (query: ListQuery = {}): Promise<PageResponse<WorkflowExecution>> {
    this.requireCapability('workflow_execution_list'); this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    const items = this.scenario === 'empty' ? [] : (this.fixtures.workflow_executions || [{ execution_id: 'execution-fixture-001', workflow_version: 1, trigger: 'SERVICE_REQUEST_CREATED', outcome: 'SUCCEEDED', actions_attempted: ['notify'], succeeded: true, occurred_at: '2026-01-15T15:00:00.000Z' }])
    return this.page(items, query)
  }

  async getWorkflowExecution (executionID: string): Promise<WorkflowExecution> {
    this.requireCapability('workflow_execution_get'); this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    const item = (this.fixtures.workflow_executions || []).find(execution => execution.execution_id === executionID)
    if (!item) throw new C311ApiError(this.fixtures.errors['not-found'], 404)
    return copy(item)
  }

  async executeWorkflowAction (input: WorkflowActionRequest, options: C311RequestOptions = {}): Promise<WorkflowActionAccepted> {
    if (!this.currentSession.authenticated || (this.currentSession.expires_at && Date.parse(this.currentSession.expires_at) <= Date.now())) {
      throw new C311ApiError(this.fixtures.errors['workflow-invalid-token'] || { error: 'INVALID_TOKEN', message: 'The workflow access token is invalid or expired.', retryable: false }, 401)
    }
    this.failIfNeeded(['workflow-invalid-client', 'workflow-invalid-token', 'workflow-insufficient-scope', 'invalid-client', 'invalid-token', 'insufficient-scope', 'forbidden', 'retryable', 'idempotency-conflict'])
    if (!this.currentSession.actor?.scopes.includes('workflow.execute')) {
      throw new C311ApiError(this.fixtures.errors['workflow-insufficient-scope'] || { error: 'INSUFFICIENT_SCOPE', message: 'The workflow token does not grant workflow.execute.', retryable: false }, 403)
    }
    const idempotencyKey = options.idempotencyKey
    if (!idempotencyKey) throw new C311ApiError({ error: 'VALIDATION_ERROR', message: 'Idempotency-Key is required.', retryable: false }, 422)
    const fingerprint = this.fingerprint(input)
    const previous = this.extensionIdempotentResponses.get(idempotencyKey)
    if (previous && previous.fingerprint !== fingerprint) this.failScenario('idempotency-conflict')
    if (previous) return copy(previous.response) as WorkflowActionAccepted
    if (!String(input.request_id || '').trim() || !String(input.action || '').trim() || !input.payload || Array.isArray(input.payload)) this.failScenario('validation')
    const execution: WorkflowExecution = { execution_id: 'execution-fixture-action', workflow_version: 1, trigger: input.action, outcome: 'SUCCEEDED', actions_attempted: [input.action], succeeded: true, occurred_at: '2026-01-15T15:00:00.000Z', response_status: 200 }
    const existing = this.fixtures.workflow_executions || (this.fixtures.workflow_executions = [])
    if (!existing.some(item => item.execution_id === execution.execution_id)) existing.push(copy(execution))
    const response: WorkflowActionAccepted = { execution_id: execution.execution_id, accepted_at: '2026-01-15T15:00:00.000Z' }
    this.extensionIdempotentResponses.set(idempotencyKey, { fingerprint, response })
    this.countWrite('workflow_action_execute')
    return copy(response)
  }

  async importCalendar (input: CalendarImport): Promise<Operation> {
    this.requireCapability('calendar_import'); this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    let events: MockCalendarEvent[]
    try {
      events = parseCalendarEvents(input.ics)
    } catch (_error) {
      this.failScenario('validation')
      events = []
    }
    if (!events.length || new Set(events.map(event => event.uid)).size !== events.length) this.failScenario('validation')
    const summary = { imported: 0, updated: 0, cancelled: 0, ignored: 0 }
    events.forEach(incoming => {
      const current = this.calendarEvents.find(event => event.uid === incoming.uid)
      if (incoming.cancelled && !current) { summary.ignored += 1; return }
      if (current) {
        current.cancelled = incoming.cancelled
        if (incoming.summary) current.summary = incoming.summary
        if (incoming.description !== undefined) current.description = incoming.description
        if (incoming.dtstart !== undefined) current.dtstart = incoming.dtstart
        if (incoming.dtend !== undefined) current.dtend = incoming.dtend
        if (incoming.rrule !== undefined) current.rrule = incoming.rrule
        if (incoming.last_modified !== undefined) current.last_modified = incoming.last_modified
        if (incoming.timezone !== undefined) current.timezone = incoming.timezone
        current.updated_at = '2026-01-15T15:00:00.000Z'
        if (incoming.cancelled) summary.cancelled += 1
        else summary.updated += 1
      } else {
        this.calendarEvents.push(copy(incoming))
        summary.imported += 1
      }
    })
    this.countWrite('calendar_import'); this.persistExtensionState()
    this.createOperation('operation-fixture-calendar-import', 'calendar_import', { summary })
    return { ...copy(this.operations.get('operation-fixture-calendar-import')!), status: 'PENDING', progress: 0, result: null, completed_at: null }
  }

  async exportCalendar (): Promise<CalendarExport> {
    this.requireCapability('calendar_export'); this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    const escape = (value: string): string => value.replace(/\\/g, '\\\\').replace(/;/g, '\\;').replace(/,/g, '\\,').replace(/\r?\n/g, '\\n')
    const dateLine = (name: string, value?: string, timezone?: string): string | null => {
      if (!value) return null
      return `${name}${timezone ? `;TZID=${timezone}` : ''}:${value}`
    }
    const lines = ['BEGIN:VCALENDAR', 'VERSION:2.0']
    this.calendarEvents.forEach(event => {
      lines.push('BEGIN:VEVENT', `UID:${escape(event.uid)}`, `SUMMARY:${escape(event.summary)}`)
      if (event.description !== undefined) lines.push(`DESCRIPTION:${escape(event.description)}`)
      const start = dateLine('DTSTART', event.dtstart, event.timezone)
      const end = dateLine('DTEND', event.dtend, event.timezone)
      if (start) lines.push(start)
      if (end) lines.push(end)
      if (event.rrule) lines.push(`RRULE:${event.rrule}`)
      if (event.last_modified) lines.push(`LAST-MODIFIED:${event.last_modified}`)
      lines.push(`STATUS:${event.cancelled ? 'CANCELLED' : 'CONFIRMED'}`, 'END:VEVENT')
    })
    lines.push('END:VCALENDAR')
    const body = `${lines.join('\r\n')}\r\n`
    return { content_type: 'text/calendar', body }
  }

  async previewMail (input: MailCompose): Promise<MailPreview> {
    this.requireCapability('mail_preview'); this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    this.validateMailInput(input)
    return { subject: input.subject, text: input.text, html: sanitizeMailHtml(input.html || ''), sanitized: true }
  }

  async sendMail (input: MailCompose, options: C311RequestOptions = {}): Promise<MailDelivery> {
    this.requireCapability('mail_send'); this.failIfNeeded(['forbidden', 'not-found', 'validation', 'idempotency-conflict'])
    this.validateMailInput(input)
    const idempotencyKey = options.idempotencyKey
    if (!idempotencyKey) throw new C311ApiError({ error: 'VALIDATION_ERROR', message: 'Idempotency-Key is required.', retryable: false }, 422)
    const fingerprint = this.fingerprint({ ...input, html: sanitizeMailHtml(input.html || '') })
    const previous = this.extensionIdempotentResponses.get(idempotencyKey)
    if (previous && previous.fingerprint !== fingerprint) this.failScenario('idempotency-conflict')
    if (previous) return copy(previous.response) as MailDelivery
    this.countWrite('mail_send')
    const response: MailDelivery = { delivery_id: 'delivery-fixture-001', status: 'PENDING', attempts: 1, updated_at: '2026-01-15T15:00:00.000Z', error: null }
    this.mailDeliveries[response.delivery_id] = copy(response)
    this.persistExtensionState()
    this.extensionIdempotentResponses.set(idempotencyKey, { fingerprint, response })
    return copy(response)
  }

  private validateMailInput (input: MailCompose): void {
    const errors: Array<{ field: string, code: 'REQUIRED' | 'INVALID_FORMAT' | 'TOO_MANY_ITEMS' }> = []
    if (!input || !Array.isArray(input.to) || input.to.length < 1) errors.push({ field: '/to', code: 'REQUIRED' })
    else input.to.forEach((recipient, index) => { if (!validMailAddress(recipient)) errors.push({ field: `/to/${index}`, code: 'INVALID_FORMAT' }) })
    if (!String(input?.subject || '').trim() || /[\r\n]/.test(String(input?.subject || ''))) errors.push({ field: '/subject', code: 'INVALID_FORMAT' })
    if (!String(input?.text || '').trim()) errors.push({ field: '/text', code: 'REQUIRED' })
    if (input?.html !== undefined && typeof input.html !== 'string') errors.push({ field: '/html', code: 'INVALID_FORMAT' })
    if (input?.template_id && !this.mailTemplates[input.template_id]) errors.push({ field: '/template_id', code: 'INVALID_FORMAT' })
    if (input?.attachments !== undefined) {
      if (!Array.isArray(input.attachments)) {
        errors.push({ field: '/attachments', code: 'INVALID_FORMAT' })
      } else if (input.attachments.length > 3) {
        errors.push({ field: '/attachments', code: 'TOO_MANY_ITEMS' })
      }
      const attachmentTokens = new Set<string>()
      if (Array.isArray(input.attachments)) input.attachments.forEach((attachment, index) => {
        const attachmentErrors = validatePortalAttachment({ filename: attachment?.filename, media_type: attachment?.media_type, size: attachment?.size })
        if (Number(attachment?.size) > MAIL_ATTACHMENT_MAX_BYTES) attachmentErrors.push({ field: '/size', code: 'OUT_OF_RANGE' })
        if (!attachment?.attachment_token || attachmentTokens.has(String(attachment.attachment_token)) || !validISODateTime(attachment?.expires_at) || attachmentErrors.length) errors.push({ field: `/attachments/${index}`, code: 'INVALID_FORMAT' })
        if (attachment?.attachment_token) attachmentTokens.add(String(attachment.attachment_token))
      })
    }
    if (errors.length) throw new C311ApiError({ error: 'VALIDATION_ERROR', message: 'The mail message is invalid.', retryable: false, errors }, 422)
  }

  async getMailDelivery (deliveryID: string): Promise<MailDelivery> {
    this.requireCapability('mail_delivery_get'); this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    const delivery = this.mailDeliveries[deliveryID]
    if (!delivery) throw new C311ApiError(this.fixtures.errors['not-found'], 404)
    const smtpRetryScenario = this.scenario === 'smtp-421' || this.scenario === 'smtp-451'
    const smtpTerminalScenario = this.scenario === 'smtp-550' || this.scenario === 'smtp-553'
    if (delivery.status === 'PENDING' && smtpRetryScenario) {
      delivery.attempts = Math.min(3, delivery.attempts + 1)
      if (delivery.attempts < 3) {
        delivery.error = copy(this.fixtures.errors[this.scenario])
      } else {
        delivery.status = 'DELIVERED'
        delivery.error = null
      }
      delivery.updated_at = '2026-01-15T15:00:00.000Z'
      this.persistExtensionState()
      return copy(delivery)
    }
    if (delivery.status === 'PENDING' && smtpTerminalScenario) {
      delivery.attempts = Math.min(3, delivery.attempts + 1)
      delivery.status = 'TERMINAL_FAILURE'
      delivery.error = copy(this.fixtures.errors[this.scenario])
      delivery.updated_at = '2026-01-15T15:00:00.000Z'
      this.persistExtensionState()
      return copy(delivery)
    }
    if (delivery.status === 'PENDING' && this.scenario === 'retryable' && this.consumeScenarioFailure('mail_delivery_get')) {
      delivery.attempts = Math.min(3, delivery.attempts + 1)
      delivery.error = copy(this.fixtures.errors.retryable)
      delivery.updated_at = '2026-01-15T15:00:00.000Z'
      this.persistExtensionState()
      return copy(delivery)
    }
    if (delivery.status === 'PENDING') delivery.attempts = Math.min(3, delivery.attempts + 1)
    if (this.scenario === 'terminal') {
      delivery.status = 'TERMINAL_FAILURE'
      delivery.error = copy(this.fixtures.errors.terminal)
    } else {
      delivery.status = 'DELIVERED'
      delivery.error = null
    }
    delivery.updated_at = '2026-01-15T15:00:00.000Z'
    this.persistExtensionState()
    return copy(delivery)
  }

  /** Mock-only template operations. The HTTP contract intentionally has no template CRUD. */
  async listMailTemplates (): Promise<MailTemplate[]> {
    this.requireCapability('mail_preview')
    return copy(Object.values(this.mailTemplates))
  }

  async updateMailTemplate (templateID: string, input: Pick<MailTemplate, 'name' | 'subject' | 'text' | 'html'>): Promise<MailTemplate> {
    this.requireCapability('mail_send')
    this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    const template = this.mailTemplates[templateID]
    if (!template) throw new C311ApiError(this.fixtures.errors['not-found'], 404)
    if (!input || !String(input.name || '').trim() || !String(input.subject || '').trim() || !String(input.text || '').trim() || !String(input.html || '').trim()) this.failScenario('validation')
    const updated = { ...template, name: input.name, subject: input.subject, text: input.text, html: input.html, version: template.version + 1, updated_at: '2026-01-15T15:00:00.000Z' }
    this.mailTemplates[templateID] = copy(updated)
    this.countWrite('mail_template_update')
    this.persistExtensionState()
    return copy(updated)
  }

  async listReportCatalogue (query: ListQuery = {}): Promise<PageResponse<ReportCatalogueItem>> {
    this.requireCapability('report_catalogue'); this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    return this.page(this.scenario === 'empty' ? [] : REPORT_CATALOGUE, query)
  }

  async shareReport (reportID: string, input: ReportShare, options: C311RequestOptions = {}): Promise<ReportDefinition> {
    this.requireCapability('saved_report_share'); this.failIfNeeded(['forbidden', 'not-found', 'validation', 'expected-version-required'])
    const report = await this.getReport(reportID)
    this.requireReportOwner(report)
    if (options.expectedVersion === undefined) this.failScenario('expected-version-required')
    this.failVersionConflictOnce('report_share', report.version)
    if (options.expectedVersion !== report.version) this.failScenario('version-conflict')
    if (!input.roles.length || new Set(input.roles).size !== input.roles.length || input.roles.some(role => !APPLICATION_ROLES.includes(role))) this.failScenario('validation')
    if (!this.isPlatformAdministrator()) {
      const owner = this.currentSession.actor
      const expandsScope = input.roles.some(role => {
        const target = this.fixtures.role_fixtures[role]?.session.actor
        if (!owner || !target) return true
        return target.department_codes.some(code => !owner.department_codes.includes(code)) || target.district_codes.some(code => !owner.district_codes.includes(code))
      })
      if (expandsScope) throw new C311ApiError({ error: 'FORBIDDEN', message: 'A shared report cannot expand the owner scope.', retryable: false }, 403)
    }
    this.reportShares[reportID] = copy(input.roles)
    this.fixtures.report_shares = copy(this.reportShares)
    const updated = { ...report, version: report.version + 1, updated_at: '2026-01-15T15:00:00.000Z' }
    const index = this.fixtures.reports.findIndex(item => item.report_id === reportID)
    if (index >= 0) this.fixtures.reports[index] = copy(updated)
    this.countWrite('saved_report_share')
    return copy(updated)
  }

  async listAuditEvents (query: ListQuery & { filters?: AuditFilters } = {}): Promise<PageResponse<AuditEvent>> {
    this.requireCapability('audit_list'); this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    const filters = query.filters || {}
    this.validateAuditFilters(filters)
    const allowed = (event: AuditEvent): boolean => Object.entries(filters).every(([key, value]) => {
      if (key === 'occurred_from') return event.occurred_at >= String(value)
      if (key === 'occurred_to') return event.occurred_at <= String(value)
      if (key === 'request_id') return this.matchesFilter(String(event.after.request_id || event.before.request_id || ''), value)
      return this.matchesFilter((event as unknown as Record<string, unknown>)[key], value)
    })
    const items = (this.scenario === 'empty' ? [] : (this.fixtures.audit_events || [])).filter(event => this.auditInScope(event) && allowed(event))
    const sort = String(query.sort || '-occurred_at').split(',').map(value => value.trim()).filter(Boolean)
    const allowedSort = new Set(['audit_id', 'actor_id', 'actor_type', 'entity_type', 'entity_id', 'event_type', 'occurred_at', 'source_channel'])
    if (sort.length > 3 || sort.some(value => !allowedSort.has(value.replace(/^[+-]/, ''))) || new Set(sort.map(value => value.replace(/^[+-]/, ''))).size !== sort.length) {
      this.validationError('The audit sort is invalid.', [{ field: '/sort', code: sort.length > 3 ? 'TOO_MANY_ITEMS' : 'INVALID_VALUE' }])
    }
    const sorted = [...items].sort((left, right) => {
      for (const expression of sort) {
        const descending = expression.startsWith('-')
        const field = expression.replace(/^[+-]/, '') as keyof AuditEvent
        const leftValue = String(left[field] ?? '')
        const rightValue = String(right[field] ?? '')
        if (leftValue === rightValue) continue
        const result = leftValue.localeCompare(rightValue)
        return descending ? -result : result
      }
      return String(left.audit_id || '').localeCompare(String(right.audit_id || ''))
    })
    const pageSize = query.page_size === undefined ? 50 : Number(query.page_size)
    if (!Number.isInteger(pageSize) || pageSize < 1 || pageSize > 100) this.failScenario('validation')
    const sortContext = sort.join(',')
    const tokenFingerprint = this.fingerprint({ page_size: pageSize, filters, sort: sortContext })
    const token = query.page_token === undefined ? '' : String(query.page_token)
    let offset = 0
    if (token) {
      const tokenState = this.auditPageTokens.get(token)
      if (!tokenState || tokenState.fingerprint !== tokenFingerprint) throw new C311ApiError({ error: 'INVALID_PAGE_TOKEN', message: 'The page token is invalid for this query.', retryable: false }, 400)
      offset = tokenState.offset
    }
    const page = sorted.slice(offset, offset + pageSize)
    let nextPageToken: string | null = null
    if (offset + pageSize < sorted.length) {
      nextPageToken = `audit-page-${++this.auditPageTokenSerial}`
      this.auditPageTokens.set(nextPageToken, { fingerprint: tokenFingerprint, offset: offset + pageSize })
    }
    return { items: copy(page), next_page_token: nextPageToken, total_count: sorted.length, applied_filters: copy(filters), sort }
  }

  async exportAuditEvents (filters: AuditFilters): Promise<Operation> {
    this.requireCapability('audit_export'); this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    this.validateAuditFilters(filters)
    const events = (this.fixtures.audit_events || []).filter(event => this.auditInScope(event) && Object.entries(filters || {}).every(([key, value]) => {
      if (key === 'occurred_from') return event.occurred_at >= String(value)
      if (key === 'occurred_to') return event.occurred_at <= String(value)
      if (key === 'request_id') return this.matchesFilter(String(event.after.request_id || event.before.request_id || ''), value)
      return this.matchesFilter((event as unknown as Record<string, unknown>)[key], value)
    }))
    const headers = ['audit_id', 'actor_id', 'actor_type', 'entity_type', 'entity_id', 'event_type', 'occurred_at', 'source_channel', 'before', 'after']
    const body = `${[headers, ...events.map(event => [event.audit_id || '', event.actor_id, event.actor_type, event.entity_type, event.entity_id, event.event_type, event.occurred_at, event.source_channel, JSON.stringify(event.before || {}), JSON.stringify(event.after || {})])].map(row => row.map(value => this.csvCell(value)).join(',')).join('\r\n')}\r\n`
    this.createOperation('operation-fixture-audit-export', 'audit_export', { exported: events.length, filters: copy(filters), download_url: '/mock-downloads/audit-fixture.csv', content_type: 'text/csv;charset=utf-8', body })
    return { ...copy(this.operations.get('operation-fixture-audit-export')!), status: 'PENDING', progress: 0, result: null, completed_at: null }
  }

  private validateAuditFilters (filters: AuditFilters): void {
    const allowed = new Set(['actor_id', 'actor_type', 'entity_id', 'entity_type', 'event_type', 'occurred_from', 'occurred_to', 'request_id', 'source_channel'])
    if (!filters || typeof filters !== 'object' || Array.isArray(filters) || Object.keys(filters).some(key => !allowed.has(key))) this.failScenario('validation')
    for (const [key, value] of Object.entries(filters || {})) {
      if (['occurred_from', 'occurred_to'].includes(key)) {
        if (!validISODateTime(value)) this.failScenario('validation')
      } else if (!Array.isArray(value) || value.length < 1 || value.some(item => !String(item).trim())) this.failScenario('validation')
      if (Array.isArray(value) && new Set(value.map(item => String(item))).size !== value.length) this.validationError('Audit filter values must be unique.', [{ field: `/filters/${key}`, code: 'DUPLICATE' }])
      if (key === 'actor_type' && (value as unknown[]).some(item => !AUDIT_ACTOR_TYPES.includes(String(item) as typeof AUDIT_ACTOR_TYPES[number]))) this.failScenario('validation')
      if (key === 'source_channel' && (value as unknown[]).some(item => !SOURCE_CHANNELS.includes(String(item) as typeof SOURCE_CHANNELS[number]))) this.failScenario('validation')
    }
    if (filters?.occurred_from && filters?.occurred_to && String(filters.occurred_from) > String(filters.occurred_to)) this.failScenario('validation')
  }

  async exportContactEmails (input: ContactEmailExportRequest): Promise<Operation> {
    this.requireCapability('contact_email_export')
    this.failIfNeeded(['forbidden', 'not-found', 'validation'])
    if (!input || typeof input.filters !== 'object' || Array.isArray(input.filters)) this.failScenario('validation')
    const filters = input.filters || {}
    const allowed = new Set(['email', 'department', 'district', 'primary_category', 'preferred_language'])
    const invalid = Object.keys(filters).find(key => !allowed.has(key))
    if (invalid) throw new C311ApiError({ error: 'VALIDATION_ERROR', message: `Filter ${invalid} is not supported.`, retryable: false, errors: [{ field: `/filters/${invalid}`, code: 'INVALID_VALUE' }] }, 422)
    const rows: string[][] = []
    const seen = new Set<string>()
    this.fixtures.requests.forEach(request => {
      const contact = request.primary_requester
      if (!this.requestInScope(request) || contact.email_opt_out !== false || !this.verifiedEmails(contact).length) return
      const emails = this.verifiedEmails(contact).filter(email => {
        if (filters.email && !this.matchesFilter(email, filters.email)) return false
        if (filters.department && !this.matchesFilter(request.owning_department, filters.department)) return false
        if (filters.district && !this.matchesFilter(request.council_district || '', filters.district)) return false
        if (filters.primary_category && !this.matchesFilter(contact.primary_category, filters.primary_category)) return false
        if (filters.preferred_language && !this.matchesFilter(contact.preferred_language, filters.preferred_language)) return false
        return true
      })
      emails.forEach(email => {
        if (seen.has(email)) return
        seen.add(email)
        rows.push([email, contact.display_name, contact.primary_category, contact.preferred_language, 'false'])
      })
    })
    rows.sort((left, right) => left[0].localeCompare(right[0]))
    const body = `${[['email', 'display_name', 'primary_category', 'preferred_language', 'opt_out'], ...rows].map(row => row.map(value => this.csvCell(value)).join(',')).join('\r\n')}\r\n`
    const contacts = rows.map(row => ({ email: row[0], display_name: row[1], primary_category: row[2], preferred_language: row[3], opt_out: false }))
    const result = { exported_count: rows.length, filters: copy(filters), contacts, download_url: '/mock-downloads/contact-emails.csv', content_type: 'text/csv;charset=utf-8', body }
    this.createOperation('operation-fixture-contact-email-export', 'contact_email_export', result)
    this.countWrite('contact_email_export')
    return { ...copy(this.operations.get('operation-fixture-contact-email-export')!), status: 'PENDING', progress: 0, result: null, completed_at: null }
  }

  async exportData (entity: 'audit-events' | 'constituents' | 'follow-up-actions' | 'service-requests', query: DataExportQuery = {}): Promise<ExportResponse> {
    if (!this.currentSession.authenticated || (this.currentSession.expires_at && Date.parse(this.currentSession.expires_at) <= Date.now())) throw new C311ApiError({ error: 'UNAUTHENTICATED', message: 'Authentication is required.', retryable: false }, 401)
    if (!this.currentSession.actor?.scopes.includes('crm.export')) throw new C311ApiError({ error: 'FORBIDDEN', message: 'Export scope required.', retryable: false }, 403)
    this.failIfNeeded(['forbidden', 'validation', 'rate-limited'], '60')
    if (!['audit-events', 'constituents', 'follow-up-actions', 'service-requests'].includes(entity)) throw new C311ApiError({ error: 'INVALID_FILTER', message: 'The export entity is not supported.', retryable: false, errors: [{ field: '/entity', code: 'INVALID_VALUE' }] }, 422)
    if (query.page_size !== undefined && (!Number.isInteger(query.page_size) || query.page_size < 1 || query.page_size > 100)) throw new C311ApiError({ error: 'INVALID_FILTER', message: 'page_size must be between 1 and 100.', retryable: false, errors: [{ field: '/page_size', code: 'OUT_OF_RANGE' }] }, 422)
    if (query.updated_since && !validISODateTime(query.updated_since)) throw new C311ApiError({ error: 'INVALID_FILTER', message: 'updated_since must be an ISO date-time.', retryable: false, errors: [{ field: '/updated_since', code: 'INVALID_FORMAT' }] }, 422)
    const rawFilters = query.filters || {}
    if (typeof rawFilters !== 'object' || Array.isArray(rawFilters)) throw new C311ApiError({ error: 'INVALID_FILTER', message: 'filters must be an object.', retryable: false, errors: [{ field: '/filters', code: 'INVALID_VALUE' }] }, 422)
    const allowedFilters: Record<string, string[]> = {
      constituents: ['constituent_id', 'email', 'department', 'district', 'primary_category', 'preferred_language', 'email_opt_out'],
      'service-requests': ['request_id', 'request_number', 'status', 'service_type', 'department', 'district', 'origin_class', 'source_channel', 'category', 'duplicate_group'],
      'audit-events': ['actor_id', 'actor_type', 'entity_id', 'entity_type', 'event_type', 'request_id', 'source_channel'],
      'follow-up-actions': ['request_id', 'action_type', 'actor', 'visibility'],
    }
    const invalidFilter = Object.keys(rawFilters).find(key => !allowedFilters[entity].includes(key))
    if (invalidFilter) throw new C311ApiError({ error: 'INVALID_FILTER', message: `Filter ${invalidFilter} is not supported for ${entity}.`, retryable: false, errors: [{ field: `/filters/${invalidFilter}`, code: 'INVALID_VALUE' }] }, 422)
    if (Object.entries(rawFilters).some(([, value]) => value === '' || (Array.isArray(value) && value.length === 0))) throw new C311ApiError({ error: 'INVALID_FILTER', message: 'Filter values must not be empty.', retryable: false }, 422)
    if (Object.values(rawFilters).some(value => value !== null && typeof value === 'object' && !Array.isArray(value))) throw new C311ApiError({ error: 'INVALID_FILTER', message: 'Filter values must be scalar or arrays.', retryable: false }, 422)
    const validateEnumFilter = (key: string, values: readonly string[]): void => {
      const value = rawFilters[key]
      if (value === undefined) return
      const items = Array.isArray(value) ? value : [value]
      if (items.some(item => !values.includes(String(item)))) throw new C311ApiError({ error: 'INVALID_FILTER', message: `Filter ${key} contains an invalid value.`, retryable: false }, 422)
      if (Array.isArray(value) && new Set(value.map(item => String(item))).size !== value.length) throw new C311ApiError({ error: 'INVALID_FILTER', message: `Filter ${key} contains duplicate values.`, retryable: false }, 422)
    }
    if (entity === 'service-requests') {
      validateEnumFilter('status', SERVICE_REQUEST_STATUSES)
      validateEnumFilter('service_type', SERVICE_TYPES)
      validateEnumFilter('department', DEPARTMENT_CODES)
      validateEnumFilter('district', DISTRICT_CODES)
      validateEnumFilter('origin_class', ORIGIN_CLASSES)
      validateEnumFilter('source_channel', SOURCE_CHANNELS)
    }
    if (entity === 'constituents') {
      validateEnumFilter('primary_category', CONTACT_CATEGORIES)
      validateEnumFilter('preferred_language', LANGUAGES)
      const optOut = rawFilters.email_opt_out
      const optOutValues = Array.isArray(optOut) ? optOut : optOut === undefined ? [] : [optOut]
      if (optOutValues.some(value => typeof value !== 'boolean' && value !== 'true' && value !== 'false')) throw new C311ApiError({ error: 'INVALID_FILTER', message: 'email_opt_out must be boolean.', retryable: false }, 422)
    }
    type ExportSource = { id: string, row: Record<string, unknown>, request?: ServiceRequest }
    const source: ExportSource[] = []
    if (entity === 'service-requests') this.fixtures.requests.filter(request => this.requestInScope(request)).forEach(request => source.push({ id: request.request_id, row: copy(request) as unknown as Record<string, unknown>, request }))
    if (entity === 'constituents') {
      const seen = new Set<string>()
      this.fixtures.requests.filter(request => this.requestInScope(request)).forEach(request => {
        const contact = request.primary_requester
        if (seen.has(contact.constituent_id)) return
        seen.add(contact.constituent_id)
        source.push({ id: contact.constituent_id, row: copy(contact) as unknown as Record<string, unknown>, request })
      })
    }
    if (entity === 'audit-events') (this.fixtures.audit_events || []).filter(event => this.auditInScope(event)).forEach(event => source.push({
      id: event.audit_id || event.entity_id,
      row: {
        ...(event.audit_id ? { audit_id: event.audit_id } : {}),
        actor_id: event.actor_id,
        actor_type: event.actor_type,
        entity_type: event.entity_type,
        entity_id: event.entity_id,
        event_type: event.event_type,
        occurred_at: event.occurred_at,
        source_channel: event.source_channel,
        before: copy(event.before),
        after: copy(event.after),
      },
      request: this.requestForAudit(event),
    }))
    if (entity === 'follow-up-actions') (this.fixtures.follow_up_actions || []).forEach(action => {
      const request = this.fixtures.requests.find(item => item.request_id === action.request_id)
      if (this.requestInScope(request)) source.push({ id: `${action.request_id}:${action.occurred_at}:${action.action_type}`, row: copy(action) as unknown as Record<string, unknown>, request })
    })
    const matches = (sourceItem: ExportSource, key: string, expected: unknown): boolean => {
      const row = sourceItem.row
      const candidate = key === 'email' ? row.emails : key === 'request_id' && row.request_id === undefined ? sourceItem.request?.request_id : key === 'department' ? sourceItem.request?.owning_department : key === 'district' ? sourceItem.request?.council_district : key === 'category' ? sourceItem.request?.primary_requester.primary_category : key === 'duplicate_group' ? sourceItem.request?.duplicate_group_id : row[key]
      return this.matchesFilter(candidate, expected)
    }
    let filtered = source.filter(item => Object.entries(rawFilters).every(([key, expected]) => matches(item, key, expected)))
    if (query.updated_since) filtered = filtered.filter(item => String(item.row.updated_at || item.row.occurred_at || item.row.created_at || '') >= query.updated_since!)
    filtered.sort((left, right) => left.id.localeCompare(right.id))
    const pageSize = query.page_size || 50
    const tokenFingerprint = this.fingerprint({
      entity,
      page_size: pageSize,
      filters: rawFilters,
      updated_since: query.updated_since || null,
      sort: query.sort || null,
    })
    let offset = 0
    if (query.page_token) {
      const tokenState = this.exportPageTokens.get(query.page_token)
      if (!tokenState || tokenState.fingerprint !== tokenFingerprint) throw new C311ApiError({ error: 'INVALID_PAGE_TOKEN', message: 'The page token is invalid for this query.', retryable: false }, 400)
      offset = tokenState.offset
    }
    const items = filtered.slice(offset, offset + pageSize).map(item => item.row)
    let nextPageToken: string | null = null
    if (offset + pageSize < filtered.length) {
      nextPageToken = `opaque-next-${++this.exportPageTokenSerial}`
      this.exportPageTokens.set(nextPageToken, { fingerprint: tokenFingerprint, offset: offset + pageSize })
    }
    return { generated_at: '2026-01-15T15:00:00.000Z', items, next_page_token: nextPageToken }
  }
}
