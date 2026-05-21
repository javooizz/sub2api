<script setup lang="ts">
/**
 * 扩展配置 → 工作台
 * 管理员维护各 onebool-flow agent 的可用端点池、分组白名单、每组模型列表。
 * 关联：sub2api 后端 /admin/extension-configs/:agent_id；onebool-flow iframe workbench 协议。
 *
 * 视觉风格与 SettingsView.vue 对齐：sticky 水平 tabs + .card 分块 + sticky 保存条。
 * tab 样式类（.workbench-tabs-shell / .workbench-tab 等）是 SettingsView.vue 中 .settings-tabs-*
 * 样式的复制（命名换前缀避免全局冲突）。若 settings 视觉更新需要同步维护本文件 <style> 块。
 */

import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { onBeforeRouteLeave } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import AppLayout from '@/components/layout/AppLayout.vue'
import { extensionConfigAPI, type ImageGenConfig } from '@/api/admin/extensionConfig'
import settingsAPI from '@/api/admin/settings'
import groupsAPI from '@/api/admin/groups'
import type { AdminGroup, CustomEndpoint } from '@/types'

const { t } = useI18n()
const appStore = useAppStore()
const AGENT_ID = 'image-gen'

// ===== tabs =====
type WorkbenchTab = 'general' | 'image-gen'
const TAB_KEYS: WorkbenchTab[] = ['general', 'image-gen']
const TAB_ICONS: Record<WorkbenchTab, string> = {
  general: 'cog',
  'image-gen': 'photo',
}
const activeTab = ref<WorkbenchTab>('general')

function selectTab(tab: WorkbenchTab) {
  activeTab.value = tab
}

const tabKeyboardActions = {
  ArrowLeft: -1,
  ArrowUp: -1,
  ArrowRight: 1,
  ArrowDown: 1,
  Home: 'first',
  End: 'last',
} as const

function focusTab(tab: WorkbenchTab) {
  window.requestAnimationFrame(() => {
    document.getElementById(`workbench-tab-${tab}`)?.focus()
  })
}

function handleTabKeydown(event: KeyboardEvent, tab: WorkbenchTab) {
  const action = tabKeyboardActions[event.key as keyof typeof tabKeyboardActions]
  if (action === undefined) return
  event.preventDefault()
  const idx = TAB_KEYS.indexOf(tab)
  let next = idx < 0 ? 0 : idx
  if (action === 'first') next = 0
  else if (action === 'last') next = TAB_KEYS.length - 1
  else next = (next + action + TAB_KEYS.length) % TAB_KEYS.length
  const nextTab = TAB_KEYS[next]
  selectTab(nextTab)
  focusTab(nextTab)
}

// ===== state =====
const loading = ref(false)
const saving = ref(false)
const endpoints = ref<CustomEndpoint[]>([])
const allGroups = ref<AdminGroup[]>([])
const oneboolOrigin = ref<string>('')
const form = ref<ImageGenConfig>({
  enabled_endpoint_names: [],
  default_endpoint_name: '',
  enabled_group_ids: [],
  group_models: {},
})
const modelInputs = ref<Record<string, string>>({})

const enabledGroups = computed<AdminGroup[]>(() =>
  allGroups.value.filter((g) => form.value.enabled_group_ids.includes(g.id)),
)

// ===== dirty tracking =====
const baselineSnapshot = ref<string>('')

function snapshot(): string {
  return JSON.stringify({ oneboolOrigin: oneboolOrigin.value, form: form.value })
}

const isDirty = computed(() => snapshot() !== baselineSnapshot.value)

function discard() {
  if (!isDirty.value) return
  if (!window.confirm(t('admin.extensionConfig.workbench.discardConfirm'))) return
  loadAll().then(() => {
    appStore.showInfo(t('admin.extensionConfig.workbench.discarded'))
  })
}

// ===== load =====
async function loadAll() {
  loading.value = true
  try {
    const [sysSettings, groups, cfg] = await Promise.all([
      settingsAPI.getSettings(),
      groupsAPI.getAll(),
      extensionConfigAPI.getAdmin(AGENT_ID).catch(() => null),
    ])
    endpoints.value = sysSettings.custom_endpoints ?? []
    allGroups.value = groups
    if (cfg?.payload) {
      oneboolOrigin.value = cfg.payload.onebool_origin ?? ''
    } else {
      oneboolOrigin.value = ''
    }
    if (cfg?.payload?.image_gen) {
      form.value = {
        enabled_endpoint_names: cfg.payload.image_gen.enabled_endpoint_names ?? [],
        default_endpoint_name: cfg.payload.image_gen.default_endpoint_name ?? '',
        enabled_group_ids: cfg.payload.image_gen.enabled_group_ids ?? [],
        group_models: cfg.payload.image_gen.group_models ?? {},
      }
    } else {
      form.value = {
        enabled_endpoint_names: [],
        default_endpoint_name: '',
        enabled_group_ids: [],
        group_models: {},
      }
    }
    baselineSnapshot.value = snapshot()
  } catch (e: unknown) {
    appStore.showError(toErrorMessage(e, t('admin.extensionConfig.workbench.loadFailed')))
  } finally {
    loading.value = false
  }
}

// ===== endpoint actions =====
function toggleEndpoint(name: string) {
  const arr = form.value.enabled_endpoint_names
  const i = arr.indexOf(name)
  if (i >= 0) {
    arr.splice(i, 1)
    if (form.value.default_endpoint_name === name) form.value.default_endpoint_name = ''
  } else {
    arr.push(name)
  }
}

function setDefaultEndpoint(name: string) {
  if (!form.value.enabled_endpoint_names.includes(name)) return
  form.value.default_endpoint_name = form.value.default_endpoint_name === name ? '' : name
}

// ===== group actions =====
function toggleGroup(id: number) {
  const arr = form.value.enabled_group_ids
  const key = String(id)
  const i = arr.indexOf(id)
  if (i >= 0) {
    arr.splice(i, 1)
    delete form.value.group_models[key]
  } else {
    arr.push(id)
    if (!form.value.group_models[key]) form.value.group_models[key] = []
  }
}

// ===== model chips =====
function addModel(gid: number) {
  const key = String(gid)
  const raw = (modelInputs.value[key] ?? '').trim()
  if (!raw) return
  const list = form.value.group_models[key] ?? []
  if (list.includes(raw)) {
    modelInputs.value[key] = ''
    return
  }
  if (list.length >= 50) {
    appStore.showError(t('admin.extensionConfig.workbench.modelLimitReached'))
    return
  }
  form.value.group_models[key] = [...list, raw]
  modelInputs.value[key] = ''
}

function removeModel(gid: number, model: string) {
  const key = String(gid)
  form.value.group_models[key] = (form.value.group_models[key] ?? []).filter((m) => m !== model)
}

// ===== save =====
async function save() {
  if (
    form.value.default_endpoint_name &&
    !form.value.enabled_endpoint_names.includes(form.value.default_endpoint_name)
  ) {
    appStore.showError(t('admin.extensionConfig.workbench.defaultEndpointInvalid'))
    return
  }
  const trimmedOrigin = oneboolOrigin.value.trim().replace(/\/+$/, '')
  if (trimmedOrigin && !/^https?:\/\/[^/\s]+$/i.test(trimmedOrigin)) {
    appStore.showError(t('admin.extensionConfig.workbench.oneboolOriginInvalid'))
    return
  }
  oneboolOrigin.value = trimmedOrigin

  saving.value = true
  try {
    await extensionConfigAPI.upsertAdmin(AGENT_ID, {
      version: 1,
      onebool_origin: trimmedOrigin,
      image_gen: form.value,
    })
    baselineSnapshot.value = snapshot()
    appStore.showSuccess(t('admin.extensionConfig.workbench.saved'))
  } catch (e: unknown) {
    appStore.showError(toErrorMessage(e, t('admin.extensionConfig.workbench.saveFailed')))
  } finally {
    saving.value = false
  }
}

/** axios interceptor 把 API error reject 成纯对象 {message, code, reason, ...}，
 *  既不是 Error 实例也不能 String()，专门拆一下避免显示 [object Object]。 */
function toErrorMessage(e: unknown, fallback: string): string {
  if (!e) return fallback
  if (e instanceof Error) return e.message || fallback
  if (typeof e === 'string') return e
  if (typeof e === 'object') {
    const obj = e as { message?: unknown; reason?: unknown }
    if (typeof obj.message === 'string' && obj.message) return obj.message
    if (typeof obj.reason === 'string' && obj.reason) return obj.reason
  }
  return fallback
}

// ===== nav guard =====
onBeforeRouteLeave((_to, _from, next) => {
  if (!isDirty.value) {
    next()
    return
  }
  if (window.confirm(t('admin.extensionConfig.workbench.discardConfirm'))) {
    next()
  } else {
    next(false)
  }
})

function handleBeforeUnload(e: BeforeUnloadEvent) {
  if (!isDirty.value) return
  e.preventDefault()
  e.returnValue = ''
}

onMounted(() => {
  loadAll()
  window.addEventListener('beforeunload', handleBeforeUnload)
})

onBeforeUnmount(() => {
  window.removeEventListener('beforeunload', handleBeforeUnload)
})
</script>

<template>
  <AppLayout>
    <div class="mx-auto max-w-5xl space-y-6 px-6 py-6 pb-24">
      <header>
        <h1 class="text-xl font-semibold text-gray-900 dark:text-gray-100">
          {{ t('admin.extensionConfig.workbench.title') }}
        </h1>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.extensionConfig.workbench.description') }}
        </p>
      </header>

      <div v-if="loading" class="flex items-center justify-center py-12">
        <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600"></div>
      </div>

      <template v-else>
        <!-- Tabs -->
        <div class="workbench-tabs-shell">
          <nav
            class="workbench-tabs-scroll"
            role="tablist"
            :aria-label="t('admin.extensionConfig.workbench.title')"
          >
            <div class="workbench-tabs">
              <button
                v-for="key in TAB_KEYS"
                :key="key"
                :id="`workbench-tab-${key}`"
                type="button"
                role="tab"
                :aria-selected="activeTab === key"
                :tabindex="activeTab === key ? 0 : -1"
                :class="['workbench-tab', activeTab === key && 'workbench-tab-active']"
                @click="selectTab(key)"
                @keydown="handleTabKeydown($event, key)"
              >
                <span class="workbench-tab-icon">
                  <svg
                    v-if="TAB_ICONS[key] === 'cog'"
                    xmlns="http://www.w3.org/2000/svg"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke-width="1.5"
                    stroke="currentColor"
                    class="h-5 w-5"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      d="M9.594 3.94c.09-.542.56-.94 1.11-.94h2.593c.55 0 1.02.398 1.11.94l.213 1.281c.063.374.313.686.645.87.074.04.147.083.22.127.324.196.72.257 1.075.124l1.217-.456a1.125 1.125 0 011.37.49l1.296 2.247a1.125 1.125 0 01-.26 1.431l-1.003.827c-.293.24-.438.613-.431.992a6.759 6.759 0 010 .255c-.007.378.138.75.43.99l1.005.828c.424.35.534.954.26 1.43l-1.298 2.247a1.125 1.125 0 01-1.369.491l-1.217-.456c-.355-.133-.75-.072-1.076.124a6.57 6.57 0 01-.22.128c-.331.183-.581.495-.644.869l-.213 1.28c-.09.543-.56.941-1.11.941h-2.594c-.55 0-1.02-.398-1.11-.94l-.213-1.281c-.062-.374-.312-.686-.644-.87a6.52 6.52 0 01-.22-.127c-.325-.196-.72-.257-1.076-.124l-1.217.456a1.125 1.125 0 01-1.369-.49l-1.297-2.247a1.125 1.125 0 01.26-1.431l1.004-.827c.292-.24.437-.613.43-.992a6.932 6.932 0 010-.255c.007-.378-.138-.75-.43-.99l-1.004-.828a1.125 1.125 0 01-.26-1.43l1.297-2.247a1.125 1.125 0 011.37-.491l1.216.456c.356.133.751.072 1.076-.124.072-.044.146-.087.22-.128.332-.183.582-.495.644-.869l.214-1.281z"
                    />
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"
                    />
                  </svg>
                  <svg
                    v-else
                    xmlns="http://www.w3.org/2000/svg"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke-width="1.5"
                    stroke="currentColor"
                    class="h-5 w-5"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      d="M2.25 15.75l5.159-5.159a2.25 2.25 0 013.182 0l5.159 5.159m-1.5-1.5l1.409-1.409a2.25 2.25 0 013.182 0l2.909 2.909m-18 3.75h16.5a1.5 1.5 0 001.5-1.5V6a1.5 1.5 0 00-1.5-1.5H3.75A1.5 1.5 0 002.25 6v12a1.5 1.5 0 001.5 1.5zm10.5-11.25h.008v.008h-.008V8.25zm.375 0a.375.375 0 11-.75 0 .375.375 0 01.75 0z"
                    />
                  </svg>
                </span>
                <span class="workbench-tab-label">
                  {{ t(`admin.extensionConfig.workbench.tabs.${key === 'image-gen' ? 'imageGen' : 'general'}`) }}
                </span>
              </button>
            </div>
          </nav>
        </div>

        <!-- Tab: general -->
        <div v-show="activeTab === 'general'" class="space-y-6">
          <section class="card">
            <div class="card-header">
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t('admin.extensionConfig.workbench.oneboolOrigin') }}
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t('admin.extensionConfig.workbench.oneboolOriginDescription') }}
              </p>
            </div>
            <div class="card-body space-y-2">
              <input
                v-model="oneboolOrigin"
                type="url"
                class="input max-w-xl"
                placeholder="https://image.sub2api.com"
                autocomplete="off"
                spellcheck="false"
              />
              <p class="input-hint">
                {{ t('admin.extensionConfig.workbench.oneboolOriginHint') }}
              </p>
            </div>
          </section>
        </div>

        <!-- Tab: image-gen -->
        <div v-show="activeTab === 'image-gen'" class="space-y-6">
          <!-- API 端点池 -->
          <section class="card">
            <div class="card-header">
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t('admin.extensionConfig.workbench.endpoints') }}
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t('admin.extensionConfig.workbench.endpointsDescription') }}
              </p>
            </div>
            <div class="card-body">
              <div
                v-if="endpoints.length === 0"
                class="rounded-lg border border-amber-200 bg-amber-50 p-4 dark:border-amber-800 dark:bg-amber-900/20"
              >
                <p class="text-sm text-amber-700 dark:text-amber-300">
                  {{ t('admin.extensionConfig.workbench.noEndpoints') }}
                </p>
              </div>
              <ul v-else class="space-y-2">
                <li v-for="ep in endpoints" :key="ep.name">
                  <label
                    class="endpoint-row"
                    :class="{ 'endpoint-row-active': form.enabled_endpoint_names.includes(ep.name) }"
                  >
                    <input
                      type="checkbox"
                      class="h-4 w-4 cursor-pointer rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-600 dark:bg-dark-700"
                      :checked="form.enabled_endpoint_names.includes(ep.name)"
                      @change="toggleEndpoint(ep.name)"
                    />
                    <div class="min-w-0 flex-1">
                      <div class="flex items-center gap-2">
                        <span class="font-medium text-gray-900 dark:text-gray-100">{{ ep.name }}</span>
                        <span
                          v-if="form.default_endpoint_name === ep.name"
                          class="inline-flex items-center rounded-full bg-primary-50 px-2 py-0.5 text-[11px] font-medium text-primary-700 dark:bg-primary-400/10 dark:text-primary-300"
                        >
                          {{ t('admin.extensionConfig.workbench.isDefault') }}
                        </span>
                      </div>
                      <div class="mt-0.5 truncate text-xs text-gray-500 dark:text-gray-400">
                        {{ ep.endpoint }}
                      </div>
                    </div>
                    <button
                      type="button"
                      class="btn btn-sm shrink-0"
                      :class="
                        form.default_endpoint_name === ep.name
                          ? 'btn-primary'
                          : 'btn-secondary'
                      "
                      :disabled="!form.enabled_endpoint_names.includes(ep.name)"
                      @click.prevent="setDefaultEndpoint(ep.name)"
                    >
                      {{ t('admin.extensionConfig.workbench.markAsDefaultButton') }}
                    </button>
                  </label>
                </li>
              </ul>
            </div>
          </section>

          <!-- 启用的分组 -->
          <section class="card">
            <div class="card-header">
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t('admin.extensionConfig.workbench.enabledGroups') }}
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t('admin.extensionConfig.workbench.enabledGroupsDescription') }}
              </p>
            </div>
            <div class="card-body">
              <div v-if="allGroups.length === 0" class="text-sm text-gray-400">
                {{ t('admin.extensionConfig.workbench.noGroups') }}
              </div>
              <div v-else class="grid grid-cols-2 gap-2 md:grid-cols-3">
                <label
                  v-for="g in allGroups"
                  :key="g.id"
                  class="endpoint-row endpoint-row-compact"
                  :class="{ 'endpoint-row-active': form.enabled_group_ids.includes(g.id) }"
                >
                  <input
                    type="checkbox"
                    class="h-4 w-4 cursor-pointer rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-600 dark:bg-dark-700"
                    :checked="form.enabled_group_ids.includes(g.id)"
                    @change="toggleGroup(g.id)"
                  />
                  <span class="truncate text-sm text-gray-900 dark:text-gray-100">{{ g.name }}</span>
                </label>
              </div>
            </div>
          </section>

          <!-- 分组下的模型 -->
          <section class="card">
            <div class="card-header">
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t('admin.extensionConfig.workbench.groupModels') }}
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t('admin.extensionConfig.workbench.groupModelsDescription') }}
              </p>
            </div>
            <div class="card-body space-y-4">
              <div v-if="enabledGroups.length === 0" class="text-sm text-gray-400">
                {{ t('admin.extensionConfig.workbench.noEnabledGroup') }}
              </div>
              <div v-for="g in enabledGroups" :key="g.id" class="group-models-block">
                <div class="mb-2 flex items-center justify-between">
                  <h3 class="text-sm font-semibold text-gray-900 dark:text-gray-100">{{ g.name }}</h3>
                  <span class="text-xs text-gray-400">
                    {{ (form.group_models[String(g.id)] ?? []).length }} / 50
                  </span>
                </div>
                <div class="flex flex-wrap gap-2">
                  <span
                    v-for="m in form.group_models[String(g.id)] ?? []"
                    :key="m"
                    class="inline-flex items-center gap-1 rounded-full bg-primary-50 px-3 py-1 text-xs font-medium text-primary-700 dark:bg-primary-400/10 dark:text-primary-300"
                  >
                    {{ m }}
                    <button
                      type="button"
                      class="ml-0.5 cursor-pointer rounded-full p-0.5 text-primary-500 transition-colors hover:bg-primary-100 hover:text-primary-700 dark:hover:bg-primary-400/20 dark:hover:text-primary-200"
                      :title="t('common.delete')"
                      :aria-label="t('common.delete')"
                      @click="removeModel(g.id, m)"
                    >
                      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" class="h-3 w-3">
                        <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" />
                      </svg>
                    </button>
                  </span>
                </div>
                <div class="mt-3 flex items-center gap-2">
                  <input
                    v-model="modelInputs[String(g.id)]"
                    type="text"
                    class="input flex-1 max-w-xs"
                    :placeholder="t('admin.extensionConfig.workbench.modelPlaceholder')"
                    @keydown.enter.prevent="addModel(g.id)"
                  />
                  <button
                    type="button"
                    class="btn btn-secondary btn-sm"
                    @click="addModel(g.id)"
                  >
                    {{ t('common.add') }}
                  </button>
                </div>
              </div>
            </div>
          </section>
        </div>
      </template>

      <!-- Sticky Save Bar -->
      <transition name="save-bar">
        <div v-if="!loading && isDirty" class="workbench-save-bar">
          <div class="mx-auto flex max-w-5xl items-center justify-between px-6 py-3">
            <span class="flex items-center gap-2 text-sm font-medium text-amber-700 dark:text-amber-300">
              <span class="inline-block h-2 w-2 rounded-full bg-amber-500"></span>
              {{ t('admin.extensionConfig.workbench.unsavedChanges') }}
            </span>
            <div class="flex items-center gap-2">
              <button type="button" class="btn btn-secondary btn-sm" :disabled="saving" @click="discard">
                {{ t('admin.extensionConfig.workbench.discardChanges') }}
              </button>
              <button type="button" class="btn btn-primary btn-sm" :disabled="saving" @click="save">
                {{ saving ? t('common.saving') : t('common.save') }}
              </button>
            </div>
          </div>
        </div>
      </transition>
    </div>
  </AppLayout>
</template>

<style scoped>
/* ============ Workbench Tab 导航 ============
   样式与 SettingsView.vue 的 .settings-tabs-* 完全同源,
   命名换前缀避免影响全局,内容保持一致。 */
.workbench-tabs-shell {
  @apply sticky z-20 -mx-1 rounded-2xl border border-white/80 bg-white/90 p-1.5 backdrop-blur-xl;
  top: 4.75rem;
  box-shadow:
    0 12px 28px rgb(15 23 42 / 0.07),
    0 1px 0 rgb(255 255 255 / 0.9) inset;
}

.workbench-tabs-scroll {
  @apply overflow-x-auto;
  -ms-overflow-style: none;
  scrollbar-width: none;
}

.workbench-tabs-scroll::-webkit-scrollbar {
  display: none;
}

.workbench-tabs {
  @apply flex min-w-max items-center gap-1;
}

.workbench-tab {
  @apply relative isolate flex h-10 min-w-[6.75rem] shrink-0 cursor-pointer items-center justify-center gap-1.5 whitespace-nowrap rounded-xl border border-transparent px-3 text-sm font-medium text-gray-600 outline-none transition-colors duration-200 ease-out dark:text-gray-300;
}

@media (min-width: 768px) {
  .workbench-tabs {
    @apply min-w-full;
  }

  .workbench-tab {
    @apply min-w-0 flex-1 basis-0 overflow-hidden px-2 text-[13px];
  }
}

.workbench-tab::before {
  @apply absolute inset-0 -z-10 rounded-xl opacity-0 transition-opacity duration-200;
  content: '';
  background: linear-gradient(135deg, rgb(248 250 252 / 0.95), rgb(241 245 249 / 0.8));
}

.workbench-tab:hover::before,
.workbench-tab:focus-visible::before {
  opacity: 1;
}

.workbench-tab:focus-visible {
  @apply ring-2 ring-primary-500/40 ring-offset-2 ring-offset-white dark:ring-offset-dark-900;
}

.workbench-tab-active {
  @apply border-primary-200/80 bg-white text-primary-700 shadow-sm dark:border-primary-400/30 dark:bg-dark-700/95 dark:text-primary-200;
  box-shadow:
    0 8px 18px rgb(15 23 42 / 0.08),
    0 1px 0 rgb(255 255 255 / 0.92) inset;
}

.workbench-tab-active::before {
  opacity: 0;
}

.workbench-tab-active::after {
  position: absolute;
  right: 0.75rem;
  bottom: 0.25rem;
  left: 0.75rem;
  height: 2px;
  border-radius: 9999px;
  content: '';
  background: linear-gradient(90deg, #14b8a6, #0ea5e9);
}

.workbench-tab-icon {
  @apply flex h-7 w-7 shrink-0 items-center justify-center rounded-lg text-gray-500 transition-colors duration-200 dark:text-gray-400;
}

.workbench-tab:hover .workbench-tab-icon,
.workbench-tab:focus-visible .workbench-tab-icon {
  @apply text-gray-700 dark:text-gray-200;
}

.workbench-tab-active .workbench-tab-icon {
  @apply bg-primary-50 text-primary-600 dark:bg-primary-400/10 dark:text-primary-300;
}

.workbench-tab-label {
  @apply min-w-0 overflow-hidden text-ellipsis whitespace-nowrap leading-none;
}

/* ============ Sticky Save Bar ============ */
.workbench-save-bar {
  @apply fixed inset-x-0 bottom-0 z-30 border-t border-gray-200/80 bg-white/90 backdrop-blur-xl dark:border-dark-700/80 dark:bg-dark-900/90;
  box-shadow: 0 -12px 28px rgb(15 23 42 / 0.08);
}

.save-bar-enter-active,
.save-bar-leave-active {
  transition: transform 200ms ease-out, opacity 200ms ease-out;
}

.save-bar-enter-from,
.save-bar-leave-to {
  transform: translateY(100%);
  opacity: 0;
}

@media (prefers-reduced-motion: reduce) {
  .save-bar-enter-active,
  .save-bar-leave-active {
    transition: none;
  }
}

/* ============ Endpoint / Group Row ============ */
.endpoint-row {
  @apply flex cursor-pointer items-center gap-3 rounded-xl border border-gray-200 bg-white px-4 py-3 transition-colors duration-150 hover:border-gray-300 dark:border-dark-700 dark:bg-dark-800/40 dark:hover:border-dark-600;
  min-height: 56px;
}

.endpoint-row-active {
  @apply border-primary-200 bg-primary-50/40 dark:border-primary-400/40 dark:bg-primary-400/5;
}

.endpoint-row:focus-within {
  @apply ring-2 ring-primary-500/30;
}

.endpoint-row-compact {
  @apply py-2;
  min-height: 44px;
}

/* ============ Group Models Sub-Card ============ */
.group-models-block {
  @apply rounded-xl border border-gray-100 bg-gray-50/50 p-4 dark:border-dark-700 dark:bg-dark-900/40;
  border-left: 3px solid theme('colors.primary.400');
}

.dark .group-models-block {
  border-left-color: theme('colors.primary.500');
}
</style>

<style>
/* Dark-mode overrides for the workbench tabs shell — unscoped to avoid
   :global() rules being stripped from production builds, mirroring SettingsView.vue. */
.dark .workbench-tabs-shell {
  border-color: rgb(51 65 85 / 0.65);
  background: rgb(15 23 42 / 0.86);
  box-shadow:
    0 16px 36px rgb(0 0 0 / 0.28),
    0 1px 0 rgb(255 255 255 / 0.06) inset;
}

.dark .workbench-tab::before {
  background: linear-gradient(135deg, rgb(30 41 59 / 0.9), rgb(51 65 85 / 0.62));
}

.dark .workbench-tab-active {
  box-shadow:
    0 12px 26px rgb(0 0 0 / 0.22),
    0 1px 0 rgb(255 255 255 / 0.08) inset;
}
</style>
