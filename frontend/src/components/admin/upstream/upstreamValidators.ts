// 表单校验纯函数:返回 { 字段: i18n 错误 key 尾段 },空对象 = 通过。
// 错误文案 key 前缀 admin.upstream.form.errors.

export interface ProviderFormShape {
  name: string
  type: 'sub2api' | 'newapi'
  site_url: string
  api_base_url: string
  username: string
  password: string
  access_token: string
  refresh_interval_minutes: number
  recharge_ratio: number
}

export type ProviderFormErrors = Partial<
  Record<'name' | 'site_url' | 'api_base_url' | 'credentials' | 'refresh_interval_minutes' | 'recharge_ratio', string>
>

export function isValidHttpURL(value: string): boolean {
  if (!value) return false
  try {
    const u = new URL(value)
    return (u.protocol === 'http:' || u.protocol === 'https:') && !!u.host
  } catch {
    return false
  }
}

/**
 * isEdit=true 时凭证允许全空(= 不修改,后端按敏感键合并保留,spec §9)。
 */
export function validateProviderForm(form: ProviderFormShape, isEdit: boolean): ProviderFormErrors {
  const errors: ProviderFormErrors = {}
  if (!form.name.trim()) errors.name = 'nameRequired'
  if (!isValidHttpURL(form.site_url)) errors.site_url = 'siteUrlInvalid'
  if (form.api_base_url.trim() && !isValidHttpURL(form.api_base_url)) {
    errors.api_base_url = 'apiBaseUrlInvalid'
  }
  const hasPasswordPair = !!form.username.trim() && !!form.password.trim()
  const hasToken = !!form.access_token.trim()
  if (!isEdit && !hasPasswordPair && !hasToken) errors.credentials = 'credentialsRequired'
  if (form.refresh_interval_minutes < 5 || form.refresh_interval_minutes > 1440) {
    errors.refresh_interval_minutes = 'intervalRange'
  }
  if (!(form.recharge_ratio > 0)) {
    errors.recharge_ratio = 'rechargeRatioPositive'
  }
  return errors
}
