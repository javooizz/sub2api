<template>
  <div class="overflow-x-auto">
    <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-gray-700">
      <thead>
        <tr class="text-left text-xs uppercase tracking-wide text-gray-500 dark:text-gray-400">
          <th class="w-8 px-3 py-2"></th>
          <th class="px-3 py-2">{{ t('modelPlaza.table.model') }}</th>
          <th class="px-3 py-2">{{ t('modelPlaza.table.provider') }}</th>
          <th class="hidden px-3 py-2 lg:table-cell">{{ t('modelPlaza.table.description') }}</th>
          <th class="px-3 py-2">{{ t('modelPlaza.table.billingType') }}</th>
          <th class="px-3 py-2">{{ t('modelPlaza.table.price') }}</th>
          <th class="hidden px-3 py-2 md:table-cell">{{ t('modelPlaza.table.groups') }}</th>
        </tr>
      </thead>
      <tbody class="divide-y divide-gray-100 dark:divide-gray-800">
        <tr
          v-for="m in models"
          :key="`${m.platform}/${m.name}`"
          class="cursor-pointer transition-colors hover:bg-gray-50 dark:hover:bg-gray-800/60"
          @click="emit('open', m)"
        >
          <td class="px-3 py-2.5" @click.stop>
            <input
              type="checkbox"
              class="h-4 w-4 cursor-pointer rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-gray-600"
              :checked="selectedNames.has(m.name)"
              :aria-label="m.name"
              @change="emit('toggle-select', m)"
            />
          </td>
          <td class="px-3 py-2.5">
            <div class="flex items-center gap-2">
              <ModelIcon :model="m.name" size="20px" />
              <span class="font-medium text-gray-900 dark:text-gray-100">{{ m.name }}</span>
              <button
                type="button"
                class="cursor-pointer rounded p-0.5 text-gray-400 hover:text-gray-600 focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary-500 dark:hover:text-gray-300"
                :title="t('modelPlaza.card.copyModelName')"
                :aria-label="t('modelPlaza.card.copyModelName')"
                @click.stop="emit('copy', m.name)"
              >
                <Icon name="copy" size="sm" />
              </button>
            </div>
          </td>
          <td class="px-3 py-2.5 text-gray-600 dark:text-gray-300">{{ m.platform }}</td>
          <td class="hidden max-w-xs truncate px-3 py-2.5 text-gray-500 dark:text-gray-400 lg:table-cell">
            {{ m.description || '-' }}
          </td>
          <td class="px-3 py-2.5">
            <span class="whitespace-nowrap text-xs text-gray-600 dark:text-gray-300">
              {{ t(`modelPlaza.billingMode.${m.billing_mode}`, m.billing_mode) }}
            </span>
          </td>
          <td class="px-3 py-2.5 font-mono text-xs text-gray-700 dark:text-gray-200">
            {{ priceSummary(m) }}
          </td>
          <td class="hidden px-3 py-2.5 text-xs text-gray-500 dark:text-gray-400 md:table-cell">
            {{ m.groups.map((x) => x.name).join(', ') }}
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import ModelIcon from '@/components/common/ModelIcon.vue'
import { formatPerMillion, formatPerRequest, effectiveMultiplier } from '@/utils/plazaPricing'
import type { PlazaModel, PlazaGroup } from '@/api/modelPlaza'

const props = defineProps<{
  models: PlazaModel[]
  selectedNames: Set<string>
  selectedGroup: PlazaGroup | null
  userRates: Record<number, number>
}>()

const emit = defineEmits<{
  open: [model: PlazaModel]
  copy: [name: string]
  'toggle-select': [model: PlazaModel]
}>()

const { t } = useI18n()

/** 表格单元价格摘要：输入/输出（或按次价）一行展示。 */
function priceSummary(m: PlazaModel): string {
  const p = m.pricing
  if (!p) return t('modelPlaza.card.noPricing')
  let mult = 1
  if (props.selectedGroup && m.groups.some((g) => g.id === props.selectedGroup!.id)) {
    mult = effectiveMultiplier(props.selectedGroup, props.userRates)
  }
  if (m.billing_mode === 'per_request' || m.billing_mode === 'image') {
    const pr = formatPerRequest(p.per_request_price, mult)
    if (pr) return `${pr} ${t('modelPlaza.pricing.perCall')}`
  }
  const input = formatPerMillion(p.input_price, mult)
  const output = formatPerMillion(p.output_price, mult)
  if (input || output) return `${input ?? '-'} / ${output ?? '-'}`
  if (p.intervals.length > 0) {
    const fin = formatPerMillion(p.intervals[0].input_price, mult)
    const fout = formatPerMillion(p.intervals[0].output_price, mult)
    if (fin || fout) return `${fin ?? '-'} / ${fout ?? '-'}`
  }
  return t('modelPlaza.card.noPricing')
}
</script>
