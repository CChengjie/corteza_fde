<template>
  <div
    v-if="errors.length"
    id="c311-error-summary"
    ref="summary"
    class="alert alert-danger"
    role="alert"
    tabindex="-1"
    :aria-label="title"
    data-c311-error-summary
  >
    <h2 class="h5 mb-2">
      {{ title }}
    </h2>
    <ul class="mb-0 pl-3">
      <li v-for="(error, index) in errors" :key="`${error.field}-${index}`">
        <a :href="`#${fieldID(error.field)}`" @click.prevent="focusField(error.field)">
          {{ error.message || error.code || error.field }}
        </a>
      </li>
    </ul>
  </div>
</template>

<script>
function normalizeC311FieldPath (field) {
  const raw = String(field ?? '').replace(/^#/, '')
  if (!raw) return ''
  if (!raw.startsWith('/')) return raw.replace(/\./g, '/')
  return raw.slice(1).split('/').map(segment => segment.replace(/~1/g, '/').replace(/~0/g, '~')).join('/')
}

function c311FieldPathAliases (field) {
  const raw = String(field ?? '').replace(/^#/, '')
  const path = normalizeC311FieldPath(raw)
  return [...new Set([raw.replace(/^\//, ''), path, path.replace(/\//g, '.')].filter(Boolean))]
}

export default {
  name: 'C311ErrorSummary',
  props: {
    errors: {
      type: Array,
      default: () => [],
    },
    title: {
      type: String,
      default: 'There is a problem',
    },
    focusOnUpdate: {
      type: Boolean,
      default: true,
    },
    fieldTargets: {
      type: Object,
      default: () => ({}),
    },
  },
  watch: {
    errors: {
      deep: true,
      handler (value) {
        if (!this.focusOnUpdate || !value.length) return
        this.$nextTick(() => this.$refs.summary?.focus())
      },
    },
  },
  methods: {
    fieldID (field) {
      const path = normalizeC311FieldPath(field)
      const aliases = c311FieldPathAliases(field)
      const targetKey = Object.keys(this.fieldTargets).find(key => {
        const targetPath = normalizeC311FieldPath(key)
        return aliases.includes(key) || aliases.includes(targetPath) || path === targetPath || path.startsWith(`${targetPath}/`)
      })
      if (targetKey) return this.fieldTargets[targetKey]
      const normalized = path
        .split('/')
        .filter(Boolean)
        .map(segment => segment.replace(/[^a-zA-Z0-9_-]/g, '-'))
        .join('-')
      return normalized.startsWith('c311-') ? normalized : `c311-${normalized}`
    },
    focusField (field) {
      const aliases = c311FieldPathAliases(field)
      const candidates = [this.fieldID(field), ...aliases.map(alias => alias.replace(/\//g, '-')), ...aliases]
      const element = candidates.map(id => document.getElementById(id)).find(Boolean)
      if (element) element.focus()
    },
    normalizeField (field) {
      return normalizeC311FieldPath(field)
    },
  },
}
</script>
