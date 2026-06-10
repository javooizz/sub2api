<template>
  <Teleport to="body">
    <Transition name="drawer-fade">
      <div
        v-if="provider"
        class="fixed inset-0 z-50 bg-black/40"
        aria-hidden="true"
        @click="emit('close')"
      />
    </Transition>
    <Transition name="drawer-slide">
      <section
        v-if="provider"
        class="fixed inset-y-0 right-0 z-50 flex w-full max-w-2xl flex-col bg-white shadow-xl dark:bg-gray-900"
        role="dialog"
        aria-modal="true"
        :aria-label="provider.name"
        @keydown.esc="emit('close')"
      >
        <!-- 头部 -->
        <header class="flex items-center justify-between border-b border-gray-200 px-5 py-4 dark:border-gray-700">
          <div class="flex items-center gap-2">
            <h2 class="text-base font-semibold text-gray-900 dark:text-gray-100">{{ provider.name }}</h2>
            <UpstreamStatusBadge :status="provider.status" />
          </div>
          <button
            type="button"
            class="cursor-pointer rounded p-1.5 text-gray-400 hover:bg-gray-100 hover:text-gray-600 focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary-500 dark:hover:bg-gray-800"
            :aria-label="t('common.close')"
            @click="emit('close')"
          >
            <Icon name="x" size="md" />
          </button>
        </header>

        <!-- Tab 导航 -->
        <nav
          class="flex gap-1 border-b border-gray-200 px-5 dark:border-gray-700"
          role="tablist"
          :aria-label="t('admin.upstream.detail.tabsLabel')"
        >
          <button
            v-for="tab in tabs"
            :key="tab"
            role="tab"
            :aria-selected="activeTab === tab"
            class="cursor-pointer border-b-2 px-3 py-2 text-sm transition-colors"
            :class="
              activeTab === tab
                ? 'border-primary-600 text-primary-600 font-medium'
                : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'
            "
            @click="activeTab = tab"
          >
            {{ t(`admin.upstream.detail.tabs.${tab}`) }}
          </button>
        </nav>

        <!-- Tab 内容区 -->
        <div class="flex-1 overflow-y-auto px-5 py-4">

          <!-- ===== 概览 ===== -->
          <div v-if="activeTab === 'overview'" class="space-y-4">
            <!-- 消耗总览(4 窗口) -->
            <UsageSummaryCards :summary="provider.usage_summary" />

            <!-- partial 提示 -->
            <div
              v-if="provider.latest_snapshot?.partial"
              class="rounded-lg bg-amber-50 p-3 text-sm text-amber-700 dark:bg-amber-900/30 dark:text-amber-300"
              role="alert"
            >
              {{ t('admin.upstream.detail.overview.partialHint') }}
            </div>

            <dl class="grid grid-cols-2 gap-4 text-sm">
              <div>
                <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.upstream.detail.overview.balance') }}</dt>
                <dd class="mt-0.5 text-lg font-semibold tabular-nums text-gray-900 dark:text-gray-100">{{ balanceText }}</dd>
              </div>
              <div>
                <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.upstream.detail.overview.user') }}</dt>
                <dd class="mt-0.5 text-gray-900 dark:text-gray-100">{{ provider.latest_snapshot?.user_info?.username ?? '—' }}</dd>
              </div>
              <div class="col-span-2">
                <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.upstream.detail.overview.effectiveApiUrl') }}</dt>
                <dd class="mt-0.5 flex items-center gap-2 font-mono text-xs text-gray-900 dark:text-gray-100">
                  <span class="break-all">{{ provider.effective_api_base_url }}</span>
                  <button
                    type="button"
                    class="cursor-pointer shrink-0 rounded p-0.5 text-gray-400 hover:text-gray-600 focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary-500"
                    @click="copy(provider.effective_api_base_url)"
                  >
                    <Icon name="copy" size="sm" />
                  </button>
                </dd>
              </div>
              <div v-if="provider.last_error" class="col-span-2">
                <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.upstream.detail.overview.lastError') }}</dt>
                <dd class="mt-0.5 break-all text-red-600 dark:text-red-400">
                  {{ provider.last_error }}
                  <!-- R2.2: blob 鉴权，不用直链 -->
                  <button
                    v-if="screenshotFile"
                    type="button"
                    class="ml-2 cursor-pointer text-primary-600 underline hover:text-primary-700 dark:text-primary-400"
                    @click="openScreenshot"
                  >
                    {{ t('admin.upstream.detail.overview.screenshot') }}
                  </button>
                </dd>
              </div>
            </dl>

            <div class="flex gap-2 border-t border-gray-100 pt-3 dark:border-gray-800">
              <button
                type="button"
                class="btn btn-secondary cursor-pointer"
                :disabled="relogining"
                @click="handleRelogin"
              >
                {{ t('admin.upstream.actions.relogin') }}
              </button>
            </div>
          </div>

          <!-- ===== 分组价格 + 消耗 ===== -->
          <div v-else-if="activeTab === 'pricing'" class="space-y-3">
            <div class="flex items-center justify-between">
              <UsageWindowSwitcher v-model="usageWindow" />
            </div>
            <table class="w-full text-sm">
              <thead>
                <tr class="border-b border-gray-200 text-left dark:border-gray-700">
                  <th class="py-2 pr-3 font-medium text-gray-500 dark:text-gray-400">{{ t('admin.upstream.detail.pricing.group') }}</th>
                  <th class="py-2 pr-3 text-right font-medium text-gray-500 dark:text-gray-400">{{ t('admin.upstream.detail.pricing.ratio') }}</th>
                  <th class="py-2 pr-3 text-right font-medium text-gray-500 dark:text-gray-400">{{ t('admin.upstream.detail.pricing.modelCount') }}</th>
                  <th class="py-2 pr-3 text-right font-medium text-gray-500 dark:text-gray-400">{{ t('admin.upstream.usage.spent') }}</th>
                  <th class="py-2 text-right font-medium text-gray-500 dark:text-gray-400">{{ t('admin.upstream.usage.paid') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-gray-800">
                <tr v-for="g in groups" :key="g.name">
                  <td class="py-2 pr-3 text-gray-900 dark:text-gray-100">
                    {{ g.name }}
                    <span
                      v-if="recentlyChangedGroups.has(g.name)"
                      class="ml-1.5 inline-block h-1.5 w-1.5 rounded-full bg-amber-500"
                      :title="t('admin.upstream.detail.pricing.changedRecently')"
                    />
                  </td>
                  <td class="py-2 pr-3 text-right tabular-nums text-gray-900 dark:text-gray-100">{{ g.ratio ?? '—' }}</td>
                  <td class="py-2 pr-3 text-right tabular-nums text-gray-900 dark:text-gray-100">{{ g.models?.length ?? 0 }}</td>
                  <td class="py-2 pr-3 text-right tabular-nums">
                    <span v-if="groupSupported" class="font-semibold text-gray-900 dark:text-gray-100">{{ formatUSD(groupUsageByName.get(g.name)?.cost_usd ?? 0) }}</span>
                    <span v-else class="text-gray-400">—</span>
                  </td>
                  <td class="py-2 text-right tabular-nums">
                    <span v-if="groupSupported" class="text-gray-500 dark:text-gray-400">{{ formatCNY(groupUsageByName.get(g.name)?.cost_cny ?? 0) }}</span>
                    <span v-else class="text-gray-400">—</span>
                  </td>
                </tr>
                <tr v-if="!groups.length">
                  <td colspan="5" class="py-8 text-center text-sm text-gray-400">—</td>
                </tr>
              </tbody>
            </table>
            <!-- sub2api 不支持分组消耗 -->
            <div
              v-if="!groupSupported"
              class="rounded-md bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:bg-amber-900/30 dark:text-amber-300"
              role="status"
            >
              {{ t('admin.upstream.usage.unsupportedGroup') }}
            </div>
            <!-- newapi 改名提示 -->
            <p v-else-if="provider.type === 'newapi'" class="text-[10px] text-gray-400">
              {{ t('admin.upstream.usage.renamedGroupNote') }}
            </p>
          </div>

          <!-- ===== 可用模型(搜索 + 分组/模型双视图) ===== -->
          <div v-else-if="activeTab === 'models'">
            <UpstreamModelsPanel :groups="groups" />
          </div>

          <!-- ===== Token ===== -->
          <div v-else-if="activeTab === 'tokens'" class="space-y-4">
            <!-- 创建表单 -->
            <form class="flex items-end gap-2" @submit.prevent="handleCreateToken">
              <div class="flex-1">
                <label class="input-label">{{ t('admin.upstream.detail.tokens.name') }}</label>
                <input
                  v-model="tokenForm.name"
                  type="text"
                  required
                  class="input"
                  :placeholder="t('admin.upstream.detail.tokens.name')"
                />
              </div>
              <div v-if="provider.type === 'newapi'" class="flex-1">
                <label class="input-label">{{ t('admin.upstream.detail.tokens.group') }}</label>
                <Select v-model="tokenForm.group" :options="groupOptions" />
              </div>
              <button
                type="submit"
                class="btn btn-primary cursor-pointer shrink-0"
                :disabled="creatingToken || !tokenForm.name.trim()"
              >
                <span v-if="creatingToken" class="flex items-center gap-1.5">
                  <span class="h-3.5 w-3.5 animate-spin rounded-full border-b-2 border-white" />
                </span>
                <span v-else>{{ t('admin.upstream.detail.tokens.create') }}</span>
              </button>
            </form>

            <!-- 消耗明细(实时密钥 ∪ breakdown) -->
            <div class="flex items-center justify-between">
              <UsageWindowSwitcher v-model="usageWindow" />
            </div>
            <UsageBreakdownTable
              :rows="keyRows"
              :supported="keySupported"
              :loading="usageLoading"
              :name-label="t('admin.upstream.usage.keyName')"
            />
          </div>

          <!-- ===== 关联帐号 ===== -->
          <div v-else-if="activeTab === 'accounts'">
            <table v-if="accounts.length" class="w-full text-sm">
              <tbody class="divide-y divide-gray-100 dark:divide-gray-800">
                <tr
                  v-for="acc in accounts"
                  :key="acc.id"
                  class="cursor-pointer hover:bg-gray-50 dark:hover:bg-gray-800/60"
                  @click="goAccount(acc)"
                >
                  <td class="py-2 pr-3 font-medium text-gray-900 dark:text-gray-100">{{ acc.name }}</td>
                  <td class="py-2 pr-3 text-gray-500 dark:text-gray-400">{{ acc.platform }}</td>
                  <td class="py-2 text-gray-500 dark:text-gray-400">{{ acc.status }}</td>
                </tr>
              </tbody>
            </table>
            <p v-else class="py-8 text-center text-sm text-gray-400">
              {{ t('admin.upstream.detail.accounts.empty') }}
            </p>
          </div>

          <!-- ===== 变更历史 ===== -->
          <!-- R2.4: ev.id / ev.type / ev.created_at / ev.detail / ev.notified -->
          <div v-else-if="activeTab === 'events'" class="space-y-3">
            <div v-if="eventsLoading" class="py-8 text-center">
              <div class="mx-auto h-6 w-6 animate-spin rounded-full border-b-2 border-primary-600" />
            </div>
            <template v-else>
              <div v-for="ev in events" :key="ev.id" class="flex gap-3 text-sm">
                <div
                  class="mt-1 h-2 w-2 shrink-0 rounded-full"
                  :class="eventDotClass(ev.type)"
                  aria-hidden="true"
                />
                <div class="min-w-0">
                  <div class="flex flex-wrap items-center gap-2">
                    <span class="font-medium text-gray-900 dark:text-gray-100">
                      {{ t(`admin.upstream.events.${ev.type}`, ev.type) }}
                    </span>
                    <span class="text-xs text-gray-400">{{ new Date(ev.created_at).toLocaleString() }}</span>
                    <span
                      v-if="ev.notified"
                      class="rounded bg-gray-100 px-1 text-[10px] text-gray-500 dark:bg-gray-800 dark:text-gray-400"
                    >
                      {{ t('admin.upstream.detail.events.notified') }}
                    </span>
                  </div>
                  <p class="break-all text-gray-600 dark:text-gray-300">{{ ev.summary }}</p>
                </div>
              </div>
              <p v-if="!events.length" class="py-8 text-center text-sm text-gray-400">
                {{ t('admin.upstream.detail.events.empty') }}
              </p>
              <button
                v-if="events.length && hasMoreEvents"
                type="button"
                class="btn btn-secondary w-full cursor-pointer"
                :disabled="loadingMore"
                @click="loadMoreEvents"
              >
                {{ t('admin.upstream.detail.events.loadMore') }}
              </button>
            </template>
          </div>

        </div>
      </section>
    </Transition>

    <!-- ===== Token 创建成功：key 一次性展示 ===== -->
    <BaseDialog
      :show="!!createdToken"
      :title="t('admin.upstream.detail.tokens.createdTitle')"
      width="normal"
      @close="createdToken = null"
    >
      <div class="space-y-3 text-sm">
        <p
          class="rounded-lg bg-amber-50 p-3 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300"
          role="alert"
        >
          {{ t('admin.upstream.detail.tokens.createdWarning') }}
        </p>
        <div>
          <label class="input-label">{{ t('admin.upstream.detail.tokens.apiUrl') }}</label>
          <code class="block break-all rounded bg-gray-100 p-2 font-mono text-xs dark:bg-gray-800 dark:text-gray-200">
            {{ createdToken?.api_base_url }}
          </code>
        </div>
        <div>
          <label class="input-label">Key</label>
          <code class="block break-all rounded bg-gray-100 p-2 font-mono text-xs dark:bg-gray-800 dark:text-gray-200">
            {{ createdToken?.token.key }}
          </code>
        </div>
        <button
          type="button"
          class="btn btn-primary w-full cursor-pointer"
          @click="copyTokenAll"
        >
          {{ t('admin.upstream.detail.tokens.copyAll') }}
        </button>
      </div>
    </BaseDialog>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import UpstreamStatusBadge from './UpstreamStatusBadge.vue'
import UsageSummaryCards from './UsageSummaryCards.vue'
import UsageWindowSwitcher from './UsageWindowSwitcher.vue'
import UsageBreakdownTable from './UsageBreakdownTable.vue'
import UpstreamModelsPanel from './UpstreamModelsPanel.vue'
import { mergeUsageRows, formatCNY, formatUSD } from './usageView'
import type { UsageWindow, MergedUsageRow, LiveScopeItem } from './usageView'
import { upstreamProvidersAPI } from '@/api/admin'
import type {
  UpstreamProvider,
  UpstreamChangeEvent,
  UpstreamLinkedAccount,
  UpstreamToken,
  UsageBreakdown,
} from '@/api/admin/upstreamProviders'
import { useClipboard } from '@/composables/useClipboard'
import { useAppStore } from '@/stores/app'

const props = defineProps<{ provider: UpstreamProvider | null }>()
const emit = defineEmits<{ close: []; changed: [] }>()

const { t } = useI18n()
const router = useRouter()
const { copyToClipboard } = useClipboard()
const appStore = useAppStore()

// ---- Tab ----
const tabs = ['overview', 'pricing', 'models', 'tokens', 'accounts', 'events'] as const
type Tab = (typeof tabs)[number]
const activeTab = ref<Tab>('overview')

// ---- 状态 ----
const tokens = ref<UpstreamToken[]>([])
const creatingToken = ref(false)
const tokenForm = ref({ name: '', group: '' })
const createdToken = ref<{ token: UpstreamToken; api_base_url: string } | null>(null)

const accounts = ref<UpstreamLinkedAccount[]>([])

const events = ref<UpstreamChangeEvent[]>([])
const eventsLoading = ref(false)
const hasMoreEvents = ref(false)
const loadingMore = ref(false)
const EVENTS_PAGE = 20

const relogining = ref(false)

// ---- 消耗(密钥/分组 breakdown) ----
// 默认窗口=今日(打开密钥管理/分组价格即看当日数据)
const usageWindow = ref<UsageWindow>('today')
// 缓存键 `${scope}:${window}` → 该 scope+window 的 breakdown(含 supported)
const breakdownCache = ref(new Map<string, UsageBreakdown>())
const usageLoading = ref(false)

const keyRows = ref<MergedUsageRow[]>([])
const keySupported = ref(true)
const groupUsageByName = ref(new Map<string, { cost_cny: number; cost_usd: number; requests: number }>())
const groupSupported = ref(true)

// ---- computed ----
const groups = computed(() => props.provider?.latest_snapshot?.groups ?? [])

const groupOptions = computed(() => [
  { label: '—', value: '' },
  ...groups.value.map((g) => ({ label: g.name, value: g.name })),
])

const balanceText = computed(() => {
  const b = props.provider?.latest_snapshot?.balance
  return b == null ? '—' : `$${b.toFixed(2)}`
})

// R2.4: snake_case — ev.detail?.group;近 7 天变更的分组高亮
const recentlyChangedGroups = computed(() => {
  const cutoff = Date.now() - 7 * 86_400_000
  const set = new Set<string>()
  for (const ev of events.value) {
    if (new Date(ev.created_at).getTime() < cutoff) continue
    const g = ev.detail?.group
    if (typeof g === 'string') set.add(g)
  }
  return set
})

// R2.2: 截图文件名从 last_error 中提取
const screenshotFile = computed(() => {
  const m = props.provider?.last_error?.match(/诊断截图: (\d{14}\.png)/)
  return m?.[1] ?? null
})

// ---- watch ----
watch(
  () => props.provider?.id,
  async (id) => {
    if (!id) return
    activeTab.value = 'overview'
    tokens.value = []
    accounts.value = []
    events.value = []
    hasMoreEvents.value = false
    // 消耗:换 provider 必清缓存与行,窗口回默认(今日),杜绝串数据
    breakdownCache.value = new Map()
    usageWindow.value = 'today'
    keyRows.value = []
    groupUsageByName.value = new Map()
    keySupported.value = true
    groupSupported.value = true
    // events 概览页就要（近 7 天高亮依赖），并行拉
    void loadEvents(id)
    void loadAccounts(id)
  },
  { immediate: true },
)

watch(activeTab, (tab) => {
  const id = props.provider?.id
  if (!id) return
  if (tab === 'tokens') void loadKeyUsage(id)
  if (tab === 'pricing') void loadGroupUsage(id)
})

// 窗口切换:重算当前 Tab(命中缓存则瞬时)
watch(usageWindow, () => {
  const id = props.provider?.id
  if (!id) return
  if (activeTab.value === 'tokens') void loadKeyUsage(id)
  if (activeTab.value === 'pricing') void loadGroupUsage(id)
})

// ---- loaders ----
async function loadTokens(id: number) {
  try {
    tokens.value = await upstreamProvidersAPI.listTokens(id)
  } catch {
    // 失败不弹 toast，列表显示空态即可
  }
}

async function loadAccounts(id: number) {
  try {
    accounts.value = await upstreamProvidersAPI.linkedAccounts(id)
  } catch {
    accounts.value = []
  }
}

async function loadEvents(id: number) {
  eventsLoading.value = true
  try {
    const list = await upstreamProvidersAPI.listEvents(id, { limit: EVENTS_PAGE })
    events.value = list
    hasMoreEvents.value = list.length === EVENTS_PAGE
  } catch {
    events.value = []
  } finally {
    eventsLoading.value = false
  }
}

// 取 breakdown(命中缓存则不请求)
async function fetchBreakdown(id: number, scope: 'key' | 'group', window: UsageWindow): Promise<UsageBreakdown | null> {
  const cacheKey = `${scope}:${window}`
  const hit = breakdownCache.value.get(cacheKey)
  if (hit) return hit
  try {
    const bd = await upstreamProvidersAPI.usage(id, { scope, window })
    breakdownCache.value.set(cacheKey, bd)
    return bd
  } catch {
    return null
  }
}

// 密钥明细:实时 tokens ∪ key breakdown
async function loadKeyUsage(id: number, reloadTokens = false) {
  usageLoading.value = true
  try {
    if (reloadTokens || tokens.value.length === 0) await loadTokens(id)
    const bd = await fetchBreakdown(id, 'key', usageWindow.value)
    keySupported.value = bd?.supported ?? true
    const live: LiveScopeItem[] = tokens.value.map((tok) => ({
      scope_key: upstreamTokenIdString(tok.id),
      scope_name: tok.name,
      meta: tok.group || undefined,
    }))
    keyRows.value = mergeUsageRows(live, bd?.items ?? [])
  } finally {
    usageLoading.value = false
  }
}

// 分组消耗:按组名取 breakdown 消耗(供价格表查;sub2api → supported:false)
async function loadGroupUsage(id: number) {
  usageLoading.value = true
  try {
    const bd = await fetchBreakdown(id, 'group', usageWindow.value)
    groupSupported.value = bd?.supported ?? true
    const m = new Map<string, { cost_cny: number; cost_usd: number; requests: number }>()
    for (const it of bd?.items ?? []) {
      m.set(it.scope_key, { cost_cny: it.cost_cny, cost_usd: it.cost_usd, requests: it.requests })
    }
    groupUsageByName.value = m
  } finally {
    usageLoading.value = false
  }
}

// token id → 字符串(与后端 scope_key 对齐:数字 id 转字符串)
function upstreamTokenIdString(id: unknown): string {
  return id == null ? '' : String(id)
}

// R2.4: 游标字段 before_created_at / before_id (snake_case)
async function loadMoreEvents() {
  const id = props.provider?.id
  if (!id || !events.value.length) return
  loadingMore.value = true
  try {
    const last = events.value[events.value.length - 1]
    const more = await upstreamProvidersAPI.listEvents(id, {
      limit: EVENTS_PAGE,
      before_created_at: last.created_at,
      before_id: last.id,
    })
    events.value = [...events.value, ...more]
    hasMoreEvents.value = more.length === EVENTS_PAGE
  } catch {
    appStore.showError(t('admin.upstream.loadFailed'))
  } finally {
    loadingMore.value = false
  }
}

// ---- actions ----
async function handleCreateToken() {
  const id = props.provider?.id
  if (!id || !tokenForm.value.name.trim()) return
  creatingToken.value = true
  try {
    createdToken.value = await upstreamProvidersAPI.createToken(id, {
      name: tokenForm.value.name.trim(),
      group: tokenForm.value.group || undefined,
    })
    tokenForm.value = { name: '', group: '' }
    void loadKeyUsage(id, true)
  } catch {
    appStore.showError(t('common.unknownError', 'Failed to create token'))
  } finally {
    creatingToken.value = false
  }
}

async function handleRelogin() {
  const id = props.provider?.id
  if (!id) return
  relogining.value = true
  try {
    await upstreamProvidersAPI.relogin(id)
    emit('changed')
  } catch {
    appStore.showError(t('admin.upstream.refreshFailed'))
  } finally {
    relogining.value = false
  }
}

// R2.2: blob 鉴权打开截图
async function openScreenshot() {
  const id = props.provider?.id
  if (!id || !screenshotFile.value) return
  try {
    const blob = await upstreamProvidersAPI.fetchDiagnostics(id, screenshotFile.value)
    const objectUrl = URL.createObjectURL(blob)
    window.open(objectUrl)
    // 短暂延迟后释放，让浏览器有时间打开
    setTimeout(() => URL.revokeObjectURL(objectUrl), 60_000)
  } catch {
    appStore.showError(t('admin.upstream.loadFailed'))
  }
}

function copy(text: string) {
  void copyToClipboard(text)
}

function copyTokenAll() {
  if (!createdToken.value) return
  void copyToClipboard(
    `${createdToken.value.api_base_url}\n${createdToken.value.token.key ?? ''}`,
  )
}

function goAccount(acc: UpstreamLinkedAccount) {
  void router.push({ path: '/admin/accounts', query: { search: acc.name } })
  emit('close')
}

// 事件类型 dot 颜色
function eventDotClass(type: string): string {
  if (type === 'balance_low' || type === 'refresh_failed' || type === 'credential_error')
    return 'bg-red-500'
  if (type === 'price_changed') return 'bg-amber-500'
  return 'bg-gray-400'
}
</script>

<style scoped>
.drawer-fade-enter-active,
.drawer-fade-leave-active {
  transition: opacity 200ms ease;
}
.drawer-fade-enter-from,
.drawer-fade-leave-to {
  opacity: 0;
}
.drawer-slide-enter-active,
.drawer-slide-leave-active {
  transition: transform 250ms ease;
}
.drawer-slide-enter-from,
.drawer-slide-leave-to {
  transform: translateX(100%);
}
@media (prefers-reduced-motion: reduce) {
  .drawer-fade-enter-active,
  .drawer-fade-leave-active,
  .drawer-slide-enter-active,
  .drawer-slide-leave-active {
    transition: none;
  }
}
</style>
