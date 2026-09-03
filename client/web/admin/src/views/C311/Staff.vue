<template>
  <c311-app-shell mode="staff" :brand="t('staff.brand', 'City 311 staff')" :title="t('staff.title', 'Requests')" :status-message="statusMessage">
    <template #nav>
      <div class="d-flex flex-wrap align-items-center gap-2">
        <c311-main-nav :items="navItems" :label="t('navigation.staff', 'Staff navigation')" />
        <c311-help-drawer help-key="staff.request.triage" :label="t('help.label', 'Help')" :title="t('help.title', 'Help')" :close-label="t('action.close', 'Close')" :content="t('help.staff.request.triage', 'Review and classify a request.')" />
        <c311-language-selector :actor-id="actorID" />
      </div>
    </template>

    <c311-data-state :state="state" :error="dataError" @retry="load">
      <template #populated>
        <c311-responsive-data
          :items="items"
          :columns="translatedColumns"
          row-key="request_id"
          :label="t('staff.queue', 'Staff request queue')"
          selectable
          :action-label="t('action.viewRequest', 'View request')"
          @select="selectRequest"
        />
      </template>
    </c311-data-state>

    <section v-if="selectedRequest" class="mt-4" data-c311-staff-request-detail>
      <h2>{{ t('staff.requestDetail', 'Request details') }}</h2>
      <c311-data-state v-if="detailState !== 'populated'" :state="detailState" :error="detailError" @retry="selectRequest(selectedRequest)" />
      <div v-else>
        <p><strong>{{ t('field.requestNumber', 'Request number') }}:</strong> {{ selectedRequest.request_number }}</p>
        <section aria-labelledby="c311-relationships-heading">
          <h3 id="c311-relationships-heading">{{ t('staff.relationships', 'Constituent relationships') }}</h3>
          <ul data-c311-relationships>
            <li v-for="relationship in relationships" :key="`${relationship.constituent_id}-${relationship.relationship_type}`">
              <span>{{ relationship.constituent_id }} · {{ relationship.relationship_type }} · {{ relationship.portal_visible ? t('staff.portalVisible', 'portal visible') : t('staff.internalOnly', 'internal only') }} · {{ relationship.notify_status ? t('staff.notifyStatus', 'notify status') : t('staff.noNotifyStatus', 'do not notify') }}</span>
              <small v-if="relationship.notification_target" class="d-block" data-c311-relationship-notification-target>{{ t('staff.notificationTarget', 'Notification target') }}: {{ relationship.notification_target }}</small>
              <small v-if="relationship.notification_result" class="d-block" data-c311-relationship-notification-result>{{ t('staff.notificationResult', 'Notification result') }}: {{ relationship.notification_result }}</small>
              <small v-for="event in relationship.audit || []" :key="event.audit_id" class="d-block" data-c311-relationship-audit>{{ t('staff.relationshipAudit', 'Relationship audit') }}: {{ event.action }} · {{ event.actor_id }} · {{ formatDate(event.occurred_at) }}</small>
              <button v-if="canUnlink && relationship.relationship_type !== 'PRIMARY_REQUESTER'" class="btn btn-link p-0 ml-2" type="button" :data-c311-action="`unlink-constituent-${relationship.constituent_id}`" :disabled="relationshipBusy" @click="unlinkConstituent(relationship)">{{ t('action.unlink', 'Unlink') }}</button>
            </li>
          </ul>
          <section v-if="relationshipAuditEvents.length" class="mt-3" data-c311-relationship-audit-events aria-labelledby="c311-relationship-audit-events-heading">
            <h4 id="c311-relationship-audit-events-heading">{{ t('staff.relationshipAuditEvents', 'Relationship audit events') }}</h4>
            <ol>
              <li v-for="event in relationshipAuditEvents" :key="`${event.audit_id}-${event.action}`">{{ event.action }} · {{ event.actor_id }} · {{ formatDate(event.occurred_at) }}</li>
            </ol>
          </section>
          <form v-if="canLink" class="border rounded p-2" data-c311-form="link-constituent" @submit.prevent="linkConstituent">
            <label for="c311-constituent-id">{{ t('field.constituentId', 'Constituent ID') }}</label>
            <input id="c311-constituent-id" v-model.trim="relationshipForm.constituent_id" class="form-control" required>
            <label for="c311-relationship-type" class="mt-2">{{ t('field.relationshipType', 'Relationship type') }}</label>
            <select id="c311-relationship-type" v-model="relationshipForm.relationship_type" class="form-control">
              <option v-for="type in relationshipTypes" :key="type" :value="type">{{ type }}</option>
            </select>
            <label class="mt-2"><input v-model="relationshipForm.portal_visible" type="checkbox"> {{ t('field.portalVisible', 'Visible in portal') }}</label>
            <label class="ml-3"><input v-model="relationshipForm.notify_status" type="checkbox"> {{ t('field.notifyStatus', 'Notify on status changes') }}</label>
            <button class="btn btn-primary mt-2" type="submit" data-c311-action="link-constituent" :disabled="relationshipBusy">{{ relationshipBusy ? t('action.working', 'Working…') : t('action.link', 'Link constituent') }}</button>
          </form>
        </section>
        <section class="mt-4" aria-labelledby="c311-notes-heading">
          <h3 id="c311-notes-heading">{{ t('staff.notes', 'Notes') }}</h3>
          <ol data-c311-notes>
            <li v-for="note in notes" :key="note.note_id">{{ note.body }} · {{ note.portal_visible ? t('staff.portalVisible', 'portal visible') : t('staff.internalOnly', 'internal only') }} · {{ formatDate(note.created_at) }}</li>
          </ol>
          <form v-if="canCreateNote" class="border rounded p-2" data-c311-form="create-note" @submit.prevent="createNote">
            <label for="c311-note-body">{{ t('field.note', 'Note') }}</label>
            <textarea id="c311-note-body" v-model.trim="noteForm.body" class="form-control" maxlength="2000" required />
            <label class="mt-2"><input v-model="noteForm.portal_visible" type="checkbox"> {{ t('field.portalVisible', 'Visible in portal') }}</label>
            <button class="btn btn-primary mt-2" type="submit" data-c311-action="create-note" :disabled="noteBusy">{{ noteBusy ? t('action.working', 'Working…') : t('action.addNote', 'Add note') }}</button>
          </form>
        </section>
        <p v-if="detailMessage" class="alert alert-info" role="status" data-c311-detail-message>{{ detailMessage }}</p>
      </div>
    </section>
  </c311-app-shell>
</template>

<script>
import * as C311JS from '@cortezaproject/corteza-js'
import { components, c311 } from '@cortezaproject/corteza-vue'

const {
  C311AppShell,
  C311DataState,
  C311HelpDrawer,
  C311LanguageSelector,
  C311MainNav,
  C311ResponsiveData,
} = components
const c311StateForError = c311?.c311StateForError
function formatDate (value) {
  try {
    return typeof C311JS.formatC311DateTime === 'function' ? C311JS.formatC311DateTime(value) : value || ''
  } catch (_error) {
    return value || ''
  }
}

export default {
  name: 'C311Staff',
  components: {
    C311AppShell,
    C311DataState,
    C311HelpDrawer,
    C311LanguageSelector,
    C311MainNav,
    C311ResponsiveData,
  },
  data: () => ({
    state: 'loading',
    dataError: null,
    statusMessage: '',
    items: [],
    selectedRequest: null,
    detailState: 'empty',
    detailError: null,
    detailMessage: '',
    relationships: [],
    audit: [],
    notes: [],
    relationshipBusy: false,
    noteBusy: false,
    relationshipForm: { constituent_id: '', relationship_type: 'AFFECTED_RESIDENT', portal_visible: true, notify_status: false },
    noteForm: { body: '', portal_visible: false },
    relationshipTypes: ['PRIMARY_REQUESTER', 'AFFECTED_RESIDENT', 'PROPERTY_OWNER', 'REPORTER', 'ORGANISATION_CONTACT'],
    columns: [
      { key: 'request_number', labelKey: 'field.requestNumber' },
      { key: 'summary', labelKey: 'field.summary' },
      { key: 'status', labelKey: 'field.status' },
      { key: 'owning_department', labelKey: 'field.department' },
      { key: 'updated_at', labelKey: 'field.updated', format: value => formatDate(value) },
    ],
  }),
  computed: {
    actorID () {
      return this.$C311 && this.$C311.session && this.$C311.session.actor ? this.$C311.session.actor.actor_id : ''
    },
    navItems () {
      return [
        { route: '/c311/staff', label: this.t('navigation.requests', 'Requests'), capability: 'staff_request_queue' },
        { route: '/c311/staff/reports', label: this.t('navigation.reports', 'Reports'), capability: 'report_catalogue' },
        { route: '/c311/staff/workflows', label: this.t('navigation.workflows', 'Workflows'), capability: 'workflow_list', scope: 'workflow.execute' },
      ]
    },
    translatedColumns () {
      return this.columns.map(column => ({ ...column, label: this.t(column.labelKey, column.labelKey) }))
    },
    canCapability () {
      return capability => typeof this.$C311?.can === 'function' ? this.$C311.can(capability) : this.$C311?.session?.actor?.capabilities?.includes(capability)
    },
    canLink () { return this.canCapability('staff_constituent_link') },
    canUnlink () { return this.canCapability('staff_constituent_unlink') },
    canCreateNote () { return this.canCapability('staff_note_create') },
    relationshipAuditEvents () { return this.audit.filter(event => ['LINKED', 'UNLINKED', 'UPDATED'].includes(event.action)) },
  },
  created () {
    this.load()
  },
  methods: {
    formatDate (value) { return formatDate(value) },
    t (key, fallback) {
      const translated = this.$t?.(`c311:${key}`)
      return translated && translated !== `c311:${key}` && translated !== key ? translated : fallback
    },
    async load () {
      this.state = 'loading'
      this.statusMessage = this.t('status.loadingRequests', 'Loading requests.')
      try {
        const provider = this.$C311?.provider
        const page = await provider?.listStaffRequests?.()
        this.items = page?.items || []
        this.dataError = null
        this.state = this.items.length ? 'populated' : 'empty'
        this.statusMessage = this.items.length ? this.t('status.requestsLoaded', 'Requests loaded.') : this.t('status.noRequests', 'No requests found.')
      } catch (error) {
        this.dataError = error
        this.state = c311StateForError?.(error) || (error?.retryable ? 'retryable-error' : 'terminal-error')
        this.statusMessage = this.t('status.requestListUnavailable', 'Request list unavailable.')
      }
    },
    async selectRequest (request) {
      if (!request || !this.$C311?.provider?.getStaffRequest) return
      this.selectedRequest = request
      this.detailState = 'loading'
      this.detailError = null
      this.detailMessage = ''
      try {
        const detail = await this.$C311.provider.getStaffRequest(request.request_id)
        this.relationships = detail?.relationships || []
        this.audit = detail?.audit || []
        this.notes = detail?.notes || []
        this.selectedRequest = detail?.request ? { ...request, ...detail.request } : request
        this.detailState = 'populated'
      } catch (error) {
        this.detailError = error
        this.detailState = c311StateForError?.(error) || (error?.retryable ? 'retryable-error' : 'terminal-error')
      }
    },
    async linkConstituent () {
      if (!this.canLink || this.relationshipBusy || !this.selectedRequest?.request_id) return
      this.relationshipBusy = true
      this.detailMessage = ''
      try {
        const detail = await this.$C311.provider.linkStaffConstituent(this.selectedRequest.request_id, { ...this.relationshipForm }, { expectedVersion: this.selectedRequest.version || 1 })
        this.relationships = detail?.relationships || this.relationships.concat({ ...this.relationshipForm })
        this.audit = detail?.audit || this.audit
        if (detail?.request) this.selectedRequest = { ...this.selectedRequest, ...detail.request }
        this.detailMessage = this.t('status.relationshipLinked', 'Constituent relationship linked.')
        this.relationshipForm.constituent_id = ''
      } catch (error) {
        this.detailError = error
        this.detailState = c311StateForError?.(error) || (error?.retryable ? 'retryable-error' : 'terminal-error')
      } finally { this.relationshipBusy = false }
    },
    async unlinkConstituent (relationship) {
      if (!this.canUnlink || this.relationshipBusy || !this.selectedRequest?.request_id) return
      this.relationshipBusy = true
      this.detailMessage = ''
      try {
        const detail = await this.$C311.provider.unlinkStaffConstituent(this.selectedRequest.request_id, relationship.constituent_id, { reason: this.t('status.relationshipUnlinkReason', 'Removed by staff.') }, { expectedVersion: this.selectedRequest.version || 1 })
        this.relationships = detail?.relationships || this.relationships.filter(item => item !== relationship)
        this.audit = detail?.audit || this.audit
        if (detail?.request) this.selectedRequest = { ...this.selectedRequest, ...detail.request }
        this.detailMessage = this.t('status.relationshipUnlinked', 'Constituent relationship removed.')
      } catch (error) {
        this.detailError = error
        this.detailState = c311StateForError?.(error) || (error?.retryable ? 'retryable-error' : 'terminal-error')
      } finally { this.relationshipBusy = false }
    },
    async createNote () {
      if (!this.canCreateNote || this.noteBusy || !this.selectedRequest?.request_id) return
      this.noteBusy = true
      this.detailMessage = ''
      try {
        const note = await this.$C311.provider.createStaffNote(this.selectedRequest.request_id, { ...this.noteForm })
        this.notes = this.notes.concat(note)
        this.noteForm.body = ''
        this.detailMessage = this.t('status.noteAdded', 'Note added.')
      } catch (error) {
        this.detailError = error
        this.detailState = c311StateForError?.(error) || (error?.retryable ? 'retryable-error' : 'terminal-error')
      } finally { this.noteBusy = false }
    },
  },
}
</script>
