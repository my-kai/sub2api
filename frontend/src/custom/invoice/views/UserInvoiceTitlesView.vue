<template>
  <AppLayout>
    <div class="space-y-6">
    <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <div>
        <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">抬头管理</h1>
        <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">维护企业发票抬头</p>
      </div>
      <button class="btn btn-primary" type="button" @click="openCreateDialog">
        <Icon name="plus" size="sm" />
        <span>新增抬头</span>
      </button>
    </div>

    <div class="rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900">
      <div v-if="loading" class="p-8 text-sm text-gray-500 dark:text-dark-400">加载中...</div>
      <div v-else-if="titles.length === 0" class="p-12 text-center text-sm text-gray-500 dark:text-dark-400">
        暂无抬头
      </div>
      <div v-else class="overflow-x-auto">
        <table class="w-full min-w-[760px] divide-y divide-gray-200 dark:divide-dark-700">
          <thead class="bg-gray-50 dark:bg-dark-800">
            <tr>
              <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">公司抬头</th>
              <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">税号</th>
              <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">接收邮箱</th>
              <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">状态</th>
              <th class="px-5 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
            <tr v-for="title in titles" :key="title.id">
              <td class="px-5 py-4 text-sm font-medium text-gray-900 dark:text-white">{{ title.company_title }}</td>
              <td class="px-5 py-4 text-sm text-gray-600 dark:text-dark-300">{{ title.tax_number }}</td>
              <td class="px-5 py-4 text-sm text-gray-600 dark:text-dark-300">{{ title.receiver_email }}</td>
              <td class="px-5 py-4 text-sm">
                <span v-if="title.is_default" class="rounded-full bg-primary-50 px-2.5 py-1 text-xs font-medium text-primary-700 dark:bg-primary-500/10 dark:text-primary-300">默认</span>
                <span v-else class="text-gray-400 dark:text-dark-500">-</span>
              </td>
              <td class="px-5 py-4 text-right text-sm">
                <div class="flex justify-end gap-2">
                  <button class="btn btn-secondary btn-sm" type="button" :disabled="saving" @click="openEditDialog(title)">编辑</button>
                  <button class="btn btn-secondary btn-sm" type="button" :disabled="title.is_default || saving" @click="setDefault(title)">设为默认</button>
                  <button class="btn btn-danger btn-sm" type="button" :disabled="saving" @click="openDeleteDialog(title)">删除</button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <BaseDialog :show="dialogOpen" :title="editingTitle ? '编辑抬头' : '新增抬头'" @close="closeDialog">
      <form id="invoice-title-form" class="space-y-4" @submit.prevent="submitTitle">
        <Input v-model="form.company_title" label="公司抬头" required />
        <Input v-model="form.tax_number" label="税号" required />
        <Input v-model="form.receiver_email" type="email" label="接收邮箱" required />
        <div class="flex items-center justify-between rounded-xl border border-gray-200 px-4 py-3 dark:border-dark-700">
          <div>
            <div class="text-sm font-medium text-gray-900 dark:text-white">默认抬头</div>
            <div class="text-xs text-gray-500 dark:text-dark-400">申请时优先选中</div>
          </div>
          <Toggle v-model="form.is_default" />
        </div>
        <p v-if="error" class="text-sm text-red-600 dark:text-red-300">{{ error }}</p>
      </form>
      <template #footer>
        <button type="button" class="btn btn-secondary" @click="closeDialog">取消</button>
        <button type="submit" form="invoice-title-form" class="btn btn-primary" :disabled="saving">{{ saving ? '保存中...' : '保存' }}</button>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="Boolean(deletingTitle)"
      title="删除抬头"
      :message="deleteMessage"
      confirm-text="删除"
      cancel-text="取消"
      danger
      @confirm="confirmDeleteTitle"
      @cancel="deletingTitle = null"
    />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Input from '@/components/common/Input.vue'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'
import {
  createInvoiceTitle,
  deleteInvoiceTitle,
  listInvoiceTitles,
  setDefaultInvoiceTitle,
  updateInvoiceTitle,
} from '../api'
import type { InvoiceTitle, InvoiceTitlePayload } from '../types'

const titles = ref<InvoiceTitle[]>([])
const appStore = useAppStore()
const loading = ref(false)
const saving = ref(false)
const dialogOpen = ref(false)
const editingTitle = ref<InvoiceTitle | null>(null)
const deletingTitle = ref<InvoiceTitle | null>(null)
const error = ref('')

const form = reactive<InvoiceTitlePayload>({
  company_title: '',
  tax_number: '',
  receiver_email: '',
  is_default: false,
})

onMounted(loadTitles)

const deleteMessage = computed(() => {
  return `确认删除 ${deletingTitle.value?.company_title || '该抬头'}？`
})

/**
 * 读取当前用户的发票抬头列表。
 */
async function loadTitles(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    titles.value = await listInvoiceTitles()
  } catch (err) {
    error.value = extractApiErrorMessage(err, '抬头列表读取失败')
    appStore.showError(error.value)
  } finally {
    loading.value = false
  }
}

/**
 * 打开新增抬头弹窗；首个抬头默认设为默认项。
 */
function openCreateDialog(): void {
  editingTitle.value = null
  Object.assign(form, { company_title: '', tax_number: '', receiver_email: '', is_default: titles.value.length === 0 })
  error.value = ''
  dialogOpen.value = true
}

/**
 * 使用当前行数据打开编辑弹窗。
 *
 * @param title - 用户选择的抬头。
 */
function openEditDialog(title: InvoiceTitle): void {
  editingTitle.value = title
  Object.assign(form, {
    company_title: title.company_title,
    tax_number: title.tax_number,
    receiver_email: title.receiver_email,
    is_default: title.is_default,
  })
  error.value = ''
  dialogOpen.value = true
}

/**
 * 关闭表单弹窗。
 */
function closeDialog(): void {
  dialogOpen.value = false
}

/**
 * 保存抬头前先做必填校验，后端仍会做最终校验。
 */
async function submitTitle(): Promise<void> {
  if (!form.company_title || !form.tax_number || !form.receiver_email) {
    error.value = '请填写完整抬头信息'
    return
  }
  saving.value = true
  error.value = ''
  try {
    const payload: InvoiceTitlePayload = {
      company_title: form.company_title.trim(),
      tax_number: form.tax_number.trim(),
      receiver_email: form.receiver_email.trim(),
      is_default: form.is_default,
    }
    if (editingTitle.value) {
      await updateInvoiceTitle(editingTitle.value.id, payload)
    } else {
      await createInvoiceTitle(payload)
    }
    dialogOpen.value = false
    await loadTitles()
    appStore.showSuccess('抬头已保存')
  } catch (err) {
    error.value = extractApiErrorMessage(err, '抬头保存失败')
    appStore.showError(error.value)
  } finally {
    saving.value = false
  }
}

/**
 * 把当前抬头设为默认项。
 *
 * @param title - 目标抬头。
 */
async function setDefault(title: InvoiceTitle): Promise<void> {
  saving.value = true
  error.value = ''
  try {
    await setDefaultInvoiceTitle(title.id)
    await loadTitles()
    appStore.showSuccess('默认抬头已更新')
  } catch (err) {
    error.value = extractApiErrorMessage(err, '默认抬头设置失败')
    appStore.showError(error.value)
  } finally {
    saving.value = false
  }
}

/**
 * 打开删除确认弹窗，避免直接调用浏览器原生确认框。
 *
 * @param title - 待删除抬头。
 */
function openDeleteDialog(title: InvoiceTitle): void {
  deletingTitle.value = title
}

/**
 * 确认删除一个抬头；历史申请保留创建时的抬头快照。
 */
async function confirmDeleteTitle(): Promise<void> {
  if (!deletingTitle.value || saving.value) return
  saving.value = true
  error.value = ''
  try {
    await deleteInvoiceTitle(deletingTitle.value.id)
    deletingTitle.value = null
    await loadTitles()
    appStore.showSuccess('抬头已删除')
  } catch (err) {
    error.value = extractApiErrorMessage(err, '抬头删除失败')
    appStore.showError(error.value)
  } finally {
    saving.value = false
  }
}
</script>
