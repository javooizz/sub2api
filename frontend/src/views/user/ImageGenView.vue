<script setup lang="ts">
/**
 * OneBoolFlow iframe 嵌入页
 *
 * 通过 postMessage 把当前用户的 default API key + sub2api 的 api_base_url
 * 推送给 iframe(onebool-flow embedded 模式)。
 *
 * ⚠ 协议规范单一真相源:
 *   ../../../../../onebool-flow/docs/integration-protocol.md
 *   (FlowConfig 字段 / postMessage 消息流 / 安全要点 / 协议变更 checklist)
 * 改本文件 postMessage 相关代码前,先读上述文档。
 *
 * onebool-flow 部署位置:
 * - Dev:http://localhost:5173 (本地 pnpm dev)
 * - Prod:https://image.sub2api.com (按 onebool-flow/docs/deploy.md 部署)
 */

import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import { keysAPI } from '@/api/keys'
import { authAPI } from '@/api/auth'
import type { ApiKey } from '@/types'

const { t } = useI18n()

// 环境变量配置:onebool-flow 部署地址
// 生产环境改成 https://image.sub2api.com
const ONEBOOL_ORIGIN = import.meta.env.VITE_ONEBOOL_ORIGIN ?? 'http://localhost:5173'

const iframeRef = ref<HTMLIFrameElement | null>(null)
const loading = ref(true)
const errorMsg = ref<string | null>(null)
const apiKey = ref<ApiKey | null>(null)
const apiBaseUrl = ref<string>('')

const iframeSrc = computed(() => `${ONEBOOL_ORIGIN}/?embedded=1`)

// 拉当前用户的 API key 与 sub2api 的 base url
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
      errorMsg.value = t('imageGen.noKey')
      return
    }
    apiKey.value = activeKeys[0]
    // 优先用后端配置的 api_base_url,否则回退到 sub2api 自身 origin
    // (sub2api 本身就是 OpenAI 兼容反代,可以直接调自己)
    apiBaseUrl.value = publicSettings?.api_base_url || window.location.origin
  } catch (e) {
    console.error('[ImageGen] load credentials failed', e)
    errorMsg.value = e instanceof Error ? e.message : String(e)
  } finally {
    loading.value = false
  }
}

function pushConfigToIframe(): void {
  if (!iframeRef.value?.contentWindow) return
  if (!apiKey.value || !apiBaseUrl.value) return
  const baseUrl = `${apiBaseUrl.value.replace(/\/+$/, '')}/v1`
  // dev: iframe 直接跨域调 sub2api(需后端开 CORS)
  // prod: 若 iframe 与 sub2api 同域(image.sub2api.com → sub2api 后端反代),apiProxy=false 即可
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
    ONEBOOL_ORIGIN
  )
}

function onMessage(event: MessageEvent): void {
  // 严格 origin 校验
  if (event.origin !== ONEBOOL_ORIGIN) return
  const data = event.data
  if (!data || typeof data !== 'object' || typeof data.type !== 'string') return

  if (data.type === 'flow.ready') {
    pushConfigToIframe()
  } else if (data.type === 'flow.requestKey') {
    // iframe 主动请求(比如 key 过期),重新拉一次再推送
    void loadCredentials().then(() => {
      if (!iframeRef.value?.contentWindow || !apiKey.value) return
      iframeRef.value.contentWindow.postMessage(
        {
          type: 'flow.keyResolved',
          requestId: data.requestId,
          key: apiKey.value.key,
        },
        ONEBOOL_ORIGIN
      )
    })
  }
}

onMounted(() => {
  window.addEventListener('message', onMessage)
  void loadCredentials()
})

onUnmounted(() => {
  window.removeEventListener('message', onMessage)
})
</script>

<template>
  <AppLayout>
    <div class="flex flex-col h-full">
      <!-- 顶部状态条 -->
      <div
        v-if="loading || errorMsg"
        class="flex items-center gap-3 px-4 py-2 text-sm border-b border-gray-200 dark:border-dark-700"
        :class="errorMsg ? 'bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-300' : 'bg-gray-50 dark:bg-dark-800 text-gray-600 dark:text-gray-300'"
      >
        <template v-if="loading">
          <span
            class="inline-block h-3 w-3 animate-spin rounded-full border-2 border-current border-r-transparent"
          />
          <span>{{ t('imageGen.loading') }}</span>
        </template>
        <template v-else>
          <span>⚠️ {{ errorMsg }}</span>
          <button
            class="ml-auto btn btn-secondary btn-sm"
            @click="loadCredentials"
          >
            {{ t('common.retry') }}
          </button>
        </template>
      </div>

      <!-- iframe -->
      <iframe
        ref="iframeRef"
        :src="iframeSrc"
        class="flex-1 w-full border-0 bg-white dark:bg-dark-900"
        allow="clipboard-write"
        :title="t('imageGen.title')"
      />
    </div>
  </AppLayout>
</template>

<style scoped>
/* 让 AppLayout 内容区铺满高度,iframe 占满主区 */
:deep(.app-layout main) {
  padding: 0;
  height: calc(100vh - 64px); /* 减去 sub2api 顶部 header 高度 */
}
</style>
