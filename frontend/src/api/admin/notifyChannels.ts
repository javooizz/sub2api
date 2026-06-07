/**
 * Admin Notify Channels API endpoints
 * Handles notification channel management for upstream provider events
 */

import { apiClient } from '../client'

// ---- 类型 ----

export interface NotifyChannel {
  id: number
  name: string
  type: 'email' | 'webhook'
  scope: string
  enabled: boolean
  events: string[]
  config: Record<string, unknown> // webhook headers 值已脱敏为 ***
  last_sent_at: string | null
  last_error: string
}

export interface NotifyChannelInput {
  name: string
  type: 'email' | 'webhook'
  scope: string
  enabled?: boolean
  events?: string[]
  config: Record<string, unknown>
}

// ---- API (R3.1 解包写法: const {data} = await ...; return data) ----

export async function list(scope = 'upstream'): Promise<NotifyChannel[]> {
  const { data } = await apiClient.get<NotifyChannel[]>('/admin/notify-channels', {
    params: { scope }
  })
  return data
}

export async function create(input: NotifyChannelInput): Promise<NotifyChannel> {
  const { data } = await apiClient.post<NotifyChannel>('/admin/notify-channels', input)
  return data
}

export async function update(id: number, input: NotifyChannelInput): Promise<NotifyChannel> {
  const { data } = await apiClient.put<NotifyChannel>(`/admin/notify-channels/${id}`, input)
  return data
}

export async function remove(id: number): Promise<{ deleted: boolean }> {
  const { data } = await apiClient.delete<{ deleted: boolean }>(`/admin/notify-channels/${id}`)
  return data
}

export async function test(id: number): Promise<{ sent: boolean }> {
  const { data } = await apiClient.post<{ sent: boolean }>(`/admin/notify-channels/${id}/test`)
  return data
}

const notifyChannelsAPI = { list, create, update, remove, test }

export { notifyChannelsAPI }
export default notifyChannelsAPI
