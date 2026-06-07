<template>
  <BaseDialog :show="show" :title="t('admin.upstream.notify.title')" width="wide" @close="emit('close')">
    <div class="space-y-4">
      <!-- 渠道列表 -->
      <table v-if="channels.length && !editing" class="w-full text-sm">
        <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
          <tr
            v-for="ch in channels"
            :key="ch.id"
            class="hover:bg-gray-50 dark:hover:bg-dark-800/50 transition-colors duration-150"
          >
            <td class="py-2.5 pr-4 font-medium text-gray-900 dark:text-gray-100">{{ ch.name }}</td>
            <td class="py-2.5 pr-4 text-gray-500 dark:text-dark-400">{{ t(`admin.upstream.notify.${ch.type}`) }}</td>
            <td class="py-2.5 pr-4">
              <span
                :class="[
                  'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium',
                  ch.enabled
                    ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
                    : 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-dark-400',
                ]"
              >
                {{ ch.enabled ? t('common.enabled') : t('common.disabled') }}
              </span>
            </td>
            <td
              class="py-2.5 pr-4 max-w-[200px] truncate text-xs text-red-500 dark:text-red-400"
              :title="ch.last_error"
            >
              {{ ch.last_error }}
            </td>
            <td class="py-2.5">
              <div class="flex items-center gap-1">
                <button
                  type="button"
                  class="rounded-lg p-1.5 text-gray-400 transition-colors duration-150 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-dark-700 dark:hover:text-dark-300 cursor-pointer"
                  :title="t('admin.upstream.notify.test')"
                  @click="handleTest(ch)"
                >
                  <Icon name="mail" size="sm" />
                </button>
                <button
                  type="button"
                  class="rounded-lg p-1.5 text-gray-400 transition-colors duration-150 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-dark-700 dark:hover:text-dark-300 cursor-pointer"
                  :title="t('common.edit')"
                  @click="startEdit(ch)"
                >
                  <Icon name="edit" size="sm" />
                </button>
                <button
                  type="button"
                  class="rounded-lg p-1.5 text-red-400 transition-colors duration-150 hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400 cursor-pointer"
                  :title="t('common.delete')"
                  @click="handleDelete(ch)"
                >
                  <Icon name="trash" size="sm" />
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
      <p v-else-if="!editing" class="py-8 text-center text-sm text-gray-400 dark:text-dark-500">—</p>

      <!-- 编辑/创建表单 -->
      <form v-if="editing" class="space-y-4" @submit.prevent="handleSave">
        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="input-label">{{ t('common.name') }}</label>
            <input
              v-model="form.name"
              type="text"
              required
              class="input"
            />
          </div>
          <div>
            <label class="input-label">{{ t('admin.upstream.notify.type') }}</label>
            <Select
              v-model="form.type"
              :options="typeOptions"
              :disabled="!!editing.id"
            />
          </div>
        </div>

        <!-- Email 配置 -->
        <div v-if="form.type === 'email'">
          <label class="input-label">{{ t('admin.upstream.notify.recipients') }}</label>
          <input
            v-model="form.recipients"
            type="text"
            placeholder="ops@example.com, dev@example.com"
            class="input"
          />
        </div>

        <!-- Webhook 配置 -->
        <template v-else>
          <div>
            <label class="input-label">{{ t('admin.upstream.notify.webhookUrl') }}</label>
            <input
              v-model="form.url"
              type="text"
              placeholder="https://oapi.dingtalk.com/robot/send?access_token=..."
              class="input"
            />
          </div>
          <div>
            <label class="input-label">{{ t('admin.upstream.notify.headers') }}</label>
            <textarea
              v-model="form.headers"
              rows="2"
              placeholder='{"Authorization": "Bearer xxx"}'
              class="input font-mono text-xs resize-none"
            />
          </div>
          <div>
            <label class="input-label">{{ t('admin.upstream.notify.bodyTemplate') }}</label>
            <textarea
              v-model="form.bodyTemplate"
              rows="3"
              :placeholder="bodyTemplatePlaceholder"
              class="input font-mono text-xs resize-none"
            />
          </div>
        </template>

        <!-- 订阅事件 -->
        <div>
          <label class="input-label">{{ t('admin.upstream.notify.events') }}</label>
          <div class="flex flex-wrap gap-3 mt-1">
            <label
              v-for="ev in allEvents"
              :key="ev"
              class="flex cursor-pointer items-center gap-1.5 text-xs text-gray-700 dark:text-gray-300 min-h-[44px] min-w-[44px] select-none"
            >
              <input
                v-model="form.events"
                type="checkbox"
                :value="ev"
                class="h-3.5 w-3.5 rounded border-gray-300 text-primary-600 focus:ring-primary-500 cursor-pointer"
              />
              {{ t(`admin.upstream.events.${ev}`) }}
            </label>
          </div>
        </div>

        <!-- 启用开关 -->
        <div class="flex items-center gap-2 min-h-[44px]">
          <input
            id="ch-enabled"
            v-model="form.enabled"
            type="checkbox"
            class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 cursor-pointer"
          />
          <label for="ch-enabled" class="cursor-pointer text-sm text-gray-700 dark:text-gray-300">
            {{ t('common.enabled') }}
          </label>
        </div>

        <!-- 表单错误提示 -->
        <div v-if="formError" role="alert" class="rounded-lg bg-red-50 px-4 py-2.5 text-sm text-red-600 dark:bg-red-900/20 dark:text-red-400">
          {{ formError }}
        </div>

        <!-- 表单操作 -->
        <div class="flex justify-end gap-2 pt-2">
          <button
            type="button"
            class="btn btn-secondary cursor-pointer"
            @click="cancelEdit"
          >
            {{ t('common.cancel') }}
          </button>
          <button
            type="submit"
            class="btn btn-primary cursor-pointer"
            :disabled="saving"
          >
            {{ t('common.save') }}
          </button>
        </div>
      </form>
    </div>

    <template #footer>
      <button
        v-if="!editing"
        type="button"
        class="btn btn-primary cursor-pointer"
        @click="startCreate"
      >
        {{ t('admin.upstream.notify.addChannel') }}
      </button>
    </template>
  </BaseDialog>

  <!-- 删除确认对话框 -->
  <ConfirmDialog
    :show="!!deleteTarget"
    :title="t('common.delete')"
    :message="t('admin.upstream.notify.deleteConfirm')"
    :danger="true"
    @confirm="confirmDelete"
    @cancel="deleteTarget = null"
  />
</template>

<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import { notifyChannelsAPI } from '@/api/admin'
import type { NotifyChannel } from '@/api/admin/notifyChannels'
import { useAppStore } from '@/stores/app'

const props = defineProps<{ show: boolean }>()
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const appStore = useAppStore()

const channels = ref<NotifyChannel[]>([])
const editing = ref<{ id?: number } | null>(null)
const saving = ref(false)
const formError = ref('')
const deleteTarget = ref<NotifyChannel | null>(null)

// body_template 占位符含 Go template 双花括号，避免 Vue 编译误解析
const bodyTemplatePlaceholder = computed(
  () => '{"msgtype":"text","text":{"content":"' + '{{' + '.Title' + '}}' + '","at":{"isAtAll":false}}}'
)

const allEvents = [
  'balance_low',
  'price_changed',
  'model_added',
  'model_removed',
  'group_added',
  'group_removed',
  'refresh_failed',
  'credential_error',
]

const typeOptions = [
  { label: t('admin.upstream.notify.email'), value: 'email' },
  { label: t('admin.upstream.notify.webhook'), value: 'webhook' },
]

const form = ref({
  name: '',
  type: 'email' as 'email' | 'webhook',
  enabled: true,
  recipients: '',
  url: '',
  headers: '',
  bodyTemplate: '',
  events: [] as string[],
})

watch(
  () => props.show,
  (show) => {
    if (show) void load()
    editing.value = null
    formError.value = ''
    deleteTarget.value = null
  }
)

async function load() {
  try {
    channels.value = await notifyChannelsAPI.list('upstream')
  } catch {
    // 静默失败 — 列表显示空即可
  }
}

function startCreate() {
  form.value = {
    name: '',
    type: 'email',
    enabled: true,
    recipients: '',
    url: '',
    headers: '',
    bodyTemplate: '',
    events: [],
  }
  editing.value = {}
  formError.value = ''
}

function startEdit(ch: NotifyChannel) {
  const cfg = ch.config as {
    recipients?: string[]
    url?: string
    headers?: Record<string, string>
    body_template?: string
  }
  form.value = {
    name: ch.name,
    type: ch.type,
    enabled: ch.enabled,
    recipients: (cfg.recipients ?? []).join(', '),
    url: cfg.url ?? '',
    // headers 已被后端脱敏为 ***，原样回显（含 ***），保存时原样发送，后端 merge 还原
    headers: cfg.headers ? JSON.stringify(cfg.headers) : '',
    bodyTemplate: cfg.body_template ?? '',
    events: ch.events ?? [],
  }
  editing.value = { id: ch.id }
  formError.value = ''
}

function cancelEdit() {
  editing.value = null
  formError.value = ''
}

function buildConfig(): Record<string, unknown> {
  if (form.value.type === 'email') {
    return {
      recipients: form.value.recipients
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean),
    }
  }
  const cfg: Record<string, unknown> = { url: form.value.url.trim() }
  if (form.value.headers.trim()) {
    try {
      cfg.headers = JSON.parse(form.value.headers)
    } catch {
      // 非法 JSON 时忽略 headers，后端将保留原有值
    }
  }
  if (form.value.bodyTemplate.trim()) {
    cfg.body_template = form.value.bodyTemplate
  }
  return cfg
}

async function handleSave() {
  saving.value = true
  formError.value = ''
  try {
    const input = {
      name: form.value.name,
      type: form.value.type,
      scope: 'upstream',
      enabled: form.value.enabled,
      events: form.value.events,
      config: buildConfig(),
    }
    if (editing.value?.id) {
      await notifyChannelsAPI.update(editing.value.id, input)
    } else {
      await notifyChannelsAPI.create(input)
    }
    editing.value = null
    await load()
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : String(err)
    formError.value = msg || t('admin.upstream.notify.saveFailed')
    appStore.showError(formError.value)
  } finally {
    saving.value = false
  }
}

async function handleTest(ch: NotifyChannel) {
  try {
    await notifyChannelsAPI.test(ch.id)
    appStore.showSuccess(t('admin.upstream.notify.testOk'))
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : String(err)
    appStore.showError(msg || t('admin.upstream.notify.testFailed'))
  } finally {
    // 刷新列表以更新 last_error / last_sent_at
    await load()
  }
}

function handleDelete(ch: NotifyChannel) {
  deleteTarget.value = ch
}

async function confirmDelete() {
  if (!deleteTarget.value) return
  const ch = deleteTarget.value
  deleteTarget.value = null
  try {
    await notifyChannelsAPI.remove(ch.id)
    await load()
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : String(err)
    appStore.showError(msg || t('admin.upstream.notify.deleteFailed'))
  }
}
</script>
