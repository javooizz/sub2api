<template>
  <article
    class="card group relative flex cursor-pointer flex-col gap-3 p-4 transition-shadow hover:shadow-md focus-within:ring-2 focus-within:ring-primary-500"
    role="button"
    tabindex="0"
    @click="emit('open', model)"
    @keydown.enter="emit('open', model)"
    @keydown.space.prevent="emit('open', model)"
  >
    <div class="flex items-start justify-between gap-2">
      <div class="flex min-w-0 items-center gap-2.5">
        <ModelIcon :model="model.name" size="28px" />
        <div class="min-w-0">
          <h3 class="truncate text-sm font-semibold text-gray-900 dark:text-gray-100">
            {{ model.name }}
          </h3>
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ model.platform }}</p>
        </div>
      </div>
      <div class="flex shrink-0 items-center gap-1.5" @click.stop>
        <button
          type="button"
          class="cursor-pointer rounded p-1 text-gray-400 opacity-0 transition-opacity hover:text-gray-600 focus-visible:opacity-100 focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary-500 group-hover:opacity-100 dark:hover:text-gray-300"
          :title="t('modelPlaza.card.copyModelName')"
          :aria-label="t('modelPlaza.card.copyModelName')"
          @click="emit('copy', model.name)"
        >
          <Icon name="copy" size="sm" />
        </button>
        <input
          type="checkbox"
          class="h-4 w-4 cursor-pointer rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-gray-600"
          :checked="selected"
          :aria-label="model.name"
          @change="emit('toggle-select', model)"
        />
      </div>
    </div>

    <p v-if="model.description" class="line-clamp-2 text-xs text-gray-500 dark:text-gray-400">
      {{ model.description }}
    </p>

    <!-- 价格区 -->
    <div class="space-y-1 text-xs">
      <template v-if="priceLines.length > 0">
        <div
          v-for="line in priceLines"
          :key="line.label"
          class="flex items-baseline justify-between text-gray-600 dark:text-gray-300"
        >
          <span>{{ line.label }}</span>
          <span class="font-mono font-medium text-gray-900 dark:text-gray-100">
            {{ line.value }} <span class="font-sans text-[10px] text-gray-400">{{ line.unit }}</span>
          </span>
        </div>
      </template>
      <p v-else class="text-gray-400 dark:text-gray-500">{{ t('modelPlaza.card.noPricing') }}</p>
    </div>

    <div class="mt-auto flex flex-wrap items-center gap-1.5 pt-1">
      <span
        class="rounded-full px-2 py-0.5 text-[11px] font-medium"
        :class="billingBadgeClass"
      >
        {{ t(`modelPlaza.billingMode.${model.billing_mode}`, model.billing_mode) }}
      </span>
      <span
        v-if="(model.pricing?.intervals?.length ?? 0) > 0"
        class="rounded-full bg-violet-100 px-2 py-0.5 text-[11px] font-medium text-violet-700 dark:bg-violet-900/40 dark:text-violet-300"
      >
        {{ t('modelPlaza.card.dynamicTiers', { count: model.pricing!.intervals.length }) }}
      </span>
      <span
        v-if="activeMultiplier !== null"
        class="rounded-full bg-blue-100 px-2 py-0.5 font-mono text-[11px] font-medium text-blue-700 dark:bg-blue-900/40 dark:text-blue-300"
        :title="multiplierTitle"
      >
        {{ t('modelPlaza.card.multiplierBadge', { multiplier: activeMultiplier }) }}
      </span>
      <span class="ml-auto text-[11px] text-gray-400">
        {{ t('modelPlaza.card.groupCount', { count: model.groups.length }) }}
      </span>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import ModelIcon from '@/components/common/ModelIcon.vue'
import { formatPerMillion, formatPerRequest, effectiveMultiplier } from '@/utils/plazaPricing'
import type { PlazaModel, PlazaGroup } from '@/api/modelPlaza'

const props = defineProps<{
  model: PlazaModel
  selected: boolean
  /** 选中分组时卡片显示该分组折算价；null 显示基准价。 */
  selectedGroup: PlazaGroup | null
  userRates: Record<number, number>
}>()

const emit = defineEmits<{
  open: [model: PlazaModel]
  copy: [name: string]
  'toggle-select': [model: PlazaModel]
}>()

const { t } = useI18n()

/** 当前生效倍率：选中分组（且模型挂在该分组）→ 分组倍率（用户专属优先）；否则 null（基准价）。 */
const activeMultiplier = computed<number | null>(() => {
  const g = props.selectedGroup
  if (!g) return null
  if (!props.model.groups.some((x) => x.id === g.id)) return null
  return effectiveMultiplier(g, props.userRates)
})

const multiplierTitle = computed(() => {
  const g = props.selectedGroup
  if (!g) return ''
  return props.userRates[g.id] !== undefined
    ? t('modelPlaza.card.userRateBadge')
    : t('modelPlaza.card.groupPrice', { group: g.name })
})

interface PriceLine {
  label: string
  value: string
  unit: string
}

const priceLines = computed<PriceLine[]>(() => {
  const p = props.model.pricing
  if (!p) return []
  const m = activeMultiplier.value ?? 1
  const perM = t('modelPlaza.pricing.perMillionTokens')
  const perCall = t('modelPlaza.pricing.perCall')
  const lines: PriceLine[] = []

  if (props.model.billing_mode === 'per_request' || props.model.billing_mode === 'image') {
    const pr = formatPerRequest(p.per_request_price, m)
    if (pr) lines.push({ label: t('modelPlaza.pricing.perRequest'), value: pr, unit: perCall })
    const img = formatPerMillion(p.image_output_price, m)
    if (img) lines.push({ label: t('modelPlaza.pricing.imageOutput'), value: img, unit: perM })
  }
  const input = formatPerMillion(p.input_price, m)
  if (input) lines.push({ label: t('modelPlaza.pricing.input'), value: input, unit: perM })
  const output = formatPerMillion(p.output_price, m)
  if (output) lines.push({ label: t('modelPlaza.pricing.output'), value: output, unit: perM })
  const cacheRead = formatPerMillion(p.cache_read_price, m)
  if (cacheRead) lines.push({ label: t('modelPlaza.pricing.cacheRead'), value: cacheRead, unit: perM })
  const cacheWrite = formatPerMillion(p.cache_write_price, m)
  if (cacheWrite) lines.push({ label: t('modelPlaza.pricing.cacheWrite'), value: cacheWrite, unit: perM })

  // 区间定价且 flat 全空 → 用第一档价格展示（带"动态计费"标签提示）
  if (lines.length === 0 && p.intervals.length > 0) {
    const first = p.intervals[0]
    const fin = formatPerMillion(first.input_price, m)
    if (fin) lines.push({ label: t('modelPlaza.pricing.input'), value: fin, unit: perM })
    const fout = formatPerMillion(first.output_price, m)
    if (fout) lines.push({ label: t('modelPlaza.pricing.output'), value: fout, unit: perM })
  }
  return lines.slice(0, 4)
})

const billingBadgeClass = computed(() => {
  switch (props.model.billing_mode) {
    case 'per_request':
      return 'bg-orange-100 text-orange-700 dark:bg-orange-900/40 dark:text-orange-300'
    case 'image':
      return 'bg-pink-100 text-pink-700 dark:bg-pink-900/40 dark:text-pink-300'
    default:
      return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300'
  }
})
</script>
