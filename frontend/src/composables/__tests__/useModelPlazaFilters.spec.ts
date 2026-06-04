import { describe, it, expect } from 'vitest'
import { ref } from 'vue'
import { useModelPlazaFilters } from '@/composables/useModelPlazaFilters'
import type { PlazaModel, PlazaGroup } from '@/api/modelPlaza'

function g(id: number, overrides: Partial<PlazaGroup> = {}): PlazaGroup {
  return {
    id,
    name: `g${id}`,
    platform: 'anthropic',
    subscription_type: 'standard',
    rate_multiplier: 1,
    is_exclusive: false,
    accessible: true,
    ...overrides,
  }
}

function model(overrides: Partial<PlazaModel> = {}): PlazaModel {
  return {
    name: 'claude-sonnet-4-6',
    platform: 'anthropic',
    description: '',
    billing_mode: 'token',
    pricing: null,
    groups: [g(1)],
    ...overrides,
  }
}

const dataset: PlazaModel[] = [
  model(),
  model({ name: 'claude-opus-4-6', groups: [g(1), g(2)] }),
  model({ name: 'gpt-5.2', platform: 'openai', billing_mode: 'token', groups: [g(3, { platform: 'openai' })] }),
  model({
    name: 'gpt-image-2',
    platform: 'openai',
    billing_mode: 'per_request',
    description: '图像生成',
    groups: [g(3, { platform: 'openai' })],
  }),
]

describe('useModelPlazaFilters', () => {
  it('无筛选时返回全部', () => {
    const { filtered } = useModelPlazaFilters(ref(dataset))
    expect(filtered.value).toHaveLength(4)
  })

  it('按平台筛选', () => {
    const { filters, filtered } = useModelPlazaFilters(ref(dataset))
    filters.platform = 'openai'
    expect(filtered.value.map((m) => m.name)).toEqual(['gpt-5.2', 'gpt-image-2'])
  })

  it('按分组筛选（模型挂多个分组时命中任一即保留）', () => {
    const { filters, filtered } = useModelPlazaFilters(ref(dataset))
    filters.groupId = 2
    expect(filtered.value.map((m) => m.name)).toEqual(['claude-opus-4-6'])
  })

  it('按计费类型筛选', () => {
    const { filters, filtered } = useModelPlazaFilters(ref(dataset))
    filters.billingMode = 'per_request'
    expect(filtered.value.map((m) => m.name)).toEqual(['gpt-image-2'])
  })

  it('搜索命中模型名或描述（大小写不敏感）', () => {
    const { filters, filtered } = useModelPlazaFilters(ref(dataset))
    filters.search = 'OPUS'
    expect(filtered.value.map((m) => m.name)).toEqual(['claude-opus-4-6'])
    filters.search = '图像'
    expect(filtered.value.map((m) => m.name)).toEqual(['gpt-image-2'])
  })

  it('组合筛选取交集', () => {
    const { filters, filtered } = useModelPlazaFilters(ref(dataset))
    filters.platform = 'openai'
    filters.billingMode = 'token'
    expect(filtered.value.map((m) => m.name)).toEqual(['gpt-5.2'])
  })

  it('platformOptions 按 search 过滤后计数并按名排序', () => {
    const { filters, platformOptions } = useModelPlazaFilters(ref(dataset))
    expect(platformOptions.value).toEqual([
      { value: 'anthropic', count: 2 },
      { value: 'openai', count: 2 },
    ])
    filters.search = 'gpt'
    expect(platformOptions.value).toEqual([{ value: 'openai', count: 2 }])
  })

  it('groupOptions 全量去重并按名排序', () => {
    const { groupOptions } = useModelPlazaFilters(ref(dataset))
    expect(groupOptions.value.map((x) => x.id)).toEqual([1, 2, 3])
  })

  it('selectedGroup 返回选中的分组对象', () => {
    const { filters, selectedGroup } = useModelPlazaFilters(ref(dataset))
    expect(selectedGroup.value).toBeNull()
    filters.groupId = 3
    expect(selectedGroup.value?.platform).toBe('openai')
  })

  it('reset 清空全部筛选', () => {
    const { filters, filtered, reset } = useModelPlazaFilters(ref(dataset))
    filters.platform = 'openai'
    filters.groupId = 3
    filters.billingMode = 'token'
    filters.search = 'gpt'
    reset()
    expect(filtered.value).toHaveLength(4)
    expect(filters.platform).toBeNull()
    expect(filters.search).toBe('')
  })
})
