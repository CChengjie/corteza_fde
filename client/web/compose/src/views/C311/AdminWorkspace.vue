<template>
  <c311-app-shell title="City 311 administration" main-id="c311-admin-main">
    <template #nav><c311-main-nav :items="navItems" aria-label="Administration navigation" /></template>
    <c311-data-state v-if="state !== 'populated'" :state="state" :error="error" @retry="load" />
    <template v-else>
      <h1 class="h3 mb-3">City 311 administration</h1>
      <div v-if="message" class="alert alert-success" role="status">{{ message }}</div>
      <c311-error-summary :errors="formErrors" title="Review this operation" />
      <section v-if="identity" class="mb-4" aria-labelledby="identity-heading">
        <h2 id="identity-heading" class="h4">Identity configuration</h2>
        <form @submit.prevent="saveIdentity"><div class="form-check"><input id="oidc-enabled" v-model="identity.oidc_enabled" class="form-check-input" type="checkbox"><label for="oidc-enabled" class="form-check-label">Enable OpenID Connect</label></div><div class="form-check"><input id="saml-enabled" v-model="identity.saml_enabled" class="form-check-input" type="checkbox"><label for="saml-enabled" class="form-check-label">Enable SAML</label></div><p class="text-muted mt-2">OIDC secret configured: {{ identity.oidc_client_secret_configured ? 'Yes' : 'No' }}</p><button class="btn btn-primary" :disabled="busy">Save identity configuration</button></form>
      </section>
      <section v-if="integrations !== null" class="mb-4" aria-labelledby="integrations-heading"><h2 id="integrations-heading" class="h4">Integrations</h2><table class="table table-sm"><thead><tr><th>ID</th><th>Kind</th><th>Active</th><th>Secret configured</th><th /></tr></thead><tbody><tr v-for="item in integrations" :key="item.integration_id"><td>{{ item.integration_id }}</td><td>{{ item.kind }}</td><td>{{ item.active ? 'Yes' : 'No' }}</td><td>{{ item.secret_configured ? 'Yes' : 'No' }}</td><td><button class="btn btn-sm btn-outline-primary" :disabled="busy" @click="toggleIntegration(item)">{{ item.active ? 'Disable' : 'Enable' }}</button><button class="btn btn-sm btn-outline-secondary ml-1" :disabled="busy" @click="rotateIntegration(item)">Rotate secret</button></td></tr></tbody></table></section>
      <section v-if="branding" class="mb-4" aria-labelledby="branding-heading">
        <h2 id="branding-heading" class="h4">Branding</h2>
        <form @submit.prevent="saveBranding">
          <div class="form-row"><div class="form-group col-md-6"><label for="org-name">Organisation name</label><input id="org-name" v-model.trim="branding.organisation_name" class="form-control"></div><div class="form-group col-md-3"><label for="primary-colour">Primary colour</label><input id="primary-colour" v-model.trim="branding.primary_colour" class="form-control"></div><div class="form-group col-md-3"><label for="accent-colour">Accent colour</label><input id="accent-colour" v-model.trim="branding.accent_colour" class="form-control"></div></div>
          <button class="btn btn-primary" :disabled="busy">Save branding</button><button class="btn btn-outline-primary ml-2" type="button" :disabled="busy" @click="publishBranding">Publish</button>
        </form>
      </section>
      <section v-if="content !== null" class="mb-4" aria-labelledby="content-heading">
        <h2 id="content-heading" class="h4">Public content</h2>
        <div class="form-group"><label for="content-key">Content item</label><select id="content-key" v-model="selectedContentKey" class="form-control" @change="selectContent"><option v-for="item in content" :key="item.content_key" :value="item.content_key">{{ item.content_key }} ({{ item.state }})</option></select></div>
        <form v-if="selectedContent" @submit.prevent="saveContent"><label for="content-body">Sanitized HTML</label><textarea id="content-body" v-model="selectedContent.body" class="form-control" rows="6" /><button class="btn btn-primary mt-2" :disabled="busy">Save draft</button><button class="btn btn-outline-primary mt-2 ml-2" type="button" :disabled="busy" @click="publishContent">Publish</button></form>
      </section>
      <section v-if="categories !== null" class="mb-4" aria-labelledby="categories-heading">
        <h2 id="categories-heading" class="h4">Contact categories</h2>
        <table class="table table-sm"><thead><tr><th>Code</th><th>English label</th><th>Active</th></tr></thead><tbody><tr v-for="item in categories" :key="item.code"><td>{{ item.code }}</td><td>{{ item.labels.EN }}</td><td>{{ item.active ? 'Yes' : 'No' }}</td></tr></tbody></table>
        <form class="form-row" @submit.prevent="createCategory"><div class="col-md-3"><label for="category-code">Code</label><input id="category-code" v-model.trim="category.code" class="form-control"></div><div class="col-md-5"><label for="category-label">English label</label><input id="category-label" v-model.trim="category.label" class="form-control"></div><div class="col-md-2 d-flex align-items-end"><button class="btn btn-outline-primary" :disabled="busy">Add category</button></div></form>
      </section>
      <section v-if="customFields !== null" class="mb-4" aria-labelledby="fields-heading"><h2 id="fields-heading" class="h4">Custom fields</h2><table class="table table-sm"><thead><tr><th>Key</th><th>Entity</th><th>Type</th><th>Required</th><th>Active</th></tr></thead><tbody><tr v-for="item in customFields" :key="item.key"><td>{{ item.key }}</td><td>{{ item.entity }}</td><td>{{ item.field_type }}</td><td>{{ item.required ? 'Yes' : 'No' }}</td><td>{{ item.active ? 'Yes' : 'No' }}</td></tr></tbody></table></section>
      <section v-if="workflows !== null" aria-labelledby="workflow-heading"><h2 id="workflow-heading" class="h4">Workflows</h2><table class="table table-sm"><thead><tr><th>Name</th><th>Trigger</th><th>State</th><th /></tr></thead><tbody><tr v-for="item in workflows" :key="item.workflow_id"><td>{{ item.name }}</td><td>{{ item.trigger }}</td><td>{{ item.active ? 'Active' : 'Inactive' }}</td><td><button class="btn btn-sm btn-outline-primary" :disabled="busy" @click="toggleWorkflow(item)">{{ item.active ? 'Deactivate' : 'Activate' }}</button></td></tr></tbody></table><p class="text-muted" role="status">{{ executions ? executions.length : 0 }} workflow executions loaded.</p></section>
    </template>
  </c311-app-shell>
</template>

<script>
import { components, c311 } from '@cortezaproject/corteza-vue'
const { C311AppShell, C311DataState, C311ErrorSummary, C311MainNav } = components
const stateForError = c311?.c311StateForError
export default {
  name: 'C311AdminWorkspace', components: { C311AppShell, C311DataState, C311ErrorSummary, C311MainNav },
  data: () => ({ state: 'loading', error: null, busy: false, message: '', formErrors: [], identity: null, integrations: null, branding: null, content: null, selectedContentKey: '', selectedContent: null, categories: null, customFields: null, workflows: null, executions: null, category: { code: '', label: '' } }),
  computed: { provider () { return this.$C311?.provider }, navItems () { return [{ route: '/c311/admin', label: 'Administration' }, { route: '/c311/staff/requests', label: 'Requests' }, { route: '/c311', label: 'Public portal' }] } },
  created () { this.load() },
  methods: {
    fail (error) { this.error = error; this.formErrors = [{ field: 'form', code: error?.code || error?.error || 'OPERATION_FAILED', message: error?.message || 'The operation could not be completed.' }] },
    async load () { this.state = 'loading'; this.error = null; const results = await Promise.allSettled([this.provider.getIdentityConfiguration(), this.provider.listIntegrations({ page_size: 100 }), this.provider.getAdminBranding(), this.provider.listAdminContent({ page_size: 100 }), this.provider.listAdminCategories({ page_size: 100 }), this.provider.listAdminCustomFields({ page_size: 100 }), this.provider.listWorkflows({ page_size: 100 }), this.provider.listWorkflowExecutions({ page_size: 100 })]); const value = (index, fallback) => results[index].status === 'fulfilled' ? results[index].value : fallback; this.identity = value(0, null); const integrations = value(1, null); this.integrations = integrations ? integrations.items || [] : null; this.branding = value(2, null); const content = value(3, null); this.content = content ? content.items || [] : null; const categories = value(4, null); this.categories = categories ? categories.items || [] : null; const customFields = value(5, null); this.customFields = customFields ? customFields.items || [] : null; const workflows = value(6, null); this.workflows = workflows ? workflows.items || [] : null; const executions = value(7, null); this.executions = executions ? executions.items || [] : null; this.selectedContentKey = this.content?.[0]?.content_key || ''; this.selectContent(); if (results.every(result => result.status === 'rejected')) { this.error = results[0].reason; this.state = stateForError?.(this.error) || 'terminal-error' } else this.state = 'populated' },
    selectContent () { const item = this.content.find(value => value.content_key === this.selectedContentKey); this.selectedContent = item ? { ...item } : null },
    async run (operation, message) { if (this.busy) return; this.busy = true; this.formErrors = []; this.message = ''; try { await operation(); this.message = message; await this.load() } catch (error) { this.fail(error) } finally { this.busy = false } },
    saveBranding () { return this.run(() => this.provider.updateBranding(this.branding, { expectedVersion: this.branding.version }), 'Branding draft saved.') },
    saveIdentity () { return this.run(() => this.provider.updateIdentityConfiguration({ oidc_enabled: this.identity.oidc_enabled, saml_enabled: this.identity.saml_enabled }, { expectedVersion: this.identity.version }), 'Identity configuration saved.') },
    toggleIntegration (integration) { return this.run(() => this.provider.updateIntegration(integration.integration_id, { active: !integration.active }, { expectedVersion: integration.version }), 'Integration updated.') },
    rotateIntegration (integration) { return this.run(() => this.provider.rotateIntegrationSecret(integration.integration_id, { expectedVersion: integration.version }), 'Integration secret rotated.') },
    publishBranding () { return this.run(() => this.provider.publishBranding({ expectedVersion: this.branding.version }), 'Branding published.') },
    saveContent () { return this.run(() => this.provider.updateAdminContent(this.selectedContent.content_key, { body: this.selectedContent.body }, { expectedVersion: this.selectedContent.version }), 'Content draft saved.') },
    publishContent () { return this.run(() => this.provider.publishAdminContent(this.selectedContent.content_key, { expectedVersion: this.selectedContent.version }), 'Content published.') },
    createCategory () { const input = { code: this.category.code, active: true, labels: { EN: this.category.label } }; return this.run(async () => { await this.provider.createAdminCategory(input); this.category = { code: '', label: '' } }, 'Category created.') },
    toggleWorkflow (workflow) { const operation = workflow.active ? this.provider.deactivateWorkflow(workflow.workflow_id, { expectedVersion: workflow.version }) : this.provider.activateWorkflow(workflow.workflow_id, { expectedVersion: workflow.version }); return this.run(() => operation, `Workflow ${workflow.active ? 'deactivated' : 'activated'}.`) },
  },
}
</script>
