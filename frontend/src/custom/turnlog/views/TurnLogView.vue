<template>
  <AppLayout>
    <TablePageLayout>
      <template #actions>
        <div class="flex justify-end">
          <button type="button" class="btn btn-primary" @click="openCapture">
            <Icon name="plus" size="sm" />
            {{ t('admin.turnLog.capture.open') }}
          </button>
        </div>
      </template>
      <template #filters>
        <div class="card p-4 sm:p-6">
          <div class="flex flex-wrap items-end gap-4">
            <div class="w-full sm:w-auto sm:min-w-[180px]">
              <label class="input-label">{{ t('admin.turnLog.filters.accountId') }}</label>
              <input v-model.trim="filters.account_id" type="text" inputmode="numeric" class="input" @keyup.enter="load" />
            </div>
            <div class="w-full sm:w-auto sm:min-w-[160px]">
              <label class="input-label">{{ t('admin.turnLog.filters.status') }}</label>
              <Select v-model="filters.status_code" :options="statusOptions" @change="load" />
            </div>
            <button type="button" class="btn btn-primary" :disabled="loading" @click="load">{{ t('common.search') }}</button>
            <button type="button" class="btn btn-secondary" :disabled="loading" @click="resetFilters">{{ t('common.reset') }}</button>
          </div>
          <div class="mt-5 flex flex-wrap items-end gap-4 border-t border-gray-200 pt-4 dark:border-dark-700">
            <div class="w-full sm:w-auto sm:min-w-[220px]">
              <label class="input-label">{{ t('admin.turnLog.config.retentionDays') }}</label>
              <input v-model.number="retentionDays" type="number" min="1" max="3650" class="input" />
            </div>
            <button type="button" class="btn btn-secondary" :disabled="configSaving" @click="saveConfig">
              {{ configSaving ? t('common.saving') : t('common.save') }}
            </button>
            <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.turnLog.config.range') }}</span>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="logs" :loading="loading" row-key="id">
          <template #cell-created_at="{ value }"><span class="whitespace-nowrap">{{ formatTime(value) }}</span></template>
          <template #cell-account="{ row }">
            <div class="min-w-0 max-w-[240px]">
              <div class="truncate font-medium text-gray-900 dark:text-white">{{ row.account_name || '—' }}</div>
              <div class="text-xs text-gray-500">#{{ row.account_id }}</div>
            </div>
          </template>
          <template #cell-status_code="{ value }">
            <span class="rounded bg-amber-100 px-2 py-1 font-mono text-xs font-semibold text-amber-800 dark:bg-amber-900/30 dark:text-amber-300">{{ value }}</span>
          </template>
          <template #cell-payload="{ row }">
            <span class="text-xs text-gray-500 dark:text-gray-400">
              {{ row.body_truncated ? t('admin.turnLog.truncated') : row.body_complete ? t('admin.turnLog.complete') : t('admin.turnLog.partial') }}
            </span>
          </template>
          <template #cell-actions="{ row }">
            <button type="button" class="inline-flex items-center gap-1 font-medium text-primary-600" @click="openDetail(row.id)">
              <Icon name="eye" size="sm" />{{ t('common.view') }}
            </button>
          </template>
          <template #empty>
            <div class="py-10 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('admin.turnLog.empty') }}</div>
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination v-if="total > 0" :total="total" :page="page" :page-size="pageSize" @update:page="onPageChange" @update:pageSize="onPageSizeChange" />
      </template>
    </TablePageLayout>

    <BaseDialog :show="detailVisible" :title="t('admin.turnLog.detail.title')" width="wide" @close="detailVisible = false">
      <div v-if="detailLoading" class="py-12 text-center text-sm text-gray-500">{{ t('common.loading') }}</div>
      <div v-else-if="detail" class="space-y-5 py-2">
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
          <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900"><div class="text-xs text-gray-500">{{ t('admin.turnLog.columns.account') }}</div><div class="mt-1 font-medium">{{ detail.account_name || '—' }} (#{{ detail.account_id }})</div></div>
          <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900"><div class="text-xs text-gray-500">{{ t('admin.turnLog.columns.status') }}</div><div class="mt-1 font-mono font-semibold">{{ detail.status_code }}</div></div>
          <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900"><div class="text-xs text-gray-500">{{ t('admin.turnLog.columns.time') }}</div><div class="mt-1">{{ formatTime(detail.created_at) }}</div></div>
        </div>
        <section><h4 class="mb-1.5 text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.turnLog.detail.headers') }}</h4><pre class="max-h-72 overflow-auto rounded-xl bg-gray-50 p-4 font-mono text-xs leading-relaxed dark:bg-dark-900">{{ JSON.stringify(detail.response_headers, null, 2) }}</pre></section>
        <section><h4 class="mb-1.5 text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.turnLog.detail.body') }}</h4><pre class="max-h-[30rem] overflow-auto rounded-xl bg-gray-50 p-4 font-mono text-xs leading-relaxed dark:bg-dark-900">{{ detail.response_body || '—' }}</pre></section>
        <p v-if="detail.headers_truncated || detail.body_truncated || !detail.body_complete" class="text-xs text-amber-600 dark:text-amber-400">{{ t('admin.turnLog.detail.truncatedHint') }}</p>
      </div>
    </BaseDialog>

    <BaseDialog :show="captureVisible" :title="t('admin.turnLog.capture.title')" width="extra-wide" :close-on-escape="!captureLoading" :show-close-button="!captureLoading" @close="closeCapture">
      <form id="turn-log-capture-form" class="space-y-5" @submit.prevent="submitCapture">
        <div class="grid gap-4 sm:grid-cols-2">
          <div>
            <label class="input-label mb-1.5 block">{{ t('admin.turnLog.capture.account') }}</label>
            <Select v-model="captureAccountID" :options="captureAccountOptions" :loading="captureAccountsLoading" :placeholder="t('admin.turnLog.capture.accountPlaceholder')" :disabled="captureLoading" :searchable="'auto'" :aria-label="t('admin.turnLog.capture.account')" @change="onCaptureAccountChange" />
          </div>
          <Input v-model="captureModel" :label="t('admin.turnLog.capture.model')" :disabled="captureLoading" required />
        </div>
        <div>
          <label class="input-label mb-1.5 block">{{ t('admin.turnLog.capture.ip') }}</label>
          <Select v-model="captureProxyID" :options="captureProxyOptions" :disabled="captureLoading || !captureAccountID" :aria-label="t('admin.turnLog.capture.ip')" />
        </div>
        <p v-if="captureError" role="alert" class="border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-900 dark:bg-red-950/30 dark:text-red-300">{{ captureError }}</p>
        <section v-if="captureResult" class="space-y-4 border border-gray-200 p-4 dark:border-dark-700">
          <div class="flex items-center gap-3"><span class="text-xs text-gray-500">{{ t('admin.turnLog.capture.status') }}</span><span class="font-mono font-semibold">{{ captureResult.status_code }}</span></div>
          <div><h4 class="mb-1.5 text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.turnLog.detail.headers') }}</h4><pre class="max-h-72 overflow-auto rounded-xl bg-gray-50 p-4 font-mono text-xs leading-relaxed dark:bg-dark-900">{{ JSON.stringify(captureResult.response_headers, null, 2) }}</pre></div>
          <div><h4 class="mb-1.5 text-xs font-bold uppercase tracking-wider text-gray-400">{{ t('admin.turnLog.detail.body') }}</h4><pre class="max-h-[30rem] overflow-auto rounded-xl bg-gray-50 p-4 font-mono text-xs leading-relaxed dark:bg-dark-900">{{ captureResult.response_body || '—' }}</pre></div>
          <p v-if="captureResult.headers_truncated || captureResult.body_truncated" class="text-xs text-amber-600 dark:text-amber-400">{{ t('admin.turnLog.capture.truncated') }}</p>
        </section>
      </form>
      <template #footer>
        <button type="button" class="btn btn-secondary" :disabled="captureLoading" @click="closeCapture">{{ t('common.cancel') }}</button>
        <button type="submit" form="turn-log-capture-form" class="btn btn-primary" :disabled="!captureAccountID || !captureModel.trim() || captureLoading">
          <Icon name="play" size="sm" :class="{ 'animate-pulse': captureLoading }" />
          {{ captureLoading ? t('admin.turnLog.capture.loading') : t('admin.turnLog.capture.submit') }}
        </button>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import Input from '@/components/common/Input.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import type { Column } from '@/components/common/types'
import { captureTurnState, getTurnLog, getTurnLogConfig, listTurnLogOAuthAccounts, listTurnLogs, updateTurnLogConfig } from '../api'
import type { TurnLog, TurnLogCaptureResult, TurnLogOAuthAccount } from '../types'

const { t } = useI18n()
const loading = ref(false)
const configSaving = ref(false)
const logs = ref<TurnLog[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const retentionDays = ref(30)
const filters = reactive({ account_id: '', status_code: '' })
const detailVisible = ref(false)
const detailLoading = ref(false)
const detail = ref<TurnLog | null>(null)
const captureVisible = ref(false)
const captureLoading = ref(false)
const captureAccountsLoading = ref(false)
const captureError = ref('')
const captureAccounts = ref<TurnLogOAuthAccount[]>([])
const captureAccountID = ref<number | null>(null)
const captureProxyID = ref<number | null>(null)
const captureModel = ref('gpt-6-astra')
const captureResult = ref<TurnLogCaptureResult | null>(null)

const columns = computed<Column[]>(() => [
  { key: 'created_at', label: t('admin.turnLog.columns.time') },
  { key: 'account', label: t('admin.turnLog.columns.account') },
  { key: 'status_code', label: t('admin.turnLog.columns.status') },
  { key: 'payload', label: t('admin.turnLog.columns.payload') },
  { key: 'actions', label: t('common.actions') },
])

const statusOptions = computed(() => [
  { value: '', label: t('common.all') },
  { value: '217', label: '217' },
  { value: '292', label: '292' },
])

const captureAccountOptions = computed(() => captureAccounts.value.map(account => ({ value: account.id, label: account.name || `#${account.id}` })))
const selectedCaptureAccount = computed(() => captureAccounts.value.find(account => account.id === captureAccountID.value))
const captureProxyOptions = computed(() => [
  { value: null, label: t('admin.turnLog.capture.accountDefaultIp') },
  { value: 0, label: t('admin.turnLog.capture.directIp') },
  ...(selectedCaptureAccount.value?.proxies ?? []).map(proxy => ({ value: proxy.id, label: `${proxy.name || proxy.host} (${proxy.host}:${proxy.port})` })),
])

async function load() {
  loading.value = true
  try {
    const result = await listTurnLogs({ page: page.value, page_size: pageSize.value, account_id: filters.account_id || undefined, status_code: filters.status_code || undefined })
    logs.value = result.items
    total.value = result.total
  } finally {
    loading.value = false
  }
}

async function loadConfig() {
  retentionDays.value = (await getTurnLogConfig()).retention_days
}

async function saveConfig() {
  if (retentionDays.value < 1 || retentionDays.value > 3650) return
  configSaving.value = true
  try {
    retentionDays.value = (await updateTurnLogConfig(retentionDays.value)).retention_days
  } finally {
    configSaving.value = false
  }
}

function resetFilters() { filters.account_id = ''; filters.status_code = ''; page.value = 1; load() }
function onPageChange(next: number) { page.value = next; load() }
function onPageSizeChange(next: number) { pageSize.value = next; page.value = 1; load() }

async function openDetail(id: number) {
  detailVisible.value = true
  detailLoading.value = true
  try { detail.value = await getTurnLog(id) } finally { detailLoading.value = false }
}

async function openCapture() {
  captureVisible.value = true
  captureError.value = ''
  captureResult.value = null
  captureModel.value = 'gpt-6-astra'
  captureAccountID.value = null
  captureProxyID.value = null
  captureAccountsLoading.value = true
  try {
    captureAccounts.value = await listTurnLogOAuthAccounts()
  } catch {
    captureError.value = t('admin.turnLog.capture.loadAccountsFailed')
  } finally {
    captureAccountsLoading.value = false
  }
}

function onCaptureAccountChange() {
  captureProxyID.value = null
  captureResult.value = null
  captureError.value = ''
}

async function submitCapture() {
  if (!captureAccountID.value || !captureModel.value.trim()) return
  captureLoading.value = true
  captureError.value = ''
  captureResult.value = null
  try {
    captureResult.value = await captureTurnState(captureAccountID.value, captureModel.value.trim(), captureProxyID.value)
  } catch {
    captureError.value = t('admin.turnLog.capture.failed')
  } finally {
    captureLoading.value = false
  }
}

function closeCapture() {
  if (!captureLoading.value) captureVisible.value = false
}

function formatTime(value: string) { return new Date(value).toLocaleString() }
onMounted(() => { loadConfig(); load() })
</script>
