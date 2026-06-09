<template>
  <div>
    <!-- partial:provider 级采集状态,影响所有窗口 → 整体提示条(非历史专属) -->
    <div
      v-if="summary?.partial"
      role="status"
      class="mb-2 rounded-md bg-amber-50 px-3 py-1.5 text-xs text-amber-700 dark:bg-amber-900/30 dark:text-amber-300"
    >
      {{ t('admin.upstream.usage.partial') }}<span v-if="summary.partial_reason">：{{ summary.partial_reason }}</span>
    </div>

    <!-- null 空态 -->
    <div
      v-if="!summary"
      class="rounded-lg bg-gray-50 px-3 py-4 text-center text-xs text-gray-400 dark:bg-gray-800/50"
    >
      {{ t('admin.upstream.usage.empty') }}
    </div>

    <!-- 4 窗口卡片 -->
    <div v-else class="grid grid-cols-4 gap-2">
      <div
        v-for="w in windows"
        :key="w.key"
        class="rounded-lg border px-2.5 py-2"
        :class="
          w.key === 'month'
            ? 'border-primary-200 bg-primary-50 dark:border-primary-800 dark:bg-primary-900/20'
            : 'border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-800'
        "
      >
        <div class="text-[10px] text-gray-500 dark:text-gray-400">{{ t(`admin.upstream.usage.${w.key}`) }}</div>
        <div class="mt-0.5 font-semibold tabular-nums text-gray-900 dark:text-gray-100">{{ formatCNY(w.stat.cost_cny) }}</div>
        <div
          v-if="w.key === 'total' && summary.backfilled_from"
          class="mt-0.5 text-[10px] text-gray-400"
        >
          {{ t('admin.upstream.usage.backfilledFrom', { date: summary.backfilled_from.slice(0, 10) }) }}
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { UsageSummary, UsageWindowStat } from '@/api/admin/upstreamProviders'
import { formatCNY } from './usageView'
import type { UsageWindow } from './usageView'

const props = defineProps<{ summary: UsageSummary | null | undefined }>()
const { t } = useI18n()

const windows = computed<{ key: UsageWindow; stat: UsageWindowStat }[]>(() => {
  const s = props.summary
  if (!s) return []
  return [
    { key: 'today', stat: s.today },
    { key: 'week', stat: s.week },
    { key: 'month', stat: s.month },
    { key: 'total', stat: s.total },
  ]
})
</script>
