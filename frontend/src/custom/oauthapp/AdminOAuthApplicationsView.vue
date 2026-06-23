<template>
  <AppLayout>
    <div class="mx-auto max-w-7xl space-y-5">
      <header class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <div class="flex items-center gap-2">
            <Icon name="key" size="lg" class="text-primary-500" />
            <h1 class="text-lg font-semibold text-gray-900 dark:text-white">应用管理</h1>
          </div>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">管理第三方应用授权。</p>
        </div>
        <div class="flex flex-wrap gap-2">
          <button type="button" class="btn btn-secondary" :disabled="loading" @click="loadApplications">
            <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
            <span>刷新</span>
          </button>
          <button type="button" class="btn btn-primary" @click="openCreateDialog">
            <Icon name="plus" size="sm" />
            <span>新建应用</span>
          </button>
        </div>
      </header>

      <div v-if="errorMessage" class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-300">
        {{ errorMessage }}
      </div>

      <section class="overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div v-if="loading" class="flex justify-center py-12">
          <LoadingSpinner size="lg" />
        </div>

        <div v-else-if="applications.length === 0" class="py-12 text-center">
          <Icon name="key" size="xl" class="mx-auto text-gray-300 dark:text-dark-500" />
          <p class="mt-3 text-sm text-gray-500 dark:text-dark-400">暂无应用</p>
        </div>

        <div v-else class="overflow-x-auto">
          <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
            <thead class="bg-gray-50 dark:bg-dark-700/60">
              <tr>
                <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-300">应用</th>
                <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-300">AccessKey</th>
                <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-300">白名单域名</th>
                <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-300">状态</th>
                <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-300">更新时间</th>
                <th class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-300">操作</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="app in applications" :key="app.id" class="hover:bg-gray-50 dark:hover:bg-dark-700/40">
                <td class="px-4 py-3">
                  <div class="font-medium text-gray-900 dark:text-white">{{ app.name }}</div>
                  <div class="text-xs text-gray-500 dark:text-dark-400">创建于 {{ formatDateTime(app.createdAt) }}</div>
                </td>
                <td class="px-4 py-3">
                  <div class="flex items-center gap-2">
                    <code class="rounded bg-gray-100 px-2 py-1 text-xs text-gray-700 dark:bg-dark-700 dark:text-dark-200">{{ app.accessKey }}</code>
                    <button type="button" class="text-gray-400 hover:text-gray-700 dark:hover:text-dark-200" title="复制" @click="copyText(app.accessKey)">
                      <Icon name="copy" size="sm" />
                    </button>
                  </div>
                </td>
                <td class="px-4 py-3">
                  <div class="flex max-w-sm flex-wrap gap-1">
                    <span
                      v-for="domain in app.allowedDomains"
                      :key="domain"
                      class="rounded-full bg-gray-100 px-2 py-1 text-xs text-gray-700 dark:bg-dark-700 dark:text-dark-200"
                    >
                      {{ domain }}
                    </span>
                  </div>
                </td>
                <td class="px-4 py-3">
                  <span :class="statusClass(app.status)">
                    {{ app.status === 'enabled' ? '启用' : '停用' }}
                  </span>
                </td>
                <td class="px-4 py-3 text-sm text-gray-500 dark:text-dark-400">{{ formatDateTime(app.updatedAt) }}</td>
                <td class="px-4 py-3">
                  <div class="flex justify-end gap-1">
                    <button type="button" class="rounded-lg p-1.5 text-gray-500 hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-dark-700 dark:hover:text-dark-200" title="编辑" @click="openEditDialog(app)">
                      <Icon name="edit" size="sm" />
                    </button>
                    <button type="button" class="rounded-lg p-1.5 text-gray-500 hover:bg-blue-50 hover:text-blue-600 dark:hover:bg-blue-900/20 dark:hover:text-blue-300" title="重置密钥" @click="openResetDialog(app)">
                      <Icon name="refresh" size="sm" />
                    </button>
                    <button type="button" class="rounded-lg p-1.5 text-gray-500 hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-300" title="删除" @click="openDeleteDialog(app)">
                      <Icon name="trash" size="sm" />
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </div>

    <BaseDialog :show="formDialogOpen" :title="editingApplication ? '编辑应用' : '新建应用'" width="wide" @close="closeFormDialog">
      <form id="oauth-application-form" class="space-y-4" @submit.prevent="saveApplication">
        <div>
          <label class="input-label">应用名称</label>
          <input v-model.trim="form.name" class="input" type="text" required placeholder="请输入应用名称" />
        </div>
        <div>
          <label class="input-label">白名单域名</label>
          <textarea
            v-model="domainsText"
            class="input min-h-[120px]"
            rows="5"
            required
            placeholder="example.com&#10;*.example.com"
          ></textarea>
          <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">每行一个域名，支持 *.example.com。</p>
        </div>
        <div class="flex items-center justify-between rounded-xl border border-gray-200 px-4 py-3 dark:border-dark-700">
          <div>
            <div class="text-sm font-medium text-gray-900 dark:text-white">启用应用</div>
            <div class="text-xs text-gray-500 dark:text-dark-400">停用后不可发起授权或换取 token。</div>
          </div>
          <Toggle v-model="form.enabled" />
        </div>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="closeFormDialog">取消</button>
          <button type="submit" form="oauth-application-form" class="btn btn-primary" :disabled="saving">
            <Icon v-if="saving" name="refresh" size="sm" class="animate-spin" />
            <Icon v-else name="check" size="sm" />
            <span>保存</span>
          </button>
        </div>
      </template>
    </BaseDialog>

    <BaseDialog :show="secretDialogOpen" title="保存密钥" width="normal" @close="closeSecretDialog">
      <div class="space-y-4">
        <p class="text-sm text-gray-600 dark:text-gray-300">AccessSecret 只显示一次，请及时复制。</p>
        <div class="space-y-2">
          <label class="input-label">AccessKey</label>
          <div class="flex gap-2">
            <code class="min-w-0 flex-1 break-all rounded-lg bg-gray-100 px-3 py-2 text-xs text-gray-700 dark:bg-dark-700 dark:text-dark-200">{{ secretPayload?.application.accessKey }}</code>
            <button type="button" class="btn btn-secondary" @click="copyText(secretPayload?.application.accessKey || '')">复制</button>
          </div>
        </div>
        <div class="space-y-2">
          <label class="input-label">AccessSecret</label>
          <div class="flex gap-2">
            <code class="min-w-0 flex-1 break-all rounded-lg bg-gray-100 px-3 py-2 text-xs text-gray-700 dark:bg-dark-700 dark:text-dark-200">{{ secretPayload?.accessSecret }}</code>
            <button type="button" class="btn btn-secondary" @click="copyText(secretPayload?.accessSecret || '')">复制</button>
          </div>
        </div>
      </div>
      <template #footer>
        <button type="button" class="btn btn-primary" @click="closeSecretDialog">我已保存</button>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="Boolean(confirmAction)"
      :title="confirmTitle"
      :message="confirmMessage"
      confirm-text="确认"
      cancel-text="取消"
      danger
      @confirm="confirmPendingAction"
      @cancel="confirmAction = null"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime } from '@/utils/format'
import {
  createOAuthApplication,
  deleteOAuthApplication,
  listOAuthApplications,
  resetOAuthApplicationSecret,
  updateOAuthApplication,
} from './adminApi'
import type { OAuthApplication, OAuthApplicationFormPayload, OAuthApplicationSecretResponse } from './types'

type ConfirmAction = 'delete' | 'reset'

const appStore = useAppStore()
const applications = ref<OAuthApplication[]>([])
const loading = ref(false)
const saving = ref(false)
const errorMessage = ref('')
const formDialogOpen = ref(false)
const editingApplication = ref<OAuthApplication | null>(null)
const domainsText = ref('')
const secretDialogOpen = ref(false)
const secretPayload = ref<OAuthApplicationSecretResponse | null>(null)
const confirmAction = ref<ConfirmAction | null>(null)
const pendingApplication = ref<OAuthApplication | null>(null)

const form = reactive({
  name: '',
  enabled: true,
})

const confirmTitle = computed(() => {
  if (confirmAction.value === 'reset') return '重置密钥'
  return '删除应用'
})

const confirmMessage = computed(() => {
  const name = pendingApplication.value?.name || '当前应用'
  if (confirmAction.value === 'reset') return `确认重置 ${name} 的 AccessSecret？`
  return `确认删除 ${name}？`
})

/**
 * 读取应用记录，并把最新结果保持在页面上。
 */
async function loadApplications(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    applications.value = await listOAuthApplications()
  } catch (error: unknown) {
    errorMessage.value = extractApiErrorMessage(error, '应用列表读取失败')
  } finally {
    loading.value = false
  }
}

/**
 * 打开新建弹窗，并默认启用应用。
 */
function openCreateDialog(): void {
  editingApplication.value = null
  form.name = ''
  form.enabled = true
  domainsText.value = ''
  formDialogOpen.value = true
}

/**
 * 使用当前应用值打开编辑弹窗。
 *
 * @param app - 表格中选中的应用
 */
function openEditDialog(app: OAuthApplication): void {
  editingApplication.value = app
  form.name = app.name
  form.enabled = app.status === 'enabled'
  domainsText.value = app.allowedDomains.join('\n')
  formDialogOpen.value = true
}

function closeFormDialog(): void {
  if (!saving.value) formDialogOpen.value = false
}

function closeSecretDialog(): void {
  secretDialogOpen.value = false
  secretPayload.value = null
}

function buildPayload(): OAuthApplicationFormPayload {
  return {
    name: form.name.trim(),
    allowedDomains: domainsText.value.split(/\r?\n/).map((item) => item.trim()).filter(Boolean),
    status: form.enabled ? 'enabled' : 'disabled',
  }
}

/**
 * 保存新建或已有 OAuth 应用。
 */
async function saveApplication(): Promise<void> {
  const payload = buildPayload()
  if (!payload.name || payload.allowedDomains.length === 0) {
    appStore.showError('请填写应用名称和白名单域名')
    return
  }

  saving.value = true
  try {
    if (editingApplication.value) {
      const updated = await updateOAuthApplication(editingApplication.value.id, payload)
      applications.value = applications.value.map((item) => item.id === updated.id ? updated : item)
      appStore.showSuccess('应用已保存')
    } else {
      secretPayload.value = await createOAuthApplication(payload)
      applications.value = [secretPayload.value.application, ...applications.value]
      secretDialogOpen.value = true
      appStore.showSuccess('应用已创建')
    }
    formDialogOpen.value = false
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, '应用保存失败'))
  } finally {
    saving.value = false
  }
}

function openResetDialog(app: OAuthApplication): void {
  pendingApplication.value = app
  confirmAction.value = 'reset'
}

function openDeleteDialog(app: OAuthApplication): void {
  pendingApplication.value = app
  confirmAction.value = 'delete'
}

/**
 * 用户确认后执行待处理的危险操作。
 */
async function confirmPendingAction(): Promise<void> {
  const app = pendingApplication.value
  const action = confirmAction.value
  if (!app || !action) return

  confirmAction.value = null
  try {
    if (action === 'reset') {
      secretPayload.value = await resetOAuthApplicationSecret(app.id)
      applications.value = applications.value.map((item) => item.id === app.id ? secretPayload.value!.application : item)
      secretDialogOpen.value = true
      appStore.showSuccess('密钥已重置')
      return
    }
    await deleteOAuthApplication(app.id)
    applications.value = applications.value.filter((item) => item.id !== app.id)
    appStore.showSuccess('应用已删除')
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, '操作失败'))
  } finally {
    pendingApplication.value = null
  }
}

async function copyText(value: string): Promise<void> {
  if (!value) return
  await navigator.clipboard.writeText(value)
  appStore.showSuccess('已复制')
}

function statusClass(status: OAuthApplication['status']): string {
  return [
    'inline-flex rounded-full px-2 py-1 text-xs font-medium',
    status === 'enabled'
      ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300'
      : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-300',
  ].join(' ')
}

onMounted(() => {
  void loadApplications()
})
</script>
