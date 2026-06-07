<template>
  <!-- 占位:Task 20 替换为上游管理设置对话框实现 -->
  <div v-if="false" />
</template>

<script setup lang="ts">
// props/emits 声明确保父组件绑定不报 TS 错
defineProps<{
  show: boolean
}>()

defineEmits<{
  close: []
  saved: []
}>()
</script>
