/**
 * Admin Model Plaza API
 * 开关走专项 settings 端点（不进全量 settings PUT）；模型清单走后端聚合端点。
 */

import { apiClient } from '../client'

export interface ModelCatalogSettings {
  enabled: boolean
}

/** 模型身份（platform + name 唯一），"模型描述"编辑器的行数据源。 */
export interface ModelIdentity {
  platform: string
  name: string
}

export async function getSettings(): Promise<ModelCatalogSettings> {
  const { data } = await apiClient.get<ModelCatalogSettings>('/admin/settings/model-catalog')
  return data
}

export async function updateSettings(payload: ModelCatalogSettings): Promise<ModelCatalogSettings> {
  const { data } = await apiClient.put<ModelCatalogSettings>('/admin/settings/model-catalog', payload)
  return data
}

export async function listModels(): Promise<ModelIdentity[]> {
  const { data } = await apiClient.get<ModelIdentity[]>('/admin/model-catalog/models')
  return data
}

export const adminModelCatalogAPI = { getSettings, updateSettings, listModels }

export default adminModelCatalogAPI
