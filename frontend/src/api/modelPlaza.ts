/**
 * User Model Plaza API (non-admin)
 * 模型广场：以模型为中心的定价目录（标准基准价 + 可见分组倍率）。
 * 价格单位 USD per token，展示层 ×1e6 为 $/1M tokens（见 utils/plazaPricing.ts）。
 */

import { apiClient } from './client'
import type { BillingMode } from '@/constants/channel'

export interface PlazaGroup {
  id: number
  name: string
  platform: string
  /** 'standard' | 'subscription' — 订阅分组视觉加深，同 API 密钥页。 */
  subscription_type: string
  /** 分组默认倍率。用户专属倍率（若有）经 /groups/rates 在前端 join。 */
  rate_multiplier: number
  is_exclusive: boolean
  /** false = 公开订阅型但未订阅 → 前端显示"需订阅"标签。 */
  accessible: boolean
}

export interface PlazaPricingInterval {
  min_tokens: number
  max_tokens: number | null
  tier_label?: string
  input_price: number | null
  output_price: number | null
  cache_write_price: number | null
  cache_read_price: number | null
  per_request_price: number | null
}

export interface PlazaModelPricing {
  billing_mode: BillingMode
  input_price: number | null
  output_price: number | null
  cache_write_price: number | null
  cache_read_price: number | null
  image_output_price: number | null
  per_request_price: number | null
  intervals: PlazaPricingInterval[]
}

/** 模型唯一身份 = (platform, name)。 */
export interface PlazaModel {
  name: string
  platform: string
  description: string
  billing_mode: BillingMode
  /** null = 无可展示定价（前端显示"价格未配置"）。 */
  pricing: PlazaModelPricing | null
  groups: PlazaGroup[]
}

export interface ModelPlazaResponse {
  enabled: boolean
  announcement: string
  models: PlazaModel[]
}

/** 获取当前用户可见的模型广场视图。开关关闭时 enabled=false 且 models 为空。 */
export async function getModelPlaza(options?: { signal?: AbortSignal }): Promise<ModelPlazaResponse> {
  const { data } = await apiClient.get<ModelPlazaResponse>('/model-plaza', {
    signal: options?.signal,
  })
  return data
}

export const modelPlazaAPI = { getModelPlaza }

export default modelPlazaAPI
