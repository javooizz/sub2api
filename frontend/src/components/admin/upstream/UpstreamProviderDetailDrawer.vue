<template>
  <!-- 占位:Task 19 替换为完整抽屉实现 -->
  <div v-if="false" />
</template>

<script setup lang="ts">
import type { UpstreamProvider } from '@/api/admin/upstreamProviders'

// props/emits 声明确保父组件绑定不报 TS 错
defineProps<{
  provider?: UpstreamProvider | null
}>()

defineEmits<{
  close: []
  changed: []
}>()
</script>
