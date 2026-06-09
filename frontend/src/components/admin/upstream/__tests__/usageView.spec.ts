import { describe, it, expect } from 'vitest'
import { mergeUsageRows, formatCNY, formatUSD, formatRequests } from '../usageView'
import type { UsageBreakdownRow } from '@/api/admin/upstreamProviders'

const bd = (over: Partial<UsageBreakdownRow>): UsageBreakdownRow => ({
  scope_key: '1', scope_name: 'k1', deleted: false,
  cost_usd: 0, cost_cny: 0, requests: 0, tokens: 0, ...over,
})

describe('mergeUsageRows', () => {
  it('新增:实时列表有、breakdown 无 → 补 0 消耗行(deleted=false)', () => {
    const rows = mergeUsageRows(
      [{ scope_key: '9', scope_name: 'fresh', meta: 'default' }],
      [],
    )
    expect(rows).toHaveLength(1)
    expect(rows[0]).toMatchObject({ scope_key: '9', scope_name: 'fresh', meta: 'default', deleted: false, cost_cny: 0, requests: 0 })
  })

  it('删除:breakdown 有、实时列表无 → 保留后端 deleted=true(前端不翻案)', () => {
    const rows = mergeUsageRows([], [bd({ scope_key: '7', scope_name: 'old', deleted: true, cost_cny: 43.8, requests: 410 })])
    expect(rows).toHaveLength(1)
    expect(rows[0]).toMatchObject({ scope_key: '7', deleted: true, cost_cny: 43.8 })
  })

  it('改名:两侧同 key → 名称以实时为准,消耗用 breakdown', () => {
    const rows = mergeUsageRows(
      [{ scope_key: '1', scope_name: 'newName', meta: 'vip' }],
      [bd({ scope_key: '1', scope_name: 'oldName', cost_cny: 88.4, requests: 920 })],
    )
    expect(rows[0]).toMatchObject({ scope_name: 'newName', meta: 'vip', cost_cny: 88.4, deleted: false })
  })

  it('排序:未删除按 cost_cny 降序在前,已删除置底', () => {
    const rows = mergeUsageRows(
      [{ scope_key: '1', scope_name: 'a' }, { scope_key: '2', scope_name: 'b' }],
      [
        bd({ scope_key: '1', cost_cny: 10 }),
        bd({ scope_key: '2', cost_cny: 200 }),
        bd({ scope_key: '8', scope_name: 'gone', deleted: true, cost_cny: 999 }),
      ],
    )
    expect(rows.map((r) => r.scope_key)).toEqual(['2', '1', '8'])
  })
})

describe('format helpers', () => {
  it('formatCNY/USD 两位小数带符号', () => {
    expect(formatCNY(342.1)).toBe('¥342.10')
    expect(formatUSD(420)).toBe('$420.00')
  })
  it('formatRequests 千位转 k', () => {
    expect(formatRequests(920)).toBe('920')
    expect(formatRequests(1000)).toBe('1.0k')
    expect(formatRequests(3100)).toBe('3.1k')
  })
})
