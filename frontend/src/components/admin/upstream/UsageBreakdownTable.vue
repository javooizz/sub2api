<template>
  <div>
    <!-- supported:false(sub2api 分组)→ 占位,不渲染数值行 -->
    <div v-if="!supported">
      <div
        class="rounded-md bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:bg-amber-900/30 dark:text-amber-300"
        role="status"
      >
        {{ t('admin.upstream.usage.unsupportedGroup') }}
      </div>
    </div>

    <!-- loading -->
    <div v-else-if="loading" class="py-8 text-center">
      <div class="mx-auto h-6 w-6 animate-spin rounded-full border-b-2 border-primary-600" />
    </div>

    <!-- 空态 -->
    <p v-else-if="!rows.length" class="py-8 text-center text-sm text-gray-400">
      {{ t('admin.upstream.usage.empty') }}
    </p>

    <!-- 明细表:消耗($)为主、实付(¥)为辅 -->
    <table v-else class="w-full text-sm">
      <thead>
        <tr class="border-b border-gray-200 text-left dark:border-gray-700">
          <th class="py-2 pr-3 font-medium text-gray-500 dark:text-gray-400">{{ nameLabel }}</th>
          <th class="py-2 pr-3 text-right font-medium text-gray-500 dark:text-gray-400">{{ t('admin.upstream.usage.spent') }}</th>
          <th class="py-2 pr-3 text-right font-medium text-gray-500 dark:text-gray-400">{{ t('admin.upstream.usage.paid') }}</th>
          <th class="py-2 text-right font-medium text-gray-500 dark:text-gray-400">{{ t('admin.upstream.usage.requests') }}</th>
        </tr>
      </thead>
      <tbody class="divide-y divide-gray-100 dark:divide-gray-800">
        <tr v-for="r in rows" :key="r.scope_key" :class="r.deleted ? 'text-gray-400 dark:text-gray-500' : ''">
          <td class="py-2 pr-3">
            <span :class="r.deleted ? '' : 'text-gray-900 dark:text-gray-100'">{{ r.scope_name }}</span>
            <span
              v-if="r.deleted"
              class="ml-1.5 rounded bg-gray-100 px-1 text-[10px] text-gray-500 dark:bg-gray-800 dark:text-gray-400"
            >{{ t('admin.upstream.usage.deleted') }}</span>
            <span v-if="r.meta" class="ml-1.5 text-xs text-gray-400">{{ r.meta }}</span>
          </td>
          <td class="py-2 pr-3 text-right tabular-nums">
            <span class="font-semibold" :class="r.deleted ? '' : 'text-gray-900 dark:text-gray-100'">{{ formatUSD(r.cost_usd) }}</span>
          </td>
          <td class="py-2 pr-3 text-right tabular-nums text-gray-500 dark:text-gray-400">{{ formatCNY(r.cost_cny) }}</td>
          <td class="py-2 text-right tabular-nums text-gray-700 dark:text-gray-300">{{ formatRequests(r.requests) }}</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { formatCNY, formatUSD, formatRequests } from './usageView'
import type { MergedUsageRow } from './usageView'

defineProps<{
  rows: MergedUsageRow[]
  supported: boolean
  loading: boolean
  nameLabel: string
}>()
const { t } = useI18n()
</script>
