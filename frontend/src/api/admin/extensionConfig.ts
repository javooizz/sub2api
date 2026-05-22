/**
 * Extension Configs API
 * 配套 onebool-flow agents 工作台（管理员侧 CRUD + 用户侧 ensure-key）
 * 协议参考：onebool-flow/docs/integration-protocol.md §9
 *
 * 响应封装：apiClient 已通过 interceptor unwrap 服务端 {code, message, data} 信封，
 * 所以 apiClient.get<T>() 直接返回 T。
 */

import { apiClient } from '../client'

export interface ImageGenConfig {
  enabled_endpoint_names: string[]
  default_endpoint_name: string
  enabled_group_ids: number[]
  /** key 是字符串化 group_id（json 不允许 int key） */
  group_models: Record<string, string[]>
}

export interface ExtensionConfigPayload {
  version?: number
  /**
   * onebool-flow 部署 origin（如 https://image.sub2api.com 或 http://localhost:5173）。
   * sub2api 用它构造 iframe src + postMessage target origin。
   * 空串 → 前端 fallback 到 VITE_ONEBOOL_ORIGIN env / 'http://localhost:5173'。
   */
  onebool_origin?: string
  image_gen?: ImageGenConfig
}

export interface ExtensionConfigRecord {
  agent_id: string
  payload: ExtensionConfigPayload
  updated_by: number | null
  created_at: string
  updated_at: string
}

export interface UserExtensionConfig {
  agent_id: string
  onebool_origin: string
  image_gen: null | {
    endpoints: Array<{ name: string; endpoint: string }>
    default_endpoint_name: string | null
    groups: Array<{ id: number; name: string; description?: string; models: string[] }>
  }
}

export interface EnsureKeyResponse {
  api_key: string
  key_id: number
  base_url: string
  group_id: number
  group_name: string
  created: boolean
  endpoint_name: string | null
}

export const extensionConfigAPI = {
  async getAdmin(agentId: string): Promise<ExtensionConfigRecord> {
    const { data } = await apiClient.get<ExtensionConfigRecord>(
      `/admin/extension-configs/${encodeURIComponent(agentId)}`,
    )
    return data
  },

  async upsertAdmin(
    agentId: string,
    payload: ExtensionConfigPayload,
  ): Promise<ExtensionConfigRecord> {
    const { data } = await apiClient.put<ExtensionConfigRecord>(
      `/admin/extension-configs/${encodeURIComponent(agentId)}`,
      { payload },
    )
    return data
  },

  async deleteAdmin(agentId: string): Promise<void> {
    await apiClient.delete(`/admin/extension-configs/${encodeURIComponent(agentId)}`)
  },

  async getForUser(agentId: string): Promise<UserExtensionConfig> {
    const { data } = await apiClient.get<UserExtensionConfig>(
      `/extension-configs/${encodeURIComponent(agentId)}`,
    )
    return data
  },

  /** 用户侧：查/建 user-scoped key（带 Idempotency-Key 头自动幂等） */
  async ensureKey(
    agentId: string,
    groupId: number,
    endpointName?: string,
  ): Promise<EnsureKeyResponse> {
    const idempotencyKey =
      typeof crypto !== 'undefined' && 'randomUUID' in crypto
        ? crypto.randomUUID()
        : `${Date.now()}-${Math.random().toString(36).slice(2)}`
    const { data } = await apiClient.post<EnsureKeyResponse>(
      `/extension-configs/${encodeURIComponent(agentId)}/ensure-key`,
      { group_id: groupId, endpoint_name: endpointName },
      { headers: { 'Idempotency-Key': idempotencyKey } },
    )
    return data
  },
}

export default extensionConfigAPI
