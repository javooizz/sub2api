<template>
  <AppLayout>
    <div class="mx-auto max-w-4xl space-y-5 pb-24">
      <div v-if="loading" class="space-y-5">
        <div v-for="i in 4" :key="i" class="card h-36 animate-pulse bg-gray-100 dark:bg-gray-800" />
      </div>
      <template v-else>
      <!-- 功能开关 -->
      <section class="card p-5">
        <h2 class="mb-1 text-base font-semibold text-gray-900 dark:text-gray-100">
          {{ t('admin.extensionConfig.modelPlaza.enableSection') }}
        </h2>
        <p class="mb-4 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.extensionConfig.modelPlaza.enableHint') }}
        </p>
        <label class="flex w-fit cursor-pointer items-center gap-3">
          <input
            v-model="enabled"
            type="checkbox"
            class="h-5 w-5 cursor-pointer rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-gray-600"
          />
          <span class="text-sm font-medium text-gray-900 dark:text-gray-100">
            {{ t('admin.extensionConfig.modelPlaza.enableLabel') }}
          </span>
        </label>
      </section>

      <!-- 展示排除 -->
      <section class="card p-5">
        <h2 class="mb-4 text-base font-semibold text-gray-900 dark:text-gray-100">
          {{ t('admin.extensionConfig.modelPlaza.exclusionSection') }}
        </h2>
        <div class="grid gap-6 md:grid-cols-2">
          <div>
            <h3 class="mb-1 text-sm font-medium text-gray-700 dark:text-gray-200">
              {{ t('admin.extensionConfig.modelPlaza.excludedChannels') }}
            </h3>
            <p class="mb-2 text-xs text-gray-400">
              {{ t('admin.extensionConfig.modelPlaza.excludedChannelsHint') }}
            </p>
            <div class="max-h-56 space-y-1 overflow-y-auto rounded-lg border border-gray-200 p-2 dark:border-gray-700">
              <p v-if="channels.length === 0" class="p-2 text-xs text-gray-400">
                {{ t('admin.extensionConfig.modelPlaza.noChannels') }}
              </p>
              <label
                v-for="ch in channels"
                :key="ch.id"
                class="flex cursor-pointer items-center gap-2 rounded px-2 py-1 text-sm hover:bg-gray-50 dark:hover:bg-gray-800"
              >
                <input
                  type="checkbox"
                  class="h-4 w-4 cursor-pointer rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-gray-600"
                  :checked="form.excluded_channel_ids.includes(ch.id)"
                  @change="toggleId(form.excluded_channel_ids, ch.id)"
                />
                <span class="truncate text-gray-700 dark:text-gray-200">{{ ch.name }}</span>
              </label>
            </div>
          </div>
          <div>
            <h3 class="mb-1 text-sm font-medium text-gray-700 dark:text-gray-200">
              {{ t('admin.extensionConfig.modelPlaza.excludedGroups') }}
            </h3>
            <p class="mb-2 text-xs text-gray-400">
              {{ t('admin.extensionConfig.modelPlaza.excludedGroupsHint') }}
            </p>
            <div class="max-h-56 space-y-1 overflow-y-auto rounded-lg border border-gray-200 p-2 dark:border-gray-700">
              <p v-if="groups.length === 0" class="p-2 text-xs text-gray-400">
                {{ t('admin.extensionConfig.modelPlaza.noGroups') }}
              </p>
              <label
                v-for="g in groups"
                :key="g.id"
                class="flex cursor-pointer items-center gap-2 rounded px-2 py-1 text-sm hover:bg-gray-50 dark:hover:bg-gray-800"
              >
                <input
                  type="checkbox"
                  class="h-4 w-4 cursor-pointer rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-gray-600"
                  :checked="form.excluded_group_ids.includes(g.id)"
                  @change="toggleId(form.excluded_group_ids, g.id)"
                />
                <span class="truncate text-gray-700 dark:text-gray-200">{{ g.name }}</span>
                <span class="ml-auto shrink-0 text-xs text-gray-400">{{ g.platform }}</span>
              </label>
            </div>
          </div>
        </div>
      </section>

      <!-- 模型描述 -->
      <section class="card p-5">
        <h2 class="mb-1 text-base font-semibold text-gray-900 dark:text-gray-100">
          {{ t('admin.extensionConfig.modelPlaza.descriptionsSection') }}
        </h2>
        <p class="mb-3 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.extensionConfig.modelPlaza.descriptionsHint') }}
        </p>
        <input
          v-model="modelSearch"
          type="text"
          class="input mb-3 w-full sm:w-64"
          :aria-label="t('admin.extensionConfig.modelPlaza.searchModels')"
          :placeholder="t('admin.extensionConfig.modelPlaza.searchModels')"
        />
        <div class="max-h-96 space-y-2 overflow-y-auto">
          <div
            v-for="m in filteredModelList"
            :key="`${m.platform}/${m.name}`"
            class="flex flex-col gap-1 rounded-lg border border-gray-100 p-2.5 dark:border-gray-800 sm:flex-row sm:items-center sm:gap-3"
          >
            <div class="flex w-full shrink-0 items-center gap-2 sm:w-64">
              <ModelIcon :model="m.name" size="18px" />
              <span class="truncate font-mono text-xs text-gray-800 dark:text-gray-200" :title="m.name">
                {{ m.name }}
              </span>
              <span class="shrink-0 text-[10px] text-gray-400">{{ m.platform }}</span>
            </div>
            <input
              :value="form.model_descriptions[descKey(m)] ?? ''"
              type="text"
              maxlength="500"
              class="input flex-1 text-sm"
              :aria-label="m.name"
              :placeholder="t('admin.extensionConfig.modelPlaza.descriptionPlaceholder')"
              @input="setDescription(descKey(m), ($event.target as HTMLInputElement).value)"
            />
          </div>
        </div>
        <!-- 孤儿描述：payload 有、当前清单没有的模型（保存时保留） -->
        <div v-if="orphanDescriptions.length > 0" class="mt-3 rounded-lg bg-amber-50 p-3 text-xs text-amber-700 dark:bg-amber-900/20 dark:text-amber-300">
          <p class="mb-1 font-medium">{{ t('admin.extensionConfig.modelPlaza.orphanDescriptions') }}</p>
          <p class="font-mono">{{ orphanDescriptions.join(', ') }}</p>
        </div>
      </section>

      <!-- 公告 -->
      <section class="card p-5">
        <h2 class="mb-1 text-base font-semibold text-gray-900 dark:text-gray-100">
          {{ t('admin.extensionConfig.modelPlaza.announcementSection') }}
        </h2>
        <p class="mb-3 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.extensionConfig.modelPlaza.announcementHint') }}
        </p>
        <textarea
          v-model="form.announcement"
          rows="4"
          maxlength="2000"
          class="input w-full font-mono text-sm"
          :aria-label="t('admin.extensionConfig.modelPlaza.announcementSection')"
          :placeholder="t('admin.extensionConfig.modelPlaza.announcementPlaceholder')"
        />
        <p class="mt-1 text-right text-xs text-gray-400">{{ form.announcement.length }} / 2000</p>
      </section>
      </template>

      <!-- sticky 保存条 -->
      <div
        v-if="!loading && isDirty"
        class="fixed inset-x-0 bottom-0 z-40 border-t border-gray-200 bg-white/95 px-4 py-3 backdrop-blur dark:border-gray-700 dark:bg-gray-900/95"
        role="region"
        aria-live="polite"
      >
        <div class="mx-auto flex max-w-4xl items-center justify-end gap-3">
          <button type="button" class="btn btn-secondary" :disabled="saving" @click="discard">
            {{ t('admin.extensionConfig.modelPlaza.discard') }}
          </button>
          <button type="button" class="btn btn-primary" :disabled="saving" @click="save">
            {{ saving ? t('admin.extensionConfig.modelPlaza.saving') : t('admin.extensionConfig.modelPlaza.save') }}
          </button>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
/**
 * 扩展配置 → 模型广场
 * 开关存 settings 专项端点（PUT /admin/settings/model-plaza，不进全量 settings）；
 * 黑名单/模型描述/公告存 extension_configs（agent_id='model-plaza'）。
 * 一个保存按钮提交两个请求，部分失败分别提示。
 */

import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { onBeforeRouteLeave } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import ModelIcon from '@/components/common/ModelIcon.vue'
import { useAppStore } from '@/stores/app'
import { extensionConfigAPI, type ModelPlazaConfig } from '@/api/admin/extensionConfig'
import adminModelPlazaAPI, { type ModelIdentity } from '@/api/admin/modelPlaza'
import { list as listChannels } from '@/api/admin/channels'
import groupsAPI from '@/api/admin/groups'
import type { AdminGroup } from '@/types'
import { extractApiErrorMessage } from '@/utils/apiError'

const AGENT_ID = 'model-plaza'

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const saving = ref(false)
const enabled = ref(false)
const channels = ref<{ id: number; name: string }[]>([])
const groups = ref<AdminGroup[]>([])
const modelList = ref<ModelIdentity[]>([])
const modelSearch = ref('')
/** payload 原样保留的全部描述（含孤儿项），编辑只覆盖清单内的 key。 */
const form = ref<ModelPlazaConfig>({
  excluded_channel_ids: [],
  excluded_group_ids: [],
  model_descriptions: {},
  announcement: '',
})

const filteredModelList = computed(() => {
  const q = modelSearch.value.trim().toLowerCase()
  if (!q) return modelList.value
  return modelList.value.filter((m) => m.name.toLowerCase().includes(q))
})

/** 模型描述复合键（2026-06-05 修订，与后端校验/广场注入一致）：platform/name */
const descKey = (m: { platform: string; name: string }) => `${m.platform}/${m.name}`

/** payload 中存在、但当前模型清单没有的描述 key（渠道临时下线等场景，保存时保留）。 */
const orphanDescriptions = computed(() => {
  const inList = new Set(modelList.value.map(descKey))
  return Object.keys(form.value.model_descriptions).filter((key) => !inList.has(key))
})

// ===== dirty 跟踪 =====
const baselineSnapshot = ref('')

function snapshot(): string {
  return JSON.stringify({ enabled: enabled.value, form: form.value })
}

const isDirty = computed(() => snapshot() !== baselineSnapshot.value)

function discard() {
  if (!isDirty.value) return
  if (!window.confirm(t('admin.extensionConfig.modelPlaza.discardConfirm'))) return
  loadAll().then(() => appStore.showInfo(t('admin.extensionConfig.modelPlaza.discarded')))
}

// ===== 交互 =====
function toggleId(list: number[], id: number) {
  const idx = list.indexOf(id)
  if (idx >= 0) list.splice(idx, 1)
  else list.push(id)
}

function setDescription(key: string, value: string) {
  const v = value.trim()
  if (v) form.value.model_descriptions[key] = v
  else delete form.value.model_descriptions[key]
}

// ===== load / save =====
async function loadAll() {
  loading.value = true
  try {
    const [settings, channelPage, allGroups, models, cfg] = await Promise.all([
      adminModelPlazaAPI.getSettings(),
      listChannels(1, 200),
      groupsAPI.getAll(),
      adminModelPlazaAPI.listModels(),
      extensionConfigAPI.getAdmin(AGENT_ID).catch(() => null),
    ])
    enabled.value = settings.enabled
    channels.value = channelPage.items.map((c) => ({ id: c.id, name: c.name }))
    groups.value = allGroups
    modelList.value = models
    const mp = cfg?.payload?.model_plaza
    form.value = {
      excluded_channel_ids: mp?.excluded_channel_ids ?? [],
      excluded_group_ids: mp?.excluded_group_ids ?? [],
      model_descriptions: { ...(mp?.model_descriptions ?? {}) },
      announcement: mp?.announcement ?? '',
    }
    baselineSnapshot.value = snapshot()
  } catch (e: unknown) {
    appStore.showError(extractApiErrorMessage(e, t('admin.extensionConfig.modelPlaza.loadFailed')))
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  // 两个请求独立提交：部分失败分别提示，已成功部分不回滚。
  const failures: string[] = []
  try {
    await adminModelPlazaAPI.updateSettings({ enabled: enabled.value })
  } catch {
    failures.push(t('admin.extensionConfig.modelPlaza.switchSaveFailed'))
  }
  try {
    await extensionConfigAPI.upsertAdmin(AGENT_ID, {
      version: 1,
      model_plaza: form.value,
    })
  } catch (e: unknown) {
    failures.push(extractApiErrorMessage(e, t('admin.extensionConfig.modelPlaza.configSaveFailed')))
  }
  saving.value = false
  if (failures.length > 0) {
    appStore.showError(failures.join('；'))
    return
  }
  baselineSnapshot.value = snapshot()
  appStore.showSuccess(t('admin.extensionConfig.modelPlaza.saved'))
}

// 离开守卫（同工作台页模式）
onBeforeRouteLeave(() => {
  if (!isDirty.value) return true
  return window.confirm(t('admin.extensionConfig.modelPlaza.discardConfirm'))
})

function beforeUnload(e: BeforeUnloadEvent) {
  if (isDirty.value) e.preventDefault()
}

onMounted(() => {
  loadAll()
  window.addEventListener('beforeunload', beforeUnload)
})
onBeforeUnmount(() => window.removeEventListener('beforeunload', beforeUnload))
</script>
