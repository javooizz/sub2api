<script setup lang="ts">
/**
 * OneBoolFlow iframe 嵌入页 - 出图助手
 *
 * 通过 postMessage 把当前用户的 default API key + sub2api 的 api_base_url
 * 推送给 iframe（onebool-flow embedded 模式 + agent=image-gen）。
 *
 * ⚠ 协议规范单一真相源：
 *   ../../../../../onebool-flow/docs/integration-protocol.md
 *
 * onebool-flow 部署位置：
 * - Dev: http://localhost:5173（本地 pnpm dev）
 * - Prod: https://image.sub2api.com（按 onebool-flow/docs/deploy.md 部署）
 */

import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import OneboolFlowEmbed from '@/components/user/OneboolFlowEmbed.vue'

const { t } = useI18n()
</script>

<template>
  <AppLayout>
    <OneboolFlowEmbed
      agent="image-gen"
      :title="t('imageGen.title')"
      :loading-text="t('imageGen.loading')"
      :no-key-text="t('imageGen.noKey')"
    />
  </AppLayout>
</template>

<style scoped>
/* 让 AppLayout 的 <main> 撑满视口减 header(h-16 = 64px),并去掉默认 padding,
   这样内层 OneboolFlowEmbed 的 flex-1 iframe 可以铺满。*/
:deep(main) {
  padding: 0 !important;
  height: calc(100vh - 64px);
}
</style>
