/**
 * Admin Upstream Providers API endpoints
 * Handles upstream provider management (balance/price monitoring, tokens, events)
 */

import { apiClient } from '../client'

// ---- 类型(与后端 DTO 对齐) ----

export interface UpstreamGroupSnapshot {
  name: string
  ratio?: number
  description?: string
  models?: string[]
}

export interface UpstreamSnapshot {
  fetched_at: string
  balance?: number
  currency?: string
  user_info?: { id?: number; username?: string }
  groups?: UpstreamGroupSnapshot[]
  model_pricing?: Record<string, unknown>
  partial: boolean
}

export interface UpstreamCredentialStatus {
  has_password: boolean
  has_access_token: boolean
  access_token_tail?: string
}

export interface UpstreamProvider {
  id: number
  name: string
  type: 'sub2api' | 'newapi'
  site_url: string
  api_base_url: string
  effective_api_base_url: string
  status: 'active' | 'credential_error' | 'unreachable' | 'disabled'
  credentials: Record<string, unknown> // 已脱敏
  credential_status: UpstreamCredentialStatus
  balance_threshold: number | null
  notify_on_price_change: boolean
  refresh_interval_minutes: number
  recharge_ratio: number
  usage_summary?: UsageSummary | null
  latest_snapshot?: UpstreamSnapshot | null
  last_refreshed_at: string | null
  last_error: string
  consecutive_failures: number
  remark: string
  created_at: string
}

export interface UpstreamProviderInput {
  name: string
  type: 'sub2api' | 'newapi'
  site_url: string
  api_base_url?: string
  credentials?: Record<string, unknown>
  balance_threshold?: number | null
  notify_on_price_change?: boolean
  refresh_interval_minutes?: number
  recharge_ratio?: number
  remark?: string
}

// R2.4: 事件 DTO snake_case
export interface UpstreamChangeEvent {
  id: number
  provider_id: number
  type: string
  summary: string
  detail?: Record<string, unknown>
  notified: boolean
  created_at: string
}

export interface UpstreamLinkedAccount {
  id: number
  name: string
  platform: string
  status: string
  base_url: string
}

export interface UpstreamToken {
  id?: unknown
  name: string
  key?: string
  group?: string
  expires_at?: string
  raw?: Record<string, unknown>
}

export interface UpstreamManagementSettings {
  browser_cdp_url: string
  proxy_url: string
  allow_private_webhook: boolean
}

export interface UsageWindowStat {
  cost_usd: number
  cost_cny: number
  requests: number
}
export interface UsageSummary {
  today: UsageWindowStat
  week: UsageWindowStat
  month: UsageWindowStat
  total: UsageWindowStat
  backfilled_from?: string | null
  partial: boolean
  partial_reason?: string
}
export interface UsageBreakdownRow {
  scope_key: string
  scope_name: string
  deleted: boolean
  cost_usd: number
  cost_cny: number
  requests: number
  tokens: number
}
export interface UsageBreakdown {
  scope: 'key' | 'group' | 'model'
  window: string
  supported: boolean
  items: UsageBreakdownRow[]
}

// ---- API (R3.1 解包写法: const {data} = await ...; return data) ----

export async function list(): Promise<UpstreamProvider[]> {
  const { data } = await apiClient.get<UpstreamProvider[]>('/admin/upstream-providers')
  return data
}

export async function getById(id: number): Promise<UpstreamProvider> {
  const { data } = await apiClient.get<UpstreamProvider>(`/admin/upstream-providers/${id}`)
  return data
}

export async function create(input: UpstreamProviderInput): Promise<UpstreamProvider> {
  const { data } = await apiClient.post<UpstreamProvider>('/admin/upstream-providers', input)
  return data
}

export async function update(id: number, input: UpstreamProviderInput): Promise<UpstreamProvider> {
  const { data } = await apiClient.put<UpstreamProvider>(`/admin/upstream-providers/${id}`, input)
  return data
}

export async function remove(id: number): Promise<{ deleted: boolean }> {
  const { data } = await apiClient.delete<{ deleted: boolean }>(`/admin/upstream-providers/${id}`)
  return data
}

export async function refresh(id: number): Promise<UpstreamProvider> {
  const { data } = await apiClient.post<UpstreamProvider>(`/admin/upstream-providers/${id}/refresh`)
  return data
}

export async function relogin(id: number): Promise<UpstreamProvider> {
  const { data } = await apiClient.post<UpstreamProvider>(`/admin/upstream-providers/${id}/relogin`)
  return data
}

export async function testConnection(
  input: UpstreamProviderInput & { provider_id?: number }
): Promise<UpstreamSnapshot> {
  const { data } = await apiClient.post<UpstreamSnapshot>('/admin/upstream-providers/test', input)
  return data
}

export async function linkedAccounts(id: number): Promise<UpstreamLinkedAccount[]> {
  const { data } = await apiClient.get<UpstreamLinkedAccount[]>(
    `/admin/upstream-providers/${id}/linked-accounts`
  )
  return data
}

export async function listTokens(id: number): Promise<UpstreamToken[]> {
  const { data } = await apiClient.get<UpstreamToken[]>(`/admin/upstream-providers/${id}/tokens`)
  return data
}

export async function createToken(
  id: number,
  input: { name: string; group?: string }
): Promise<{ token: UpstreamToken; api_base_url: string }> {
  const { data } = await apiClient.post<{ token: UpstreamToken; api_base_url: string }>(
    `/admin/upstream-providers/${id}/tokens`,
    input
  )
  return data
}

// R2.4: 游标参数 before_created_at/before_id
export async function listEvents(
  id: number,
  params?: { limit?: number; before_created_at?: string; before_id?: number }
): Promise<UpstreamChangeEvent[]> {
  const { data } = await apiClient.get<UpstreamChangeEvent[]>(
    `/admin/upstream-providers/${id}/events`,
    { params }
  )
  return data
}

// R2.2: 诊断截图鉴权 — 返回 Blob (admin API 需 Authorization 头，不能直链 <a href>)
export async function fetchDiagnostics(id: number, file: string): Promise<Blob> {
  const { data } = await apiClient.get<Blob>(
    `/admin/upstream-providers/${id}/diagnostics/${file}`,
    { responseType: 'blob' }
  )
  return data
}

export async function getSettings(): Promise<UpstreamManagementSettings> {
  const { data } = await apiClient.get<UpstreamManagementSettings>(
    '/admin/settings/upstream-management'
  )
  return data
}

export async function updateSettings(
  input: UpstreamManagementSettings
): Promise<UpstreamManagementSettings> {
  const { data } = await apiClient.put<UpstreamManagementSettings>(
    '/admin/settings/upstream-management',
    input
  )
  return data
}

export async function usage(
  id: number,
  params: { scope: 'key' | 'group' | 'model'; window?: 'today' | 'week' | 'month' | 'total' },
): Promise<UsageBreakdown> {
  const { data } = await apiClient.get<UsageBreakdown>(`/admin/upstream-providers/${id}/usage`, { params })
  return data
}

const upstreamProvidersAPI = {
  list,
  getById,
  create,
  update,
  remove,
  refresh,
  relogin,
  testConnection,
  linkedAccounts,
  listTokens,
  createToken,
  listEvents,
  fetchDiagnostics,
  getSettings,
  updateSettings,
  usage,
}

export { upstreamProvidersAPI }
export default upstreamProvidersAPI
