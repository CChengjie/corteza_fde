<template>
  <c311-app-shell mode="staff" :brand="t('staff.brand', 'City 311 staff')" :title="isDetailView ? t('staff.detailTitle', 'Request detail') : t('staff.title', 'Requests')" :status-message="statusMessage">
    <template #nav>
      <div class="d-flex flex-wrap align-items-center gap-2">
        <c311-main-nav :items="navItems" :label="t('navigation.staff', 'Staff navigation')" />
        <c311-help-drawer help-key="staff.request.triage" :label="t('help.label', 'Help')" :title="t('help.title', 'Help')" :close-label="t('action.close', 'Close')" :content="t('help.staff.request.triage', 'Review and classify a request.')" />
        <c311-language-selector :actor-id="actorID" />
      </div>
    </template>

    <div v-if="!isDetailView" class="mb-3" data-c311-staff-filters>
      <form class="form-row" @submit.prevent="applyFilters">
        <div class="form-group col-md-2"><label for="c311-filter-status">{{ t('field.status', 'Status') }}</label><select id="c311-filter-status" v-model="filters.status" class="form-control"><option value="">{{ t('filter.all', 'All') }}</option><option v-for="status in statuses" :key="status" :value="status">{{ status }}</option></select></div>
        <div class="form-group col-md-2"><label for="c311-filter-service-type">{{ t('field.serviceType', 'Service type') }}</label><select id="c311-filter-service-type" v-model="filters.service_type" class="form-control"><option value="">{{ t('filter.all', 'All') }}</option><option v-for="type in serviceTypes" :key="type" :value="type">{{ type }}</option></select></div>
        <div class="form-group col-md-2"><label for="c311-filter-department">{{ t('field.department', 'Department') }}</label><select id="c311-filter-department" v-model="filters.department" class="form-control"><option value="">{{ t('filter.all', 'All') }}</option><option v-for="department in departments" :key="department" :value="department">{{ department }}</option></select></div>
        <div class="form-group col-md-2"><label for="c311-filter-district">{{ t('field.district', 'District') }}</label><select id="c311-filter-district" v-model="filters.district" class="form-control"><option value="">{{ t('filter.all', 'All') }}</option><option v-for="district in districts" :key="district" :value="district">{{ district }}</option></select></div>
        <div class="form-group col-md-2"><label for="c311-filter-origin">{{ t('field.origin', 'Origin') }}</label><select id="c311-filter-origin" v-model="filters.origin_class" class="form-control"><option value="">{{ t('filter.all', 'All') }}</option><option value="EXTERNAL">EXTERNAL</option><option value="INTERNAL">INTERNAL</option></select></div>
        <div class="form-group col-md-2"><label for="c311-filter-source">{{ t('field.source', 'Source channel') }}</label><select id="c311-filter-source" v-model="filters.source_channel" class="form-control"><option value="">{{ t('filter.all', 'All') }}</option><option value="PORTAL_ANONYMOUS">PORTAL_ANONYMOUS</option><option value="PORTAL_AUTHENTICATED">PORTAL_AUTHENTICATED</option><option value="STAFF_IN_PERSON">STAFF_IN_PERSON</option><option value="API">API</option></select></div>
        <div class="form-group col-md-2"><label for="c311-filter-assignee">{{ t('field.assignee', 'Assignee') }}</label><input id="c311-filter-assignee" v-model="filters.assignee" class="form-control"></div>
        <div class="form-group col-md-2"><label for="c311-filter-collaborator">{{ t('field.collaborator', 'Collaborator') }}</label><input id="c311-filter-collaborator" v-model="filters.collaborator" class="form-control"></div>
        <div class="form-group col-md-2"><label for="c311-filter-category">{{ t('field.category', 'Category') }}</label><input id="c311-filter-category" v-model="filters.category" class="form-control"></div>
        <div class="form-group col-md-2"><label for="c311-filter-created-from">{{ t('field.createdFrom', 'Created from') }}</label><input id="c311-filter-created-from" v-model="filters.created_from" type="datetime-local" class="form-control"></div>
        <div class="form-group col-md-2"><label for="c311-filter-created-to">{{ t('field.createdTo', 'Created to') }}</label><input id="c311-filter-created-to" v-model="filters.created_to" type="datetime-local" class="form-control"></div>
        <div class="form-group col-md-2"><label for="c311-filter-duplicate">{{ t('field.duplicateGroup', 'Duplicate group') }}</label><input id="c311-filter-duplicate" v-model="filters.duplicate_group" class="form-control"></div>
        <div class="form-group col-md-2"><label for="c311-filter-sort">{{ t('field.sort', 'Sort') }}</label><input id="c311-filter-sort" v-model="sort" class="form-control" :placeholder="t('field.sortPlaceholder', '-updated_at')"></div>
        <div class="form-group col-md-2"><label for="c311-filter-page-size">{{ t('field.pageSize', 'Page size') }}</label><input id="c311-filter-page-size" v-model.number="pageSize" type="number" min="1" max="100" class="form-control"></div>
        <div class="form-group col-md-2 d-flex align-items-end"><button class="btn btn-primary" type="submit" data-c311-action="apply-staff-filters">{{ t('action.applyFilters', 'Apply filters') }}</button></div>
      </form>
    </div>

    <button v-if="isDetailView" type="button" class="btn btn-link px-0 mb-3" data-c311-action="back-to-staff-queue" @click="backToQueue">{{ t('action.backToQueue', 'Back to queue') }}</button>

    <c311-data-state :state="state" :error="dataError" @retry="retryCurrentView">
      <template #populated>
        <section v-if="!isDetailView" aria-labelledby="c311-staff-heading">
          <h2 id="c311-staff-heading" tabindex="-1" class="h2">{{ t('staff.title', 'Requests') }}</h2>
          <p class="sr-only" aria-live="polite">{{ statusMessage }}</p>
          <c311-responsive-data :items="items" :columns="translatedColumns" row-key="request_id" :label="t('staff.queue', 'Staff request queue')" selectable :action-label="t('action.viewRequest', 'View request')" @select="selectRequest" />
          <div class="d-flex justify-content-between align-items-center mt-3" data-c311-pagination>
            <button type="button" class="btn btn-outline-secondary" :disabled="!pageTokenHistory.length" data-c311-action="previous-page" @click="previousPage">{{ t('action.previous', 'Previous') }}</button>
            <span>{{ page.total_count }} {{ t('staff.results', 'results') }}</span>
            <button type="button" class="btn btn-outline-secondary" :disabled="!page.next_page_token" data-c311-action="next-page" @click="nextPage">{{ t('action.next', 'Next') }}</button>
          </div>
        </section>

        <section v-else-if="detail" aria-labelledby="c311-staff-detail-heading" data-c311-request-detail data-c311-staff-request-detail>
          <h2 id="c311-staff-detail-heading" tabindex="-1" class="h2">{{ detail.request.request_number || detail.request.request_id }}</h2>
          <div class="row">
            <div class="col-lg-8">
              <dl class="row">
                <dt class="col-sm-4">{{ t('field.requestNumber', 'Request number') }}</dt><dd class="col-sm-8">{{ detail.request.request_number || detail.request.request_id }}</dd>
                <dt class="col-sm-4">{{ t('field.summary', 'Summary') }}</dt><dd class="col-sm-8">{{ detail.request.summary }}</dd>
                <dt class="col-sm-4">{{ t('field.status', 'Status') }}</dt><dd class="col-sm-8"><code>{{ detail.request.status }}</code> <span data-c311-status-label>({{ statusLabel(detail.request.status) }})</span></dd>
                <dt class="col-sm-4">{{ t('field.serviceType', 'Service type') }}</dt><dd class="col-sm-8">{{ detail.request.service_type }}</dd>
                <dt class="col-sm-4">{{ t('field.department', 'Department') }}</dt><dd class="col-sm-8">{{ detail.request.owning_department }}</dd>
                <dt class="col-sm-4">{{ t('field.district', 'District') }}</dt><dd class="col-sm-8">{{ detail.request.council_district || '-' }}</dd>
                <dt class="col-sm-4">{{ t('field.origin', 'Origin') }}</dt><dd class="col-sm-8">{{ detail.request.origin_class }}</dd>
                <dt class="col-sm-4">{{ t('field.source', 'Source channel') }}</dt><dd class="col-sm-8">{{ detail.request.source_channel }}</dd>
                <dt class="col-sm-4">{{ t('field.requester', 'Requester') }}</dt><dd class="col-sm-8">{{ detail.request.primary_requester ? detail.request.primary_requester.display_name : '-' }}</dd>
                <dt class="col-sm-4">{{ t('field.location', 'Location') }}</dt><dd class="col-sm-8">{{ detail.request.location ? detail.request.location.address.line1 : '-' }}</dd>
                <dt class="col-sm-4">{{ t('field.assignee', 'Assignee') }}</dt><dd class="col-sm-8">{{ detail.primary_assignee_id || '-' }}</dd>
                <dt id="c311-collaborators-heading" tabindex="-1" class="col-sm-4">{{ t('field.collaborators', 'Collaborators') }}</dt><dd class="col-sm-8"><ul class="mb-0" data-c311-collaborators><li v-for="collaborator in detail.collaborator_ids" :key="collaborator">{{ collaborator }}</li><li v-if="!detail.collaborator_ids.length">-</li></ul></dd>
                <dt class="col-sm-4">{{ t('field.version', 'Version') }}</dt><dd class="col-sm-8"><code>{{ detail.request.version }}</code></dd>
              </dl>

              <section class="mb-3" aria-labelledby="c311-relationships-heading">
                <h3 id="c311-relationships-heading" tabindex="-1">{{ t('staff.relationships', 'Constituent relationships') }}</h3>
                <ul data-c311-relationships>
                  <li v-for="relationship in relationships" :key="`${relationship.constituent_id}-${relationship.relationship_type}`">
                    <span>{{ relationship.constituent_id }} · {{ relationship.relationship_type }} · {{ relationship.portal_visible ? t('staff.portalVisible', 'portal visible') : t('staff.internalOnly', 'internal only') }} · {{ relationship.notify_status ? t('staff.notifyStatus', 'notify status') : t('staff.noNotifyStatus', 'do not notify') }}</span>
                    <small v-if="relationship.notification_target" class="d-block" data-c311-relationship-notification-target>{{ t('staff.notificationTarget', 'Notification target') }}: {{ relationship.notification_target }}</small>
                    <small v-if="relationship.notification_result" class="d-block" data-c311-relationship-notification-result>{{ t('staff.notificationResult', 'Notification result') }}: {{ relationship.notification_result }}</small>
                    <small v-for="event in relationship.audit || []" :key="event.audit_id" class="d-block" data-c311-relationship-audit>{{ t('staff.relationshipAudit', 'Relationship audit') }}: {{ event.action }} · {{ event.actor_id }} · {{ formatDate(event.occurred_at) }}</small>
                    <button v-if="canUnlink && relationship.relationship_type !== 'PRIMARY_REQUESTER'" class="btn btn-link p-0 ml-2" type="button" :data-c311-action="`unlink-constituent-${relationship.constituent_id}`" :disabled="relationshipBusy" @click="unlinkConstituent(relationship)">{{ t('action.unlink', 'Unlink') }}</button>
                  </li>
                  <li v-if="!relationships.length">{{ t('state.empty', 'No entries') }}</li>
                </ul>
                <section v-if="relationshipAuditEvents.length" class="mt-3" data-c311-relationship-audit-events aria-labelledby="c311-relationship-audit-events-heading">
                  <h4 id="c311-relationship-audit-events-heading">{{ t('staff.relationshipAuditEvents', 'Relationship audit events') }}</h4>
                  <ol><li v-for="event in relationshipAuditEvents" :key="`${event.audit_id}-${event.action}`">{{ event.action }} · {{ event.actor_id }} · {{ formatDate(event.occurred_at) }}</li></ol>
                </section>
                <button v-if="can('staff_request_detail')" type="button" class="btn btn-outline-secondary btn-sm mb-2" data-c311-action="manage-relationships" @click="focusSection('#c311-relationships-heading')">{{ t('action.manageRelationships', 'Manage relationships') }}</button>
                <form v-if="canLink" class="border rounded p-2" data-c311-form="link-constituent" @submit.prevent="linkConstituent">
                  <label for="c311-constituent-id">{{ t('field.constituentId', 'Constituent ID') }}</label>
                  <input id="c311-constituent-id" v-model.trim="relationshipForm.constituent_id" class="form-control" required>
                  <label for="c311-relationship-type" class="mt-2">{{ t('field.relationshipType', 'Relationship type') }}</label>
                  <select id="c311-relationship-type" v-model="relationshipForm.relationship_type" class="form-control"><option v-for="type in relationshipTypes" :key="type" :value="type">{{ type }}</option></select>
                  <label class="mt-2"><input v-model="relationshipForm.portal_visible" type="checkbox"> {{ t('field.portalVisible', 'Visible in portal') }}</label>
                  <label class="ml-3"><input v-model="relationshipForm.notify_status" type="checkbox"> {{ t('field.notifyStatus', 'Notify on status changes') }}</label>
                  <button class="btn btn-primary mt-2" type="submit" data-c311-action="link-constituent" :disabled="relationshipBusy">{{ relationshipBusy ? t('action.working', 'Working...') : t('action.link', 'Link constituent') }}</button>
                </form>
                <button v-if="canUnlink && !relationships.length" type="button" class="btn btn-outline-secondary btn-sm" data-c311-action="unlink-constituent" disabled :title="t('status.noRelationshipToUnlink', 'There is no relationship to unlink.')">{{ t('action.unlinkConstituent', 'Unlink constituent') }}</button>
              </section>

              <section class="mb-3" aria-labelledby="c311-notes-heading">
                <h3 id="c311-notes-heading">{{ t('staff.notes', 'Notes') }}</h3>
                <ol data-c311-notes><li v-for="note in notes" :key="note.note_id">{{ note.body }} · {{ note.portal_visible ? t('staff.portalVisible', 'portal visible') : t('staff.internalOnly', 'internal only') }} · {{ formatDate(note.created_at) }}</li><li v-if="!notes.length">{{ t('state.empty', 'No entries') }}</li></ol>
                <form v-if="canCreateNote" class="border rounded p-2" data-c311-form="create-note" @submit.prevent="createNote">
                  <label for="c311-note-body">{{ t('field.note', 'Note') }}</label>
                  <textarea id="c311-note-body" v-model.trim="noteForm.body" class="form-control" maxlength="2000" required></textarea>
                  <label class="mt-2"><input v-model="noteForm.portal_visible" type="checkbox"> {{ t('field.portalVisible', 'Visible in portal') }}</label>
                  <button class="btn btn-primary mt-2" type="submit" data-c311-action="create-note" :disabled="noteBusy">{{ noteBusy ? t('action.working', 'Working...') : t('action.addNote', 'Add note') }}</button>
                </form>
              </section>

              <section class="mb-3" aria-labelledby="c311-history-heading"><h3 id="c311-history-heading">{{ t('staff.history', 'History') }}</h3><ul data-c311-history><li v-for="entry in detail.history" :key="`${entry.action}-${entry.occurred_at}`">{{ entry.action }} {{ formatDate(entry.occurred_at) }}</li><li v-if="!detail.history.length">{{ t('state.empty', 'No entries') }}</li></ul></section>
              <section v-if="can('audit_list')" class="mb-3" aria-labelledby="c311-audit-heading"><h3 id="c311-audit-heading">{{ t('staff.audit', 'Audit') }}</h3><ul data-c311-audit><li v-for="(entry, index) in detail.audit" :key="index">{{ entry.action || entry.event || t('state.unknown', 'Recorded event') }}</li><li v-if="!detail.audit.length">{{ t('state.empty', 'No entries') }}</li></ul></section>
              <p v-else class="small text-muted" data-c311-audit-unavailable>{{ t('status.auditUnavailable', 'Audit details require additional permission.') }}</p>
              <section class="mb-3" aria-labelledby="c311-reminders-heading"><h3 id="c311-reminders-heading">{{ t('staff.reminders', 'Reminders') }}</h3><ul data-c311-reminders><li v-for="reminder in detail.reminders" :key="reminder.reminder_id">{{ reminder.title }} ({{ reminder.status }})</li><li v-if="!detail.reminders.length">{{ t('state.empty', 'No reminders') }}</li></ul></section>
              <section class="mb-3" aria-labelledby="c311-attachments-heading"><h3 id="c311-attachments-heading">{{ t('staff.attachments', 'Attachments') }}</h3><ul data-c311-attachments><li v-for="attachment in attachmentEntries" :key="attachment.attachment_token || attachment.filename"><span data-c311-attachment-entry>{{ attachment.filename || t('staff.attachment', 'Attachment') }}</span> <small>{{ attachment.media_type || '' }} · {{ attachment.size || 0 }} bytes</small> <code>{{ attachment.attachment_token }}</code></li><li v-if="!attachmentEntries.length">{{ t('state.empty', 'No attachments') }}</li></ul></section>
              <section aria-labelledby="c311-external-work-order-heading"><h3 id="c311-external-work-order-heading">{{ t('staff.externalWorkOrder', 'External work order') }}</h3><span>{{ detail.external_work_order ? JSON.stringify(detail.external_work_order) : '-' }}</span></section>
            </div>
            <div class="col-lg-4">
              <div class="border rounded p-3" data-c311-staff-actions>
                <h3 class="h5">{{ t('staff.actions', 'Available actions') }}</h3>
                <button v-if="canRecordAction('staff_request_transition', 'TRIAGE', ['SUBMITTED'])" type="button" class="btn btn-primary btn-sm mr-2 mb-2" data-c311-action="transition-request" data-c311-action-kind="triage" @click="markActionUnavailable('triage')">{{ t('action.triage', 'Triage') }}</button>
                <button v-if="hasAdditionalTransition" type="button" class="btn btn-outline-primary btn-sm mr-2 mb-2" data-c311-action="transition-status" @click="markActionUnavailable('transition')">{{ t('action.transitionStatus', 'Transition status') }}</button>
                <button v-if="can('staff_request_detail')" type="button" class="btn btn-outline-secondary btn-sm mr-2 mb-2" data-c311-action="edit-request" disabled :title="t('status.actionNotAvailable', 'This workflow action is available in the next staff workflow stage.')">{{ t('action.edit', 'Edit request') }}</button>
                <button v-if="canRecordAction('staff_request_reassign', 'ASSIGN', ['SUBMITTED', 'TRIAGED', 'ASSIGNED', 'IN_PROGRESS'])" type="button" class="btn btn-outline-primary btn-sm mr-2 mb-2" data-c311-action="reassign-request" @click="markActionUnavailable('reassign')">{{ t('action.reassign', 'Reassign') }}</button>
                <button v-if="canManageCollaborators" type="button" class="btn btn-outline-secondary btn-sm mr-2 mb-2" data-c311-action="manage-collaborators" @click="focusSection('#c311-collaborators-heading')">{{ t('action.manageCollaborators', 'Manage collaborators') }}</button>
                <button v-if="canRecordAction('staff_origin_override', null, statuses)" type="button" class="btn btn-outline-secondary btn-sm mr-2 mb-2" data-c311-action="override-origin" @click="markActionUnavailable('origin-override')">{{ t('action.overrideOrigin', 'Override origin class') }}</button>
                <button v-if="canRecordAction('staff_reminder_create', null, ['DRAFT', 'SUBMITTED', 'TRIAGED', 'ASSIGNED', 'IN_PROGRESS', 'RESOLVED'])" type="button" class="btn btn-outline-primary btn-sm mr-2 mb-2" data-c311-action="create-reminder" @click="markActionUnavailable('reminder-create')">{{ t('action.createReminder', 'Create reminder') }}</button>
                <button v-if="canRecordAction('staff_reminder_action', null, ['DRAFT', 'SUBMITTED', 'TRIAGED', 'ASSIGNED', 'IN_PROGRESS', 'RESOLVED']) && detail.reminders.length" type="button" class="btn btn-outline-secondary btn-sm mr-2 mb-2" data-c311-action="complete-reminder" @click="markActionUnavailable('reminder-complete')">{{ t('action.completeReminder', 'Complete reminder') }}</button>
                <button v-if="canRecordAction('staff_reopen_approve', 'APPROVE_REOPEN', ['RESOLVED', 'CLOSED'])" type="button" class="btn btn-outline-primary btn-sm mr-2 mb-2" data-c311-action="approve-reopen" @click="markActionUnavailable('reopen')">{{ t('action.approveReopen', 'Approve reopen') }}</button>
                <button v-if="can('audit_list')" type="button" class="btn btn-outline-secondary btn-sm mr-2 mb-2" data-c311-action="view-audit" @click="markActionUnavailable('audit')">{{ t('action.viewAudit', 'View audit') }}</button>
                <p v-if="!can('audit_list')" class="small text-muted" data-c311-audit-action-unavailable>{{ t('status.auditUnavailable', 'Audit details require additional permission.') }}</p>
                <p v-if="actionMessage" class="text-info mt-2" role="status" aria-live="polite" data-c311-action-message>{{ actionMessage }}</p>
                <p v-if="actionError" class="text-danger mt-2" role="alert" data-c311-action-error>{{ actionError.message || actionError }}</p>
              </div>
              <p v-if="detailMessage" class="alert alert-info mt-3" role="status" data-c311-detail-message>{{ detailMessage }}</p>
            </div>
          </div>
        </section>
      </template>
    </c311-data-state>
  </c311-app-shell>
</template>

<script>
import * as C311JS from '@cortezaproject/corteza-js'
import { components, c311 } from '@cortezaproject/corteza-vue'

const { C311AppShell, C311DataState, C311HelpDrawer, C311LanguageSelector, C311MainNav, C311ResponsiveData } = components
const c311StateForError = c311?.c311StateForError
const formatDate = value => {
  try { return typeof C311JS.formatC311DateTime === 'function' ? C311JS.formatC311DateTime(value) : value || '' } catch (_error) { return value || '' }
}

export default {
  name: 'C311Staff',
  components: { C311AppShell, C311DataState, C311HelpDrawer, C311LanguageSelector, C311MainNav, C311ResponsiveData },
  data: () => ({
    state: 'loading', dataError: null, statusMessage: '', actionError: null, actionMessage: '', detailMessage: '', detail: null, selectedRequest: null, items: [],
    page: { items: [], next_page_token: null, total_count: 0 }, pageTokenHistory: [],
    filters: { status: '', service_type: '', department: '', district: '', origin_class: '', source_channel: '', assignee: '', collaborator: '', category: '', created_from: '', created_to: '', duplicate_group: '' },
    sort: '', pageSize: 50, requestGeneration: 0,
    relationships: [], audit: [], notes: [], relationshipBusy: false, noteBusy: false,
    relationshipForm: { constituent_id: '', relationship_type: 'AFFECTED_RESIDENT', portal_visible: true, notify_status: false },
    noteForm: { body: '', portal_visible: false }, relationshipTypes: ['PRIMARY_REQUESTER', 'AFFECTED_RESIDENT', 'PROPERTY_OWNER', 'REPORTER', 'ORGANISATION_CONTACT'],
    statuses: ['DRAFT', 'SUBMITTED', 'TRIAGED', 'ASSIGNED', 'IN_PROGRESS', 'RESOLVED', 'CLOSED', 'REOPENED'], serviceTypes: ['TREE_MAINTENANCE', 'POTHOLE', 'MISSED_TRASH', 'GENERAL_INQUIRY'], departments: ['PUBLIC_WORKS', 'STREETS', 'SANITATION', 'GENERAL_SERVICES'], districts: ['NORTH', 'CENTRAL', 'SOUTH'],
    columns: [
      { key: 'request_number', labelKey: 'field.requestNumber' }, { key: 'summary', labelKey: 'field.summary' }, { key: 'status', labelKey: 'field.status' }, { key: 'owning_department', labelKey: 'field.department' }, { key: 'updated_at', labelKey: 'field.updated', format: value => formatDate(value) },
      { key: 'service_type', labelKey: 'field.serviceType' }, { key: 'origin_class', labelKey: 'field.origin' }, { key: 'source_channel', labelKey: 'field.source' }, { key: 'council_district', labelKey: 'field.district' }, { key: 'primary_assignee_id', labelKey: 'field.assignee' }, { key: 'duplicate_group_id', labelKey: 'field.duplicateGroup' }, { key: 'version', labelKey: 'field.version' },
    ],
  }),
  computed: {
    actor () { return this.$C311?.session?.actor || null }, actorID () { return this.actor?.actor_id || '' },
    isDetailRoute () { return !!this.$route?.params?.request_id },
    isDetailView () { return this.isDetailRoute || !!this.detail },
    navItems () { return [{ route: '/c311/staff', label: this.t('navigation.requests', 'Requests'), capability: 'staff_request_queue' }, { route: '/c311/staff/reports', label: this.t('navigation.reports', 'Reports'), capability: 'report_catalogue' }, { route: '/c311/staff/workflows', label: this.t('navigation.workflows', 'Workflows'), capability: 'workflow_list', scope: 'workflow.execute' }] },
    translatedColumns () { return this.columns.map(column => ({ ...column, label: this.t(column.labelKey, column.labelKey), ...(column.key === 'status' ? { format: value => `${value} (${this.statusLabel(value)})` } : {}) })) },
    requestQuery () { return { ...Object.fromEntries(Object.entries(this.filters).filter(([, value]) => value !== '')), page_size: this.pageSize, ...(this.sort ? { sort: this.sort } : {}) } },
    canCapability () { return capability => typeof this.$C311?.can === 'function' ? this.$C311.can(capability) : !!this.actor?.capabilities?.includes(capability) },
    canLink () { return this.canRecordAction('staff_constituent_link') }, canUnlink () { return this.canRecordAction('staff_constituent_unlink') }, canCreateNote () { return this.canRecordAction('staff_note_create') },
    canManageCollaborators () { return !!this.detail?.request?.status && this.statuses.includes(this.detail.request.status) && (this.can('staff_collaborator_add') || this.can('staff_collaborator_remove')) },
    hasAdditionalTransition () { return this.can('staff_request_transition') && ['START_PROGRESS', 'RESOLVE', 'CLOSE', 'REQUEST_REOPEN'].some(action => this.hasAvailableAction(action)) },
    relationshipAuditEvents () { return this.audit.filter(event => ['LINKED', 'UNLINKED', 'UPDATED'].includes(event.action)) },
    attachmentEntries () {
      if (this.detail?.attachments?.length) return this.detail.attachments
      return (this.detail?.request?.custom_fields?.attachment_tokens || []).map(attachment_token => ({ attachment_token }))
    },
  },
  watch: { '$route.params.request_id': 'loadFromRoute' },
  created () { this.restoreQueueState(); this.loadFromRoute() },
  methods: {
    t (key, fallback) { const translated = this.$t?.(`c311:${key}`); return translated && translated !== `c311:${key}` && translated !== key ? translated : fallback },
    formatDate, statusLabel (status) { return this.t(`status.value.${String(status).toLowerCase()}`, status) }, can (capability) { return this.canCapability(capability) },
    hasAvailableAction (action) { return !!this.detail?.available_actions?.includes(action) },
    canRecordAction (capability, action = null, statuses = []) {
      if (!this.detail || !this.can(capability)) return false
      if (action && !this.hasAvailableAction(action)) return false
      return !statuses.length || statuses.includes(this.detail.request.status)
    },
    focusSection (selector) { this.$nextTick(() => this.$el.querySelector(selector)?.focus()) },
    markActionUnavailable () { this.actionError = null; this.actionMessage = this.t('status.actionNotAvailable', 'This workflow action is available in the next staff workflow stage.') },
    normalizeDetail (response) { return { ...response, available_actions: response?.available_actions || [], collaborator_ids: response?.collaborator_ids || [], reminders: response?.reminders || [], attachments: response?.attachments || [], history: response?.history || [], audit: response?.audit || [], relationships: response?.relationships || [], notes: response?.notes || [] } },
    applyFilters () { this.pageTokenHistory = []; this.load() },
    async loadFromRoute () { this.detail = null; this.selectedRequest = null; this.relationships = []; this.notes = []; this.audit = []; this.detailMessage = ''; return this.isDetailRoute ? this.loadDetail(this.$route.params.request_id) : this.load() },
    retryCurrentView () { return this.isDetailRoute ? this.loadDetail(this.$route.params.request_id) : this.load() },
    restoreQueueState () { try { const raw = sessionStorage.getItem('c311.staff.queue'); if (raw) { const saved = JSON.parse(raw); this.filters = { ...this.filters, ...(saved.filters || {}) }; this.sort = saved.sort || ''; this.pageSize = saved.pageSize || 50; this.pageTokenHistory = saved.pageTokenHistory || [] } } catch (_error) {} },
    persistQueueState () { try { sessionStorage.setItem('c311.staff.queue', JSON.stringify({ filters: this.filters, sort: this.sort, pageSize: this.pageSize, pageTokenHistory: this.pageTokenHistory })) } catch (_error) {} },
    async load () {
      const generation = ++this.requestGeneration; this.persistQueueState(); this.state = 'loading'; this.dataError = null; this.statusMessage = this.t('status.loadingRequests', 'Loading requests.'); this.detail = null
      try { const page = await this.$C311?.provider?.listStaffRequests?.({ ...this.requestQuery, ...(this.pageTokenHistory.length ? { page_token: this.pageTokenHistory[this.pageTokenHistory.length - 1] } : {}) }); if (generation !== this.requestGeneration || this.isDetailRoute) return; this.page = page || { items: [], next_page_token: null, total_count: 0 }; this.items = this.page.items || []; this.state = this.items.length ? 'populated' : 'empty'; this.statusMessage = this.items.length ? this.t('status.requestsLoaded', 'Requests loaded.') : this.t('status.noRequests', 'No requests found.') } catch (error) { if (generation !== this.requestGeneration || this.isDetailRoute) return; this.dataError = error; this.state = c311StateForError?.(error) || (error?.retryable ? 'retryable-error' : 'terminal-error'); this.statusMessage = this.t('status.requestListUnavailable', 'Request list unavailable.') }
    },
    async loadDetail (requestID = this.$route?.params?.request_id) {
      if (!requestID) return this.load(); const generation = ++this.requestGeneration; this.state = 'loading'; this.dataError = null; this.statusMessage = this.t('status.loadingRequest', 'Loading request.')
      try { const response = await this.$C311?.provider?.getStaffRequest?.(requestID); if (generation !== this.requestGeneration) return; const detail = this.normalizeDetail(response); this.detail = detail; this.selectedRequest = detail.request || null; this.relationships = detail.relationships; this.notes = detail.notes; this.audit = detail.audit; this.state = 'populated'; this.statusMessage = this.t('status.requestLoaded', 'Request loaded.'); this.$nextTick(() => this.$el.querySelector('[data-c311-request-detail] h2')?.focus()) } catch (error) { if (generation !== this.requestGeneration) return; this.detail = null; this.dataError = error; this.state = c311StateForError?.(error) || (error?.retryable ? 'retryable-error' : 'terminal-error'); this.statusMessage = this.t('status.requestUnavailable', 'Request unavailable.') }
    },
    async selectRequest (item) { if (!item) return; if (this.$router?.push && Array.isArray(this.$route?.matched)) return this.$router.push({ name: 'c311.staff.detail', params: { request_id: item.request_id } }); return this.loadDetail(item.request_id) },
    backToQueue () { if (this.$router?.push) this.$router.push({ name: 'c311.staff' }); else { this.$route.params.request_id = undefined; this.loadFromRoute() } },
    nextPage () { if (this.page.next_page_token) { this.pageTokenHistory.push(this.page.next_page_token); this.load() } }, previousPage () { if (this.pageTokenHistory.length) { this.pageTokenHistory.pop(); this.load() } },
    async linkConstituent () { if (!this.canLink || this.relationshipBusy || !this.detail) return; this.relationshipBusy = true; this.detailMessage = ''; try { const result = this.normalizeDetail(await this.$C311.provider.linkStaffConstituent(this.detail.request.request_id, { ...this.relationshipForm }, { expectedVersion: this.detail.request.version })); this.detail = result; this.selectedRequest = result.request; this.relationships = result.relationships; this.audit = result.audit; this.detailMessage = this.t('status.relationshipLinked', 'Constituent relationship linked.'); this.relationshipForm.constituent_id = '' } catch (error) { this.actionError = error; this.detailMessage = ''; this.statusMessage = this.t('status.actionUnavailable', 'Action unavailable.') } finally { this.relationshipBusy = false } },
    async unlinkConstituent (relationship) { if (!this.canUnlink || this.relationshipBusy || !this.detail) return; this.relationshipBusy = true; this.detailMessage = ''; try { const result = this.normalizeDetail(await this.$C311.provider.unlinkStaffConstituent(this.detail.request.request_id, relationship.constituent_id, { reason: this.t('status.relationshipUnlinkReason', 'Removed by staff.') }, { expectedVersion: this.detail.request.version })); this.detail = result; this.selectedRequest = result.request; this.relationships = result.relationships; this.audit = result.audit; this.detailMessage = this.t('status.relationshipUnlinked', 'Constituent relationship removed.') } catch (error) { this.actionError = error; this.detailMessage = ''; this.statusMessage = this.t('status.actionUnavailable', 'Action unavailable.') } finally { this.relationshipBusy = false } },
    async createNote () { if (!this.canCreateNote || this.noteBusy || !this.detail) return; this.noteBusy = true; this.detailMessage = ''; try { const note = await this.$C311.provider.createStaffNote(this.detail.request.request_id, { ...this.noteForm }); this.notes = this.notes.concat(note); this.noteForm.body = ''; this.detailMessage = this.t('status.noteAdded', 'Note added.') } catch (error) { this.actionError = error; this.detailMessage = ''; this.statusMessage = this.t('status.actionUnavailable', 'Action unavailable.') } finally { this.noteBusy = false } },
  },
}
</script>
