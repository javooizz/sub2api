import { describe, it, expect } from 'vitest'
import zh from '../locales/zh'
import en from '../locales/en'

/**
 * 回归防护:vue-i18n 把 '@' 视为 linked-message 语法(`@:key`)。
 *
 * 当消息值里出现【裸 '@'】(例如代理 URL 占位符 `user:pass@host`),在生产构建开启
 * JIT 编译(__INTLIFY_JIT_COMPILATION__)时,该消息会在【首次渲染、即组件用到它时】
 * 才编译;裸 '@' 触发非法 linked-message 解析,使消息编译失败,进而中断【正在渲染该
 * 消息的组件】的渲染。
 *
 * 真实事故:`admin.upstream.settings.proxyUrlPlaceholder` 的裸 '@' 导致「采集设置」
 * 弹窗(UpstreamSettingsDialog → BaseDialog 的 Teleport)整条渲染被打断,点击按钮无反应。
 * 仅在生产/JIT 构建复现,dev/vitest 不复现,故必须用【静态扫描】兜底。
 *
 * 约定:字面 '@' 必须转义为 `{'@'}`(参见 zh.ts 中 proxies 批量导入提示的正确写法)。
 */

type AnyMsg = Record<string, unknown>

function collectBareAt(obj: AnyMsg, path: string[], out: string[]): void {
  for (const [k, v] of Object.entries(obj)) {
    const p = [...path, k]
    if (typeof v === 'string') {
      // 去掉合法转义 {'@'} 之后若仍含 '@',即为未转义的裸 @
      const stripped = v.replace(/\{'@'\}/g, '')
      if (stripped.includes('@')) {
        out.push(`${p.join('.')} = ${JSON.stringify(v)}`)
      }
    } else if (v && typeof v === 'object') {
      collectBareAt(v as AnyMsg, p, out)
    }
  }
}

describe("i18n 消息安全:禁止裸 '@'(vue-i18n linked-message 语法)", () => {
  for (const [name, loc] of [['zh', zh], ['en', en]] as const) {
    it(`${name}: 不存在未转义的裸 '@'(字面 @ 必须写成 {'@'})`, () => {
      const offenders: string[] = []
      collectBareAt(loc as AnyMsg, [], offenders)
      expect(
        offenders,
        `发现未转义的裸 '@'(会在生产 JIT 编译时打断组件渲染),请改写为 {'@'}:\n${offenders.join('\n')}`,
      ).toEqual([])
    })
  }
})
