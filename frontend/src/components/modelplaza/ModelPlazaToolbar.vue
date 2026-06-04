<template>
  <div class="flex flex-wrap items-center gap-3">
    <div class="relative w-full sm:w-72">
      <Icon
        name="search"
        size="md"
        class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500"
      />
      <input
        :value="search"
        type="text"
        :placeholder="t('modelPlaza.searchPlaceholder')"
        class="input pl-10"
        @input="emit('update:search', ($event.target as HTMLInputElement).value)"
      />
    </div>

    <div v-if="selectedCount > 0" class="flex items-center gap-2">
      <span class="text-xs text-gray-500 dark:text-gray-400">
        {{ t('modelPlaza.toolbar.selectedCount', { count: selectedCount }) }}
      </span>
      <button type="button" class="btn btn-secondary btn-sm" @click="emit('copy-selected')">
        <Icon name="copy" size="sm" class="mr-1" />
        {{ t('modelPlaza.toolbar.copySelected') }}
      </button>
      <button type="button" class="btn btn-secondary btn-sm" @click="emit('clear-selection')">
        {{ t('modelPlaza.toolbar.clearSelection') }}
      </button>
    </div>

    <div class="ml-auto flex items-center gap-1 rounded-lg border border-gray-200 p-0.5 dark:border-gray-700">
      <button
        type="button"
        class="view-toggle"
        :class="{ 'view-toggle-active': view === 'card' }"
        :aria-pressed="view === 'card'"
        @click="emit('update:view', 'card')"
      >
        {{ t('modelPlaza.toolbar.cardView') }}
      </button>
      <button
        type="button"
        class="view-toggle"
        :class="{ 'view-toggle-active': view === 'table' }"
        :aria-pressed="view === 'table'"
        @click="emit('update:view', 'table')"
      >
        {{ t('modelPlaza.toolbar.tableView') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

defineProps<{
  search: string
  view: 'card' | 'table'
  selectedCount: number
}>()

const emit = defineEmits<{
  'update:search': [value: string]
  'update:view': [value: 'card' | 'table']
  'copy-selected': []
  'clear-selection': []
}>()

const { t } = useI18n()
</script>

<style scoped>
.view-toggle {
  cursor: pointer;
  border-radius: 0.375rem;
  padding: 0.25rem 0.625rem;
  font-size: 0.75rem;
  color: rgb(107 114 128);
  transition: all 150ms;
}
.dark .view-toggle {
  color: rgb(156 163 175);
}
.view-toggle-active {
  background-color: rgb(239 246 255);
  color: rgb(29 78 216);
  font-weight: 500;
}
.dark .view-toggle-active {
  background-color: rgb(30 58 138 / 0.3);
  color: rgb(147 197 253);
}
</style>
