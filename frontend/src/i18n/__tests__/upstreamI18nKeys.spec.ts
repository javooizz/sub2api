import { describe, it, expect } from 'vitest'
import zh from '../locales/zh'
import en from '../locales/en'

// 防回归:upstream 块必须在 admin 下,不在 payment 下
describe('upstream i18n keys', () => {
  for (const [name, loc] of [['zh', zh], ['en', en]] as const) {
    it(`${name}: admin.upstream 存在且 payment.upstream 不存在`, () => {
      expect((loc as any).admin?.upstream).toBeTypeOf('object')
      expect((loc as any).payment?.upstream).toBeUndefined()
    })
    it(`${name}: 关键 key 可解析`, () => {
      const u = (loc as any).admin.upstream
      expect(u.title).toBeTypeOf('string')
      expect(u.status.active).toBeTypeOf('string')
      expect(u.detail.tabs.overview).toBeTypeOf('string')
      expect(u.notify.title).toBeTypeOf('string')
      expect(u.settings.title).toBeTypeOf('string')
      expect(u.form.errors.nameRequired).toBeTypeOf('string')
    })
  }
})
