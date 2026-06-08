<template>
  <BaseDialog :show="show" :title="t('admin.upstream.settings.title')" width="normal" @close="emit('close')">
    <form class="space-y-5" @submit.prevent="handleSave">
      <!-- CDP URL 输入 -->
      <div>
        <label for="cdp-url" class="input-label">{{ t('admin.upstream.settings.cdpUrl') }}</label>
        <input
          id="cdp-url"
          v-model="form.browser_cdp_url"
          type="text"
          :placeholder="t('admin.upstream.settings.cdpUrlPlaceholder')"
          class="input font-mono text-xs"
        />
        <p
          v-if="!form.browser_cdp_url"
          class="mt-1.5 text-xs text-amber-600 dark:text-amber-400"
        >
          {{ t('admin.upstream.settings.browserDisabledHint') }}
        </p>
      </div>

      <!-- 全局代理 URL(同时用于 HTTP 采集与 CloakBrowser 过盾) -->
      <div>
        <label for="proxy-url" class="input-label">{{ t('admin.upstream.settings.proxyUrl') }}</label>
        <input
          id="proxy-url"
          v-model="form.proxy_url"
          type="text"
          :placeholder="t('admin.upstream.settings.proxyUrlPlaceholder')"
          class="input font-mono text-xs"
        />
        <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.upstream.settings.proxyUrlHint') }}
        </p>
      </div>

      <!-- 私网 Webhook 开关 -->
      <div class="flex items-center gap-2 min-h-[44px]">
        <input
          id="allow-private"
          v-model="form.allow_private_webhook"
          type="checkbox"
          class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 cursor-pointer"
        />
        <label
          for="allow-private"
          class="cursor-pointer text-sm text-gray-700 dark:text-gray-300"
        >
          {{ t('admin.upstream.settings.allowPrivateWebhook') }}
        </label>
      </div>

      <!-- 错误提示 -->
      <div v-if="saveError" role="alert" class="rounded-lg bg-red-50 px-4 py-2.5 text-sm text-red-600 dark:bg-red-900/20 dark:text-red-400">
        {{ saveError }}
      </div>
    </form>

    <template #footer>
      <button
        type="button"
        class="btn btn-primary cursor-pointer"
        :disabled="saving"
        @click="handleSave"
      >
        {{ t('common.save') }}
      </button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { upstreamProvidersAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'

const props = defineProps<{ show: boolean }>()
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const appStore = useAppStore()

const saving = ref(false)
const saveError = ref('')
const form = ref({
  browser_cdp_url: '',
  proxy_url: '',
  allow_private_webhook: false,
})

watch(
  () => props.show,
  async (show) => {
    if (!show) return
    saveError.value = ''
    try {
      form.value = await upstreamProvidersAPI.getSettings()
    } catch {
      // 加载失败时使用默认值，不阻断打开
    }
  }
)

async function handleSave() {
  saving.value = true
  saveError.value = ''
  try {
    await upstreamProvidersAPI.updateSettings(form.value)
    appStore.showSuccess(t('admin.upstream.settings.saveSuccess'))
    emit('close')
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : String(err)
    saveError.value = msg || t('admin.upstream.settings.saveFailed')
    appStore.showError(saveError.value)
  } finally {
    saving.value = false
  }
}
</script>
