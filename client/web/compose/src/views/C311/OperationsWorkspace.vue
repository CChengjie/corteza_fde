<template>
  <c311-app-shell title="City 311 operations" main-id="c311-operations-main">
    <template #nav><c311-main-nav :items="navItems" aria-label="Operations navigation" /></template>
    <main>
      <h1 class="h3 mb-3">Operations</h1>
      <c311-error-summary :errors="formErrors" title="Review this operation" />
      <div v-if="message" class="alert alert-success" role="status">{{ message }}</div>

      <section class="mb-4" aria-labelledby="calendar-heading"><h2 id="calendar-heading" class="h4">Reminder calendar</h2><label for="calendar-ics">ICS calendar</label><textarea id="calendar-ics" v-model="calendarICS" class="form-control" rows="7" spellcheck="false" /><button class="btn btn-primary mt-2" type="button" :disabled="busy" @click="importCalendar">Import ICS</button><button class="btn btn-outline-primary mt-2 ml-2" type="button" :disabled="busy" @click="exportCalendar">Export ICS</button><a v-if="calendarDownload" :href="calendarDownload" download="city311-reminders.ics" class="btn btn-link mt-2">Download exported calendar</a></section>

      <section class="mb-4" aria-labelledby="workflow-heading"><h2 id="workflow-heading" class="h4">Workflows</h2><p v-if="workflowError" class="text-muted" role="status">Workflow access is unavailable for this account.</p><template v-else><div class="table-responsive"><table class="table table-sm"><thead><tr><th>Name</th><th>Trigger</th><th>Active</th><th>Version</th><th /></tr></thead><tbody><tr v-for="workflow in workflows" :key="workflow.workflow_id"><td>{{ workflow.name }}</td><td>{{ workflow.trigger }}</td><td>{{ workflow.active ? 'Yes' : 'No' }}</td><td>{{ workflow.version }}</td><td><button class="btn btn-sm btn-link" type="button" :disabled="busy" @click="editWorkflow(workflow)">Edit</button><button class="btn btn-sm btn-link" type="button" :disabled="busy" @click="testWorkflow(workflow)">Test</button><button class="btn btn-sm btn-link" type="button" :disabled="busy" @click="toggleWorkflow(workflow)">{{ workflow.active ? 'Deactivate' : 'Activate' }}</button></td></tr></tbody></table></div><form @submit.prevent="saveWorkflow"><label for="workflow-definition">Workflow definition</label><textarea id="workflow-definition" v-model="workflowJSON" class="form-control font-monospace" rows="12" spellcheck="false" /><button class="btn btn-primary mt-2" type="submit" :disabled="busy">{{ selectedWorkflow ? 'Update workflow' : 'Create workflow' }}</button><button v-if="selectedWorkflow" class="btn btn-link mt-2" type="button" @click="resetWorkflow">Cancel</button></form><label class="mt-3" for="workflow-test-request">Request ID for test</label><input id="workflow-test-request" v-model.trim="workflowTestRequestID" class="form-control" placeholder="sr-..." /><h3 class="h6 mt-3">Execution log</h3><p v-if="!executions.length" class="text-muted" role="status">No workflow executions found.</p><ul v-else class="list-group"><li v-for="execution in executions" :key="execution.execution_id" class="list-group-item"><strong>{{ execution.workflow_id }}</strong> {{ execution.status }} <button class="btn btn-sm btn-link" type="button" @click="loadExecution(execution.execution_id)">Details</button></li></ul></template></section>

      <section class="mb-4" aria-labelledby="reports-heading"><h2 id="reports-heading" class="h4">Reports and exports</h2><p v-if="reportError" class="text-muted" role="status">Report access is unavailable for this account.</p><template v-else><div class="table-responsive"><table class="table table-sm"><thead><tr><th>Name</th><th>Entity</th><th>Version</th><th /></tr></thead><tbody><tr v-for="report in reports" :key="report.report_id"><td>{{ report.name }}</td><td>{{ report.entity }}</td><td>{{ report.version }}</td><td><button class="btn btn-sm btn-link" type="button" @click="editReport(report)">Edit</button><button class="btn btn-sm btn-link" type="button" @click="runReport(report)">Run</button><button class="btn btn-sm btn-link" type="button" @click="exportReport(report)">Export CSV</button></td></tr></tbody></table></div><form @submit.prevent="saveReport"><label for="report-definition">Report definition</label><textarea id="report-definition" v-model="reportJSON" class="form-control font-monospace" rows="10" spellcheck="false" /><button class="btn btn-primary mt-2" type="submit" :disabled="busy">{{ selectedReport ? 'Update report' : 'Create report' }}</button><button v-if="selectedReport" class="btn btn-link mt-2" type="button" @click="resetReport">Cancel</button></form><p class="text-muted mt-3">Paged CRM data export is available only to authenticated integration clients through the API client-credentials contract.</p><button class="btn btn-outline-primary" type="button" :disabled="busy" @click="exportAudit">Export audit events</button><button class="btn btn-outline-primary ml-2" type="button" :disabled="busy" @click="exportContacts">Export eligible contact emails</button></template></section>

      <section class="mb-4" aria-labelledby="mail-heading"><h2 id="mail-heading" class="h4">Rich email</h2><form @submit.prevent="sendMail"><div class="form-row"><div class="form-group col-md-5"><label for="mail-recipient">Recipient</label><input id="mail-recipient" v-model.trim="mail.to" class="form-control" type="email" required></div><div class="form-group col-md-7"><label for="mail-subject">Subject</label><input id="mail-subject" v-model.trim="mail.subject" class="form-control" required></div></div><label for="mail-html">HTML body</label><textarea id="mail-html" v-model="mail.html" class="form-control" rows="6" /><button class="btn btn-outline-primary mt-2" type="button" :disabled="busy" @click="previewMail">Preview</button><button class="btn btn-primary mt-2 ml-2" type="submit" :disabled="busy">Send email</button></form></section>
      <pre v-if="result" class="border p-3 bg-light" aria-live="polite">{{ result }}</pre>
    </main>
  </c311-app-shell>
</template>

<script>
import { components } from '@cortezaproject/corteza-vue'

const { C311AppShell, C311ErrorSummary, C311MainNav } = components
const emptyWorkflow = () => JSON.stringify({ workflow_id: '', name: '', trigger: 'SERVICE_REQUEST_CREATED', active: false, conditions: [], actions: [] }, null, 2)
const emptyReport = () => JSON.stringify({ name: '', entity: 'service_requests', columns: [], filters: {}, sort: [] }, null, 2)
const plainTextFromHTML = (value) => {
  let plain = ''
  let inTag = false
  for (const character of String(value || '')) {
    if (character === '<') inTag = true
    else if (character === '>') inTag = false
    else if (!inTag) plain += character
  }
  return plain
}

export default {
  name: 'C311OperationsWorkspace', components: { C311AppShell, C311ErrorSummary, C311MainNav },
  data: () => ({ busy: false, message: '', formErrors: [], result: '', workflows: [], executions: [], reports: [], workflowError: false, reportError: false, workflowJSON: emptyWorkflow(), reportJSON: emptyReport(), selectedWorkflow: null, selectedReport: null, workflowTestRequestID: '', calendarICS: '', calendarDownload: '', mail: { to: '', subject: '', html: '' } }),
  computed: { provider () { return this.$C311?.provider }, navItems () { return [{ route: '/c311/staff/requests', label: 'Requests' }, { route: '/c311/operations', label: 'Operations' }, { route: '/c311/admin', label: 'Administration' }, { route: '/c311', label: 'Public portal' }] } },
  created () { this.load() },
  methods: {
    fail (error) { this.formErrors = [{ field: 'form', code: error?.code || error?.error || 'OPERATION_FAILED', message: error?.message || 'The operation could not be completed.' }] },
    async run (operation, message) { if (this.busy) return; this.busy = true; this.message = ''; this.formErrors = []; try { const value = await operation(); this.result = value === undefined ? '' : JSON.stringify(value, null, 2); this.message = message; return value } catch (error) { this.fail(error); return undefined } finally { this.busy = false } },
    async load () { const [workflows, executions, reports] = await Promise.allSettled([this.provider.listWorkflows({ page_size: 100 }), this.provider.listWorkflowExecutions({ page_size: 100 }), this.provider.listReports({ page_size: 100 })]); this.workflowError = workflows.status === 'rejected'; this.reportError = reports.status === 'rejected'; this.workflows = workflows.status === 'fulfilled' ? workflows.value.items || [] : []; this.executions = executions.status === 'fulfilled' ? executions.value.items || [] : []; this.reports = reports.status === 'fulfilled' ? reports.value.items || [] : [] },
    parse (value, label) { try { return JSON.parse(value) } catch (_) { throw new Error(`${label} must contain valid JSON.`) } },
    importCalendar () { return this.run(() => this.provider.importCalendar({ ics: this.calendarICS }), 'Calendar import accepted.') },
    exportCalendar () { return this.run(async () => { const exported = await this.provider.exportCalendar(); const blob = new Blob([exported.body], { type: exported.content_type || 'text/calendar' }); if (this.calendarDownload) URL.revokeObjectURL(this.calendarDownload); this.calendarDownload = URL.createObjectURL(blob); return { content_type: exported.content_type, event_count: (exported.body.match(/BEGIN:VEVENT/g) || []).length } }, 'Calendar exported.') },
    editWorkflow (workflow) { this.selectedWorkflow = workflow; this.workflowJSON = JSON.stringify(workflow, null, 2) },
    resetWorkflow () { this.selectedWorkflow = null; this.workflowJSON = emptyWorkflow() },
    saveWorkflow () { return this.run(async () => { const input = this.parse(this.workflowJSON, 'Workflow definition'); const saved = this.selectedWorkflow ? await this.provider.updateWorkflow(this.selectedWorkflow.workflow_id, input, { expectedVersion: this.selectedWorkflow.version }) : await this.provider.createWorkflow(input); await this.load(); this.resetWorkflow(); return saved }, 'Workflow saved.') },
    toggleWorkflow (workflow) { return this.run(async () => { const value = workflow.active ? await this.provider.deactivateWorkflow(workflow.workflow_id, { expectedVersion: workflow.version }) : await this.provider.activateWorkflow(workflow.workflow_id, { expectedVersion: workflow.version }); await this.load(); return value }, 'Workflow state changed.') },
    testWorkflow (workflow) { if (!this.workflowTestRequestID) { this.fail({ code: 'REQUIRED', message: 'Enter a request ID before testing a workflow.' }); return } return this.run(() => this.provider.testWorkflow(workflow.workflow_id, { request_id: this.workflowTestRequestID }), 'Workflow test accepted.') },
    loadExecution (id) { return this.run(() => this.provider.getWorkflowExecution(id), 'Workflow execution loaded.') },
    editReport (report) { this.selectedReport = report; this.reportJSON = JSON.stringify(report, null, 2) },
    resetReport () { this.selectedReport = null; this.reportJSON = emptyReport() },
    saveReport () { return this.run(async () => { const input = this.parse(this.reportJSON, 'Report definition'); const saved = this.selectedReport ? await this.provider.updateReport(this.selectedReport.report_id, input, { expectedVersion: this.selectedReport.version }) : await this.provider.createReport(input); await this.load(); this.resetReport(); return saved }, 'Report saved.') },
    runReport (report) { return this.run(() => this.provider.runReport({ definition: report }), 'Report run accepted.') },
    exportReport (report) { return this.run(() => this.provider.exportReport(report.report_id), 'Report export accepted.') },
    exportAudit () { return this.run(() => this.provider.exportAuditEvents({}), 'Audit export accepted.') },
    exportContacts () { return this.run(() => this.provider.exportContactEmails({ filters: {} }), 'Contact email export accepted.') },
    previewMail () { return this.run(() => this.provider.previewMail({ to: [this.mail.to], subject: this.mail.subject, text: plainTextFromHTML(this.mail.html), html: this.mail.html }), 'Mail preview generated.') },
    sendMail () { return this.run(() => this.provider.sendMail({ to: [this.mail.to], subject: this.mail.subject, text: plainTextFromHTML(this.mail.html), html: this.mail.html }), 'Mail delivery accepted.') },
  },
}
</script>
