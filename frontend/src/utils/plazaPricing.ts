/**
 * 模型广场价格格式化与倍率折算。
 * 存储单位 USD per token；展示统一 $/1M tokens（×1e6）。
 */

import type { PlazaGroup } from '@/api/modelPlaza'

/**
 * 去掉小数尾零，但至少保留 2 位小数：
 * "1.4000" → "1.40"，"0.1750" → "0.175"，"3.0000" → "3.00"
 */
function trimTrailingZeros(s: string): string {
  let out = s
  while (out.endsWith('0')) {
    const candidate = out.slice(0, -1)
    const dec = candidate.split('.')[1] ?? ''
    if (dec.length < 2) break
    out = candidate
  }
  return out
}

/** 每 token 价 → "$X.XX"（/1M tokens 口径）。null/undefined → null。 */
export function formatPerMillion(
  perToken: number | null | undefined,
  multiplier = 1,
): string | null {
  if (perToken === null || perToken === undefined) return null
  return '$' + trimTrailingZeros((perToken * 1_000_000 * multiplier).toFixed(4))
}

/** 按次价格 → "$X.XX"。null/undefined → null。 */
export function formatPerRequest(
  price: number | null | undefined,
  multiplier = 1,
): string | null {
  if (price === null || price === undefined) return null
  return '$' + trimTrailingZeros((price * multiplier).toFixed(4))
}

/** 分组生效倍率：用户专属倍率（/groups/rates）优先，否则分组默认倍率。 */
export function effectiveMultiplier(
  group: PlazaGroup,
  userRates: Record<number, number>,
): number {
  return userRates[group.id] ?? group.rate_multiplier
}
