import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import UsageBreakdownTable from '../UsageBreakdownTable.vue'
import type { MergedUsageRow } from '../usageView'

const translations: Record<string, string> = {
  'admin.upstream.usage.unsupportedGroup': '该类型(sub2api)暂不支持采集分组消耗',
  'admin.upstream.usage.empty': '暂无消耗数据',
  'admin.upstream.usage.spent': '消耗',
  'admin.upstream.usage.paid': '实付',
  'admin.upstream.usage.quota': '额度',
  'admin.upstream.usage.requests': '请求',
  'admin.upstream.usage.deleted': '已删除',
}

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => translations[key] ?? key,
  }),
}))

const mountTable = (props: Record<string, unknown>) =>
  mount(UsageBreakdownTable, { props })

const row = (over: Partial<MergedUsageRow>): MergedUsageRow => ({
  scope_key: '1', scope_name: 'k1', deleted: false, cost_usd: 0, cost_cny: 0, requests: 0, ...over,
})

describe('UsageBreakdownTable', () => {
  it('supported=false → 显示不支持占位,不渲染表格', () => {
    const w = mountTable({ rows: [], supported: false, loading: false, nameLabel: '分组' })
    expect(w.text()).toContain('不支持')
    expect(w.find('table').exists()).toBe(false)
  })

  it('loading=true → 显示 spinner', () => {
    const w = mountTable({ rows: [], supported: true, loading: true, nameLabel: '密钥' })
    expect(w.find('.animate-spin').exists()).toBe(true)
  })

  it('空 rows → 空态文案', () => {
    const w = mountTable({ rows: [], supported: true, loading: false, nameLabel: '密钥' })
    expect(w.text()).toContain('暂无消耗数据')
  })

  it('deleted 行带「已删除」标', () => {
    const w = mountTable({
      rows: [row({ scope_key: '7', scope_name: 'old', deleted: true, cost_cny: 43.8 })],
      supported: true, loading: false, nameLabel: '密钥',
    })
    expect(w.text()).toContain('old')
    expect(w.text()).toContain('已删除')
    expect(w.text()).toContain('¥43.80')
  })

  it('消耗($)为主、实付(¥)为辅:$ 加粗主显 + ¥ 副显 + 两列表头', () => {
    const w = mountTable({
      rows: [row({ scope_key: '9', scope_name: 'live', cost_usd: 12.3, cost_cny: 1.76, requests: 400 })],
      supported: true, loading: false, nameLabel: '密钥',
    })
    // 消耗列:美元为主且加粗
    const bold = w.findAll('.font-semibold').map((e) => e.text())
    expect(bold.some((t) => t.includes('$12.30'))).toBe(true)
    // 实付列:人民币为辅
    expect(w.text()).toContain('¥1.76')
    // 表头:消耗 + 实付 两列(口径不再合并为「消耗(实付)」)
    const headers = w.findAll('th').map((h) => h.text())
    expect(headers).toContain('消耗')
    expect(headers).toContain('实付')
  })
})
