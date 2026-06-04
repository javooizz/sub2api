<template>
  <AppLayout>
    <div class="flex flex-col gap-4 lg:flex-row">
      <!-- 桌面筛选侧栏 / 移动端折叠 -->
      <div class="lg:w-60 lg:shrink-0">
        <details class="card p-4 lg:hidden" :open="false">
          <summary class="cursor-pointer text-sm font-semibold text-gray-900 dark:text-gray-100">
            {{ t('modelPlaza.filters.title') }}
          </summary>
          <div class="mt-3">
            <ModelPlazaFilters
              :filters="filters"
              :platform-options="platformOptions"
              :billing-mode-options="billingModeOptions"
              :group-options="groupOptions"
              :total-count="models.length"
              :user-rates="userRates"
              @reset="reset"
              @update="Object.assign(filters, $event)"
            />
          </div>
        </details>
        <div class="card sticky top-4 hidden p-4 lg:block">
          <ModelPlazaFilters
            :filters="filters"
            :platform-options="platformOptions"
            :billing-mode-options="billingModeOptions"
            :group-options="groupOptions"
            :total-count="models.length"
            :user-rates="userRates"
            @reset="reset"
            @update="Object.assign(filters, $event)"
          />
        </div>
      </div>

      <div class="min-w-0 flex-1 space-y-4">
        <!-- 公告 -->
        <div
          v-if="announcementHtml"
          class="card border-l-4 border-l-blue-500 p-4"
          role="note"
        >
          <h3 class="mb-1 text-sm font-semibold text-gray-900 dark:text-gray-100">
            {{ t('modelPlaza.announcementTitle') }}
          </h3>
          <!-- eslint-disable-next-line vue/no-v-html — marked + DOMPurify 消毒后渲染 -->
          <div class="announcement-prose text-sm text-gray-600 dark:text-gray-300" v-html="announcementHtml" />
        </div>

        <!-- 工具栏（未启用/加载失败时隐藏，避免空态上方冗余控件） -->
        <ModelPlazaToolbar
          v-if="enabled && !loadError"
          :search="filters.search"
          :view="view"
          :selected-count="selectedNames.size"
          @update:search="filters.search = $event"
          @update:view="view = $event"
          @copy-selected="copySelected"
          @clear-selection="selectedNames = new Set()"
        />

        <!-- 加载骨架（>300ms 由 loading 状态自然覆盖；骨架本身常显于 loading） -->
        <div v-if="loading" class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          <div v-for="i in 6" :key="i" class="card h-44 animate-pulse bg-gray-100 dark:bg-gray-800" />
        </div>

        <!-- 加载失败：可重试 -->
        <div v-else-if="loadError" class="card flex flex-col items-center gap-3 py-16 text-center">
          <Icon name="exclamationTriangle" size="lg" class="text-amber-400" />
          <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('modelPlaza.loadFailed') }}</p>
          <button type="button" class="btn btn-secondary" @click="load">
            <Icon name="refresh" size="sm" class="mr-1" />
            {{ t('modelPlaza.retry') }}
          </button>
        </div>

        <!-- 功能未启用 -->
        <div v-else-if="!enabled" class="card flex flex-col items-center gap-2 py-16 text-center">
          <Icon name="lock" size="lg" class="text-gray-300 dark:text-gray-600" />
          <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('modelPlaza.disabled') }}</p>
        </div>

        <!-- 空态 -->
        <div v-else-if="filtered.length === 0" class="card flex flex-col items-center gap-2 py-16 text-center">
          <Icon name="search" size="lg" class="text-gray-300 dark:text-gray-600" />
          <p class="text-sm text-gray-500 dark:text-gray-400">
            {{ models.length === 0 ? t('modelPlaza.empty') : t('modelPlaza.emptyFiltered') }}
          </p>
        </div>

        <!-- 卡片视图 -->
        <div v-else-if="view === 'card'" class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          <ModelCard
            v-for="m in filtered"
            :key="`${m.platform}/${m.name}`"
            :model="m"
            :selected="selectedNames.has(m.name)"
            :selected-group="selectedGroup"
            :user-rates="userRates"
            @open="detailModel = $event"
            @copy="copyName"
            @toggle-select="toggleSelect"
          />
        </div>

        <!-- 表格视图 -->
        <div v-else class="card">
          <ModelPlazaTable
            :models="filtered"
            :selected-names="selectedNames"
            :selected-group="selectedGroup"
            :user-rates="userRates"
            @open="detailModel = $event"
            @copy="copyName"
            @toggle-select="toggleSelect"
          />
        </div>
      </div>
    </div>

    <ModelDetailDrawer
      :model="detailModel"
      :user-rates="userRates"
      @close="detailModel = null"
      @copy="copyName"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import ModelPlazaFilters from '@/components/modelplaza/ModelPlazaFilters.vue'
import ModelPlazaToolbar from '@/components/modelplaza/ModelPlazaToolbar.vue'
import ModelCard from '@/components/modelplaza/ModelCard.vue'
import ModelPlazaTable from '@/components/modelplaza/ModelPlazaTable.vue'
import ModelDetailDrawer from '@/components/modelplaza/ModelDetailDrawer.vue'
import modelPlazaAPI, { type PlazaModel } from '@/api/modelPlaza'
import userGroupsAPI from '@/api/groups'
import { useModelPlazaFilters } from '@/composables/useModelPlazaFilters'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const loadError = ref(false)
const enabled = ref(true)
const announcement = ref('')
const models = ref<PlazaModel[]>([])
const userRates = ref<Record<number, number>>({})
const view = ref<'card' | 'table'>('card')
const detailModel = ref<PlazaModel | null>(null)
const selectedNames = ref<Set<string>>(new Set())

const { filters, filtered, platformOptions, billingModeOptions, groupOptions, selectedGroup, reset } =
  useModelPlazaFilters(models)

marked.setOptions({ breaks: true, gfm: true })

/** 公告：marked 渲染 + DOMPurify 消毒（同 AnnouncementPopup.vue 模式，严禁直插）。 */
const announcementHtml = computed(() => {
  if (!announcement.value) return ''
  const html = marked.parse(announcement.value) as string
  return DOMPurify.sanitize(html)
})

async function load() {
  loading.value = true
  loadError.value = false
  try {
    // 广场数据与用户专属倍率并发拉取；倍率失败只降级为默认倍率展示。
    const [plaza, rates] = await Promise.all([
      modelPlazaAPI.getModelPlaza(),
      userGroupsAPI.getUserGroupRates().catch(() => ({}) as Record<number, number>),
    ])
    enabled.value = plaza.enabled
    announcement.value = plaza.announcement || ''
    models.value = plaza.models
    userRates.value = rates
  } catch (err: unknown) {
    loadError.value = true
    appStore.showError(extractApiErrorMessage(err, t('modelPlaza.loadFailed')))
  } finally {
    loading.value = false
  }
}

async function copyName(name: string) {
  try {
    await navigator.clipboard.writeText(name)
    appStore.showSuccess(t('modelPlaza.card.copied'))
  } catch {
    appStore.showError(t('common.error'))
  }
}

function toggleSelect(m: PlazaModel) {
  const next = new Set(selectedNames.value)
  if (next.has(m.name)) next.delete(m.name)
  else next.add(m.name)
  selectedNames.value = next
}

async function copySelected() {
  if (selectedNames.value.size === 0) return
  await copyName([...selectedNames.value].join(','))
}

onMounted(load)
</script>

<style scoped>
.announcement-prose :deep(p) {
  margin: 0.25rem 0;
}
.announcement-prose :deep(a) {
  color: rgb(37 99 235);
  text-decoration: underline;
}
.announcement-prose :deep(ul) {
  list-style: disc;
  padding-left: 1.25rem;
}
</style>
