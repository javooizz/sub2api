import { describe, it, expect } from 'vitest'
import {
  formatPerMillion,
  formatPerRequest,
  effectiveMultiplier,
  effectiveImageMultiplier,
  imageTierLines,
  isImageTierModel,
} from '@/utils/plazaPricing'
import type { PlazaGroup, PlazaModel } from '@/api/modelPlaza'

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

function imageModel(overrides: Partial<PlazaModel> = {}): PlazaModel {
  return {
    name: 'gpt-image-2',
    platform: 'openai',
    description: '',
    billing_mode: 'image',
    pricing: {
      billing_mode: 'image',
      input_price: null,
      output_price: null,
      cache_write_price: null,
      cache_read_price: null,
      image_output_price: null,
      per_request_price: 0.6,
      intervals: [
        { min_tokens: 0, max_tokens: null, tier_label: '1K', input_price: null, output_price: null, cache_write_price: null, cache_read_price: null, per_request_price: 0.6 },
        { min_tokens: 0, max_tokens: null, tier_label: '2K', input_price: null, output_price: null, cache_write_price: null, cache_read_price: null, per_request_price: 0.6 },
        { min_tokens: 0, max_tokens: null, tier_label: '4K', input_price: null, output_price: null, cache_write_price: null, cache_read_price: null, per_request_price: 0.8 },
      ],
    },
    groups: [],
    ...overrides,
  }
}

function imageGroup(overrides: Partial<PlazaGroup> = {}): PlazaGroup {
  return group({
    id: 23,
    name: 'gpt-image',
    platform: 'openai',
    rate_multiplier: 0.1,
    image_pricing: { allowed: true, price_1k: 0.6, price_2k: 0.6, price_4k: 0.8, multiplier_override: null },
    ...overrides,
  })
}

describe('isImageTierModel', () => {
  it('image 模式且带按次档位 → true', () => {
    expect(isImageTierModel(imageModel())).toBe(true)
  })
  it('token 模式 / 无 pricing / 无按次价 → false', () => {
    expect(isImageTierModel(imageModel({ billing_mode: 'token' }))).toBe(false)
    expect(isImageTierModel(imageModel({ pricing: null }))).toBe(false)
    const m = imageModel()
    m.pricing!.per_request_price = null
    m.pricing!.intervals = []
    expect(isImageTierModel(m)).toBe(false)
  })
})

describe('effectiveImageMultiplier', () => {
  it('multiplier_override 优先(image_rate_independent,不吃用户专属倍率)', () => {
    const g = imageGroup({ image_pricing: { allowed: true, price_1k: null, price_2k: null, price_4k: null, multiplier_override: 1 } })
    expect(effectiveImageMultiplier(g, { 23: 0.05 })).toBe(1)
  })
  it('无 override → 常规生效倍率(用户专属优先,否则分组默认)', () => {
    expect(effectiveImageMultiplier(imageGroup(), { 23: 0.05 })).toBe(0.05)
    expect(effectiveImageMultiplier(imageGroup(), {})).toBe(0.1)
  })
  it('override=0 → 倍率 0(免费出图,不吃用户专属倍率)', () => {
    const g = imageGroup({ image_pricing: { allowed: true, price_1k: 0.6, price_2k: 0.6, price_4k: 0.8, multiplier_override: 0 } })
    expect(effectiveImageMultiplier(g, { 23: 0.5 })).toBe(0)
  })
})

describe('imageTierLines', () => {
  it('group=null → 模型级基准三档(倍率 1)', () => {
    expect(imageTierLines(imageModel(), null, {})).toEqual([
      { tier: '1K', value: '$0.60' },
      { tier: '2K', value: '$0.60' },
      { tier: '4K', value: '$0.80' },
    ])
  })
  it('分组档价 × 分组倍率(0.1x)', () => {
    expect(imageTierLines(imageModel(), imageGroup(), {})).toEqual([
      { tier: '1K', value: '$0.06' },
      { tier: '2K', value: '$0.06' },
      { tier: '4K', value: '$0.08' },
    ])
  })
  it('分组档价为 null(渠道按次价遮蔽)→ 回落模型级档价 × 倍率', () => {
    const g = imageGroup({ image_pricing: { allowed: true, price_1k: null, price_2k: null, price_4k: null, multiplier_override: null } })
    expect(imageTierLines(imageModel(), g, {})).toEqual([
      { tier: '1K', value: '$0.06' },
      { tier: '2K', value: '$0.06' },
      { tier: '4K', value: '$0.08' },
    ])
  })
  it('用户专属倍率参与折算;override 时不参与', () => {
    expect(imageTierLines(imageModel(), imageGroup(), { 23: 0.5 })[0].value).toBe('$0.30')
    const g = imageGroup({ image_pricing: { allowed: true, price_1k: 0.6, price_2k: 0.6, price_4k: 0.8, multiplier_override: 1 } })
    expect(imageTierLines(imageModel(), g, { 23: 0.5 })[0].value).toBe('$0.60')
  })
  it('无 pricing → 空数组', () => {
    expect(imageTierLines(imageModel({ pricing: null }), null, {})).toEqual([])
  })
  it('档价 0 → 显示 $0.00(免费档,?? 不回落)', () => {
    const g = imageGroup({ image_pricing: { allowed: true, price_1k: 0, price_2k: 0, price_4k: 0, multiplier_override: null } })
    expect(imageTierLines(imageModel(), g, {}).map((l) => l.value)).toEqual(['$0.00', '$0.00', '$0.00'])
  })
})
