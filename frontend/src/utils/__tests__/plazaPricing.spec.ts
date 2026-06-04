import { describe, it, expect } from 'vitest'
import { formatPerMillion, formatPerRequest, effectiveMultiplier } from '@/utils/plazaPricing'
import type { PlazaGroup } from '@/api/modelPlaza'

function group(overrides: Partial<PlazaGroup> = {}): PlazaGroup {
  return {
    id: 3,
    name: 'cc_max',
    platform: 'anthropic',
    subscription_type: 'standard',
    rate_multiplier: 1.8,
    is_exclusive: false,
    accessible: true,
    ...overrides,
  }
}

describe('formatPerMillion', () => {
  it('每 token 价 ×1e6 显示，去尾零但至少 2 位小数', () => {
    expect(formatPerMillion(1.75e-7)).toBe('$0.175')
    expect(formatPerMillion(1.4e-6)).toBe('$1.40')
    expect(formatPerMillion(3e-6)).toBe('$3.00')
    expect(formatPerMillion(2.0625e-6)).toBe('$2.0625')
  })

  it('null/undefined → null', () => {
    expect(formatPerMillion(null)).toBeNull()
    expect(formatPerMillion(undefined)).toBeNull()
  })

  it('零价（免费/未配置为 0）正常显示', () => {
    expect(formatPerMillion(0)).toBe('$0.00')
    expect(formatPerRequest(0)).toBe('$0.00')
  })

  it('倍率参与折算', () => {
    expect(formatPerMillion(9e-7, 1.8)).toBe('$1.62')
    expect(formatPerMillion(9e-7, 0.5)).toBe('$0.45')
  })
})

describe('formatPerRequest', () => {
  it('按次价格直接显示', () => {
    expect(formatPerRequest(0.08)).toBe('$0.08')
    expect(formatPerRequest(0.266)).toBe('$0.266')
  })

  it('null → null；倍率折算', () => {
    expect(formatPerRequest(null)).toBeNull()
    expect(formatPerRequest(0.1, 0.5)).toBe('$0.05')
  })
})

describe('effectiveMultiplier', () => {
  it('用户专属倍率优先，否则用分组默认', () => {
    expect(effectiveMultiplier(group(), { 3: 1.2 })).toBe(1.2)
    expect(effectiveMultiplier(group(), {})).toBe(1.8)
    expect(effectiveMultiplier(group(), { 99: 1.2 })).toBe(1.8)
  })
})
