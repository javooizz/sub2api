/**
 * User Model Plaza API (non-admin)
 * 模型广场：以模型为中心的定价目录（标准基准价 + 可见分组倍率）。
 * 价格单位 USD per token，展示层 ×1e6 为 $/1M tokens（见 utils/catalogPricing.ts）。
 */

import { apiClient } from './client'
import type { BillingMode } from '@/constants/channel'

export interface CatalogGroupImagePricing {
  /** false = 该分组不允许出图,前端显示提示。 */
  allowed: boolean
  /** 解析后的分组基准档价(后端已含 fallback);渠道按次价优先时为 null(用模型级 pricing)。 */
  price_1k: number | null
  price_2k: number | null
  price_4k: number | null
  /** image_rate_independent 时非 null:固定倍率,不吃用户专属倍率。 */
  multiplier_override: number | null
}

export interface CatalogGroup {
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
  /** 出图计费展示信息;仅图像生成模型的分组带(规格 2026-06-07)。 */
  image_pricing?: CatalogGroupImagePricing | null
}

export interface CatalogPricingInterval {
  min_tokens: number
  max_tokens: number | null
  tier_label?: string
  input_price: number | null
  output_price: number | null
  cache_write_price: number | null
  cache_read_price: number | null
  per_request_price: number | null
}

export interface CatalogModelPricing {
  billing_mode: BillingMode
  input_price: number | null
  output_price: number | null
  cache_write_price: number | null
  cache_read_price: number | null
  image_output_price: number | null
  per_request_price: number | null
  intervals: CatalogPricingInterval[]
}

/** 模型唯一身份 = (platform, name)。 */
export interface CatalogModel {
  name: string
  platform: string
  description: string
  billing_mode: BillingMode
  /** null = 无可展示定价（前端显示"价格未配置"）。 */
  pricing: CatalogModelPricing | null
  groups: CatalogGroup[]
}

export interface ModelCatalogResponse {
  enabled: boolean
  announcement: string
  models: CatalogModel[]
}

/** 获取当前用户可见的模型广场视图。开关关闭时 enabled=false 且 models 为空。 */
export async function getModelCatalog(options?: { signal?: AbortSignal }): Promise<ModelCatalogResponse> {
  const { data } = await apiClient.get<ModelCatalogResponse>('/model-catalog', {
    signal: options?.signal,
  })
  return data
}

export const modelCatalogAPI = { getModelCatalog }

export default modelCatalogAPI
