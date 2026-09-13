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
      </section>

      <section v-if="detail" class="mt-4" aria-labelledby="c311-request-heading">
        <h2 id="c311-request-heading" class="h4">{{ detail.request.request_number }} {{ detail.request.summary }}</h2>
        <p><strong>Status:</strong> {{ detail.request.status }} <strong class="ml-3">Department:</strong> {{ detail.request.owning_department }}</p>
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
            <label class="mt-2" for="c311-reminder-due">Due at</label><input id="c311-reminder-due" v-model="reminderForm.due_at" class="form-control" type="datetime-local">
            <label class="mt-2" for="c311-reminder-recipient">Recipient staff ID</label><input id="c311-reminder-recipient" v-model.trim="reminderForm.recipient_staff_id" class="form-control">
            <button class="btn btn-outline-primary mt-2" type="submit" :disabled="busy">Create reminder</button>
          </form>
        </div>
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

export default {
  name: 'C311StaffWorkspace',
  components: { C311AppShell, C311DataState, C311ErrorSummary, C311MainNav },
  data: () => ({
    state: 'loading', error: null, items: [], detail: null, busy: false, message: '', formErrors: [], statuses,
    transitionForm: { to_status: 'TRIAGED', reason: '' }, assignmentForm: { assignee_id: '', reason: '' },
    noteForm: { body: '', portal_visible: false }, reminderForm: { title: '', due_at: '', recipient_staff_id: '', timezone: 'America/New_York', channel: 'IN_APP' },
  }),
  computed: {
    provider () { return this.$C311?.provider },
    navItems () { return [{ route: '/c311/staff/requests', label: 'Requests' }, { route: '/c311/staff/submit', label: 'Create request' }, { route: '/c311', label: 'Public portal' }] },
    fieldTargets () { return { form: 'c311-transition-status', assignee_id: 'c311-assignee', body: 'c311-staff-note', title: 'c311-reminder-title', due_at: 'c311-reminder-due', recipient_staff_id: 'c311-reminder-recipient' } },
  },
  created () { this.loadQueue() },
  methods: {
    t (value) { return value },
    setError (error) { this.error = error; this.formErrors = [{ field: 'form', code: error?.code || error?.error || 'OPERATION_FAILED', message: error?.message || 'The operation could not be completed.' }] },
    async loadQueue () {
      this.state = 'loading'; this.error = null
      try { const page = await this.provider.listStaffRequests({ page_size: 100 }); this.items = page.items || []; this.state = 'populated' } catch (error) { this.error = error; this.state = stateForError?.(error) || 'terminal-error' }
    },
    async selectRequest (item) {
      this.busy = true; this.message = ''; this.formErrors = []
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
      const input = { ...this.reminderForm, due_at: new Date(this.reminderForm.due_at).toISOString() }
      return this.run(() => this.provider.createStaffReminder(this.detail.request.request_id, input), 'Reminder created.')
    },
    closeSelected () {
      const input = { action: 'CLOSE', changes: { status: 'CLOSED' }, request_items: [{ request_id: this.detail.request.request_id, expected_version: this.detail.request.version }] }
      return this.run(() => this.provider.bulkStaffRequests(input), 'Request closed.')
    },
  },
}
</script>
