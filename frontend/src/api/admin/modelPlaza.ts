/**
 * Admin Model Plaza API
 * 开关走专项 settings 端点（不进全量 settings PUT）；模型清单走后端聚合端点。
 */

import { apiClient } from '../client'

export interface ModelPlazaSettings {
  enabled: boolean
}

/** 模型身份（platform + name 唯一），"模型描述"编辑器的行数据源。 */
export interface ModelIdentity {
  platform: string
  name: string
}

export async function getSettings(): Promise<ModelPlazaSettings> {
  const { data } = await apiClient.get<ModelPlazaSettings>('/admin/settings/model-plaza')
  return data
}

export async function updateSettings(payload: ModelPlazaSettings): Promise<ModelPlazaSettings> {
  const { data } = await apiClient.put<ModelPlazaSettings>('/admin/settings/model-plaza', payload)
  return data
}

export async function listModels(): Promise<ModelIdentity[]> {
  const { data } = await apiClient.get<ModelIdentity[]>('/admin/model-plaza/models')
  return data
}

export const adminModelPlazaAPI = { getSettings, updateSettings, listModels }

export default adminModelPlazaAPI
