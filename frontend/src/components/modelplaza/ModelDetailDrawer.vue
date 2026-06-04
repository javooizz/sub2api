<template>
  <Teleport to="body">
    <Transition name="drawer-fade">
      <div
        v-if="model"
        class="fixed inset-0 z-50 bg-black/40"
        aria-hidden="true"
        @click="emit('close')"
      />
    </Transition>
    <Transition name="drawer-slide">
      <section
        v-if="model"
        ref="panelRef"
        class="fixed inset-y-0 right-0 z-50 flex w-full max-w-lg flex-col bg-white shadow-xl dark:bg-gray-900"
        role="dialog"
        aria-modal="true"
        :aria-label="model.name"
        @keydown.esc="emit('close')"
      >
        <!-- 头部 -->
        <header class="flex items-center justify-between border-b border-gray-200 px-5 py-4 dark:border-gray-700">
          <div class="flex min-w-0 items-center gap-3">
            <ModelIcon :model="model.name" size="32px" />
            <div class="min-w-0">
              <h2 class="truncate text-base font-semibold text-gray-900 dark:text-gray-100">
                {{ model.name }}
              </h2>
              <p class="text-xs text-gray-500 dark:text-gray-400">{{ model.platform }}</p>
            </div>
            <button
              type="button"
              class="cursor-pointer rounded p-1 text-gray-400 hover:text-gray-600 focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary-500 dark:hover:text-gray-300"
              :title="t('modelPlaza.card.copyModelName')"
              :aria-label="t('modelPlaza.card.copyModelName')"
              @click="emit('copy', model.name)"
            >
              <Icon name="copy" size="sm" />
            </button>
          </div>
          <button
            ref="closeBtnRef"
            type="button"
            class="cursor-pointer rounded p-1.5 text-gray-400 hover:bg-gray-100 hover:text-gray-600 focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary-500 dark:hover:bg-gray-800"
            :aria-label="t('common.close', 'Close')"
            @click="emit('close')"
          >
            <Icon name="x" size="md" />
          </button>
        </header>

        <div class="flex-1 space-y-6 overflow-y-auto px-5 py-4">
          <!-- 基本信息 -->
          <section>
            <h3 class="detail-section-title">{{ t('modelPlaza.detail.basicInfo') }}</h3>
            <p class="text-sm text-gray-600 dark:text-gray-300">
              {{ model.description || t('modelPlaza.detail.noDescription') }}
            </p>
            <span
              class="mt-2 inline-block rounded-full bg-emerald-100 px-2 py-0.5 text-[11px] font-medium text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300"
            >
              {{ t(`modelPlaza.billingMode.${model.billing_mode}`, model.billing_mode) }}
            </span>
          </section>

          <!-- 标准价格 -->
          <section>
            <h3 class="detail-section-title">{{ t('modelPlaza.detail.basePricing') }}</h3>
            <dl v-if="basePriceRows.length > 0" class="space-y-1.5 text-sm">
              <div
                v-for="row in basePriceRows"
                :key="row.label"
                class="flex items-baseline justify-between"
              >
                <dt class="text-gray-500 dark:text-gray-400">{{ row.label }}</dt>
                <dd class="font-mono text-gray-900 dark:text-gray-100">
                  {{ row.value }} <span class="font-sans text-[10px] text-gray-400">{{ row.unit }}</span>
                </dd>
              </div>
            </dl>
            <p v-else class="text-sm text-gray-400">{{ t('modelPlaza.card.noPricing') }}</p>

            <!-- 区间档位 -->
            <div v-if="(model.pricing?.intervals?.length ?? 0) > 0" class="mt-3">
              <table class="w-full text-xs">
                <thead>
                  <tr class="text-left text-gray-500 dark:text-gray-400">
                    <th class="py-1 pr-2 font-medium">{{ t('modelPlaza.detail.tierRange') }}</th>
                    <th class="py-1 pr-2 font-medium">{{ t('modelPlaza.pricing.input') }}</th>
                    <th class="py-1 font-medium">{{ t('modelPlaza.pricing.output') }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 dark:divide-gray-800">
                  <tr v-for="(iv, idx) in model.pricing!.intervals" :key="idx">
                    <td class="py-1.5 pr-2 text-gray-600 dark:text-gray-300">
                      {{ iv.tier_label || intervalRange(iv) }}
                    </td>
                    <td class="py-1.5 pr-2 font-mono">{{ formatPerMillion(iv.input_price) ?? '-' }}</td>
                    <td class="py-1.5 font-mono">{{ formatPerMillion(iv.output_price) ?? '-' }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </section>

          <!-- 分组价格对比 -->
          <section>
            <h3 class="detail-section-title">{{ t('modelPlaza.detail.groupPricing') }}</h3>
            <p class="mb-2 text-xs text-gray-400 dark:text-gray-500">
              {{ t('modelPlaza.detail.groupPricingHint') }}
            </p>
            <div class="space-y-2">
              <div
                v-for="g in model.groups"
                :key="g.id"
                class="rounded-lg border border-gray-200 p-3 dark:border-gray-700"
                :class="{ 'ring-1 ring-blue-300 dark:ring-blue-700': userRates[g.id] !== undefined }"
              >
                <div class="mb-1.5 flex items-center justify-between gap-2">
                  <div class="flex min-w-0 items-center gap-1.5">
                    <span class="truncate text-sm font-medium text-gray-900 dark:text-gray-100">{{ g.name }}</span>
                    <span
                      v-if="g.subscription_type === 'subscription' && !g.accessible"
                      class="flex shrink-0 items-center gap-0.5 rounded bg-amber-100 px-1.5 py-0.5 text-[10px] font-medium text-amber-700 dark:bg-amber-900/40 dark:text-amber-300"
                    >
                      <Icon name="lock" size="xs" />
                      {{ t('modelPlaza.filters.requiresSubscription') }}
                    </span>
                    <span
                      v-if="userRates[g.id] !== undefined"
                      class="shrink-0 rounded bg-blue-100 px-1.5 py-0.5 text-[10px] font-medium text-blue-700 dark:bg-blue-900/40 dark:text-blue-300"
                    >
                      {{ t('modelPlaza.card.userRateBadge') }}
                    </span>
                  </div>
                  <span class="shrink-0 font-mono text-xs text-gray-500 dark:text-gray-400">
                    {{ effectiveMultiplier(g, userRates) }}x
                  </span>
                </div>
                <dl class="space-y-1 text-xs">
                  <div
                    v-for="row in groupPriceRows(g)"
                    :key="row.label"
                    class="flex items-baseline justify-between"
                  >
                    <dt class="text-gray-500 dark:text-gray-400">{{ row.label }}</dt>
                    <dd class="font-mono text-gray-800 dark:text-gray-200">
                      {{ row.value }} <span class="font-sans text-[10px] text-gray-400">{{ row.unit }}</span>
                    </dd>
                  </div>
                </dl>
                <RouterLink
                  v-if="g.subscription_type === 'subscription' && !g.accessible"
                  to="/purchase"
                  class="mt-2 inline-flex items-center gap-1 text-xs font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400"
                >
                  {{ t('modelPlaza.detail.goSubscribe') }}
                  <Icon name="arrowRight" size="xs" />
                </RouterLink>
              </div>
            </div>
          </section>
        </div>
      </section>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import Icon from '@/components/icons/Icon.vue'
import ModelIcon from '@/components/common/ModelIcon.vue'
import { formatPerMillion, formatPerRequest, effectiveMultiplier } from '@/utils/plazaPricing'
import type { PlazaModel, PlazaGroup, PlazaPricingInterval } from '@/api/modelPlaza'

const props = defineProps<{
  /** null = 关闭。 */
  model: PlazaModel | null
  userRates: Record<number, number>
}>()

const emit = defineEmits<{
  close: []
  copy: [name: string]
}>()

const { t } = useI18n()
const panelRef = ref<HTMLElement | null>(null)
const closeBtnRef = ref<HTMLButtonElement | null>(null)

// 打开时聚焦关闭按钮（可访问性：焦点进入 dialog）
watch(
  () => props.model,
  (m) => {
    if (m) nextTick(() => closeBtnRef.value?.focus())
  },
)

interface PriceRow {
  label: string
  value: string
  unit: string
}

function buildPriceRows(m: PlazaModel, mult: number): PriceRow[] {
  const p = m.pricing
  if (!p) return []
  const perM = t('modelPlaza.pricing.perMillionTokens')
  const perCall = t('modelPlaza.pricing.perCall')
  const rows: PriceRow[] = []
  const push = (label: string, value: string | null, unit: string) => {
    if (value) rows.push({ label, value, unit })
  }
  if (m.billing_mode === 'per_request' || m.billing_mode === 'image') {
    push(t('modelPlaza.pricing.perRequest'), formatPerRequest(p.per_request_price, mult), perCall)
    push(t('modelPlaza.pricing.imageOutput'), formatPerMillion(p.image_output_price, mult), perM)
  }
  push(t('modelPlaza.pricing.input'), formatPerMillion(p.input_price, mult), perM)
  push(t('modelPlaza.pricing.output'), formatPerMillion(p.output_price, mult), perM)
  push(t('modelPlaza.pricing.cacheRead'), formatPerMillion(p.cache_read_price, mult), perM)
  push(t('modelPlaza.pricing.cacheWrite'), formatPerMillion(p.cache_write_price, mult), perM)
  return rows
}

const basePriceRows = computed<PriceRow[]>(() =>
  props.model ? buildPriceRows(props.model, 1) : [],
)

function groupPriceRows(g: PlazaGroup): PriceRow[] {
  if (!props.model) return []
  return buildPriceRows(props.model, effectiveMultiplier(g, props.userRates))
}

function intervalRange(iv: PlazaPricingInterval): string {
  const max = iv.max_tokens === null ? '∞' : `${iv.max_tokens}`
  return `${iv.min_tokens} - ${max}`
}
</script>

<style scoped>
.detail-section-title {
  margin-bottom: 0.5rem;
  font-size: 0.8125rem;
  font-weight: 600;
  color: rgb(17 24 39);
}
.dark .detail-section-title {
  color: rgb(243 244 246);
}

.drawer-fade-enter-active,
.drawer-fade-leave-active {
  transition: opacity 200ms ease;
}
.drawer-fade-enter-from,
.drawer-fade-leave-to {
  opacity: 0;
}
.drawer-slide-enter-active,
.drawer-slide-leave-active {
  transition: transform 250ms ease;
}
.drawer-slide-enter-from,
.drawer-slide-leave-to {
  transform: translateX(100%);
}
@media (prefers-reduced-motion: reduce) {
  .drawer-fade-enter-active,
  .drawer-fade-leave-active,
  .drawer-slide-enter-active,
  .drawer-slide-leave-active {
    transition: none;
  }
}
</style>
