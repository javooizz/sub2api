<template>
  <span
    class="inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium"
    :class="cls"
  >
    <!-- dot: 颜色+文字双编码,满足可访问性 -->
    <span class="h-1.5 w-1.5 rounded-full" :class="dot" aria-hidden="true" />
    {{ label }}
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ status: string }>()
const { t } = useI18n()

const palette: Record<string, { cls: string; dot: string }> = {
  active: {
    cls: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300',
    dot: 'bg-emerald-500',
  },
  credential_error: {
    cls: 'bg-amber-50 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300',
    dot: 'bg-amber-500',
  },
  unreachable: {
    cls: 'bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-300',
    dot: 'bg-red-500',
  },
  disabled: {
    cls: 'bg-gray-100 text-gray-500 dark:bg-gray-800 dark:text-gray-400',
    dot: 'bg-gray-400',
  },
}

const cls = computed(() => (palette[props.status] ?? palette.disabled).cls)
const dot = computed(() => (palette[props.status] ?? palette.disabled).dot)
// i18n key 不存在时 fallback 到原始 status 值,避免页面崩溃
const label = computed(() => {
  const key = `admin.upstream.status.${props.status}`
  const result = t(key)
  return result === key ? props.status : result
})
</script>
