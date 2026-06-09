// 上游消耗展示纯逻辑:union 合并(后端已标 deleted,前端只补「0 消耗新行」)+ 格式化。
import type { UsageBreakdownRow } from '@/api/admin/upstreamProviders'

export type UsageWindow = 'today' | 'week' | 'month' | 'total'

// 明细表行:统一形状。meta = 名称下方小字(密钥→所属分组;分组→倍率文本)。
// 刻意不含 tokens:Phase 2 口径只展示 ¥(实付)/ $(额度)/ 请求数,不展示 token 量(spec §3 决策表)。
export interface MergedUsageRow {
  scope_key: string
  scope_name: string
  meta?: string
  deleted: boolean
  cost_usd: number
  cost_cny: number
  requests: number
}

// 实时列表项最小形状(密钥来自 listTokens、分组来自 snapshot.groups)。
export interface LiveScopeItem {
  scope_key: string
  scope_name: string
  meta?: string
}

/**
 * 合并实时列表与 breakdown:
 * - breakdown 行整行采用(后端已给 deleted / 最新 scope_name / 消耗)。
 * - 实时列表独有(在 live、不在 breakdown)→ 补一行 deleted=false、消耗 0。
 * - 两侧同 key → 名称/meta 以实时为准(更新),消耗用 breakdown。
 */
export function mergeUsageRows(live: LiveScopeItem[], breakdown: UsageBreakdownRow[]): MergedUsageRow[] {
  const byKey = new Map<string, MergedUsageRow>()
  for (const r of breakdown) {
    byKey.set(r.scope_key, {
      scope_key: r.scope_key,
      scope_name: r.scope_name,
      deleted: r.deleted,
      cost_usd: r.cost_usd,
      cost_cny: r.cost_cny,
      requests: r.requests,
    })
  }
  for (const l of live) {
    const existing = byKey.get(l.scope_key)
    if (existing) {
      // live 名非空才覆盖(空名则保留 breakdown 的最新快照名),防御性 falsy 判断
      if (l.scope_name) existing.scope_name = l.scope_name
      if (l.meta !== undefined) existing.meta = l.meta
      existing.deleted = false // 实时列表中存在 → 必然未删除
    } else {
      byKey.set(l.scope_key, {
        scope_key: l.scope_key,
        scope_name: l.scope_name,
        meta: l.meta,
        deleted: false,
        cost_usd: 0,
        cost_cny: 0,
        requests: 0,
      })
    }
  }
  return sortUsageRows([...byKey.values()])
}

// 未删除按 cost_cny 降序在前;已删除置底(组内同样按 cost_cny 降序)。
// 返回新数组(不原地修改入参),保持纯函数语义。
export function sortUsageRows(rows: MergedUsageRow[]): MergedUsageRow[] {
  return [...rows].sort((a, b) => {
    if (a.deleted !== b.deleted) return a.deleted ? 1 : -1
    return b.cost_cny - a.cost_cny
  })
}

export function formatCNY(v: number): string {
  return `¥${v.toFixed(2)}`
}

export function formatUSD(v: number): string {
  return `$${v.toFixed(2)}`
}

export function formatRequests(n: number): string {
  return n >= 1000 ? `${(n / 1000).toFixed(1)}k` : String(n)
}
