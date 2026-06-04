/**
 * 模型广场筛选状态与派生列表（纯逻辑，不触网络，便于 vitest 覆盖）。
 *
 * 筛选语义：
 * - 组内单选互斥（platform / groupId / billingMode 各一个值或 null）
 * - 组间组合生效（交集）
 * - 维度选项计数只受 search 影响（与 newapi 一致），不受自身/其他维度影响
 */

import { computed, reactive, type Ref } from 'vue'
import type { PlazaGroup, PlazaModel } from '@/api/modelPlaza'

export interface PlazaFilterState {
  platform: string | null
  groupId: number | null
  billingMode: string | null
  search: string
}

export interface FilterOption {
  value: string
  count: number
}

function matchesSearch(m: PlazaModel, q: string): boolean {
  return m.name.toLowerCase().includes(q) || (m.description || '').toLowerCase().includes(q)
}

export function useModelPlazaFilters(models: Ref<PlazaModel[]>) {
  const filters = reactive<PlazaFilterState>({
    platform: null,
    groupId: null,
    billingMode: null,
    search: '',
  })

  /** 应用全部筛选后的模型列表。 */
  const filtered = computed<PlazaModel[]>(() => {
    const q = filters.search.trim().toLowerCase()
    return models.value.filter((m) => {
      if (q && !matchesSearch(m, q)) return false
      if (filters.platform && m.platform !== filters.platform) return false
      if (filters.billingMode && m.billing_mode !== filters.billingMode) return false
      if (filters.groupId !== null && !m.groups.some((g) => g.id === filters.groupId)) return false
      return true
    })
  })

  /** 仅应用 search 后的列表（供维度计数）。 */
  const searchScoped = computed<PlazaModel[]>(() => {
    const q = filters.search.trim().toLowerCase()
    if (!q) return models.value
    return models.value.filter((m) => matchesSearch(m, q))
  })

  function countBy(key: (m: PlazaModel) => string): FilterOption[] {
    const counts = new Map<string, number>()
    for (const m of searchScoped.value) {
      const v = key(m)
      counts.set(v, (counts.get(v) ?? 0) + 1)
    }
    return [...counts.entries()]
      .map(([value, count]) => ({ value, count }))
      .sort((a, b) => a.value.localeCompare(b.value))
  }

  /** 供应商选项（platform + 计数）。 */
  const platformOptions = computed<FilterOption[]>(() => countBy((m) => m.platform))

  /** 计费类型选项（billing_mode + 计数）。 */
  const billingModeOptions = computed<FilterOption[]>(() => countBy((m) => m.billing_mode))

  /** 分组选项：全部模型 groups 去重（保留首个对象，含倍率/订阅/accessible），按名排序。 */
  const groupOptions = computed<PlazaGroup[]>(() => {
    const seen = new Map<number, PlazaGroup>()
    for (const m of models.value) {
      for (const g of m.groups) {
        if (!seen.has(g.id)) seen.set(g.id, g)
      }
    }
    return [...seen.values()].sort((a, b) => a.name.localeCompare(b.name))
  })

  /** 当前选中的分组对象（卡片价格按其倍率折算）。 */
  const selectedGroup = computed<PlazaGroup | null>(() => {
    if (filters.groupId === null) return null
    return groupOptions.value.find((g) => g.id === filters.groupId) ?? null
  })

  function reset() {
    filters.platform = null
    filters.groupId = null
    filters.billingMode = null
    filters.search = ''
  }

  return { filters, filtered, platformOptions, billingModeOptions, groupOptions, selectedGroup, reset }
}
