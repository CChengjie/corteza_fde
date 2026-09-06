<template>
  <div class="c311-responsive-data" data-c311-responsive-data>
    <div class="c311-responsive-data__table" tabindex="-1" role="region" :aria-label="label" data-c311-responsive-table>
      <table class="table table-hover mb-0">
        <thead>
          <tr>
            <th v-if="selectionEnabled" scope="col" class="c311-responsive-data__selection-column">
              <input type="checkbox" :checked="allSelected" :aria-label="selectAllLabel" data-c311-selection="all" @change="toggleAll($event.target.checked)">
            </th>
            <th v-for="column in visibleColumns" :key="column.key" scope="col">
              {{ column.label }}
            </th>
            <th v-if="selectable" scope="col" class="c311-responsive-data__action-column">{{ actionLabel }}</th>
          </tr>
        </thead>
        <tbody>
              <tr v-for="item in items" :key="item[rowKey]" :data-c311-row="item[rowKey]" :data-c311-department="item.owning_department || undefined">
              <td v-if="selectionEnabled" data-label="" class="c311-responsive-data__selection-column">
                <input type="checkbox" :checked="isSelected(item[rowKey])" :aria-label="`${selectLabel} ${item[rowKey]}`" :data-c311-selection="item[rowKey]" @change="toggle(item[rowKey], $event.target.checked)">
              </td>
              <td v-for="column in visibleColumns" :key="column.key" :data-label="column.label">
              {{ valueFor(item, column) }}
            </td>
            <td v-if="selectable" data-label="" class="text-nowrap c311-responsive-data__action-column">
              <button type="button" class="btn btn-sm btn-link" :data-c311-action="`view-request-${item[rowKey]}`" @click="$emit('select', item)">{{ actionLabel }}</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <div class="c311-responsive-data__cards" data-c311-responsive-cards>
      <article v-for="item in items" :key="item[rowKey]" class="border rounded p-3 mb-2" :data-c311-department="item.owning_department || undefined">
        <label v-if="selectionEnabled" class="d-block mb-2"><input type="checkbox" :checked="isSelected(item[rowKey])" :aria-label="`${selectLabel} ${item[rowKey]}`" :data-c311-selection="item[rowKey]" @change="toggle(item[rowKey], $event.target.checked)"> {{ selectLabel }}</label>
        <dl class="mb-0">
          <div v-for="column in visibleColumns" :key="column.key" class="d-flex justify-content-between py-1">
            <dt class="font-weight-bold mr-3">{{ column.label }}</dt>
            <dd class="mb-0 text-right">{{ valueFor(item, column) }}</dd>
          </div>
        </dl>
        <button v-if="selectable" type="button" class="btn btn-sm btn-link p-0 mt-2" :data-c311-action="`view-request-${item[rowKey]}`" @click="$emit('select', item)">{{ actionLabel }}</button>
      </article>
    </div>
  </div>
</template>

<script>
export default {
  name: 'C311ResponsiveData',
  props: {
    items: {
      type: Array,
      default: () => [],
    },
    columns: {
      type: Array,
      required: true,
    },
    rowKey: {
      type: String,
      default: 'id',
    },
    label: {
      type: String,
      default: 'Data table',
    },
    selectable: {
      type: Boolean,
      default: false,
    },
    actionLabel: {
      type: String,
      default: 'View',
    },
    selectionEnabled: {
      type: Boolean,
      default: false,
    },
    selectedKeys: {
      type: Array,
      default: () => [],
    },
    selectLabel: {
      type: String,
      default: 'Select request',
    },
    selectAllLabel: {
      type: String,
      default: 'Select all requests',
    },
  },
  computed: {
    visibleColumns () {
      return this.columns.filter(column => column.visible !== false)
    },
    allSelected () {
      return this.items.length > 0 && this.items.every(item => this.selectedKeys.includes(item[this.rowKey]))
    },
  },
  methods: {
    valueFor (item, column) {
      return typeof column.format === 'function' ? column.format(item[column.key], item) : item[column.key]
    },
    isSelected (key) {
      return this.selectedKeys.includes(key)
    },
    toggle (key, selected) {
      const next = this.selectedKeys.filter(item => item !== key)
      if (selected) next.push(key)
      this.$emit('selection-change', next)
    },
    toggleAll (selected) {
      this.$emit('selection-change', selected ? this.items.map(item => item[this.rowKey]) : [])
    },
  },
}
</script>

<style scoped>
.c311-responsive-data__cards {
  display: none;
}

.c311-responsive-data__table {
  overflow-x: auto;
  max-width: 100%;
}

.c311-responsive-data__selection-column {
  width: 2.5rem;
  text-align: center;
}

@media (max-width: 767px) {
  .c311-responsive-data__table {
    display: none;
  }

  .c311-responsive-data__cards {
    display: block;
  }
}

@media (min-width: 768px) and (max-width: 1023px) {
  .c311-responsive-data__table th:nth-child(n+4),
  .c311-responsive-data__table td:nth-child(n+4) {
    display: none;
  }

  .c311-responsive-data__table th.c311-responsive-data__action-column,
  .c311-responsive-data__table td.c311-responsive-data__action-column {
    display: table-cell;
  }
}
</style>
