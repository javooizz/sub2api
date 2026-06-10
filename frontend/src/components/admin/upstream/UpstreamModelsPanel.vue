<template>
  <div class="space-y-3">
    <!-- 工具栏:搜索 + 视图切换 -->
    <div class="flex items-center gap-2">
      <div class="relative flex-1">
        <Icon
          name="search"
          size="sm"
          class="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-gray-400"
        />
        <input
          v-model="query"
          type="text"
          :placeholder="t('admin.upstream.detail.models.searchPlaceholder')"
          :aria-label="t('admin.upstream.detail.models.searchPlaceholder')"
          class="input w-full pl-8"
        />
        <button
          v-if="query"
          type="button"
          class="absolute right-2 top-1/2 -translate-y-1/2 cursor-pointer rounded p-0.5 text-gray-400 hover:text-gray-600 focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary-500 dark:hover:text-gray-300"
          :aria-label="t('admin.upstream.detail.models.clearSearch')"
          @click="query = ''"
        >
          <Icon name="x" size="sm" />
        </button>
      </div>
      <!-- 视图切换 segmented -->
      <div
        class="flex shrink-0 rounded-md border border-gray-200 p-0.5 dark:border-gray-700"
        role="group"
        :aria-label="t('admin.upstream.detail.models.viewLabel')"
      >
        <button
          type="button"
          class="cursor-pointer rounded px-2.5 py-1 text-xs transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary-500"
          :class="view === 'group' ? activeViewClass : idleViewClass"
          :aria-pressed="view === 'group'"
          @click="view = 'group'"
        >
          {{ t('admin.upstream.detail.models.byGroup') }}
        </button>
        <button
          type="button"
          class="cursor-pointer rounded px-2.5 py-1 text-xs transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary-500"
          :class="view === 'model' ? activeViewClass : idleViewClass"
          :aria-pressed="view === 'model'"
          @click="view = 'model'"
        >
          {{ t('admin.upstream.detail.models.byModel') }}
        </button>
      </div>
    </div>

    <!-- 统计 -->
    <p v-if="groups.length" class="text-xs text-gray-400">
      {{ t('admin.upstream.detail.models.stats', { models: uniqueModelCount, groups: groups.length }) }}
    </p>

    <!-- 空态:无分组 -->
    <p v-if="!groups.length" class="py-8 text-center text-sm text-gray-400">—</p>

    <!-- 无搜索结果 -->
    <p v-else-if="isEmpty" class="py-8 text-center text-sm text-gray-400">
      {{ t('admin.upstream.detail.models.noMatch', { query: query.trim() }) }}
    </p>

    <!-- 按分组:每组一卡(标题 + 计数徽章 + 模型 chips) -->
    <div v-else-if="view === 'group'" class="space-y-2">
      <div
        v-for="g in filteredGroups"
        :key="g.name"
        class="overflow-hidden rounded-lg border border-gray-200 dark:border-gray-700"
      >
        <div
          class="flex items-center justify-between gap-2 border-b border-gray-100 bg-gray-50 px-3 py-2 dark:border-gray-800 dark:bg-gray-800/50"
        >
          <span class="font-medium text-gray-900 dark:text-gray-100">{{ g.name }}</span>
          <span
            class="rounded-full bg-white px-2 py-0.5 text-[10px] tabular-nums text-gray-500 ring-1 ring-gray-200 dark:bg-gray-900 dark:text-gray-400 dark:ring-gray-700"
          >{{ g.models.length }}</span>
        </div>
        <div class="flex flex-wrap gap-1.5 px-3 py-2.5">
          <span
            v-for="m in g.models"
            :key="m"
            class="rounded bg-gray-100 px-2 py-0.5 font-mono text-xs text-gray-700 dark:bg-gray-800 dark:text-gray-300"
          >{{ m }}</span>
          <span v-if="!g.models.length" class="text-xs text-gray-400">—</span>
        </div>
      </div>
    </div>

    <!-- 按模型:去重模型 → 所属分组(解决"某模型在哪些分组") -->
    <div v-else class="overflow-hidden rounded-lg border border-gray-200 dark:border-gray-700">
      <div
        v-for="row in filteredModels"
        :key="row.model"
        class="flex items-start justify-between gap-3 border-b border-gray-100 px-3 py-2 last:border-b-0 hover:bg-gray-50 dark:border-gray-800 dark:hover:bg-gray-800/40"
      >
        <span class="break-all font-mono text-xs text-gray-900 dark:text-gray-100">{{ row.model }}</span>
        <div class="flex shrink-0 flex-wrap justify-end gap-1">
          <span
            v-for="gname in row.groups"
            :key="gname"
            class="rounded bg-primary-50 px-1.5 py-0.5 text-[10px] text-primary-600 dark:bg-primary-900/30 dark:text-primary-300"
          >{{ gname }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

interface UpstreamModelGroup {
  name: string
  models?: string[]
}

const props = defineProps<{ groups: UpstreamModelGroup[] }>()
const { t } = useI18n()

const query = ref('')
const view = ref<'group' | 'model'>('group')

const activeViewClass =
  'bg-primary-50 font-medium text-primary-600 dark:bg-primary-900/30 dark:text-primary-300'
const idleViewClass = 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'

const q = computed(() => query.value.trim().toLowerCase())

const normalizedGroups = computed(() =>
  props.groups.map((g) => ({ name: g.name, models: g.models ?? [] })),
)

// 去重后的唯一模型总数(跨分组)
const uniqueModelCount = computed(() => {
  const set = new Set<string>()
  for (const g of normalizedGroups.value) for (const m of g.models) set.add(m)
  return set.size
})

// 分组视图:有查询时只留含匹配模型的组,且组内只留匹配模型
const filteredGroups = computed(() => {
  if (!q.value) return normalizedGroups.value
  return normalizedGroups.value
    .map((g) => ({ name: g.name, models: g.models.filter((m) => m.toLowerCase().includes(q.value)) }))
    .filter((g) => g.models.length > 0)
})

// 模型视图:去重模型 → 所属分组,按模型名排序
const modelRows = computed(() => {
  const map = new Map<string, string[]>()
  for (const g of normalizedGroups.value) {
    for (const m of g.models) {
      const arr = map.get(m)
      if (arr) arr.push(g.name)
      else map.set(m, [g.name])
    }
  }
  return [...map.entries()]
    .map(([model, groups]) => ({ model, groups }))
    .sort((a, b) => a.model.localeCompare(b.model))
})

const filteredModels = computed(() => {
  if (!q.value) return modelRows.value
  return modelRows.value.filter((r) => r.model.toLowerCase().includes(q.value))
})

const isEmpty = computed(() =>
  view.value === 'group' ? filteredGroups.value.length === 0 : filteredModels.value.length === 0,
)
</script>
