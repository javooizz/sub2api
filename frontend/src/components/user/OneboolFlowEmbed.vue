<script setup lang="ts">
/**
 * OneBoolFlow iframe 嵌入通用组件
 *
 * 把当前用户的 default API key + sub2api 的 api_base_url 通过 postMessage 推送给
 * onebool-flow iframe，并以 props.agent 在 URL 上指明要打开的智能体（image-gen / chat）。
 *
 * ⚠ 协议规范单一真相源：
 *   ../../../../../onebool-flow/docs/integration-protocol.md
 *   （FlowConfig 字段 / postMessage 消息流 / 安全要点 / 协议变更 checklist）
 * 改本文件 postMessage 相关代码前，先读上述文档。
 */

import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { keysAPI } from '@/api/keys'
import { authAPI } from '@/api/auth'
import { extensionConfigAPI } from '@/api/admin/extensionConfig'
import type { ApiKey } from '@/types'

interface Props {
  /** onebool-flow 智能体标识，通过 ?agent= 传给 iframe（image-gen / chat / ...） */
  agent: string
  /** iframe accessibility title */
  title: string
  /** 加载中文案 */
  loadingText: string
  /** 没有可用 key 时的文案 */
  noKeyText: string
}

const props = defineProps<Props>()
const { t } = useI18n()

// onebool-flow 部署地址的 fallback 链：工作台配置 → env 变量 → localhost dev 默认
const FALLBACK_ONEBOOL_ORIGIN =
  (import.meta.env.VITE_ONEBOOL_ORIGIN as string | undefined) ?? 'http://localhost:5173'

const iframeRef = ref<HTMLIFrameElement | null>(null)
const loading = ref(true)
const errorMsg = ref<string | null>(null)
const apiKey = ref<ApiKey | null>(null)
const apiBaseUrl = ref<string>('')
/** 来自工作台的 onebool-flow 部署地址（覆盖 FALLBACK） */
const configuredOneboolOrigin = ref<string>('')
/** 等 ext config 拉完再渲染 iframe，避免 src 中途变化触发 reload */
const iframeReady = ref(false)

const oneboolOrigin = computed(() => configuredOneboolOrigin.value || FALLBACK_ONEBOOL_ORIGIN)

const iframeSrc = computed(() => {
  const url = new URL(oneboolOrigin.value)
  url.searchParams.set('embedded', '1')
  url.searchParams.set('agent', props.agent)
  // parent_origin 自动用本站 origin，传给 iframe 做严格 postMessage 校验
  if (typeof window !== 'undefined') {
    url.searchParams.set('parent_origin', window.location.origin)
  }
  return url.toString()
})

async function loadExtensionConfig(): Promise<void> {
  try {
    const cfg = await extensionConfigAPI.getForUser(props.agent)
    configuredOneboolOrigin.value = cfg.onebool_origin || ''
  } catch (e) {
    // ext config 不存在或拉取失败时静默回退到 env / localhost
    console.warn('[OneboolFlowEmbed] load extension config failed; using fallback origin', e)
    configuredOneboolOrigin.value = ''
  } finally {
    iframeReady.value = true
  }
}

async function loadCredentials(): Promise<void> {
  loading.value = true
  errorMsg.value = null
  try {
    const [keysResp, publicSettings] = await Promise.all([
      keysAPI.list(1, 50, { status: 'active', sort_by: 'created_at', sort_order: 'desc' }),
      authAPI.getPublicSettings(),
    ])
    const activeKeys = (keysResp.items ?? []).filter((k: ApiKey) => k.status === 'active')
    if (activeKeys.length === 0) {
      errorMsg.value = props.noKeyText
      return
    }
    apiKey.value = activeKeys[0]
    // 优先用后端配置的 api_base_url，否则回退到 sub2api 自身 origin
    // （sub2api 本身就是 OpenAI 兼容反代，可以直接调自己）
    apiBaseUrl.value = publicSettings?.api_base_url || window.location.origin
  } catch (e) {
    console.error('[OneboolFlowEmbed] load credentials failed', e)
    errorMsg.value = e instanceof Error ? e.message : String(e)
  } finally {
    loading.value = false
  }
}

function pushConfigToIframe(): void {
  if (!iframeRef.value?.contentWindow) return
  if (!apiKey.value || !apiBaseUrl.value) return
  const baseUrl = `${apiBaseUrl.value.replace(/\/+$/, '')}/v1`
  iframeRef.value.contentWindow.postMessage(
    {
      type: 'flow.config',
      payload: {
        apiKey: apiKey.value.key,
        baseUrl,
        provider: 'openai',
        profileId: `sub2api-key-${apiKey.value.id}`,
        apiProxy: false,
      },
    },
    oneboolOrigin.value
  )
}

function onMessage(event: MessageEvent): void {
  // 严格 origin 校验：必须来自工作台配置的 onebool-flow 部署 origin
  if (event.origin !== oneboolOrigin.value) return
  const data = event.data
  if (!data || typeof data !== 'object' || typeof data.type !== 'string') return

  if (data.type === 'flow.ready') {
    pushConfigToIframe()
  } else if (data.type === 'flow.requestKey') {
    void loadCredentials().then(() => {
      if (!iframeRef.value?.contentWindow || !apiKey.value) return
      iframeRef.value.contentWindow.postMessage(
        {
          type: 'flow.keyResolved',
          requestId: data.requestId,
          key: apiKey.value.key,
        },
        oneboolOrigin.value
      )
    })
  } else if (data.type === 'flow.requestWorkbenchConfig') {
    // 扩展配置协议 v1.1+：iframe 请求过滤后的可见配置
    void extensionConfigAPI
      .getForUser(String(data.agentId ?? props.agent))
      .then((cfg) => {
        iframeRef.value?.contentWindow?.postMessage(
          { type: 'flow.workbenchConfig', requestId: data.requestId, config: cfg },
          oneboolOrigin.value
        )
      })
      .catch((e: unknown) => {
        iframeRef.value?.contentWindow?.postMessage(
          {
            type: 'flow.workbenchConfig',
            requestId: data.requestId,
            config: null,
            error: e instanceof Error ? e.message : String(e),
          },
          oneboolOrigin.value
        )
      })
  } else if (data.type === 'flow.requestKeyForGroup') {
    // 扩展配置协议 v1.1+：iframe 选分组 → parent ensure-key
    const agentId = String(data.agentId ?? props.agent)
    const groupId = Number(data.groupId)
    const endpointName: string | undefined =
      typeof data.endpointName === 'string' ? data.endpointName : undefined
    void extensionConfigAPI
      .ensureKey(agentId, groupId, endpointName)
      .then((r) => {
        iframeRef.value?.contentWindow?.postMessage(
          {
            type: 'flow.keyForGroupResolved',
            requestId: data.requestId,
            groupId,
            apiKey: r.api_key,
            baseUrl: r.base_url,
            groupName: r.group_name,
            endpointName: r.endpoint_name,
            created: r.created,
          },
          oneboolOrigin.value
        )
      })
      .catch((e: unknown) => {
        iframeRef.value?.contentWindow?.postMessage(
          {
            type: 'flow.keyForGroupResolved',
            requestId: data.requestId,
            groupId,
            apiKey: null,
            error: e instanceof Error ? e.message : String(e),
          },
          oneboolOrigin.value
        )
      })
  }
}

onMounted(() => {
  window.addEventListener('message', onMessage)
  // 串行：先拉 ext config 拿 parent_origin → 渲染 iframe → 拉 credentials
  // 两次串行 < 300ms，期间顶部 loading banner 显示
  void loadExtensionConfig()
  void loadCredentials()
})

onUnmounted(() => {
  window.removeEventListener('message', onMessage)
})
</script>

<template>
  <div class="flex flex-col h-full">
    <!-- 顶部状态条 -->
    <div
      v-if="loading || errorMsg"
      class="flex items-center gap-3 px-4 py-2 text-sm border-b border-gray-200 dark:border-dark-700"
      :class="
        errorMsg
          ? 'bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-300'
          : 'bg-gray-50 dark:bg-dark-800 text-gray-600 dark:text-gray-300'
      "
    >
      <template v-if="loading">
        <span
          class="inline-block h-3 w-3 animate-spin rounded-full border-2 border-current border-r-transparent"
        />
        <span>{{ loadingText }}</span>
      </template>
      <template v-else>
        <span>⚠️ {{ errorMsg }}</span>
        <button class="ml-auto btn btn-secondary btn-sm" @click="loadCredentials">
          {{ t('common.retry') }}
        </button>
      </template>
    </div>

    <!-- iframe — 等 parent_origin 加载完再挂载，避免 src 变化触发 reload -->
    <iframe
      v-if="iframeReady"
      ref="iframeRef"
      :src="iframeSrc"
      class="flex-1 w-full border-0 bg-white dark:bg-dark-900"
      allow="clipboard-write"
      :title="title"
    />
  </div>
</template>

