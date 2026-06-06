/**
 * 模型广场价格格式化与倍率折算。
 * 存储单位 USD per token；展示统一 $/1M tokens（×1e6）。
 */

import type { PlazaGroup, PlazaModel } from '@/api/modelPlaza'

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

/** 图像三档价格行（规格 2026-06-07）。 */
export interface ImageTierLine {
  /** 档位标签：'1K' | '2K' | '4K'（来自后端合成/渠道 tier_label）。 */
  tier: string
  /** 已折算格式化价，如 "$0.06"。 */
  value: string
}

/** 模型是否按图像三档按次展示：billing_mode=image 且有带价按次档位/兜底价。 */
export function isImageTierModel(model: PlazaModel): boolean {
  if (model.billing_mode !== 'image') return false
  const p = model.pricing
  if (!p) return false
  return p.per_request_price !== null || p.intervals.some((iv) => iv.per_request_price !== null)
}

/**
 * 分组生效图像倍率：multiplier_override（image_rate_independent，固定、不吃用户
 * 专属倍率）优先，否则常规生效倍率（用户专属优先）。与后端 resolveImageRateMultiplier 同语义。
 */
export function effectiveImageMultiplier(
  group: PlazaGroup,
  userRates: Record<number, number>,
): number {
  return group.image_pricing?.multiplier_override ?? effectiveMultiplier(group, userRates)
}

/**
 * 构建图像模型三档按次价格行（规格 2026-06-07 §5）。
 * 档价 = （分组档价 ?? 模型级 intervals 档价）× 图像生效倍率；group=null 时为基准价（倍率 1）。
 */
export function imageTierLines(
  model: PlazaModel,
  group: PlazaGroup | null,
  userRates: Record<number, number>,
): ImageTierLine[] {
  const p = model.pricing
  if (!p) return []
  const mult = group ? effectiveImageMultiplier(group, userRates) : 1
  const groupPrices: Record<string, number | null | undefined> = {
    '1K': group?.image_pricing?.price_1k,
    '2K': group?.image_pricing?.price_2k,
    '4K': group?.image_pricing?.price_4k,
  }
  const lines: ImageTierLine[] = []
  for (const iv of p.intervals) {
    if (!iv.tier_label) continue
    const price = groupPrices[iv.tier_label] ?? iv.per_request_price
    const formatted = formatPerRequest(price, mult)
    if (formatted) lines.push({ tier: iv.tier_label, value: formatted })
  }
  return lines
}
