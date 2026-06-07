import { describe, it, expect } from 'vitest'
import { validateProviderForm, isValidHttpURL } from '../upstreamValidators'

describe('isValidHttpURL', () => {
  it('接受 http/https', () => {
    expect(isValidHttpURL('https://a.com')).toBe(true)
    expect(isValidHttpURL('http://a.com:8080/path')).toBe(true)
  })
  it('拒绝其他 scheme 与非法值', () => {
    expect(isValidHttpURL('ftp://a.com')).toBe(false)
    expect(isValidHttpURL('not a url')).toBe(false)
    expect(isValidHttpURL('')).toBe(false)
  })
})

describe('validateProviderForm', () => {
  const base = {
    name: 'demo', type: 'newapi' as const, site_url: 'https://a.com',
    api_base_url: '', username: '', password: '', access_token: 'tok',
    refresh_interval_minutes: 60,
  }
  it('合法表单通过', () => {
    expect(validateProviderForm(base, false)).toEqual({})
  })
  it('名称必填', () => {
    expect(validateProviderForm({ ...base, name: ' ' }, false).name).toBe('nameRequired')
  })
  it('官网地址校验', () => {
    expect(validateProviderForm({ ...base, site_url: 'x' }, false).site_url).toBe('siteUrlInvalid')
  })
  it('API 地址可空,但填了必须合法', () => {
    expect(validateProviderForm({ ...base, api_base_url: '' }, false).api_base_url).toBeUndefined()
    expect(validateProviderForm({ ...base, api_base_url: 'bad' }, false).api_base_url).toBe('apiBaseUrlInvalid')
  })
  it('创建时凭证至少一组', () => {
    const noCreds = { ...base, access_token: '', username: '', password: '' }
    expect(validateProviderForm(noCreds, false).credentials).toBe('credentialsRequired')
    // 编辑模式(isEdit=true)允许全空 = 不修改凭证
    expect(validateProviderForm(noCreds, true).credentials).toBeUndefined()
    // 账密成组才算
    expect(validateProviderForm({ ...noCreds, username: 'u' }, false).credentials).toBe('credentialsRequired')
    expect(validateProviderForm({ ...noCreds, username: 'u', password: 'p' }, false).credentials).toBeUndefined()
  })
  it('刷新间隔范围 5–1440', () => {
    expect(validateProviderForm({ ...base, refresh_interval_minutes: 1 }, false).refresh_interval_minutes).toBe('intervalRange')
    expect(validateProviderForm({ ...base, refresh_interval_minutes: 2000 }, false).refresh_interval_minutes).toBe('intervalRange')
  })
})
