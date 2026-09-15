<template>
  <c311-app-shell :title="t('Staff requests')" main-id="c311-staff-main">
    <template #nav>
      <c311-main-nav :items="navItems" aria-label="Staff navigation" />
    </template>

    <c311-data-state v-if="state !== 'populated'" :state="state" :error="error" @retry="loadQueue" />
    <template v-else>
      <section aria-labelledby="c311-queue-heading">
        <div class="d-flex justify-content-between align-items-center mb-3">
          <h1 id="c311-queue-heading" class="h3 mb-0">{{ t('Request queue') }}</h1>
          <router-link class="btn btn-primary" to="/c311/staff/submit">{{ t('Create request') }}</router-link>
        </div>
        <form class="form-row mb-3" @submit.prevent="loadQueue">
          <div class="col-md-2"><label for="c311-filter-status">Status</label><select id="c311-filter-status" v-model="filters.status" class="form-control"><option value="">All</option><option v-for="status in statuses" :key="status" :value="status">{{ status }}</option></select></div>
          <div class="col-md-3"><label for="c311-filter-service">Service type</label><input id="c311-filter-service" v-model.trim="filters.service_type" class="form-control"></div>
          <div class="col-md-3"><label for="c311-filter-department">Department</label><input id="c311-filter-department" v-model.trim="filters.department" class="form-control"></div>
          <div class="col-md-2 d-flex align-items-end"><button class="btn btn-outline-primary" type="submit" :disabled="busy">Apply filters</button></div>
        </form>
        <p v-if="!items.length" class="text-muted" role="status">{{ t('No matching requests.') }}</p>
        <div v-else class="table-responsive">
          <table class="table table-hover" data-c311-staff-queue>
            <thead><tr><th>Request</th><th>Summary</th><th>Status</th><th>Department</th><th /></tr></thead>
            <tbody><tr v-for="item in items" :key="item.request_id">
              <td>{{ item.request_number }}</td><td>{{ item.summary }}</td><td>{{ item.status }}</td><td>{{ item.owning_department }}</td>
              <td><button class="btn btn-sm btn-outline-primary" type="button" @click="selectRequest(item)">{{ t('Open') }}</button></td>
            </tr></tbody>
          </table>
        </div>
        <button v-if="nextPageToken" class="btn btn-outline-secondary mt-2" type="button" :disabled="busy" @click="loadQueue(true)">Load more</button>
      </section>

      <c311-error-summary v-if="formErrors.length && !detail" :errors="formErrors" title="Could not open request" />
      <section v-if="detail" class="mt-4" aria-labelledby="c311-request-heading">
        <h2 id="c311-request-heading" class="h4">{{ detail.request.request_number }} {{ detail.request.summary }}</h2>
        <p><strong>Status:</strong> {{ detail.request.status }} <strong class="ml-3">Department:</strong> {{ detail.request.owning_department }}</p>
        <dl class="row small"><dt class="col-sm-3">Primary requester</dt><dd class="col-sm-9">{{ detail.request.primary_requester.display_name }} ({{ detail.request.primary_requester.constituent_id }})</dd><dt class="col-sm-3">Location</dt><dd class="col-sm-9">{{ detail.request.location && detail.request.location.address ? detail.request.location.address.line1 : 'Not provided' }}</dd><dt class="col-sm-3">Primary assignee</dt><dd class="col-sm-9">{{ detail.primary_assignee_id || 'Unassigned' }}</dd><dt class="col-sm-3">Collaborators</dt><dd class="col-sm-9">{{ (detail.collaborator_ids || []).join(', ') || 'None' }}</dd><dt class="col-sm-3">Origin</dt><dd class="col-sm-9">{{ detail.request.origin_class }} via {{ detail.request.source_channel }}</dd><dt class="col-sm-3">Duplicate group</dt><dd class="col-sm-9">{{ detail.request.duplicate_group_id || 'None' }}</dd></dl>
        <c311-error-summary :errors="formErrors" :field-targets="fieldTargets" title="Review this operation" />
        <div v-if="message" class="alert alert-success" role="status">{{ message }}</div>

        <div class="row">
          <form class="col-md-6 mb-3" @submit.prevent="transition">
            <h3 class="h5">Change status</h3>
            <label for="c311-transition-status">New status</label>
            <select id="c311-transition-status" v-model="transitionForm.to_status" class="form-control"><option v-for="status in statuses" :key="status" :value="status">{{ status }}</option></select>
            <label class="mt-2" for="c311-transition-reason">Reason</label><textarea id="c311-transition-reason" v-model.trim="transitionForm.reason" class="form-control" rows="2" />
            <button class="btn btn-primary mt-2" type="submit" :disabled="busy">Update status</button>
          </form>
          <form class="col-md-6 mb-3" @submit.prevent="reassign">
            <h3 class="h5">Reassign</h3>
            <label for="c311-assignee">Staff ID</label><input id="c311-assignee" v-model.trim="assignmentForm.assignee_id" class="form-control">
            <label class="mt-2" for="c311-assignment-reason">Reason</label><textarea id="c311-assignment-reason" v-model.trim="assignmentForm.reason" class="form-control" rows="2" />
            <button class="btn btn-outline-primary mt-2" type="submit" :disabled="busy">Reassign request</button>
          </form>
          <form class="col-md-6 mb-3" @submit.prevent="addNote">
            <h3 class="h5">Add staff note</h3>
            <label for="c311-staff-note">Note</label><textarea id="c311-staff-note" v-model.trim="noteForm.body" class="form-control" rows="3" />
            <div class="form-check mt-2"><input id="c311-note-public" v-model="noteForm.portal_visible" class="form-check-input" type="checkbox"><label for="c311-note-public" class="form-check-label">Visible in portal</label></div>
            <button class="btn btn-outline-primary mt-2" type="submit" :disabled="busy">Add note</button>
          </form>
          <form class="col-md-6 mb-3" @submit.prevent="createReminder">
            <h3 class="h5">Create reminder</h3>
            <label for="c311-reminder-title">Title</label><input id="c311-reminder-title" v-model.trim="reminderForm.title" class="form-control">
            <label class="mt-2" for="c311-reminder-due">Due at</label><input id="c311-reminder-due" v-model="reminderForm.due_at" class="form-control" type="datetime-local" required>
            <label class="mt-2" for="c311-reminder-recipient">Recipient staff ID</label><input id="c311-reminder-recipient" v-model.trim="reminderForm.recipient_staff_id" class="form-control">
            <button class="btn btn-outline-primary mt-2" type="submit" :disabled="busy">Create reminder</button>
          </form>
          <form class="col-md-6 mb-3" @submit.prevent="addCollaborator"><h3 class="h5">Collaborator</h3><label for="c311-collaborator">Staff ID</label><input id="c311-collaborator" v-model.trim="collaboratorForm.staff_id" class="form-control"><label class="mt-2" for="c311-collaborator-reason">Reason</label><textarea id="c311-collaborator-reason" v-model.trim="collaboratorForm.reason" class="form-control" rows="2" /><button class="btn btn-outline-primary mt-2" type="submit" :disabled="busy">Add collaborator</button></form>
          <form class="col-md-6 mb-3" @submit.prevent="linkConstituent"><h3 class="h5">Linked constituent</h3><label for="c311-constituent">Constituent ID</label><input id="c311-constituent" v-model.trim="relationshipForm.constituent_id" class="form-control"><label class="mt-2" for="c311-relationship-type">Relationship</label><select id="c311-relationship-type" v-model="relationshipForm.relationship_type" class="form-control"><option v-for="type in relationshipTypes" :key="type" :value="type">{{ type }}</option></select><p class="small text-muted mb-1">The primary requester is established when the request is created and cannot be changed here.</p><div class="form-check mt-2"><input id="c311-relationship-visible" v-model="relationshipForm.portal_visible" class="form-check-input" type="checkbox"><label for="c311-relationship-visible" class="form-check-label">Visible in portal</label></div><div class="form-check"><input id="c311-relationship-notify" v-model="relationshipForm.notify_status" class="form-check-input" type="checkbox"><label for="c311-relationship-notify" class="form-check-label">Notify status changes</label></div><button class="btn btn-outline-primary mt-2" type="submit" :disabled="busy">Link constituent</button></form>
          <form class="col-md-6 mb-3" @submit.prevent="overrideOrigin"><h3 class="h5">Origin classification</h3><label for="c311-origin-class">Origin class</label><select id="c311-origin-class" v-model="originForm.origin_class" class="form-control"><option value="EXTERNAL">EXTERNAL</option><option value="INTERNAL">INTERNAL</option></select><label class="mt-2" for="c311-origin-reason">Reason</label><textarea id="c311-origin-reason" v-model.trim="originForm.reason" class="form-control" rows="2" /><button class="btn btn-outline-primary mt-2" type="submit" :disabled="busy">Override origin</button></form>
          <form class="col-md-6 mb-3" @submit.prevent="setDuplicateGroup"><h3 class="h5">Same-issue group</h3><label for="c311-duplicate-group">Group ID</label><input id="c311-duplicate-group" v-model.trim="duplicateForm.duplicate_group_id" class="form-control"><label class="mt-2" for="c311-duplicate-reason">Reason</label><textarea id="c311-duplicate-reason" v-model.trim="duplicateForm.reason" class="form-control" rows="2" /><button class="btn btn-outline-primary mt-2" type="submit" :disabled="busy">Confirm group</button><button class="btn btn-link mt-2" type="button" :disabled="busy" @click="removeDuplicateGroup">Remove group</button></form>
        </div>
        <section class="mb-3" aria-labelledby="c311-reminders-heading"><h3 id="c311-reminders-heading" class="h5">Reminders</h3><label for="c311-reminder-snooze">Snooze until</label><input id="c311-reminder-snooze" v-model="reminderSnoozeAt" class="form-control form-control-sm w-auto" type="datetime-local"><p v-if="!(detail.reminders || []).length" class="text-muted">No reminders.</p><ul v-else class="list-group"><li v-for="reminder in detail.reminders" :key="reminder.reminder_id" class="list-group-item"><strong>{{ reminder.title }}</strong> {{ reminder.status }} due {{ reminder.due_at }} <button v-if="!['COMPLETED', 'CANCELLED'].includes(reminder.status)" class="btn btn-sm btn-link" type="button" :disabled="busy" @click="reminderAction(reminder, 'SNOOZE')">Snooze</button><button v-if="!['COMPLETED', 'CANCELLED'].includes(reminder.status)" class="btn btn-sm btn-link" type="button" :disabled="busy" @click="reminderAction(reminder, 'COMPLETE')">Complete</button><button v-if="!['COMPLETED', 'CANCELLED'].includes(reminder.status)" class="btn btn-sm btn-link" type="button" :disabled="busy" @click="reminderAction(reminder, 'CANCEL')">Cancel</button></li></ul></section>
        <section class="mb-3" aria-labelledby="c311-relationships-heading"><h3 id="c311-relationships-heading" class="h5">Constituent relationships</h3><ul v-if="detail.relationships && detail.relationships.length" class="list-group"><li v-for="link in detail.relationships" :key="`${link.constituent_id}-${link.relationship_type}`" class="list-group-item">{{ link.constituent_id }} - {{ link.relationship_type }} <button class="btn btn-sm btn-link" type="button" :disabled="busy" @click="unlinkConstituent(link)">Remove</button></li></ul><p v-else class="text-muted">No linked constituents.</p></section>
        <section class="mb-3" aria-labelledby="c311-attachments-heading"><h3 id="c311-attachments-heading" class="h5">Attachments</h3><ul v-if="detail.attachments && detail.attachments.length" class="list-group"><li v-for="attachment in detail.attachments" :key="attachment.attachment_id" class="list-group-item"><button class="btn btn-link p-0" type="button" @click="downloadAttachment(attachment)">{{ attachment.filename }}</button> {{ attachment.media_type }}</li></ul><p v-else class="text-muted">No attachments.</p></section>
        <section class="mb-3" aria-labelledby="c311-history-heading"><h3 id="c311-history-heading" class="h5">History</h3><ul class="list-group"><li v-for="item in detail.history" :key="`${item.action}-${item.occurred_at}`" class="list-group-item">{{ item.action }} - {{ item.occurred_at }} - {{ item.responsible_department }}</li></ul></section>
        <section aria-labelledby="c311-audit-heading"><h3 id="c311-audit-heading" class="h5">Audit trail</h3><details><summary>{{ (detail.audit || []).length }} immutable audit events</summary><pre class="small">{{ JSON.stringify(detail.audit, null, 2) }}</pre></details></section>
        <button class="btn btn-outline-danger" type="button" :disabled="busy" @click="closeSelected">Close this request</button>
      </section>
    </template>
  </c311-app-shell>
</template>

<script>
import { components, c311 } from '@cortezaproject/corteza-vue'

const { C311AppShell, C311DataState, C311ErrorSummary, C311MainNav } = components
const stateForError = c311?.c311StateForError
const statuses = ['TRIAGED', 'ASSIGNED', 'IN_PROGRESS', 'RESOLVED', 'CLOSED', 'REOPENED']
const parseBenchmarkDateTime = (value) => {
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})$/.exec(value || '')
  if (!match) return new Date('invalid')
  const guess = Date.UTC(Number(match[1]), Number(match[2]) - 1, Number(match[3]), Number(match[4]), Number(match[5]))
  const parts = new Intl.DateTimeFormat('en-US', { timeZone: 'America/New_York', hourCycle: 'h23', year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }).formatToParts(new Date(guess)).reduce((out, part) => { out[part.type] = part.value; return out }, {})
  const displayed = Date.UTC(Number(parts.year), Number(parts.month) - 1, Number(parts.day), Number(parts.hour), Number(parts.minute))
  return new Date(guess - (displayed - guess))
}

export default {
  name: 'C311StaffWorkspace',
  components: { C311AppShell, C311DataState, C311ErrorSummary, C311MainNav },
  data: () => ({
    state: 'loading', error: null, items: [], nextPageToken: null, detail: null, busy: false, message: '', formErrors: [], statuses,
    filters: { status: '', service_type: '', department: '' },
    transitionForm: { to_status: 'TRIAGED', reason: '' }, assignmentForm: { assignee_id: '', reason: '' },
    noteForm: { body: '', portal_visible: false }, reminderForm: { title: '', due_at: '', recipient_staff_id: '', timezone: 'America/New_York', channel: 'IN_APP' }, reminderSnoozeAt: '', collaboratorForm: { staff_id: '', reason: '' }, relationshipTypes: ['AFFECTED_RESIDENT', 'PROPERTY_OWNER', 'REPORTER', 'ORGANISATION_CONTACT'], relationshipForm: { constituent_id: '', relationship_type: 'AFFECTED_RESIDENT', portal_visible: true, notify_status: true }, originForm: { origin_class: 'EXTERNAL', reason: '' }, duplicateForm: { duplicate_group_id: '', reason: '' },
  }),
  computed: {
    provider () { return this.$C311?.provider },
    navItems () { return [{ route: '/c311/staff/requests', label: 'Requests' }, { route: '/c311/staff/constituents', label: 'Constituents' }, { route: '/c311/staff/submit', label: 'Create request' }, { route: '/c311', label: 'Public portal' }] },
    fieldTargets () { return { form: 'c311-transition-status', assignee_id: 'c311-assignee', body: 'c311-staff-note', title: 'c311-reminder-title', due_at: 'c311-reminder-due', recipient_staff_id: 'c311-reminder-recipient' } },
  },
  created () { this.loadQueue() },
  methods: {
    t (value) { return value },
    setError (error) { this.error = error; this.formErrors = [{ field: 'form', code: error?.code || error?.error || 'OPERATION_FAILED', message: error?.message || 'The operation could not be completed.' }] },
    async loadQueue (append = false) {
      this.state = 'loading'; this.error = null
      try {
        const query = { page_size: 100, filters: Object.fromEntries(Object.entries(this.filters).filter(([, value]) => value)), ...(append && this.nextPageToken ? { page_token: this.nextPageToken } : {}) }
        const page = await this.provider.listStaffRequests(query)
        this.items = append ? this.items.concat(page.items || []) : (page.items || [])
        this.nextPageToken = page.next_page_token || null
        this.state = 'populated'
      } catch (error) { this.error = error; this.state = stateForError?.(error) || 'terminal-error' }
    },
    async selectRequest (item) {
      this.busy = true; this.message = ''; this.formErrors = []; this.detail = null
      try { this.detail = await this.provider.getStaffRequest(item.request_id); this.transitionForm.to_status = this.detail.request.status } catch (error) { this.setError(error) } finally { this.busy = false }
    },
    async refreshDetail () { if (this.detail) await this.selectRequest(this.detail.request) },
    async run (operation, success) {
      if (!this.detail || this.busy) return
      this.busy = true; this.message = ''; this.formErrors = []
      try { await operation(); this.message = success; await this.loadQueue(); await this.refreshDetail() } catch (error) { this.setError(error) } finally { this.busy = false }
    },
    transition () { return this.run(() => this.provider.transitionStaffRequest(this.detail.request.request_id, this.transitionForm, { expectedVersion: this.detail.request.version }), 'Request status updated.') },
    reassign () { return this.run(() => this.provider.reassignStaffRequest(this.detail.request.request_id, this.assignmentForm, { expectedVersion: this.detail.request.version }), 'Request reassigned.') },
    addNote () { return this.run(() => this.provider.createStaffNote(this.detail.request.request_id, this.noteForm), 'Note added.') },
    createReminder () {
      const dueAt = parseBenchmarkDateTime(this.reminderForm.due_at)
      if (!this.reminderForm.due_at || Number.isNaN(dueAt.getTime())) {
        this.setError({ code: 'INVALID_VALUE', message: 'Choose a valid reminder due date and time.' })
        return
      }
      const input = { ...this.reminderForm, due_at: dueAt.toISOString() }
      return this.run(() => this.provider.createStaffReminder(this.detail.request.request_id, input), 'Reminder created.')
    },
    addCollaborator () { return this.run(() => this.provider.addStaffCollaborator(this.detail.request.request_id, this.collaboratorForm.staff_id, { reason: this.collaboratorForm.reason }, { expectedVersion: this.detail.request.version }), 'Collaborator added.') },
    linkConstituent () { return this.run(() => this.provider.linkStaffConstituent(this.detail.request.request_id, this.relationshipForm, { expectedVersion: this.detail.request.version }), 'Constituent linked.') },
    unlinkConstituent (link) { return this.run(() => this.provider.unlinkStaffConstituent(this.detail.request.request_id, link.constituent_id, { reason: 'Removed from request' }, { expectedVersion: this.detail.request.version }), 'Constituent unlinked.') },
    overrideOrigin () { return this.run(() => this.provider.overrideStaffOrigin(this.detail.request.request_id, this.originForm, { expectedVersion: this.detail.request.version }), 'Origin classification updated.') },
    setDuplicateGroup () { return this.run(() => this.provider.confirmStaffDuplicateGroup(this.detail.request.request_id, this.duplicateForm, { expectedVersion: this.detail.request.version }), 'Same-issue group confirmed.') },
    removeDuplicateGroup () { return this.run(() => this.provider.removeStaffDuplicateGroup(this.detail.request.request_id, { reason: this.duplicateForm.reason || 'Removed from group' }, { expectedVersion: this.detail.request.version }), 'Same-issue group removed.') },
    reminderAction (reminder, action) { const dueAt = parseBenchmarkDateTime(this.reminderSnoozeAt); if (action === 'SNOOZE' && !this.reminderSnoozeAt) { this.setError({ code: 'REQUIRED', message: 'Choose a snooze time before snoozing a reminder.' }); return } if (action === 'SNOOZE' && Number.isNaN(dueAt.getTime())) { this.setError({ code: 'INVALID_VALUE', message: 'Choose a valid snooze date and time.' }); return } const input = action === 'SNOOZE' ? { due_at: dueAt.toISOString() } : {}; return this.run(() => this.provider.actionStaffReminder(reminder.reminder_id, action, input), `Reminder ${action.toLowerCase()}d.`) },
    async downloadAttachment (attachment) { this.formErrors = []; try { const value = await this.provider.downloadAttachment(attachment.attachment_id); const blob = new Blob([value.body], { type: value.content_type || attachment.media_type }); const link = document.createElement('a'); link.href = URL.createObjectURL(blob); link.download = attachment.filename; link.click(); URL.revokeObjectURL(link.href) } catch (error) { this.setError(error) } },
    closeSelected () {
      const input = { action: 'CLOSE', changes: { status: 'CLOSED' }, request_items: [{ request_id: this.detail.request.request_id, expected_version: this.detail.request.version }] }
      return this.run(() => this.provider.bulkStaffRequests(input, { idempotencyKey: `close-${this.detail.request.request_id}-${this.detail.request.version}` }), 'Request closed.')
    },
  },
}
</script>
