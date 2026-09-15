<template>
  <c311-app-shell title="City 311 constituents" main-id="c311-constituents-main">
    <template #nav><c311-main-nav :items="navItems" aria-label="Staff navigation" /></template>
    <c311-data-state v-if="state !== 'populated'" :state="state" :error="error" @retry="search" />
    <template v-else>
      <h1 class="h3 mb-3">Constituents</h1>
      <c311-error-summary :errors="formErrors" title="Could not load constituent details" />
      <form class="form-row mb-3" @submit.prevent="search">
        <div class="col-md-7"><label for="c311-constituent-search">Search by name, email, or ID</label><input id="c311-constituent-search" v-model.trim="query" class="form-control" type="search"></div>
        <div class="col-md-2 d-flex align-items-end"><button class="btn btn-primary" type="submit" :disabled="busy">Search</button></div>
      </form>
      <section v-if="detail" aria-labelledby="c311-constituent-detail-heading">
        <div class="d-flex justify-content-between align-items-center mb-3"><h2 id="c311-constituent-detail-heading" class="h4 mb-0">{{ detail.display_name }}</h2><button class="btn btn-outline-primary" type="button" @click="detail = null">Back to results</button></div>
        <dl class="row"><dt class="col-sm-3">Constituent ID</dt><dd class="col-sm-9">{{ detail.constituent_id }}</dd><dt class="col-sm-3">Category</dt><dd class="col-sm-9">{{ detail.primary_category }}</dd><dt class="col-sm-3">Preferred language</dt><dd class="col-sm-9">{{ detail.preferred_language }}</dd><dt class="col-sm-3">Emails</dt><dd class="col-sm-9">{{ detail.emails.join(', ') || 'None' }}</dd><dt class="col-sm-3">Phone numbers</dt><dd class="col-sm-9"><span v-for="phone in detail.phone_numbers" :key="`${phone.label}-${phone.value}`" class="d-block">{{ phone.label }}: {{ phone.value }}</span><span v-if="!detail.phone_numbers.length">None</span></dd><dt class="col-sm-3">Addresses</dt><dd class="col-sm-9"><span v-for="address in detail.addresses" :key="`${address.line1}-${address.postal_code}`" class="d-block">{{ address.line1 }}, {{ address.city }}, {{ address.region }} {{ address.postal_code }}</span><span v-if="!detail.addresses.length">None</span></dd></dl>
      </section>
      <section v-else aria-labelledby="c311-constituent-results-heading">
        <h2 id="c311-constituent-results-heading" class="h4">Search results</h2>
        <p v-if="!items.length" class="text-muted" role="status">No matching constituents.</p>
        <div v-else class="table-responsive"><table class="table table-hover"><thead><tr><th>Name</th><th>ID</th><th>Email</th><th>Category</th><th>Language</th><th /></tr></thead><tbody><tr v-for="item in items" :key="item.constituent_id"><td>{{ item.display_name }}</td><td>{{ item.constituent_id }}</td><td>{{ item.emails[0] || 'None' }}</td><td>{{ item.primary_category }}</td><td>{{ item.preferred_language }}</td><td><button class="btn btn-sm btn-outline-primary" type="button" :disabled="busy" @click="open(item)">Open</button></td></tr></tbody></table></div>
      </section>
    </template>
  </c311-app-shell>
</template>

<script>
import { components, c311 } from '@cortezaproject/corteza-vue'

const { C311AppShell, C311DataState, C311ErrorSummary, C311MainNav } = components
const stateForError = c311?.c311StateForError

export default {
  name: 'C311ConstituentWorkspace',
  components: { C311AppShell, C311DataState, C311ErrorSummary, C311MainNav },
  data: () => ({ state: 'loading', error: null, formErrors: [], busy: false, query: '', items: [], detail: null }),
  computed: {
    provider () { return this.$C311?.provider },
    navItems () { return [{ route: '/c311/staff/requests', label: 'Requests' }, { route: '/c311/staff/constituents', label: 'Constituents' }, { route: '/c311/staff/submit', label: 'Create request' }, { route: '/c311', label: 'Public portal' }] },
  },
  created () { this.search() },
  methods: {
    setError (error) { this.error = error; this.formErrors = [{ field: 'form', code: error?.code || error?.error || 'OPERATION_FAILED', message: error?.message || 'The operation could not be completed.' }] },
    async search () {
      if (this.busy) return
      this.busy = true; this.state = 'loading'; this.error = null; this.formErrors = []; this.detail = null
      try { const page = await this.provider.searchStaffConstituents({ page_size: 50, filters: this.query ? { query: [this.query] } : {} }); this.items = page.items || []; this.state = 'populated' } catch (error) { this.setError(error); this.state = stateForError?.(error) || 'terminal-error' } finally { this.busy = false }
    },
    async open (item) {
      if (this.busy) return
      this.busy = true; this.formErrors = []; this.detail = null
      try { this.detail = await this.provider.getStaffConstituent(item.constituent_id) } catch (error) { this.setError(error) } finally { this.busy = false }
    },
  },
}
</script>
