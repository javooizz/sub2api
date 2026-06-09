<template>
  <BaseDialog
    :show="show"
    :title="isEdit ? t('common.edit') : t('admin.upstream.addProvider')"
    width="wide"
    @close="emit('close')"
  >
    <form class="space-y-5" @submit.prevent="handleSubmit">
      <div class="grid grid-cols-2 gap-4">
        <div>
          <label class="input-label">{{ t('admin.upstream.form.name') }}</label>
          <input
            v-model="form.name"
            type="text"
            class="input"
            :class="{ 'border-red-500': errors.name }"
          />
          <p v-if="errors.name" class="mt-1 text-xs text-red-600 dark:text-red-400" role="alert">
            {{ errText(errors.name) }}
          </p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.upstream.form.type') }}</label>
          <Select v-model="form.type" :options="typeOptions" :disabled="isEdit" />
        </div>
      </div>

      <div>
        <label class="input-label">{{ t('admin.upstream.form.siteUrl') }}</label>
        <input
          v-model="form.site_url"
          type="text"
          placeholder="https://upstream.example.com"
          class="input"
          :class="{ 'border-red-500': errors.site_url }"
        />
        <p v-if="errors.site_url" class="mt-1 text-xs text-red-600 dark:text-red-400" role="alert">
          {{ errText(errors.site_url) }}
        </p>
      </div>

      <div>
        <label class="input-label">{{ t('admin.upstream.form.apiBaseUrl') }}</label>
        <input
          v-model="form.api_base_url"
          type="text"
          :placeholder="t('admin.upstream.form.apiBaseUrlPlaceholder')"
          class="input"
          :class="{ 'border-red-500': errors.api_base_url }"
        />
        <p v-if="errors.api_base_url" class="mt-1 text-xs text-red-600 dark:text-red-400" role="alert">
          {{ errText(errors.api_base_url) }}
        </p>
      </div>

      <fieldset class="rounded-lg border border-gray-200 p-4 dark:border-gray-700">
        <legend class="px-1 text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.upstream.form.credentials') }}
        </legend>
        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="input-label">{{ t('admin.upstream.form.username') }}</label>
            <input v-model="form.username" type="text" autocomplete="off" class="input" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.upstream.form.password') }}</label>
            <input
              v-model="form.password"
              type="password"
              autocomplete="new-password"
              class="input"
              :placeholder="passwordPlaceholder"
            />
          </div>
        </div>
        <div class="mt-3">
          <label class="input-label">{{ t('admin.upstream.form.accessToken') }}</label>
          <input
            v-model="form.access_token"
            type="password"
            autocomplete="off"
            class="input"
            :placeholder="tokenPlaceholder"
          />
        </div>
        <p v-if="errors.credentials" class="mt-2 text-xs text-red-600 dark:text-red-400" role="alert">
          {{ errText(errors.credentials) }}
        </p>
      </fieldset>

      <div class="grid grid-cols-2 gap-4">
        <div>
          <label class="input-label">{{ t('admin.upstream.form.refreshInterval') }}</label>
          <input
            v-model.number="form.refresh_interval_minutes"
            type="number"
            min="5"
            max="1440"
            class="input"
            :class="{ 'border-red-500': errors.refresh_interval_minutes }"
          />
          <p
            v-if="errors.refresh_interval_minutes"
            class="mt-1 text-xs text-red-600 dark:text-red-400"
            role="alert"
          >
            {{ errText(errors.refresh_interval_minutes) }}
          </p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.upstream.form.balanceThreshold') }}</label>
          <input
            v-model.number="form.balance_threshold"
            type="number"
            step="0.01"
            min="0"
            :placeholder="t('admin.upstream.form.balanceThresholdPlaceholder')"
            class="input"
          />
        </div>
        <div>
          <label class="input-label">{{ t('admin.upstream.form.rechargeRatio') }}</label>
          <input
            v-model.number="form.recharge_ratio"
            type="number"
            step="0.01"
            min="0.01"
            :placeholder="t('admin.upstream.form.rechargeRatioPlaceholder')"
            class="input"
            :class="{ 'border-red-500': errors.recharge_ratio }"
          />
          <p
            v-if="errors.recharge_ratio"
            class="mt-1 text-xs text-red-600 dark:text-red-400"
            role="alert"
          >
            {{ errText(errors.recharge_ratio) }}
          </p>
        </div>
      </div>

      <div class="flex items-center gap-2">
        <input
          id="notify-price"
          v-model="form.notify_on_price_change"
          type="checkbox"
          class="h-4 w-4 cursor-pointer rounded border-gray-300 text-primary-600 focus:ring-primary-500"
        />
        <label for="notify-price" class="cursor-pointer text-sm text-gray-700 dark:text-gray-300">
          {{ t('admin.upstream.form.notifyOnPriceChange') }}
        </label>
      </div>

      <div>
        <label class="input-label">{{ t('admin.upstream.form.remark') }}</label>
        <textarea v-model="form.remark" rows="2" class="input" />
      </div>

      <div
        v-if="testResult"
        class="rounded-lg bg-emerald-50 p-3 text-sm text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300"
      >
        {{ t('admin.upstream.form.testOk', { balance: testResult.balance ?? '—' }) }}
        <span v-if="testResult.partial">{{ t('admin.upstream.form.testPartial') }}</span>
      </div>
    </form>

    <template #footer>
      <button
        type="button"
        class="btn btn-secondary cursor-pointer"
        :disabled="testing"
        @click="handleTest"
      >
        <span
          v-if="testing"
          class="mr-1 inline-block h-3 w-3 animate-spin rounded-full border-b-2 border-current"
        />
        {{ t('admin.upstream.form.testConnection') }}
      </button>
      <button
        type="button"
        class="btn btn-primary cursor-pointer"
        :disabled="submitting"
        @click="handleSubmit"
      >
        {{ t('common.save') }}
      </button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import { upstreamProvidersAPI } from '@/api/admin'
import type { UpstreamProvider, UpstreamProviderInput, UpstreamSnapshot } from '@/api/admin/upstreamProviders'
import { validateProviderForm, type ProviderFormErrors } from './upstreamValidators'

const props = defineProps<{
  show: boolean
  provider?: UpstreamProvider | null
}>()
const emit = defineEmits<{ close: []; saved: [] }>()
const { t } = useI18n()
const appStore = useAppStore()

const isEdit = computed(() => !!props.provider)
const errors = ref<ProviderFormErrors>({})
const testing = ref(false)
const submitting = ref(false)
const testResult = ref<UpstreamSnapshot | null>(null)

const form = reactive({
  name: '',
  type: 'newapi' as 'newapi' | 'sub2api',
  site_url: '',
  api_base_url: '',
  username: '',
  password: '',
  access_token: '',
  balance_threshold: null as number | null,
  notify_on_price_change: true,
  refresh_interval_minutes: 60,
  recharge_ratio: 1,
  remark: '',
})

const typeOptions = computed(() => [
  { label: t('admin.upstream.type.newapi'), value: 'newapi' },
  { label: t('admin.upstream.type.sub2api'), value: 'sub2api' },
])

const tokenPlaceholder = computed(() => {
  const tail = props.provider?.credential_status?.access_token_tail
  return isEdit.value && props.provider?.credential_status?.has_access_token
    ? t('admin.upstream.form.accessTokenKept', { tail: tail ?? '****' })
    : ''
})

const passwordPlaceholder = computed(() => {
  return isEdit.value && props.provider?.credential_status?.has_password
    ? t('admin.upstream.form.passwordKept')
    : ''
})

watch(
  () => props.show,
  (show) => {
    if (!show) return
    errors.value = {}
    testResult.value = null
    const p = props.provider
    form.name = p?.name ?? ''
    form.type = p?.type ?? 'newapi'
    form.site_url = p?.site_url ?? ''
    form.api_base_url = p?.api_base_url ?? ''
    form.username = (p?.credentials?.username as string) ?? ''
    form.password = '' // 敏感键不回显
    form.access_token = ''
    form.balance_threshold = p?.balance_threshold ?? null
    form.notify_on_price_change = p?.notify_on_price_change ?? true
    form.refresh_interval_minutes = p?.refresh_interval_minutes ?? 60
    form.recharge_ratio = p?.recharge_ratio ?? 1
    form.remark = p?.remark ?? ''
  }
)

function errText(key: string): string {
  return t(`admin.upstream.form.errors.${key}`)
}

function buildCredentials(): Record<string, unknown> {
  // 只发送用户实际填写的键:空值不发送 → 后端按敏感键合并保留(spec §9)
  const creds: Record<string, unknown> = {}
  if (form.username.trim()) creds.username = form.username.trim()
  if (form.password) creds.password = form.password
  if (form.access_token.trim()) creds.access_token = form.access_token.trim()
  return creds
}

function buildInput(): UpstreamProviderInput {
  return {
    name: form.name.trim(),
    type: form.type,
    site_url: form.site_url.trim(),
    api_base_url: form.api_base_url.trim(),
    credentials: buildCredentials(),
    balance_threshold: form.balance_threshold,
    notify_on_price_change: form.notify_on_price_change,
    refresh_interval_minutes: form.refresh_interval_minutes,
    recharge_ratio: form.recharge_ratio,
    remark: form.remark,
  }
}

function validate(): boolean {
  errors.value = validateProviderForm(form, isEdit.value)
  return Object.keys(errors.value).length === 0
}

async function handleTest() {
  if (!validate()) return
  testing.value = true
  testResult.value = null
  try {
    testResult.value = await upstreamProvidersAPI.testConnection({
      ...buildInput(),
      provider_id: props.provider?.id,
    })
  } catch (err: unknown) {
    const e = err as { response?: { data?: { detail?: string } } }
    appStore.showError(e.response?.data?.detail ?? t('admin.upstream.form.testFailed'))
  } finally {
    testing.value = false
  }
}

async function handleSubmit() {
  if (!validate()) return
  submitting.value = true
  try {
    if (isEdit.value && props.provider) {
      await upstreamProvidersAPI.update(props.provider.id, buildInput())
    } else {
      await upstreamProvidersAPI.create(buildInput())
    }
    appStore.showSuccess(t(isEdit.value ? 'admin.upstream.form.updateSuccess' : 'admin.upstream.form.createSuccess'))
    emit('saved')
    emit('close')
  } catch (err: unknown) {
    const e = err as { response?: { data?: { detail?: string } } }
    appStore.showError(e.response?.data?.detail ?? t('admin.upstream.form.saveFailed'))
  } finally {
    submitting.value = false
  }
}
</script>
