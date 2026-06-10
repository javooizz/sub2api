<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex items-center justify-between gap-3">
          <h1 class="text-lg font-semibold text-gray-900 dark:text-white">
            {{ t('admin.upstream.title') }}
          </h1>
          <div class="flex items-center gap-2">
            <button
              class="btn btn-secondary cursor-pointer"
              @click="showNotifyDialog = true"
            >
              {{ t('admin.upstream.notifySettings') }}
            </button>
            <button
              class="btn btn-secondary cursor-pointer"
              @click="showSettingsDialog = true"
            >
              {{ t('admin.upstream.collectSettings') }}
            </button>
            <button
              class="btn btn-primary cursor-pointer"
              @click="openCreate"
            >
              {{ t('admin.upstream.addProvider') }}
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <!-- 加载态 -->
        <div v-if="loading" class="flex items-center justify-center py-16">
          <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600" />
        </div>

        <!-- 空态 -->
        <div
          v-else-if="providers.length === 0"
          class="flex flex-col items-center justify-center py-16 text-gray-400"
        >
          <p class="text-sm">{{ t('admin.upstream.empty') }}</p>
        </div>

        <!-- 表格（.table-wrapper 提供横向滚动，避免被 .table-scroll-container 的 overflow-hidden 裁掉操作列） -->
        <div v-else class="table-wrapper">
          <table class="data-table w-full">
          <thead>
            <tr>
              <th class="text-left">{{ t('admin.upstream.columns.name') }}</th>
              <th class="text-left">{{ t('admin.upstream.columns.siteUrl') }}</th>
              <th class="text-right">{{ t('admin.upstream.columns.balance') }}</th>
              <th class="text-left">{{ t('admin.upstream.columns.status') }}</th>
              <th class="text-right">{{ t('admin.upstream.columns.groups') }}</th>
              <th class="text-right">{{ t('admin.upstream.columns.models') }}</th>
              <th class="text-right">{{ t('admin.upstream.columns.usage') }}</th>
              <th class="text-left">{{ t('admin.upstream.columns.lastRefreshed') }}</th>
              <th class="text-left">{{ t('admin.upstream.columns.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="p in providers"
              :key="p.id"
              class="transition-colors duration-150 hover:bg-gray-50 dark:hover:bg-gray-800/60"
            >
              <!-- 名称 + 类型 -->
              <td>
                <div class="flex items-center gap-2">
                  <span class="font-medium text-gray-900 dark:text-white">{{ p.name }}</span>
                  <span
                    class="rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-600 dark:bg-gray-700 dark:text-gray-300"
                  >
                    {{ t(`admin.upstream.type.${p.type}`) }}
                  </span>
                </div>
              </td>

              <!-- 官网地址 -->
              <td class="max-w-[220px] truncate text-sm text-gray-500 dark:text-gray-400">
                {{ p.site_url }}
              </td>

              <!-- 余额(低于阈值红色) -->
              <td
                class="text-right tabular-nums"
                :class="balanceClass(p)"
              >
                {{ balanceText(p) }}
              </td>

              <!-- 状态徽标 -->
              <td>
                <UpstreamStatusBadge :status="p.status" />
              </td>

              <!-- 组数 -->
              <td class="text-right tabular-nums text-sm text-gray-700 dark:text-gray-300">
                {{ p.latest_snapshot?.groups?.length ?? '—' }}
              </td>

              <!-- 模型数 -->
              <td class="text-right tabular-nums text-sm text-gray-700 dark:text-gray-300">
                {{ modelCount(p) }}
              </td>

              <!-- 消耗(本月 $ 主显 + 小字今/周/史 $;hover 看 ¥实付与请求;无数据显 —) -->
              <td class="text-right tabular-nums">
                <template v-if="p.usage_summary">
                  <div
                    class="font-semibold text-gray-900 dark:text-gray-100"
                    :title="usageHint(p)"
                  >
                    {{ formatUSD(p.usage_summary.month.cost_usd) }}
                    <span class="text-[10px] font-normal text-gray-400">{{ t('admin.upstream.usage.month') }}</span>
                  </div>
                  <div class="text-[10px] text-gray-400">
                    {{ t('admin.upstream.usage.today') }} {{ formatUSD(p.usage_summary.today.cost_usd) }} ·
                    {{ t('admin.upstream.usage.week') }} {{ formatUSD(p.usage_summary.week.cost_usd) }} ·
                    {{ t('admin.upstream.usage.total') }} {{ formatUSD(p.usage_summary.total.cost_usd) }}
                  </div>
                </template>
                <span v-else class="text-gray-400">—</span>
              </td>

              <!-- 最后刷新 -->
              <td class="text-sm text-gray-500 dark:text-gray-400">
                {{ formatTime(p.last_refreshed_at) }}
              </td>

              <!-- 操作列 -->
              <td>
                <div class="flex items-center gap-1">
                  <!-- 刷新 -->
                  <button
                    class="btn-icon cursor-pointer"
                    :disabled="refreshingIds.has(p.id)"
                    :title="t('admin.upstream.actions.refresh')"
                    @click="handleRefresh(p)"
                  >
                    <Icon
                      name="refresh"
                      size="sm"
                      :class="{ 'animate-spin': refreshingIds.has(p.id) }"
                    />
                  </button>

                  <!-- 详情 -->
                  <button
                    class="btn-icon cursor-pointer"
                    :title="t('admin.upstream.actions.detail')"
                    @click="openDetail(p)"
                  >
                    <Icon name="eye" size="sm" />
                  </button>

                  <!-- 编辑 -->
                  <button
                    class="btn-icon cursor-pointer"
                    :title="t('admin.upstream.actions.edit')"
                    @click="openEdit(p)"
                  >
                    <Icon name="edit" size="sm" />
                  </button>

                  <!-- 删除 -->
                  <button
                    class="btn-icon cursor-pointer text-red-500 hover:text-red-700 dark:text-red-400 dark:hover:text-red-300"
                    :title="t('admin.upstream.actions.delete')"
                    @click="confirmDelete(p)"
                  >
                    <Icon name="trash" size="sm" />
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
          </table>
        </div>
      </template>
    </TablePageLayout>
  </AppLayout>

  <!-- 创建/编辑对话框 -->
  <UpstreamProviderFormDialog
    :show="showFormDialog"
    :provider="editingProvider"
    @close="showFormDialog = false"
    @saved="load"
  />

  <!-- 详情抽屉(占位,Task 19 替换) -->
  <UpstreamProviderDetailDrawer
    :provider="detailProvider"
    @close="detailProvider = null"
    @changed="load"
  />

  <!-- 通知渠道对话框(占位,Task 20 替换) -->
  <NotifyChannelsDialog
    :show="showNotifyDialog"
    @close="showNotifyDialog = false"
  />

  <!-- 采集设置对话框(占位,Task 20 替换) -->
  <UpstreamSettingsDialog
    :show="showSettingsDialog"
    @close="showSettingsDialog = false"
  />

  <!-- 删除确认 -->
  <ConfirmDialog
    :show="!!deletingProvider"
    :title="t('admin.upstream.actions.delete')"
    :message="t('admin.upstream.deleteConfirm', { name: deletingProvider?.name ?? '' })"
    :confirm-text="t('common.delete')"
    :danger="true"
    @confirm="handleDelete"
    @cancel="deletingProvider = null"
  />
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { upstreamProvidersAPI } from '@/api/admin'
import type { UpstreamProvider } from '@/api/admin/upstreamProviders'
import UpstreamProviderFormDialog from '@/components/admin/upstream/UpstreamProviderFormDialog.vue'
import UpstreamProviderDetailDrawer from '@/components/admin/upstream/UpstreamProviderDetailDrawer.vue'
import NotifyChannelsDialog from '@/components/admin/upstream/NotifyChannelsDialog.vue'
import UpstreamSettingsDialog from '@/components/admin/upstream/UpstreamSettingsDialog.vue'
import UpstreamStatusBadge from '@/components/admin/upstream/UpstreamStatusBadge.vue'
import { formatCNY, formatUSD, formatRequests } from '@/components/admin/upstream/usageView'

const { t } = useI18n()
const appStore = useAppStore()

// 状态
const loading = ref(false)
const providers = ref<UpstreamProvider[]>([])

// 对话框状态
const showFormDialog = ref(false)
const editingProvider = ref<UpstreamProvider | null>(null)
const detailProvider = ref<UpstreamProvider | null>(null)
const showNotifyDialog = ref(false)
const showSettingsDialog = ref(false)
const deletingProvider = ref<UpstreamProvider | null>(null)

// 刷新中的 ID 集合
const refreshingIds = ref(new Set<number>())

// 加载列表
async function load() {
  loading.value = true
  try {
    providers.value = await upstreamProvidersAPI.list()
  } catch (err: unknown) {
    const e = err as { response?: { data?: { detail?: string } } }
    appStore.showError(e.response?.data?.detail ?? t('admin.upstream.loadFailed'))
  } finally {
    loading.value = false
  }
}

// 余额显示
function balanceText(p: UpstreamProvider): string {
  const b = p.latest_snapshot?.balance
  return b == null ? '—' : `$${b.toFixed(2)}`
}

// 余额低于阈值时红色
function balanceClass(p: UpstreamProvider): string {
  const b = p.latest_snapshot?.balance
  if (b != null && p.balance_threshold != null && b < p.balance_threshold) {
    return 'text-red-600 dark:text-red-400 font-semibold'
  }
  return 'text-gray-700 dark:text-gray-300'
}

// 去重统计模型数
function modelCount(p: UpstreamProvider): string {
  const groups = p.latest_snapshot?.groups
  if (!groups) return '—'
  const set = new Set<string>()
  groups.forEach((g) => (g.models ?? []).forEach((m) => set.add(m)))
  return String(set.size)
}

// 时间格式化
function formatTime(v: string | null): string {
  return v ? new Date(v).toLocaleString() : '—'
}

// 列表消耗格悬停提示:本月 ¥ 实付 + 请求数
function usageHint(p: UpstreamProvider): string {
  const m = p.usage_summary?.month
  if (!m) return ''
  return `${t('admin.upstream.usage.month')} ${formatCNY(m.cost_cny)} ${t('admin.upstream.usage.paid')} · ${formatRequests(m.requests)} ${t('admin.upstream.usage.requests')}`
}

// 打开创建
function openCreate() {
  editingProvider.value = null
  showFormDialog.value = true
}

// 打开编辑
function openEdit(p: UpstreamProvider) {
  editingProvider.value = p
  showFormDialog.value = true
}

// 打开详情(获取完整数据)
async function openDetail(p: UpstreamProvider) {
  try {
    detailProvider.value = await upstreamProvidersAPI.getById(p.id)
  } catch {
    // 降级:使用列表中的数据
    detailProvider.value = p
  }
}

// 触发删除确认
function confirmDelete(p: UpstreamProvider) {
  deletingProvider.value = p
}

// 执行删除
async function handleDelete() {
  if (!deletingProvider.value) return
  try {
    await upstreamProvidersAPI.remove(deletingProvider.value.id)
    appStore.showSuccess(t('admin.upstream.deleteSuccess'))
  } catch (err: unknown) {
    const e = err as { response?: { data?: { detail?: string } } }
    appStore.showError(e.response?.data?.detail ?? t('admin.upstream.deleteFailed'))
  } finally {
    deletingProvider.value = null
    await load()
  }
}

// 手动刷新单个 provider
async function handleRefresh(p: UpstreamProvider) {
  // 触发刷新:需重建 Set 触发 Vue 响应式
  refreshingIds.value = new Set([...refreshingIds.value, p.id])
  try {
    await upstreamProvidersAPI.refresh(p.id)
    await load()
  } catch (err: unknown) {
    const e = err as { response?: { status?: number; data?: { detail?: string } } }
    if (e.response?.status === 409) {
      appStore.showError(t('admin.upstream.refreshConflict'))
    } else {
      appStore.showError(e.response?.data?.detail ?? t('admin.upstream.refreshFailed'))
      // 刷新失败也刷列表(状态/last_error 已更新)
      await load()
    }
  } finally {
    const next = new Set(refreshingIds.value)
    next.delete(p.id)
    refreshingIds.value = next
  }
}

onMounted(() => {
  void load()
})
</script>
