<template>
  <aside class="space-y-5">
    <div class="flex items-center justify-between">
      <h3 class="text-sm font-semibold text-gray-900 dark:text-gray-100">
        {{ t('modelPlaza.filters.title') }}
      </h3>
      <button
        type="button"
        class="cursor-pointer text-xs text-primary-600 hover:text-primary-700 focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary-500 dark:text-primary-400"
        @click="emit('reset')"
      >
        {{ t('modelPlaza.filters.reset') }}
      </button>
    </div>

    <!-- 供应商 -->
    <section>
      <h4 class="mb-2 text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
        {{ t('modelPlaza.filters.provider') }}
      </h4>
      <div class="space-y-1">
        <button
          type="button"
          class="filter-option"
          :class="{ 'filter-option-active': filters.platform === null }"
          @click="filters.platform = null"
        >
          <span>{{ t('modelPlaza.filters.allProviders') }}</span>
          <span class="filter-count">{{ totalCount }}</span>
        </button>
        <button
          v-for="opt in platformOptions"
          :key="opt.value"
          type="button"
          class="filter-option"
          :class="{ 'filter-option-active': filters.platform === opt.value }"
          @click="filters.platform = filters.platform === opt.value ? null : opt.value"
        >
          <span class="flex items-center gap-2">
            <ModelIcon :model="iconHintFor(opt.value)" size="16px" />
            <span>{{ opt.value }}</span>
          </span>
          <span class="filter-count">{{ opt.count }}</span>
        </button>
      </div>
    </section>

    <!-- 可用分组 -->
    <section>
      <h4 class="mb-2 text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
        {{ t('modelPlaza.filters.group') }}
      </h4>
      <div class="space-y-1">
        <button
          type="button"
          class="filter-option"
          :class="{ 'filter-option-active': filters.groupId === null }"
          @click="filters.groupId = null"
        >
          <span>{{ t('modelPlaza.filters.allGroups') }}</span>
        </button>
        <button
          v-for="grp in groupOptions"
          :key="grp.id"
          type="button"
          class="filter-option"
          :class="{
            'filter-option-active': filters.groupId === grp.id,
            'filter-option-subscription': grp.subscription_type === 'subscription',
          }"
          @click="filters.groupId = filters.groupId === grp.id ? null : grp.id"
        >
          <span class="truncate">{{ grp.name }}</span>
          <span class="flex shrink-0 items-center gap-1">
            <span
              v-if="grp.subscription_type === 'subscription' && !grp.accessible"
              class="rounded bg-amber-100 px-1 py-0.5 text-[10px] font-medium text-amber-700 dark:bg-amber-900/40 dark:text-amber-300"
            >
              {{ t('modelPlaza.filters.requiresSubscription') }}
            </span>
            <span class="filter-count">{{ multiplierLabel(grp) }}</span>
          </span>
        </button>
      </div>
    </section>

    <!-- 计费类型 -->
    <section>
      <h4 class="mb-2 text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
        {{ t('modelPlaza.filters.billingType') }}
      </h4>
      <div class="space-y-1">
        <button
          type="button"
          class="filter-option"
          :class="{ 'filter-option-active': filters.billingMode === null }"
          @click="filters.billingMode = null"
        >
          <span>{{ t('modelPlaza.filters.allBillingTypes') }}</span>
          <span class="filter-count">{{ totalCount }}</span>
        </button>
        <button
          v-for="opt in billingModeOptions"
          :key="opt.value"
          type="button"
          class="filter-option"
          :class="{ 'filter-option-active': filters.billingMode === opt.value }"
          @click="filters.billingMode = filters.billingMode === opt.value ? null : opt.value"
        >
          <span>{{ t(`modelPlaza.billingMode.${opt.value}`, opt.value) }}</span>
          <span class="filter-count">{{ opt.count }}</span>
        </button>
      </div>
    </section>
  </aside>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import ModelIcon from '@/components/common/ModelIcon.vue'
import { effectiveMultiplier } from '@/utils/plazaPricing'
import type { PlazaGroup } from '@/api/modelPlaza'
import type { PlazaFilterState, FilterOption } from '@/composables/useModelPlazaFilters'

const props = defineProps<{
  filters: PlazaFilterState
  platformOptions: FilterOption[]
  billingModeOptions: FilterOption[]
  groupOptions: PlazaGroup[]
  totalCount: number
  userRates: Record<number, number>
}>()

const emit = defineEmits<{ reset: [] }>()
const { t } = useI18n()

/** platform → ModelIcon 的提示词（ModelIcon 按模型名猜图标，平台名直接映射代表模型）。 */
function iconHintFor(platform: string): string {
  const hints: Record<string, string> = {
    anthropic: 'claude',
    openai: 'gpt',
    gemini: 'gemini',
    antigravity: 'gemini',
  }
  return hints[platform] ?? platform
}

function multiplierLabel(grp: PlazaGroup): string {
  return `${effectiveMultiplier(grp, props.userRates)}x`
}
</script>

<style scoped>
.filter-option {
  display: flex;
  width: 100%;
  cursor: pointer;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  border-radius: 0.5rem;
  padding: 0.375rem 0.625rem;
  font-size: 0.8125rem;
  color: rgb(55 65 81);
  transition: background-color 150ms;
}
.dark .filter-option {
  color: rgb(209 213 219);
}
.filter-option:hover {
  background-color: rgb(243 244 246);
}
.dark .filter-option:hover {
  background-color: rgb(31 41 55);
}
.filter-option-active {
  background-color: rgb(239 246 255);
  color: rgb(29 78 216);
  font-weight: 500;
}
.dark .filter-option-active {
  background-color: rgb(30 58 138 / 0.3);
  color: rgb(147 197 253);
}
.filter-option-subscription {
  font-weight: 500;
}
.filter-count {
  font-size: 0.6875rem;
  color: rgb(156 163 175);
}
</style>
