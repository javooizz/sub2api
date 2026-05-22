<script setup lang="ts">
/**
 * OneBoolFlow iframe 嵌入页 - 聊天助手
 *
 * 与 ImageGenView 共用 OneboolFlowEmbed 组件，差异只在 agent 参数与标题。
 *
 * ⚠ 协议规范单一真相源：
 *   ../../../../../onebool-flow/docs/integration-protocol.md
 */

import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import OneboolFlowEmbed from '@/components/user/OneboolFlowEmbed.vue'

const { t } = useI18n()
</script>

<template>
  <AppLayout>
    <OneboolFlowEmbed
      agent="chat"
      :title="t('chat.title')"
      :loading-text="t('chat.loading')"
      :no-key-text="t('chat.noKey')"
    />
  </AppLayout>
</template>

<style scoped>
:deep(.app-layout main) {
  padding: 0;
  height: calc(100vh - 64px);
}
</style>
