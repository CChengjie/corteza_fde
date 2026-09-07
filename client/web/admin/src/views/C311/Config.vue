<template>
  <c311-app-shell mode="staff" :brand="t('brand', 'City 311 administration')" :title="t('title', 'City 311 configuration')" :status-message="message">
    <template #nav><c311-main-nav :items="navItems" :label="t('navLabel', 'Administration navigation')" /><c311-language-selector :actor-id="actorID" /></template>
    <div id="c311-admin-config" data-c311-admin-config>
      <p v-if="error" class="alert alert-danger" role="alert" data-c311-error>{{ error.message || error }}</p>
      <div v-if="conflict" class="alert alert-warning" data-c311-conflict>
        <p>Server version: <strong data-c311-current-version>{{ conflict.currentVersion }}</strong>. Your edits are preserved.</p>
        <button type="button" class="btn btn-outline-secondary mr-2" data-c311-action="reload-current-version" @click="reloadConflict">Reload server version</button>
        <button type="button" class="btn btn-primary" data-c311-action="reapply-changes" :disabled="!conflict.reloaded" @click="reapplyConflict">Reapply my changes</button>
      </div>
      <p v-if="message" class="alert alert-info" role="status" aria-live="polite" data-c311-message>{{ message }}</p>
      <nav class="nav nav-tabs mb-3" aria-label="Configuration sections">
        <button v-for="item in tabs" :key="item.key" type="button" class="nav-link" :class="{ active: tab === item.key }" :data-c311-tab="item.key" @click="selectTab(item.key)">{{ item.label }}</button>
      </nav>
      <section v-if="loading" aria-live="polite" data-c311-loading>Loading...</section>

      <section v-else-if="tab === 'branding' && branding" data-c311-branding>
        <h2>Branding</h2>
        <form @submit.prevent="saveBranding()">
          <label>Organisation name<input v-model.trim="brandingForm.organisation_name" class="form-control" data-c311-field="organisation_name"></label>
          <label>Primary colour<input v-model="brandingForm.primary_colour" class="form-control" type="color"></label>
          <label>Accent colour<input v-model="brandingForm.accent_colour" class="form-control" type="color"></label>
          <label>Font family<input v-model.trim="brandingForm.font_family" class="form-control"></label>
          <label>Logo URL<input v-model.trim="brandingForm.logo_url" class="form-control" type="url"></label>
          <label>Public header<input v-model="brandingForm.public_header" class="form-control"></label>
          <label>Public footer<input v-model="brandingForm.public_footer" class="form-control"></label>
          <div class="mt-2"><button v-if="can('admin_branding_update')" class="btn btn-primary mr-2" data-c311-action="save-branding">Save draft</button><button v-if="can('admin_branding_preview')" type="button" class="btn btn-outline-secondary mr-2" data-c311-action="preview-branding" @click="previewBranding">Preview</button><button v-if="can('admin_branding_publish')" type="button" class="btn btn-success" data-c311-action="publish-branding" @click="publishBranding">Publish</button></div>
        </form>
        <p>Version: <span data-c311-version>{{ branding.version }}</span> · {{ branding.published ? 'Published' : 'Draft' }}</p>
        <div v-if="preview" class="border p-3" data-c311-preview><strong>{{ preview.organisation_name }}</strong><p>{{ preview.public_header }}</p></div>
        <version-history v-if="can('admin_branding_versions')" :items="brandingVersions" :can-rollback="can('admin_branding_rollback')" kind="branding" @rollback="rollbackBranding" />
      </section>

      <section v-else-if="tab === 'content'" data-c311-content>
        <h2>Public content and help</h2>
        <button type="button" class="btn btn-outline-secondary mr-2" data-c311-action="edit-content" @click="contentMode = 'content'; loadContent()">Public content</button>
        <button type="button" class="btn btn-outline-secondary" data-c311-action="edit-help" @click="contentMode = 'help'; loadHelp()">Help</button>
        <template v-if="contentMode === 'content'">
          <label>Content key<select v-model="contentKey" class="form-control" data-c311-field="content_key" @change="loadContent"><option v-for="key in contentKeys" :key="key" :value="key">{{ key }}</option></select></label>
          <textarea v-if="content" v-model="contentForm.body" class="form-control mt-2" rows="8" data-c311-field="body"></textarea>
          <div class="mt-2"><button v-if="can('admin_content_update')" class="btn btn-primary mr-2" data-c311-action="save-content" @click="saveContent()">Save draft</button><button v-if="can('admin_content_preview')" class="btn btn-outline-secondary mr-2" data-c311-action="preview-content" @click="previewContent">Preview</button><button v-if="can('admin_content_publish')" class="btn btn-success" data-c311-action="publish-content" @click="publishContent">Publish</button></div>
          <p v-if="content">Version: <span data-c311-content-version>{{ content.version }}</span> · {{ content.published ? 'Published' : 'Draft' }}</p>
          <pre v-if="previewContentResult" class="border p-3 text-wrap" data-c311-content-preview>{{ previewContentResult.body }}</pre>
          <version-history v-if="can('admin_content_versions')" :items="contentVersions" :can-rollback="can('admin_content_rollback')" kind="content" @rollback="rollbackContent" />
        </template>
        <template v-else>
          <label>Help key<select v-model="helpKey" class="form-control" data-c311-field="help_key" @change="loadHelp"><option v-for="key in helpKeys" :key="key" :value="key">{{ key }}</option></select></label>
          <label>Language<select v-model="helpLanguage" class="form-control" data-c311-field="help_language" @change="loadHelp"><option v-for="language in languages" :key="language" :value="language">{{ language }}</option></select></label>
          <textarea v-if="help" v-model="helpForm.body" class="form-control mt-2" rows="8" data-c311-field="help_body"></textarea>
          <div class="mt-2"><button v-if="can('admin_help_update')" type="button" class="btn btn-primary mr-2" data-c311-action="save-help" @click="saveHelp()">Save help</button><button v-if="can('admin_help_preview')" type="button" class="btn btn-outline-secondary mr-2" data-c311-action="preview-help" @click="previewHelp">Preview</button><button v-if="can('admin_help_publish')" type="button" class="btn btn-success" data-c311-action="publish-help" @click="publishHelp">Publish</button></div>
          <p v-if="help">Version: <span data-c311-help-version>{{ help.version }}</span> · {{ help.state }} · {{ help.published ? 'Published' : 'Draft' }}</p>
          <pre v-if="previewHelpResult" class="border p-3 text-wrap" data-c311-help-preview>{{ previewHelpResult.body }}</pre>
          <version-history v-if="can('admin_help_versions')" :items="helpVersions" :can-rollback="can('admin_help_rollback')" kind="help" @rollback="rollbackHelp" />
        </template>
      </section>

      <section v-else-if="tab === 'categories'" data-c311-categories>
        <h2>Contact categories</h2>
        <form v-for="category in categories" :key="category.code" class="border p-2 mb-2" :data-c311-category="category.code" @submit.prevent="saveCategory(category)">
          <strong>{{ category.code }}</strong> · v{{ category.version }}
          <input v-model.trim="category.labels.EN" class="form-control" aria-label="English label">
          <input v-model.trim="category.labels.ES" class="form-control" aria-label="Spanish label">
          <input v-model.trim="category.labels.VI" class="form-control" aria-label="Vietnamese label">
          <label><input v-model="category.active" type="checkbox"> Active</label>
          <button v-if="can('admin_categories_update')" class="btn btn-sm btn-primary ml-2" :data-c311-action="`save-category-${category.code}`">Save</button>
        </form>
        <form v-if="can('admin_categories_create')" @submit.prevent="createCategory"><input v-model.trim="newCategory.code" required pattern="[A-Z0-9_-]+" placeholder="Code" class="form-control"><input v-model.trim="newCategory.label" required placeholder="English label" class="form-control mt-1"><button class="btn btn-primary mt-2" data-c311-action="create-category">Add category</button></form>
      </section>

      <section v-else data-c311-fields>
        <h2>Custom fields</h2>
        <ul><li v-for="field in customFields" :key="field.key" :data-c311-field-key="field.key"><button type="button" class="btn btn-link" :data-c311-action="`edit-field-${field.key}`" @click="editField(field)">{{ field.key }} · {{ field.labels.EN }} · {{ field.field_type }} · {{ field.active ? 'active' : 'inactive' }}</button></li></ul>
        <form v-if="can(editingField ? 'admin_custom_fields_update' : 'admin_custom_fields_create')" @submit.prevent="saveField()">
          <input v-model.trim="fieldForm.key" required pattern="[a-z][a-z0-9_.-]*" placeholder="Key" class="form-control" :disabled="!!editingField" data-c311-field="field_key">
          <input v-model.trim="fieldForm.label" required placeholder="English label" class="form-control mt-1" data-c311-field="field_label">
          <select v-model="fieldForm.entity" class="form-control mt-1"><option value="service_request">service_request</option><option value="constituent">constituent</option></select>
          <select v-model="fieldForm.field_type" class="form-control mt-1" data-c311-field="field_type"><option v-for="type in fieldTypes" :key="type">{{ type }}</option></select>
          <label><input v-model="fieldForm.required" type="checkbox"> Required</label><label><input v-model="fieldForm.active" type="checkbox"> Active</label>
          <label>Ordered choices<textarea v-model="fieldForm.choices" class="form-control" data-c311-field="field_choices"></textarea></label>
          <label>Default value JSON<textarea v-model="fieldForm.defaultValue" class="form-control" data-c311-field="field_default"></textarea></label>
          <label>Validation JSON<textarea v-model="fieldForm.validation" class="form-control" data-c311-field="field_validation"></textarea></label>
          <button class="btn btn-primary" data-c311-action="save-field">{{ editingField ? 'Save field' : 'Add field' }}</button><button v-if="editingField" type="button" class="btn btn-link" @click="resetField">Cancel</button>
        </form>
      </section>
    </div>
  </c311-app-shell>
</template>

<script>
import { components } from '@cortezaproject/corteza-vue'
const { C311AppShell, C311LanguageSelector, C311MainNav } = components
const VersionHistory = { props: ['items', 'canRollback', 'kind'], template: '<div :data-c311-history="kind"><h3>Version history</h3><ul><li v-for="item in items" :key="item.version">Version {{ item.version }} · {{ item.published ? \'Published\' : \'Draft\' }} <button v-if="canRollback" type="button" class="btn btn-link" :data-c311-action="\'rollback-\' + kind + \'-\' + item.version" @click="$emit(\'rollback\', item.version)">Rollback</button></li></ul></div>' }
const emptyField = () => ({ key: '', label: '', entity: 'service_request', field_type: 'TEXT', required: false, active: true, choices: '', defaultValue: '', validation: '{}' })

export default {
  name: 'C311Config',
  components: { C311AppShell, C311LanguageSelector, C311MainNav, VersionHistory },
  data: () => ({ tab: 'branding', loading: true, error: null, message: '', conflict: null, branding: null, brandingForm: {}, preview: null, brandingVersions: [], contentMode: 'content', content: null, contentKey: 'HOME', contentForm: {}, previewContentResult: null, contentVersions: [], help: null, helpKey: 'admin.branding.publish', helpLanguage: 'EN', helpForm: {}, previewHelpResult: null, helpVersions: [], categories: [], newCategory: { code: '', label: '' }, customFields: [], editingField: null, fieldForm: emptyField() }),
  computed: {
    actor () { return this.$C311?.session?.actor || null },
    actorID () { return this.actor?.actor_id || '' },
    tabs () { return [{ key: 'branding', label: this.t('brandingTab', 'Branding'), capability: 'admin_branding_get' }, { key: 'content', label: this.t('contentTab', 'Content and help'), capability: 'admin_content_get' }, { key: 'categories', label: this.t('categoriesTab', 'Categories'), capability: 'admin_categories_list' }, { key: 'fields', label: this.t('fieldsTab', 'Custom fields'), capability: 'admin_custom_fields_list' }].filter(tab => this.can(tab.capability)) },
    contentKeys: () => ['HOME', 'SERVICE_CATALOGUE', 'HELP', 'FOOTER', 'TERMS'],
    languages: () => ['EN', 'ES', 'VI'],
    helpKeys: () => ['admin.branding.publish', 'admin.workflow.author', 'public.request.lookup', 'public.request.submit', 'staff.report.create', 'staff.request.bulk-update', 'staff.request.reassign', 'staff.request.triage'],
    fieldTypes: () => ['TEXT', 'INTEGER', 'DECIMAL', 'DATE', 'DATETIME', 'BOOLEAN', 'SINGLE_CHOICE', 'MULTI_CHOICE'],
    navItems () { return [{ route: '/c311/admin', label: this.t('title', 'City 311 configuration'), capability: 'admin_branding_get' }] },
  },
  created () { if (!this.tabs.length) return this.$router?.replace?.({ name: 'c311.forbidden' }); if (!this.tabs.some(item => item.key === this.tab)) this.tab = this.tabs[0].key; this.loadTab() },
  methods: {
    t (key, fallback) { const value = this.$t?.(`c311:admin.${key}`); return value && !String(value).includes(`c311:admin.${key}`) ? value : fallback },
    can (capability) { return !!this.$C311?.can?.(capability) || !!this.actor?.capabilities?.includes(capability) },
    clearStatus () { this.error = null; this.message = ''; this.conflict = null },
    selectTab (tab) { this.tab = tab; this.clearStatus(); this.loadTab() },
    async loadTab () { this.loading = true; try { if (this.tab === 'branding') await this.loadBranding(); else if (this.tab === 'content') await (this.contentMode === 'help' ? this.loadHelp() : this.loadContent()); else if (this.tab === 'categories') this.categories = (await this.$C311.provider.listAdminCategories()).items; else this.customFields = (await this.$C311.provider.listAdminCustomFields()).items } catch (error) { this.error = error } finally { this.loading = false } },
    async loadBranding () { this.branding = await this.$C311.provider.getAdminBranding(); this.brandingForm = { ...this.branding }; if (this.can('admin_branding_versions')) this.brandingVersions = (await this.$C311.provider.listBrandingVersions()).items },
    async loadContent () { this.content = await this.$C311.provider.getAdminContent(this.contentKey); this.contentForm = { body: this.content.body }; this.previewContentResult = null; if (this.can('admin_content_versions')) this.contentVersions = (await this.$C311.provider.listAdminContentVersions(this.contentKey)).items },
    async loadHelp () { this.help = await this.$C311.provider.getAdminHelp(this.helpKey, this.helpLanguage); this.helpForm = { body: this.help.body }; this.previewHelpResult = null; if (this.can('admin_help_versions')) this.helpVersions = (await this.$C311.provider.listAdminHelpVersions(this.helpKey, { language: this.helpLanguage })).items },
    handleError (error, resource, draft, reapply) { this.error = error; if (error?.code === 'VERSION_CONFLICT') this.conflict = { resource, currentVersion: error.currentVersion, draft, reapply, reloaded: false } },
    async reloadConflict () { const conflict = this.conflict; if (!conflict) return; if (conflict.resource === 'branding') await this.loadBranding(); else if (conflict.resource === 'content') await this.loadContent(); else if (conflict.resource === 'help') await this.loadHelp(); else if (conflict.resource === 'category') { this.categories = (await this.$C311.provider.listAdminCategories()).items; const current = this.categories.find(category => category.code === conflict.draft.code); if (current) conflict.draft = { ...conflict.draft, version: current.version, updated_at: current.updated_at } } else { this.customFields = (await this.$C311.provider.listAdminCustomFields()).items; const current = this.customFields.find(field => field.key === this.editingField); if (current) conflict.draft = { ...conflict.draft, version: current.version, updated_at: current.updated_at } } conflict.reloaded = true; this.conflict = conflict; this.error = null },
    async reapplyConflict () { if (!this.conflict?.reloaded) return; const { draft, reapply } = this.conflict; this.conflict = null; await reapply(draft) },
    async saveBranding (draft = { ...this.brandingForm }) { try { this.clearStatus(); this.branding = await this.$C311.provider.updateBranding(draft, { expectedVersion: this.branding.version }); this.message = 'Branding draft saved.'; await this.loadBranding() } catch (error) { this.handleError(error, 'branding', draft, value => this.saveBranding(value)) } },
    async previewBranding () { try { this.preview = await this.$C311.provider.previewBranding(this.brandingForm) } catch (error) { this.error = error } },
    async publishBranding () { try { this.branding = await this.$C311.provider.publishBranding({ expectedVersion: this.branding.version }); this.message = 'Branding published.'; await this.loadBranding() } catch (error) { this.handleError(error, 'branding', { ...this.brandingForm }, () => this.publishBranding()) } },
    async rollbackBranding (targetVersion) { try { this.branding = await this.$C311.provider.rollbackBranding({ target_version: targetVersion }, { expectedVersion: this.branding.version }); this.message = 'Branding rolled back.'; await this.loadBranding() } catch (error) { this.handleError(error, 'branding', { targetVersion }, value => this.rollbackBranding(value.targetVersion)) } },
    async saveContent (draft = { ...this.contentForm }) { try { this.clearStatus(); this.content = await this.$C311.provider.updateAdminContent(this.contentKey, draft, { expectedVersion: this.content.version }); this.message = 'Content draft saved.'; await this.loadContent() } catch (error) { this.handleError(error, 'content', draft, value => this.saveContent(value)) } },
    async previewContent () { try { this.previewContentResult = await this.$C311.provider.previewAdminContent(this.contentKey, this.contentForm) } catch (error) { this.error = error } },
    async publishContent () { try { this.content = await this.$C311.provider.publishAdminContent(this.contentKey, { expectedVersion: this.content.version }); this.message = 'Content published.'; await this.loadContent() } catch (error) { this.handleError(error, 'content', { ...this.contentForm }, () => this.publishContent()) } },
    async rollbackContent (targetVersion) { try { this.content = await this.$C311.provider.rollbackAdminContent(this.contentKey, { target_version: targetVersion }, { expectedVersion: this.content.version }); this.message = 'Content rolled back.'; await this.loadContent() } catch (error) { this.handleError(error, 'content', { targetVersion }, value => this.rollbackContent(value.targetVersion)) } },
    async saveHelp (draft = { ...this.helpForm }) { try { this.clearStatus(); this.help = await this.$C311.provider.updateAdminHelp(this.helpKey, { language: this.helpLanguage, body: draft.body }, { expectedVersion: this.help.version }); this.helpForm = { body: this.help.body }; this.message = 'Help saved.'; await this.loadHelp() } catch (error) { this.handleError(error, 'help', draft, value => this.saveHelp(value)) } },
    async previewHelp () { try { this.previewHelpResult = await this.$C311.provider.previewAdminHelp(this.helpKey, { language: this.helpLanguage, body: this.helpForm.body }) } catch (error) { this.error = error } },
    async publishHelp () { try { this.help = await this.$C311.provider.publishAdminHelp(this.helpKey, this.helpLanguage, { expectedVersion: this.help.version }); this.message = 'Help published.'; await this.loadHelp() } catch (error) { this.handleError(error, 'help', { ...this.helpForm }, () => this.publishHelp()) } },
    async rollbackHelp (targetVersion) { try { this.help = await this.$C311.provider.rollbackAdminHelp(this.helpKey, { target_version: targetVersion }, this.helpLanguage, { expectedVersion: this.help.version }); this.message = 'Help rolled back.'; await this.loadHelp() } catch (error) { this.handleError(error, 'help', { targetVersion }, value => this.rollbackHelp(value.targetVersion)) } },
    async saveCategory (category, draft = { code: category.code, active: category.active, labels: { ...category.labels }, version: category.version }) { try { this.clearStatus(); Object.assign(category, await this.$C311.provider.updateAdminCategory(category.code, { code: category.code, active: category.active, labels: category.labels }, { expectedVersion: category.version })); this.message = 'Category saved.' } catch (error) { this.handleError(error, 'category', draft, value => this.saveCategory(category, value)) } },
    async createCategory () { try { this.categories.push(await this.$C311.provider.createAdminCategory({ code: this.newCategory.code, active: true, labels: { EN: this.newCategory.label } })); this.newCategory = { code: '', label: '' } } catch (error) { this.error = error } },
    editField (field) { this.editingField = field.key; this.fieldForm = { key: field.key, label: field.labels.EN || '', entity: field.entity, field_type: field.field_type, required: field.required, active: field.active, choices: (field.choice_values || []).join('\n'), defaultValue: field.default === undefined ? '' : JSON.stringify(field.default), validation: JSON.stringify(field.validation || {}) } },
    resetField () { this.editingField = null; this.fieldForm = emptyField() },
    fieldPayload () { const choices = this.fieldForm.choices.split('\n').map(value => value.trim()).filter(Boolean); if (new Set(choices).size !== choices.length) throw new Error('Choice values must be unique.'); let validation; let defaultValue; try { validation = JSON.parse(this.fieldForm.validation || '{}') } catch (_) { throw new Error('Validation must be valid JSON.') } try { defaultValue = this.fieldForm.defaultValue === '' ? undefined : JSON.parse(this.fieldForm.defaultValue) } catch (_) { throw new Error('Default value must be valid JSON.') } const current = this.customFields.find(field => field.key === this.editingField); return { key: this.fieldForm.key, labels: { EN: this.fieldForm.label }, entity: this.fieldForm.entity, field_type: this.fieldForm.field_type, required: this.fieldForm.required, active: this.fieldForm.active, choice_values: choices, ...(defaultValue === undefined ? {} : { default: defaultValue }), validation, version: current?.version || 1, updated_at: current?.updated_at || '2026-01-15T15:00:00.000Z' } },
    async saveField (draft) { try { this.clearStatus(); const payload = draft || this.fieldPayload(); if (this.editingField) { const index = this.customFields.findIndex(field => field.key === this.editingField); this.$set(this.customFields, index, await this.$C311.provider.updateAdminCustomField(this.editingField, payload, { expectedVersion: payload.version })) } else this.customFields.push(await this.$C311.provider.createAdminCustomField(payload)); this.message = 'Custom field saved.'; this.resetField() } catch (error) { if (this.editingField && error?.code === 'VERSION_CONFLICT') this.handleError(error, 'field', draft || this.fieldPayload(), value => this.saveField(value)); else this.error = error } },
  },
}
</script>
