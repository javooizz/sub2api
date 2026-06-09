<template>
  <div
    class="flex items-center gap-1 text-xs"
    role="group"
    :aria-label="t('admin.upstream.usage.windowLabel')"
  >
    <button
      v-for="w in windows"
      :key="w"
      type="button"
      class="cursor-pointer rounded px-2 py-1 transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary-500"
      :class="
        modelValue === w
          ? 'bg-primary-50 font-semibold text-primary-600 dark:bg-primary-900/30 dark:text-primary-300'
          : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'
      "
      :aria-pressed="modelValue === w"
      @click="emit('update:modelValue', w)"
    >
      {{ t(`admin.upstream.usage.${w}`) }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { UsageWindow } from './usageView'

defineProps<{ modelValue: UsageWindow }>()
const emit = defineEmits<{ 'update:modelValue': [UsageWindow] }>()
const { t } = useI18n()

const windows: UsageWindow[] = ['today', 'week', 'month', 'total']
</script>
