<template>
  <c311-app-shell
    mode="staff"
    :brand="t('staff.brand', 'City 311 staff')"
    :title="title"
    :status-message="message"
  >
    <template #nav>
      <c311-main-nav
        :items="navItems"
        :label="t('navigation.staff', 'Staff navigation')"
      />
    </template>
    <p
      v-if="message"
      role="status"
      aria-live="polite"
      data-c311-status
    >
      {{ message }}
    </p>
    <div
      v-if="operation"
      class="alert alert-info"
      data-c311-operation
      role="status"
      aria-live="polite"
    >
      <strong>{{ operation.kind }}</strong> · <span data-c311-operation-status aria-live="polite">{{ operation.status }}</span>
      <pre
        v-if="operation.result"
        class="c311-output"
        data-c311-operation-result
      >{{ JSON.stringify(operation.result, null, 2) }}</pre>
      <p
        v-if="operation.error"
        class="mb-0"
        data-c311-operation-error
      >
        {{ operation.error.message }}
      </p>
    </div>
    <c311-error-summary
      v-if="error"
      :errors="errorEntries"
      :title="t('error.review', 'Review the operation result')"
      :field-targets="{ form: 'c311-extension-error' }"
    />
    <div
      v-if="error"
      id="c311-extension-error"
      class="alert alert-danger"
      role="alert"
      tabindex="-1"
      data-c311-error
    >
      {{ error.message || error }}
      <button
        v-if="error.retryable"
        class="btn btn-link"
        data-c311-action="retry"
        @click="load"
      >
        {{ t('action.retry', 'Retry') }}
      </button>
      <div
        v-if="conflictError"
        class="mt-2"
        data-c311-conflict-recovery
      >
        <p
          class="mb-1"
          data-c311-current-version
        >
          {{ t('conflict.serverVersion', 'Server version') }}: {{ conflictError.currentVersion || conflictError.current_version }}
        </p>
        <button
          type="button"
          class="btn btn-link btn-sm pl-0 mr-2"
          data-c311-action="extensions-reload-version"
          @click="reloadWorkflowConflict"
        >
          {{ t('action.reloadVersion', 'Reload current version') }}
        </button>
        <button
          v-if="conflictReloaded"
          type="button"
          class="btn btn-link btn-sm"
          data-c311-action="extensions-reapply"
          @click="reapplyWorkflowConflict"
        >
          {{ t('action.reapply', 'Reapply my changes') }}
        </button>
      </div>
    </div>
    <c311-data-state
      :state="state"
      :error="error"
      @retry="load"
    >
      <template #populated>
        <section
          v-if="mode === 'workflows'"
          data-c311-workflows
        >
          <div class="d-flex justify-content-between align-items-center mb-3">
            <h2>{{ t('workflow.list', 'Workflows') }}</h2>
            <button
              v-if="can('workflow_create')"
              class="btn btn-primary"
              data-c311-action="workflow-create"
              @click="newWorkflow"
            >
              {{ t('workflow.create', 'New workflow') }}
            </button>
          </div>
          <div
            class="form-row border rounded p-3 mb-3"
            data-c311-workflow-editor
          >
            <div class="form-group col-md-4">
              <label for="c311-workflow-name">{{ t('field.name', 'Name') }}</label><input
                id="c311-workflow-name"
                v-model.trim="workflowForm.name"
                class="form-control"
              >
            </div>
            <div class="form-group col-md-4">
              <label for="c311-workflow-trigger">{{ t('workflow.trigger', 'Trigger') }}</label><select
                id="c311-workflow-trigger"
                v-model="workflowForm.trigger"
                class="form-control"
              >
                <option value="SERVICE_REQUEST_CREATED">
                  SERVICE_REQUEST_CREATED
                </option><option value="SERVICE_REQUEST_STATUS_CHANGED">
                  SERVICE_REQUEST_STATUS_CHANGED
                </option>
              </select>
            </div>
            <div class="form-group col-md-4 d-flex align-items-end">
              <button
                v-if="workflowForm.workflow_id ? can('workflow_update') : can('workflow_create')"
                class="btn btn-primary"
                data-c311-action="workflow-save"
                :disabled="busy.workflow"
                @click="saveWorkflow"
              >
                {{ t('action.save', 'Save') }}
              </button>
            </div>
            <div class="form-group col-md-6">
              <label for="c311-workflow-conditions">{{ t('workflow.conditions', 'Conditions (JSON)') }}</label><textarea
                id="c311-workflow-conditions"
                v-model="workflowForm.conditionsText"
                class="form-control"
                rows="2"
              />
            </div>
            <div class="form-group col-md-6">
              <label for="c311-workflow-test-request">{{ t('workflow.testRequest', 'Test request ID') }}</label><input
                id="c311-workflow-test-request"
                v-model.trim="workflowTestRequestID"
                class="form-control"
              >
            </div>
            <div class="form-group col-md-6">
              <label for="c311-workflow-actions">{{ t('workflow.actions', 'Actions (JSON)') }}</label><textarea
                id="c311-workflow-actions"
                v-model="workflowForm.actionsText"
                class="form-control"
                rows="2"
              />
            </div>
          </div>
          <div
            v-for="workflow in workflows"
            :key="workflow.workflow_id"
            class="border rounded p-3 mb-2"
            data-c311-workflow
          >
            <h3>{{ workflow.name }}</h3><p><code>{{ workflow.trigger }}</code> · v{{ workflow.version }} · {{ workflow.active ? t('state.active', 'ACTIVE') : t('state.inactive', 'INACTIVE') }}</p>
            <button
              v-if="can('workflow_update')"
              class="btn btn-link"
              data-c311-action="workflow-edit"
              @click="editWorkflow(workflow)"
            >
              {{ t('action.edit', 'Edit') }}
            </button>
            <button
              v-if="can('workflow_test')"
              class="btn btn-outline-secondary mr-2"
              data-c311-action="workflow-test"
              :disabled="busy.workflow"
              @click="testWorkflow(workflow)"
            >
              {{ t('workflow.test', 'Test') }}
            </button>
            <button
              v-if="workflow.active && can('workflow_deactivate')"
              class="btn btn-outline-warning mr-2"
              data-c311-action="workflow-deactivate"
              @click="toggleWorkflow(workflow)"
            >
              {{ t('workflow.deactivate', 'Deactivate') }}
            </button>
            <button
              v-if="!workflow.active && can('workflow_activate')"
              class="btn btn-outline-success"
              data-c311-action="workflow-activate"
              @click="toggleWorkflow(workflow)"
            >
              {{ t('workflow.activate', 'Activate') }}
            </button>
          </div>
          <p v-if="!workflows.length">
            {{ t('state.empty', 'No data.') }}
          </p>
          <h2>{{ t('workflow.executions', 'Execution log') }}</h2>
          <ul data-c311-workflow-executions>
            <li
              v-for="execution in executions"
              :key="execution.execution_id"
            >
              {{ execution.execution_id }} · {{ execution.outcome }} · {{ execution.error ? execution.error.message : execution.occurred_at }}
            </li>
          </ul>
        </section>
        <section
          v-else-if="mode === 'reports'"
          data-c311-reports
        >
          <h2>{{ t('report.list', 'Reports') }}</h2>
          <ul data-c311-report-catalogue>
            <li
              v-for="item in reportCatalogue"
              :key="item.report_key"
            >
              <strong>{{ item.name }}</strong> · <code>{{ item.report_key }}</code><br>
              {{ t('report.filters', 'Filters') }}: {{ item.supported_filters.join(', ') || '—' }} ·
              {{ t('report.group', 'Grouping') }}: {{ item.supported_grouping.join(', ') || '—' }} ·
              {{ t('report.sort', 'Sort') }}: {{ item.supported_sort.join(', ') || '—' }}
            </li>
          </ul>
          <div class="form-row">
            <div class="form-group col-md-3">
              <label for="c311-report-name">{{ t('field.name', 'Name') }}</label><input
                id="c311-report-name"
                v-model.trim="reportForm.name"
                class="form-control"
              >
            </div>
            <div class="form-group col-md-3">
              <label for="c311-report-entity">{{ t('report.entity', 'Entity') }}</label><select
                id="c311-report-entity"
                v-model="reportForm.entity"
                class="form-control"
              >
                <option value="service_requests">
                  service_requests
                </option><option value="constituents">
                  constituents
                </option><option value="follow_up_actions">
                  follow_up_actions
                </option>
              </select>
            </div>
            <div class="form-group col-md-3">
              <label for="c311-report-columns">{{ t('field.columns', 'Columns (max 20)') }}</label><input
                id="c311-report-columns"
                v-model="reportForm.columnsText"
                class="form-control"
              ><small>{{ reportColumns.length }}/20</small>
            </div>
            <div class="form-group col-md-2">
              <label for="c311-report-group">{{ t('report.group', 'Group (max 1)') }}</label><input
                id="c311-report-group"
                v-model.trim="reportForm.grouping"
                class="form-control"
              >
            </div>
            <div class="form-group col-md-2">
              <label for="c311-report-sort">{{ t('report.sort', 'Sort (max 3)') }}</label><input
                id="c311-report-sort"
                v-model="reportForm.sortText"
                class="form-control"
              ><small>{{ reportSort.length }}/3</small>
            </div>
            <div class="form-group col-md-2">
              <label for="c311-report-filters">{{ t('report.filters', 'Filters (JSON)') }}</label><input
                id="c311-report-filters"
                v-model="reportForm.filtersText"
                class="form-control"
              >
            </div>
            <div class="form-group col-md-2 d-flex align-items-end">
              <button
                v-if="can('saved_report_create')"
                :disabled="!reportValid || busy.report"
                class="btn btn-primary"
                data-c311-action="report-save"
                @click="saveReport"
              >
                {{ t('action.save', 'Save') }}
              </button>
            </div>
          </div>
          <p
            v-if="!reportValid"
            class="text-danger"
            role="alert"
            data-c311-report-validation
          >
            {{ reportValidationMessage }}
          </p>
          <div
            v-for="report in reports"
            :key="report.report_id"
            class="border rounded p-2 mb-2"
            data-c311-report
            :data-c311-report-id="report.report_id"
          >
            <strong>{{ report.name }}</strong> · {{ report.entity }} · {{ report.columns.length }} {{ t('report.columns', 'columns') }}<button
              v-if="can('report_run')"
              class="btn btn-link"
              data-c311-action="report-run"
              @click="runReport(report)"
            >
              {{ t('report.run', 'Run') }}
            </button><button
              v-if="can('saved_report_share')"
              class="btn btn-link"
              data-c311-action="report-share"
              @click="shareReport(report)"
            >
              {{ t('report.share', 'Share') }}
            </button><button
              v-if="can('report_export')"
              class="btn btn-link"
              data-c311-action="report-export"
              @click="exportReport(report)"
            >
              {{ t('report.export', 'Export UTF-8 CSV') }}
            </button>
          </div>
          <pre
            v-if="csvPreview"
            class="c311-output"
            data-c311-report-csv
          >{{ csvPreview }}</pre>
        </section>
        <section
          v-else-if="mode === 'mail'"
          data-c311-mail
        >
          <h2>{{ t('mail.title', 'Mail') }}</h2>
          <div class="form-group">
            <label for="c311-mail-template">{{ t('mail.template', 'Template') }}</label><select
              id="c311-mail-template"
              v-model="mail.template_id"
              class="form-control"
              @change="selectMailTemplate"
            >
              <option value="">
                {{ t('mail.custom', 'Custom message') }}
              </option><option
                v-for="template in mailTemplates"
                :key="template.template_id"
                :value="template.template_id"
              >
                {{ template.name }} ({{ template.template_id }})
              </option>
            </select>
          </div>
          <div class="form-group">
            <label for="c311-mail-to">{{ t('mail.to', 'Recipients') }}</label><input
              id="c311-mail-to"
              v-model="mail.to"
              class="form-control"
            >
          </div><div class="form-group">
            <label for="c311-mail-subject">{{ t('mail.subject', 'Subject') }}</label><input
              id="c311-mail-subject"
              v-model="mail.subject"
              class="form-control"
            >
          </div><div class="form-group">
            <label for="c311-mail-text">{{ t('mail.text', 'Message') }}</label><textarea
              id="c311-mail-text"
              v-model="mail.text"
              class="form-control"
            />
          </div>
          <div class="form-group">
            <label for="c311-mail-html">{{ t('mail.html', 'Rich HTML') }}</label><textarea
              id="c311-mail-html"
              v-model="mail.html"
              class="form-control"
              rows="5"
            />
          </div>
          <button
            v-if="can('mail_preview')"
            class="btn btn-outline-secondary mr-2"
            data-c311-action="mail-preview"
            @click="previewMail"
          >
            {{ t('mail.preview', 'Preview') }}
          </button><button
            v-if="can('mail_send')"
            class="btn btn-primary"
            data-c311-action="mail-send"
            :disabled="busy.mail"
            @click="sendMail"
          >
            {{ t('mail.send', 'Send') }}
          </button><button
            v-if="delivery && delivery.status === 'PENDING'"
            class="btn btn-link"
            data-c311-action="mail-refresh"
            @click="refreshMailDelivery"
          >
            {{ t('action.refresh', 'Refresh status') }}
          </button>
          <button
            v-if="mail.template_id && can('mail_send') && providerSupportsMailTemplates"
            class="btn btn-outline-secondary ml-2"
            data-c311-action="mail-template-save"
            @click="saveMailTemplate"
          >
            {{ t('mail.templateSave', 'Save template') }}
          </button>
          <div
            v-if="mailPreview && mailPreview.sanitized === true"
            class="c311-output c311-mail-preview"
            data-c311-mail-preview
            v-html="mailPreview.html"
          /><p
            v-else-if="mailPreview"
            class="text-danger"
            role="alert"
            data-c311-mail-preview-error
          >
            {{ t('mail.previewUnsafe', 'Preview was rejected because the provider did not mark the HTML as safe.') }}
          </p><p
            v-if="delivery"
            data-c311-mail-delivery
          >
            {{ delivery.delivery_id }} · {{ delivery.status }} · <span data-c311-mail-attempts>{{ delivery.attempts }}</span><span v-if="delivery.error"> · {{ delivery.error.message }}</span>
          </p><p
            v-if="mailFailure.message || (delivery && delivery.error)"
            class="text-danger"
            role="alert"
            data-c311-mail-error
          >
            {{ mailFailure.message || delivery.error.message }}
          </p>
        </section>
        <section
          v-else-if="mode === 'calendar'"
          data-c311-calendar
        >
          <h2>{{ t('calendar.title', 'Calendar') }}</h2><label
            for="c311-calendar-import"
            class="sr-only"
          >{{ t('calendar.import', 'Import ICS') }}</label><input
            id="c311-calendar-import"
            type="file"
            accept="text/calendar,.ics"
            data-c311-action="calendar-import"
            @change="importCalendar"
          ><button
            v-if="can('calendar_export')"
            class="btn btn-primary ml-2"
            data-c311-action="calendar-export"
            @click="exportCalendar"
          >
            {{ t('calendar.export', 'Export ICS') }}
          </button>
          <ul data-c311-calendar-events>
            <li
              v-for="event in calendarEvents"
              :key="event.uid"
            >
              <strong>{{ event.uid }}</strong> · {{ event.summary }} · {{ event.cancelled ? t('calendar.cancelled', 'CANCELLED') : t('calendar.active', 'ACTIVE') }} <button
                v-if="can('calendar_import')"
                class="btn btn-link"
                data-c311-action="calendar-update"
                @click="updateCalendarEvent(event)"
              >
                {{ t('action.update', 'Update') }}
              </button><button
                v-if="can('calendar_import')"
                class="btn btn-link"
                data-c311-action="calendar-cancel"
                @click="cancelCalendarEvent(event)"
              >
                {{ t('action.cancel', 'Cancel') }}
              </button>
            </li>
          </ul><pre
            v-if="calendar"
            class="c311-output"
            data-c311-calendar-content
          >{{ calendar }}</pre>
        </section>
        <section
          v-else-if="mode === 'oauth'"
          data-c311-oauth
        >
          <h2>{{ t('oauth.title', 'Workflow OAuth2 action') }}</h2>
          <div class="form-group">
            <label for="c311-oauth-request">{{ t('field.requestId', 'Request ID') }}</label>
            <input
              id="c311-oauth-request"
              v-model.trim="workflowAction.request_id"
              class="form-control"
            >
          </div>
          <div class="form-group">
            <label for="c311-oauth-action">{{ t('field.action', 'Action') }}</label>
            <input
              id="c311-oauth-action"
              v-model.trim="workflowAction.action"
              class="form-control"
            >
          </div>
          <div class="form-group">
            <label for="c311-oauth-payload">{{ t('field.payload', 'Payload (JSON)') }}</label>
            <textarea
              id="c311-oauth-payload"
              v-model="workflowAction.payloadText"
              class="form-control"
            />
          </div>
          <button
            data-c311-action="workflow-action-execute"
            class="btn btn-primary mr-2"
            :disabled="busy.oauth"
            @click="executeWorkflowOAuthAction"
          >
            {{ t('oauth.execute', 'Execute with OAuth2 client credentials') }}
          </button>
          <p
            data-c311-oauth-status
            role="status"
            aria-live="polite"
          >
            {{ oauthStatus }}
          </p>
          <pre
            v-if="actionExecution"
            class="c311-output"
            data-c311-workflow-action-result
          >{{ JSON.stringify(actionExecution, null, 2) }}</pre>
        </section>
        <section
          v-else
          data-c311-audit
        >
          <h2>{{ t('audit.title', 'Audit and data export') }}</h2>
          <div
            class="form-row"
            data-c311-audit-filters
          >
            <div class="form-group col-md-3">
              <label for="c311-audit-event">{{ t('audit.eventType', 'Event type') }}</label><input
                id="c311-audit-event"
                v-model.trim="auditForm.eventType"
                class="form-control"
              >
            </div>
            <div class="form-group col-md-3">
              <label for="c311-audit-actor">{{ t('audit.actorId', 'Actor ID') }}</label><input
                id="c311-audit-actor"
                v-model.trim="auditForm.actorId"
                class="form-control"
              >
            </div>
            <div class="form-group col-md-3">
              <label for="c311-audit-actor-type">{{ t('audit.actorType', 'Actor type') }}</label><input
                id="c311-audit-actor-type"
                v-model.trim="auditForm.actorType"
                class="form-control"
              >
            </div>
            <div class="form-group col-md-3">
              <label for="c311-audit-entity-id">{{ t('audit.entityId', 'Entity ID') }}</label><input
                id="c311-audit-entity-id"
                v-model.trim="auditForm.entityId"
                class="form-control"
              >
            </div>
            <div class="form-group col-md-3">
              <label for="c311-audit-entity-type">{{ t('audit.entityType', 'Entity type') }}</label><input
                id="c311-audit-entity-type"
                v-model.trim="auditForm.entityType"
                class="form-control"
              >
            </div>
            <div class="form-group col-md-3">
              <label for="c311-audit-request">{{ t('audit.requestId', 'Request ID') }}</label><input
                id="c311-audit-request"
                v-model.trim="auditForm.requestId"
                class="form-control"
              >
            </div>
            <div class="form-group col-md-3">
              <label for="c311-audit-source">{{ t('audit.sourceChannel', 'Source channel') }}</label><input
                id="c311-audit-source"
                v-model.trim="auditForm.sourceChannel"
                class="form-control"
              >
            </div>
            <div class="form-group col-md-3">
              <label for="c311-audit-from">{{ t('audit.from', 'Occurred from') }}</label><input
                id="c311-audit-from"
                v-model.trim="auditForm.occurredFrom"
                class="form-control"
              >
            </div>
            <div class="form-group col-md-3">
              <label for="c311-audit-to">{{ t('audit.to', 'Occurred to') }}</label><input
                id="c311-audit-to"
                v-model.trim="auditForm.occurredTo"
                class="form-control"
              >
            </div>
            <div class="form-group col-md-2">
              <label for="c311-audit-page-size">{{ t('pagination.pageSize', 'Page size') }}</label><input
                id="c311-audit-page-size"
                v-model.number="auditForm.pageSize"
                type="number"
                min="1"
                max="100"
                class="form-control"
              >
            </div>
            <div class="form-group col-md-4">
              <label for="c311-audit-page-token">{{ t('pagination.pageToken', 'Page token') }}</label><input
                id="c311-audit-page-token"
                v-model.trim="auditForm.pageToken"
                class="form-control"
              >
            </div>
            <div class="form-group col-md-3 d-flex align-items-end">
              <button
                class="btn btn-outline-secondary"
                data-c311-action="audit-filter"
                @click="applyAuditFilters"
              >
                {{ t('action.apply', 'Apply') }}
              </button>
              <button
                v-if="auditNextPageToken"
                class="btn btn-link ml-2"
                data-c311-action="audit-next-page"
                @click="nextAuditPage"
              >
                {{ t('pagination.next', 'Next page') }}
              </button>
            </div>
          </div>
          <button
            v-if="can('audit_export')"
            class="btn btn-outline-primary mr-2"
            data-c311-action="audit-export"
            @click="exportAudit"
          >
            {{ t('audit.export', 'Export audit') }}
          </button>
          <div
            v-if="can('contact_email_export')"
            class="form-row mt-3"
            data-c311-contact-email-export
          >
            <div class="form-group col-md-8">
              <label for="c311-contact-email-filters">{{ t('audit.contactEmailFilters', 'Contact email filters (JSON)') }}</label><input
                id="c311-contact-email-filters"
                v-model="contactEmailFiltersText"
                class="form-control"
              >
            </div>
            <div class="form-group col-md-4 d-flex align-items-end">
              <button
                class="btn btn-outline-primary"
                data-c311-action="contact-email-export"
                @click="exportContactEmails"
              >
                {{ t('audit.contactEmailExport', 'Export contact emails') }}
              </button>
            </div>
          </div>
          <div
            class="form-row mt-3"
            data-c311-data-export-form
          >
            <div class="form-group col-md-3">
              <label for="c311-export-entity">{{ t('audit.entity', 'Entity') }}</label><select
                id="c311-export-entity"
                v-model="dataExportForm.entity"
                class="form-control"
              >
                <option value="constituents">
                  constituents
                </option><option value="service-requests">
                  service-requests
                </option><option value="audit-events">
                  audit-events
                </option><option value="follow-up-actions">
                  follow-up-actions
                </option>
              </select>
            </div>
            <div class="form-group col-md-3">
              <label for="c311-export-filters">{{ t('report.filters', 'Filters (JSON)') }}</label><input
                id="c311-export-filters"
                v-model="dataExportForm.filtersText"
                class="form-control"
              >
            </div>
            <div class="form-group col-md-2">
              <label for="c311-export-size">{{ t('pagination.pageSize', 'Page size') }}</label><input
                id="c311-export-size"
                v-model.number="dataExportForm.pageSize"
                type="number"
                min="1"
                max="100"
                class="form-control"
              >
            </div>
            <div class="form-group col-md-2">
              <label for="c311-export-token">{{ t('pagination.pageToken', 'Page token') }}</label><input
                id="c311-export-token"
                v-model.trim="dataExportForm.pageToken"
                class="form-control"
              >
            </div>
            <div class="form-group col-md-2">
              <label for="c311-export-updated">{{ t('audit.updatedSince', 'Updated since') }}</label><input
                id="c311-export-updated"
                v-model.trim="dataExportForm.updatedSince"
                class="form-control"
              >
            </div>
          </div>
          <button
            v-if="hasExportScope"
            class="btn btn-primary"
            data-c311-action="data-export"
            @click="exportData"
          >
            {{ t('audit.dataExport', 'Export data') }}
          </button>
          <pre
            v-if="exportResult"
            class="c311-output"
            data-c311-data-export-result
          >{{ JSON.stringify(exportResult, null, 2) }}</pre>
          <ul>
            <li
              v-for="event in audits"
              :key="event.audit_id"
            >
              {{ event.event_type }} · {{ event.entity_id }} · {{ event.occurred_at }}
            </li>
          </ul>
          <p
            v-if="!audits.length"
            data-c311-audit-empty
          >
            {{ t('state.empty', 'No data.') }}
          </p>
        </section>
      </template>
      <template #empty>
        <p>{{ t('state.empty', 'No data.') }}</p>
      </template>
    </c311-data-state>
  </c311-app-shell>
</template>
<script>
import { components, c311 } from '@cortezaproject/corteza-vue'

const FIXTURE_TIME = '2026-01-15T15:00:00.000Z'
const AUDIT_FILTER_STORAGE_KEY = 'c311:fe09:audit-filters'
const c311StateForError = c311?.c311StateForError

export default {
  name: 'C311Extensions',
  components: { C311AppShell: components.C311AppShell, C311DataState: components.C311DataState, C311ErrorSummary: components.C311ErrorSummary, C311MainNav: components.C311MainNav },
  data: () => ({ state: 'loading', error: null, conflictError: null, conflictReloaded: false, pendingWorkflowConflict: null, message: '', operation: null, workflows: [], executions: [], reports: [], reportCatalogue: [], audits: [], auditNextPageToken: null, workflowForm: { workflow_id: '', name: '', trigger: 'SERVICE_REQUEST_CREATED', conditionsText: '[]', actionsText: '[]', version: 1 }, workflowTestRequestID: 'request-fixture-001', workflowAction: { request_id: 'request-fixture-001', action: 'notify_department', payloadText: '{}' }, actionExecution: null, reportForm: { name: '', entity: 'service_requests', columnsText: 'request_number,status', grouping: '', sortText: '-created_at', filtersText: '{}' }, mail: { to: '', subject: '', text: '', html: '', template_id: '' }, mailTemplates: [], mailPreview: null, delivery: null, mailFailure: {}, calendar: '', calendarEvents: [], oauthStatus: '', csvPreview: '', auditForm: { eventType: '', actorId: '', actorType: '', entityId: '', entityType: '', requestId: '', sourceChannel: '', occurredFrom: '', occurredTo: '', pageSize: 50, pageToken: '' }, contactEmailFiltersText: '{}', dataExportForm: { entity: 'constituents', filtersText: '{}', pageSize: 50, pageToken: '', updatedSince: '' }, exportResult: null, mode: 'audit', busy: { workflow: false, report: false, mail: false, calendar: false, oauth: false } }),
  computed: {
    title () { return ({ workflows: this.t('workflow.title', 'Workflows'), reports: this.t('report.title', 'Reports'), mail: this.t('mail.title', 'Mail'), calendar: this.t('calendar.title', 'Calendar'), oauth: this.t('oauth.title', 'Workflow OAuth2 action'), audit: this.t('audit.title', 'Audit and export') })[this.mode] },
    navItems () { return [{ route: '/c311/staff/workflows', label: this.t('workflow.title', 'Workflows'), capability: 'workflow_list' }, { route: '/c311/staff/reports', label: this.t('report.title', 'Reports'), capability: 'report_catalogue' }, { route: '/c311/staff/mail', label: this.t('mail.title', 'Mail'), capability: 'mail_preview' }, { route: '/c311/staff/calendar', label: this.t('calendar.title', 'Calendar'), capability: 'calendar_export' }, { route: '/c311/staff/audit', label: this.t('audit.title', 'Audit'), capability: 'audit_list' }, { route: '/c311/staff/oauth', label: this.t('oauth.title', 'Workflow OAuth2 action'), scope: 'workflow.execute' }].filter(item => (!item.capability || this.can(item.capability)) && (!item.scope || this.hasScope(item.scope))) },
    hasExportScope () { return !!this.$C311?.session?.actor?.scopes?.includes('crm.export') },
    providerSupportsMailTemplates () { return typeof this.$C311?.provider?.listMailTemplates === 'function' && typeof this.$C311?.provider?.updateMailTemplate === 'function' },
    reportColumns () { return this.reportForm.columnsText.split(',').map(value => value.trim()).filter(Boolean) },
    reportSort () { return this.reportForm.sortText.split(',').map(value => value.trim()).filter(Boolean) },
    reportFilters () { try { return JSON.parse(this.reportForm.filtersText || '{}') } catch (_error) { return null } },
    errorEntries () {
      const value = this.error || {}
      return [{ field: 'form', code: value.code || value.error || 'OPERATION_FAILED', message: value.message || String(value) }]
    },
    reportValid () { return !!this.reportForm.name && this.reportColumns.length > 0 && this.reportColumns.length <= 20 && this.reportFilters && !Array.isArray(this.reportFilters) && !this.reportForm.grouping.split(',').filter(Boolean).slice(1).length && this.reportSort.length <= 3 },
    reportValidationMessage () { if (!this.reportForm.name) return this.t('report.nameRequired', 'Name is required.'); if (this.reportColumns.length > 20) return this.t('report.tooManyColumns', 'A report can contain at most 20 columns.'); if (this.reportSort.length > 3) return this.t('report.tooManySort', 'A report can contain at most 3 sort fields.'); if (!this.reportFilters || Array.isArray(this.reportFilters)) return this.t('report.invalidFilters', 'Filters must be a JSON object.'); return this.t('report.invalid', 'Report definition is invalid.') },
  },
  watch: {
    '$route.path': 'load',
    workflowForm: { deep: true, handler () { this.persistExtensionForms() } },
    reportForm: { deep: true, handler () { this.persistExtensionForms() } },
  },
  created () { this.restoreAuditFilters(); this.restoreExtensionForms(); this.load() },
  methods: {
    t (key, fallback) { const value = this.$t?.(`c311:${key}`); return value && value !== `c311:${key}` && value !== key ? value : fallback },
    can (capability) { return !!this.$C311?.can?.(capability) || !!this.$C311?.session?.actor?.capabilities?.includes(capability) },
    hasScope (scope) { return !!this.$C311?.hasScope?.(scope) || !!this.$C311?.session?.actor?.scopes?.includes(scope) },
    persistExtensionForms () {
      try {
        sessionStorage.setItem('c311:fe10:extension-forms', JSON.stringify({
          workflowForm: this.workflowForm,
          reportForm: this.reportForm,
        }))
      } catch (error) {
        // Browser storage is optional and must never block editing.
        return undefined
      }
    },
    restoreExtensionForms () {
      try {
        const raw = sessionStorage.getItem('c311:fe10:extension-forms')
        if (!raw) return
        const saved = JSON.parse(raw)
        if (saved.workflowForm && typeof saved.workflowForm === 'object') this.workflowForm = { ...this.workflowForm, ...saved.workflowForm }
        if (saved.reportForm && typeof saved.reportForm === 'object') this.reportForm = { ...this.reportForm, ...saved.reportForm }
      } catch (error) {
        return undefined
      }
    },
    async withBusy (name, task) {
      if (this.busy[name]) return
      this.$set(this.busy, name, true)
      try { return await task() } finally { this.$set(this.busy, name, false) }
    },
    async load () { this.error = null; this.operation = null; this.state = 'loading'; this.mode = this.$route.path.includes('/workflows') ? 'workflows' : this.$route.path.includes('/reports') ? 'reports' : this.$route.path.includes('/mail') ? 'mail' : this.$route.path.includes('/calendar') ? 'calendar' : this.$route.path.includes('/oauth') ? 'oauth' : 'audit'; try { if (this.mode === 'workflows') { const page = await this.$C311.provider.listWorkflows(); this.workflows = page.items; this.executions = (await this.$C311.provider.listWorkflowExecutions()).items } else if (this.mode === 'reports') { this.reportCatalogue = (await this.$C311.provider.listReportCatalogue()).items; this.reports = (await this.$C311.provider.listReports()).items } else if (this.mode === 'mail') { await this.loadMailTemplates() } else if (this.mode === 'calendar') await this.refreshCalendar(); else if (this.mode === 'audit') await this.loadAudit(); this.state = this.mode === 'audit' ? 'populated' : (this.mode === 'workflows' ? this.workflows : this.mode === 'reports' ? this.reports : [1]).length ? 'populated' : 'empty' } catch (error) { this.error = error; this.state = c311StateForError?.(error) || (error.retryable ? 'retryable-error' : 'terminal-error') } },
    async resolveOperation (pending) { this.operation = pending; if (!pending?.operation_id) return pending; let result = pending; let attempts = 0; while (['PENDING', 'RUNNING'].includes(result.status) && attempts < 5) { result = await this.$C311.provider.getOperation(pending.operation_id); this.operation = result; attempts += 1 } if (result.status === 'FAILED') this.error = result.error || new Error(this.t('operation.failed', 'Operation failed.')); return result },
    newWorkflow () { this.workflowForm = { workflow_id: '', name: '', trigger: 'SERVICE_REQUEST_CREATED', conditionsText: '[]', actionsText: '[]', version: 1 } },
    editWorkflow (workflow) { this.workflowForm = { workflow_id: workflow.workflow_id, name: workflow.name, trigger: workflow.trigger, conditionsText: JSON.stringify(workflow.conditions || []), actionsText: JSON.stringify(workflow.actions || []), version: workflow.version } },
    parseJSON (value) { try { return JSON.parse(value) } catch (_error) { throw new Error(this.t('workflow.invalidJson', 'Conditions and actions must be valid JSON.')) } },
    async saveWorkflow () { return this.withBusy('workflow', async () => { let input; try { input = { workflow_id: this.workflowForm.workflow_id || 'workflow-local-001', name: this.workflowForm.name, trigger: this.workflowForm.trigger, active: false, conditions: this.parseJSON(this.workflowForm.conditionsText), actions: this.parseJSON(this.workflowForm.actionsText), version: this.workflowForm.version, updated_at: FIXTURE_TIME }; const result = input.workflow_id === 'workflow-local-001' && !this.workflows.some(item => item.workflow_id === input.workflow_id) ? await this.$C311.provider.createWorkflow(input) : await this.$C311.provider.updateWorkflow(input.workflow_id, input, { expectedVersion: input.version }); const index = this.workflows.findIndex(item => item.workflow_id === result.workflow_id); if (index < 0) this.workflows.push(result); else this.$set(this.workflows, index, result); this.workflowForm.version = result.version; this.error = null; this.conflictError = null; this.pendingWorkflowConflict = null; this.message = this.t('workflow.saved', 'Workflow saved.') } catch (error) { this.error = error; if (error?.status === 409 && input) { this.conflictError = error; this.conflictReloaded = false; this.pendingWorkflowConflict = { ...input } } } }) },
    async testWorkflow (workflow) { try { const result = await this.resolveOperation(await this.$C311.provider.testWorkflow(workflow.workflow_id, { request_id: this.workflowTestRequestID })); this.executions = (await this.$C311.provider.listWorkflowExecutions()).items; this.message = `${result.operation_id} · ${result.status}` } catch (error) { this.error = error } },
    async reloadWorkflowConflict () { const pending = this.pendingWorkflowConflict; if (!pending) return; try { const server = await this.$C311.provider.getWorkflow(pending.workflow_id); const index = this.workflows.findIndex(item => item.workflow_id === server.workflow_id); if (index >= 0) this.$set(this.workflows, index, server); this.workflowForm = { ...this.workflowForm, workflow_id: server.workflow_id, trigger: server.trigger, version: server.version }; this.conflictReloaded = true; this.error = null } catch (error) { this.error = error } },
    async reapplyWorkflowConflict () { const pending = this.pendingWorkflowConflict; if (!pending || !this.conflictReloaded) return; const current = this.workflows.find(item => item.workflow_id === pending.workflow_id); if (!current) return; try { const result = await this.$C311.provider.updateWorkflow(current.workflow_id, { ...current, name: pending.name, trigger: pending.trigger, conditions: pending.conditions, actions: pending.actions, active: current.active, version: current.version, updated_at: FIXTURE_TIME }, { expectedVersion: current.version }); const index = this.workflows.findIndex(item => item.workflow_id === result.workflow_id); if (index >= 0) this.$set(this.workflows, index, result); this.workflowForm = { ...this.workflowForm, version: result.version }; this.conflictError = null; this.pendingWorkflowConflict = null; this.conflictReloaded = false; this.error = null; this.message = this.t('workflow.saved', 'Workflow saved.') } catch (error) { this.error = error; this.conflictError = error?.status === 409 ? error : null } },
    async toggleWorkflow (workflow) { try { const fn = workflow.active ? 'deactivateWorkflow' : 'activateWorkflow'; const updated = await this.$C311.provider[fn](workflow.workflow_id, { expectedVersion: workflow.version }); Object.assign(workflow, updated) } catch (error) { this.error = error } },
    async saveReport () { if (!this.reportValid) return; return this.withBusy('report', async () => { try { const input = { report_id: 'report-local-001', name: this.reportForm.name, entity: this.reportForm.entity, columns: this.reportColumns, filters: this.reportFilters, grouping: this.reportForm.grouping || null, sort: this.reportSort, version: 1, updated_at: FIXTURE_TIME }; this.reports.push(await this.$C311.provider.createReport(input)); this.message = this.t('report.saved', 'Report saved.') } catch (error) { this.error = error } }) },
    async runReport (report) { try { const result = await this.resolveOperation(await this.$C311.provider.runReport({ definition: report })); this.message = `${result.operation_id} · ${result.status}` } catch (error) { this.error = error } },
    async shareReport (report) { try { this.message = (await this.$C311.provider.shareReport(report.report_id, { roles: ['supervisor'] }, { expectedVersion: report.version })).report_id } catch (error) { this.error = error } },
    async exportReport (report) { try { const result = await this.resolveOperation(await this.$C311.provider.exportReport(report.report_id, { format: 'CSV', idempotencyKey: `report-${report.report_id}` })); const csv = String(result.result?.body || ''); if (!csv) throw new Error(this.t('report.csvInvalid', 'CSV encoding is invalid.')); if (typeof TextEncoder !== 'undefined' && typeof TextDecoder !== 'undefined') { const decoded = new TextDecoder('utf-8', { fatal: true }).decode(new TextEncoder().encode(csv)); if (decoded !== csv) throw new Error(this.t('report.csvInvalid', 'CSV encoding is invalid.')) } this.csvPreview = csv; this.message = `${result.operation_id} · ${result.status}` } catch (error) { this.error = error } },
    mailInput () { return { to: this.mail.to.split(',').map(value => value.trim()).filter(Boolean), subject: this.mail.subject, text: this.mail.text, html: this.mail.html || `<p>${this.mail.text}</p>`, template_id: this.mail.template_id || null } },
    mailIdempotencyKey () { const value = JSON.stringify(this.mailInput()); let hash = 2166136261; for (let index = 0; index < value.length; index += 1) hash = Math.imul(hash ^ value.charCodeAt(index), 16777619); return `mail-${(hash >>> 0).toString(16)}` },
    async loadMailTemplates () { if (!this.providerSupportsMailTemplates) return; this.mailTemplates = await this.$C311.provider.listMailTemplates() },
    selectMailTemplate () { const template = this.mailTemplates.find(item => item.template_id === this.mail.template_id); if (!template) return; this.mail.subject = template.subject; this.mail.text = template.text; this.mail.html = template.html },
    async saveMailTemplate () { if (!this.mail.template_id || !this.providerSupportsMailTemplates) return; try { const template = await this.$C311.provider.updateMailTemplate(this.mail.template_id, { name: this.mailTemplates.find(item => item.template_id === this.mail.template_id)?.name || this.mail.template_id, subject: this.mail.subject, text: this.mail.text, html: this.mail.html || `<p>${this.mail.text}</p>` }); const index = this.mailTemplates.findIndex(item => item.template_id === template.template_id); if (index >= 0) this.$set(this.mailTemplates, index, template); this.message = this.t('mail.templateSaved', 'Mail template saved.') } catch (error) { this.mailFailure = error } },
    async previewMail () { try { this.mailPreview = await this.$C311.provider.previewMail(this.mailInput()); this.mailFailure = {} } catch (error) { this.mailFailure = error } },
    async sendMail () { return this.withBusy('mail', async () => { try { this.delivery = await this.$C311.provider.sendMail(this.mailInput(), { idempotencyKey: this.mailIdempotencyKey() }); this.mailFailure = {} } catch (error) { this.mailFailure = error } }) },
    async refreshMailDelivery () { if (!this.delivery) return; try { this.delivery = await this.$C311.provider.getMailDelivery(this.delivery.delivery_id); this.mailFailure = {} } catch (error) { this.mailFailure = error } },
    async importCalendar (event) { const file = event.target.files?.[0]; if (!file) return; try { const result = await this.resolveOperation(await this.$C311.provider.importCalendar({ ics: await file.text() })); this.message = `${result.operation_id} · ${result.status}`; await this.refreshCalendar() } catch (error) { this.error = error } },
    parseCalendar (ics) {
      const events = []
      let properties = null
      String(ics || '').split(/\r?\n/).forEach(line => {
        if (line === 'BEGIN:VEVENT') {
          properties = []
          return
        }
        if (line === 'END:VEVENT') {
          if (!properties) return
          const field = name => {
            const property = properties.find(item => item.name === name)
            return property?.value || ''
          }
          const property = name => properties.find(item => item.name === name) || { value: '', timezone: '' }
          const start = property('DTSTART')
          const end = property('DTEND')
          const event = {
            uid: field('UID'),
            summary: field('SUMMARY'),
            description: field('DESCRIPTION'),
            dtstart: start.value,
            dtend: end.value,
            rrule: field('RRULE'),
            last_modified: field('LAST-MODIFIED'),
            timezone: start.timezone || end.timezone || '',
            cancelled: field('STATUS') === 'CANCELLED',
          }
          if (event.uid) events.push(event)
          properties = null
          return
        }
        if (!properties) return
        const separator = line.indexOf(':')
        if (separator < 1) return
        const nameAndParams = line.slice(0, separator).split(';')
        const name = nameAndParams.shift().toUpperCase()
        const timezone = nameAndParams.find(param => param.startsWith('TZID='))?.slice(5) || ''
        properties.push({ name, timezone, value: line.slice(separator + 1).trim() })
      })
      return events
    },
    async refreshCalendar () { this.calendar = (await this.$C311.provider.exportCalendar()).body; this.calendarEvents = this.parseCalendar(this.calendar) },
    calendarImportBody (event, cancelled = false) { const timezone = event.timezone || 'America/New_York'; const start = event.dtstart || '20260115T100000'; const end = event.dtend || '20260115T110000'; const modified = event.last_modified || '20260115T090000Z'; const dateLine = (name, value) => `${name}${String(value).endsWith('Z') ? '' : `;TZID=${timezone}`}:${value}`; return ['BEGIN:VCALENDAR', 'VERSION:2.0', 'BEGIN:VEVENT', `UID:${event.uid}`, `SUMMARY:${cancelled ? event.summary : `${event.summary} (updated)`}`, `DESCRIPTION:${event.description || 'Fixture calendar event.'}`, dateLine('DTSTART', start), dateLine('DTEND', end), ...(event.rrule ? [`RRULE:${event.rrule}`] : []), `STATUS:${cancelled ? 'CANCELLED' : 'CONFIRMED'}`, `LAST-MODIFIED:${modified}`, 'END:VEVENT', 'END:VCALENDAR', ''].join('\r\n') },
    async updateCalendarEvent (event) { try { const result = await this.resolveOperation(await this.$C311.provider.importCalendar({ ics: this.calendarImportBody(event) })); await this.refreshCalendar(); this.message = `${this.t('calendar.updated', 'Calendar event updated.')} ${result.status}` } catch (error) { this.error = error } },
    async cancelCalendarEvent (event) { try { const result = await this.resolveOperation(await this.$C311.provider.importCalendar({ ics: this.calendarImportBody(event, true) })); await this.refreshCalendar(); this.message = `${this.t('calendar.cancelledMessage', 'Calendar event cancelled.')} ${result.status}` } catch (error) { this.error = error } },
    async exportCalendar () { try { await this.refreshCalendar() } catch (error) { this.error = error } },
    auditFilters () { return { ...(this.auditForm.eventType ? { event_type: [this.auditForm.eventType] } : {}), ...(this.auditForm.actorId ? { actor_id: [this.auditForm.actorId] } : {}), ...(this.auditForm.actorType ? { actor_type: [this.auditForm.actorType] } : {}), ...(this.auditForm.entityId ? { entity_id: [this.auditForm.entityId] } : {}), ...(this.auditForm.entityType ? { entity_type: [this.auditForm.entityType] } : {}), ...(this.auditForm.requestId ? { request_id: [this.auditForm.requestId] } : {}), ...(this.auditForm.sourceChannel ? { source_channel: [this.auditForm.sourceChannel] } : {}), ...(this.auditForm.occurredFrom ? { occurred_from: this.auditForm.occurredFrom } : {}), ...(this.auditForm.occurredTo ? { occurred_to: this.auditForm.occurredTo } : {}) } },
    persistAuditFilters () { try { sessionStorage.setItem(AUDIT_FILTER_STORAGE_KEY, JSON.stringify(this.auditForm)) } catch (_error) { /* Browser storage is optional. */ } },
    restoreAuditFilters () { try { Object.assign(this.auditForm, JSON.parse(sessionStorage.getItem(AUDIT_FILTER_STORAGE_KEY) || '{}')) } catch (_error) { /* Browser storage is optional. */ } },
    async loadAudit () { this.persistAuditFilters(); const page = await this.$C311.provider.listAuditEvents({ filters: this.auditFilters(), page_size: this.auditForm.pageSize, ...(this.auditForm.pageToken ? { page_token: this.auditForm.pageToken } : {}) }); this.audits = page.items; this.auditNextPageToken = page.next_page_token },
    async applyAuditFilters () { this.auditForm.pageToken = ''; this.auditNextPageToken = null; await this.loadAudit() },
    async nextAuditPage () { if (!this.auditNextPageToken) return; this.auditForm.pageToken = this.auditNextPageToken; await this.loadAudit() },
    async exportAudit () { try { const result = await this.resolveOperation(await this.$C311.provider.exportAuditEvents(this.auditFilters())); this.message = `${result.operation_id} · ${result.status}` } catch (error) { this.error = error } },
    async exportContactEmails () { try { const filters = this.parseJSON(this.contactEmailFiltersText || '{}'); const result = await this.resolveOperation(await this.$C311.provider.exportContactEmails({ filters })); this.message = `${result.operation_id} · ${result.status}` } catch (error) { this.error = error } },
    async exportData () { try { const filters = this.parseJSON(this.dataExportForm.filtersText || '{}'); this.exportResult = await this.$C311.provider.exportData(this.dataExportForm.entity, { filters, page_size: this.dataExportForm.pageSize, ...(this.dataExportForm.pageToken ? { page_token: this.dataExportForm.pageToken } : {}), ...(this.dataExportForm.updatedSince ? { updated_since: this.dataExportForm.updatedSince } : {}) }); this.message = this.t('audit.exportReady', 'Export page is ready.') } catch (error) { this.error = error } },
    async executeWorkflowOAuthAction () { return this.withBusy('oauth', async () => { try { const payload = this.parseJSON(this.workflowAction.payloadText || '{}'); const accepted = await this.$C311.provider.executeWorkflowAction({ action: this.workflowAction.action, request_id: this.workflowAction.request_id, payload }, { idempotencyKey: `workflow-action-${this.workflowAction.request_id}-${this.workflowAction.action}` }); this.actionExecution = await this.$C311.provider.getWorkflowExecution(accepted.execution_id); this.oauthStatus = `${accepted.execution_id} · ${accepted.accepted_at} · ${this.actionExecution.outcome}` } catch (error) { this.error = error; this.oauthStatus = `${error.code || error.error || 'ERROR'}: ${error.message || this.t('oauth.failed', 'Workflow OAuth2 action failed.')}` } }) },
  },
}
</script>
<style scoped>
.c311-output {
  max-width: 100%;
  overflow-x: auto;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
</style>
