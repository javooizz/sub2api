export const FORK_APP_CONFIG = {
  apiName: '1Bool API',
  pill: '支持 Claude Code · CodeX · Gemini CLI',
  headline: ['让每一个想法，', '都配得上'] as const,
  headlineAccent: '最好的模型',
  subtitle: '一个 API，接通 Claude、GPT、Gemini 的全部能力',
  logos: ['Claude', 'OpenAI', 'Gemini'] as const,
} as const

export type ForkAppConfig = typeof FORK_APP_CONFIG
