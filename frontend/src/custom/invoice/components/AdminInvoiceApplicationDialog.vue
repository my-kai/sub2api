<template>
  <BaseDialog :show="show" title="代开申请" width="extra-wide" @close="closeDialog">
    <div class="space-y-5">
      <section class="space-y-3">
        <Input v-model="searchKeyword" label="目标用户" placeholder="输入邮箱搜索" :disabled="submitting" />
        <div v-if="searchResults.length > 0" class="max-h-40 overflow-y-auto rounded-lg border border-gray-200 dark:border-dark-700">
          <button
            v-for="user in searchResults"
            :key="user.id"
            type="button"
            class="flex w-full items-center justify-between px-3 py-2 text-left text-sm hover:bg-gray-50 dark:hover:bg-dark-700"
            @click="selectUser(user)"
          >
            <span class="text-gray-700 dark:text-dark-200">{{ user.email }}</span>
            <span class="text-xs text-gray-400">#{{ user.id }}</span>
          </button>
        </div>
        <div v-if="selectedUser" class="flex items-center justify-between rounded-lg bg-primary-50 px-3 py-2 text-sm text-primary-700 dark:bg-primary-500/10 dark:text-primary-300">
          <span>{{ selectedUser.email }}（#{{ selectedUser.id }}）</span>
          <button type="button" class="btn btn-secondary btn-sm" :disabled="submitting" @click="clearUser">更换用户</button>
        </div>
      </section>

      <div v-if="selectedUser" class="grid gap-4 lg:grid-cols-[320px_1fr]">
        <section class="space-y-3 rounded-xl border border-gray-200 p-4 dark:border-dark-700">
          <div class="flex items-center justify-between">
            <h3 class="text-sm font-medium text-gray-900 dark:text-white">发票抬头</h3>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="submitting" @click="openTitleEditor()">新增</button>
          </div>
          <div v-if="titles.length === 0" class="text-sm text-gray-500 dark:text-dark-400">暂无抬头，请先新增</div>
          <div v-else class="space-y-2">
            <Select v-model="selectedTitleID" :options="titleOptions" placeholder="请选择抬头" :searchable="false" />
            <div v-for="title in titles" :key="title.id" class="flex items-center justify-between gap-2 rounded-lg bg-gray-50 px-3 py-2 text-xs dark:bg-dark-800">
              <span class="min-w-0 truncate text-gray-700 dark:text-dark-200">{{ title.company_title }}<span v-if="title.is_default" class="ml-1 text-primary-600">默认</span></span>
              <span class="flex shrink-0 gap-1">
                <button type="button" class="text-primary-600 hover:underline" :disabled="submitting" @click="openTitleEditor(title)">编辑</button>
                <button v-if="!title.is_default" type="button" class="text-primary-600 hover:underline" :disabled="submitting" @click="setDefault(title.id)">设为默认</button>
                <button type="button" class="text-red-600 hover:underline" :disabled="submitting" @click="titleToDelete = title">删除</button>
              </span>
            </div>
          </div>
        </section>

        <section class="space-y-3 rounded-xl border border-gray-200 p-4 dark:border-dark-700">
          <h3 class="text-sm font-medium text-gray-900 dark:text-white">选择充值订单</h3>
          <div v-if="loading" class="py-8 text-center text-sm text-gray-500 dark:text-dark-400">加载中...</div>
          <div v-else-if="orders.length === 0" class="py-8 text-center text-sm text-gray-500 dark:text-dark-400">暂无可代开订单</div>
          <div v-else class="max-h-80 overflow-auto">
            <table class="w-full min-w-[560px] divide-y divide-gray-200 text-sm dark:divide-dark-700">
              <thead class="sticky top-0 bg-gray-50 dark:bg-dark-800">
                <tr>
                  <th class="w-12 px-3 py-2 text-left text-xs text-gray-500">选择</th>
                  <th class="px-3 py-2 text-left text-xs text-gray-500">充值订单</th>
                  <th class="px-3 py-2 text-left text-xs text-gray-500">金额</th>
                  <th class="px-3 py-2 text-left text-xs text-gray-500">完成时间</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
                <tr v-for="order in orders" :key="order.id" class="cursor-pointer hover:bg-gray-50 dark:hover:bg-dark-800/70" @click="toggleOrder(order.id)">
                  <td class="px-3 py-2"><input v-model="selectedOrderIDs" :value="order.id" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600" @click.stop /></td>
                  <td class="max-w-[220px] truncate px-3 py-2 text-gray-700 dark:text-dark-200">{{ order.out_trade_no || `订单 #${order.id}` }}</td>
                  <td class="whitespace-nowrap px-3 py-2 font-medium text-gray-900 dark:text-white">{{ formatInvoiceAmount(order.pay_amount, order.currency) }}</td>
                  <td class="whitespace-nowrap px-3 py-2 text-gray-500 dark:text-dark-400">{{ formatInvoiceDate(order.completed_at || order.paid_at || order.created_at) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <div class="flex justify-between rounded-lg bg-gray-50 px-3 py-2 text-sm dark:bg-dark-800">
            <span class="text-gray-500 dark:text-dark-400">已选 {{ selectedOrderIDs.length }} 笔</span>
            <span class="font-semibold text-gray-900 dark:text-white">{{ formatInvoiceAmount(selectedTotal, selectedCurrency) }}</span>
          </div>
        </section>
      </div>
      <p v-if="errorMessage" class="text-sm text-red-600 dark:text-red-300">{{ errorMessage }}</p>
    </div>

    <template #footer>
      <button type="button" class="btn btn-secondary" :disabled="submitting" @click="closeDialog">取消</button>
      <button type="button" class="btn btn-primary" :disabled="submitting || !selectedUser" @click="submit">{{ submitting ? '提交中...' : '提交代开申请' }}</button>
    </template>
  </BaseDialog>

  <BaseDialog :show="titleEditorOpen" :title="editingTitle ? '编辑用户抬头' : '新增用户抬头'" width="normal" @close="titleEditorOpen = false">
    <form id="admin-invoice-title-form" class="space-y-4" @submit.prevent="saveTitle">
      <Input v-model="titleForm.company_title" label="公司抬头" required />
      <Input v-model="titleForm.tax_number" label="税号" required />
      <Input v-model="titleForm.receiver_email" type="email" label="接收邮箱" required />
      <div class="flex items-center justify-between rounded-lg border border-gray-200 px-3 py-2 dark:border-dark-700">
        <span class="text-sm text-gray-700 dark:text-dark-200">设为默认抬头</span>
        <Toggle v-model="titleForm.is_default" />
      </div>
    </form>
    <template #footer>
      <button type="button" class="btn btn-secondary" :disabled="titleSaving" @click="titleEditorOpen = false">取消</button>
      <button type="submit" form="admin-invoice-title-form" class="btn btn-primary" :disabled="titleSaving">{{ titleSaving ? '保存中...' : '保存' }}</button>
    </template>
  </BaseDialog>

  <ConfirmDialog
    :show="Boolean(titleToDelete)"
    title="删除抬头"
    :message="`确认删除 ${titleToDelete?.company_title || '该抬头'}？`"
    confirm-text="删除"
    cancel-text="取消"
    danger
    @confirm="deleteTitle"
    @cancel="titleToDelete = null"
  />
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Input from '@/components/common/Input.vue'
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import { searchUsers, type SimpleUser } from '@/api/admin/usage'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'
import {
  createAdminInvoiceApplication,
  createAdminInvoiceTitle,
  deleteAdminInvoiceTitle,
  listAdminEligibleInvoiceOrders,
  listAdminInvoiceTitles,
  setAdminDefaultInvoiceTitle,
  updateAdminInvoiceTitle,
} from '../api'
import type { EligibleInvoiceOrder, InvoiceTitle, InvoiceTitlePayload } from '../types'
import { formatInvoiceAmount, formatInvoiceDate } from '../utils'

const props = defineProps<{ show: boolean }>()
const emit = defineEmits<{ (event: 'close'): void; (event: 'created'): void }>()
const appStore = useAppStore()
const searchKeyword = ref('')
const searchResults = ref<SimpleUser[]>([])
const selectedUser = ref<SimpleUser | null>(null)
const titles = ref<InvoiceTitle[]>([])
const orders = ref<EligibleInvoiceOrder[]>([])
const selectedOrderIDs = ref<number[]>([])
const selectedTitleID = ref<string | number | boolean | null>(0)
const loading = ref(false)
const submitting = ref(false)
const errorMessage = ref('')
const titleEditorOpen = ref(false)
const titleSaving = ref(false)
const editingTitle = ref<InvoiceTitle | null>(null)
const titleToDelete = ref<InvoiceTitle | null>(null)
const titleForm = reactive<InvoiceTitlePayload>({ company_title: '', tax_number: '', receiver_email: '', is_default: false })
let searchSequence = 0

const titleOptions = computed(() => titles.value.map(title => ({ value: title.id, label: title.company_title })))
const selectedOrders = computed(() => orders.value.filter(order => selectedOrderIDs.value.includes(order.id)))
const selectedTotal = computed(() => selectedOrders.value.reduce((sum, order) => sum + Number(order.pay_amount || 0), 0))
const selectedCurrency = computed(() => selectedOrders.value[0]?.currency || 'CNY')

watch(() => props.show, (show) => {
  if (!show) return
  reset()
})

watch(searchKeyword, (value) => {
  const keyword = value.trim()
  const sequence = ++searchSequence
  searchResults.value = []
  if (!keyword) return
  void searchUsers(keyword).then(users => {
    if (sequence === searchSequence) searchResults.value = users.filter(user => !user.deleted)
  }).catch(() => {
    if (sequence === searchSequence) errorMessage.value = '用户搜索失败'
  })
})

function reset(): void {
  searchKeyword.value = ''
  searchResults.value = []
  selectedUser.value = null
  titles.value = []
  orders.value = []
  selectedOrderIDs.value = []
  selectedTitleID.value = 0
  errorMessage.value = ''
}

/** Selects the target user and reloads data scoped to that user. */
function selectUser(user: SimpleUser): void {
  selectedUser.value = user
  searchKeyword.value = user.email
  searchResults.value = []
  selectedOrderIDs.value = []
  selectedTitleID.value = 0
  void loadUserData(user.id)
}

function clearUser(): void {
  selectedUser.value = null
  titles.value = []
  orders.value = []
  selectedOrderIDs.value = []
  selectedTitleID.value = 0
  searchKeyword.value = ''
}

async function loadUserData(userID: number): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    const [titleList, orderList] = await Promise.all([listAdminInvoiceTitles(userID), listAdminEligibleInvoiceOrders(userID)])
    if (!selectedUser.value || selectedUser.value.id !== userID) return
    titles.value = titleList
    orders.value = orderList
    selectedTitleID.value = titleList.find(title => title.is_default)?.id || titleList[0]?.id || 0
  } catch (err) {
    errorMessage.value = extractApiErrorMessage(err, '用户开票信息读取失败')
  } finally {
    loading.value = false
  }
}

/** Keeps row clicks and checkbox changes on one order-selection path. */
function toggleOrder(orderID: number): void {
  selectedOrderIDs.value = selectedOrderIDs.value.includes(orderID)
    ? selectedOrderIDs.value.filter(id => id !== orderID)
    : [...selectedOrderIDs.value, orderID]
}

/** Opens the title editor with either a blank title or the selected snapshot. */
function openTitleEditor(title?: InvoiceTitle): void {
  editingTitle.value = title || null
  Object.assign(titleForm, title
    ? { company_title: title.company_title, tax_number: title.tax_number, receiver_email: title.receiver_email, is_default: title.is_default }
    : { company_title: '', tax_number: '', receiver_email: '', is_default: titles.value.length === 0 })
  titleEditorOpen.value = true
}

/** Persists a target-user title and keeps the selected title in sync. */
async function saveTitle(): Promise<void> {
  if (!selectedUser.value || !titleForm.company_title.trim() || !titleForm.tax_number.trim() || !titleForm.receiver_email.trim()) {
    errorMessage.value = '请填写完整抬头信息'
    return
  }
  titleSaving.value = true
  try {
    const payload = { ...titleForm, company_title: titleForm.company_title.trim(), tax_number: titleForm.tax_number.trim(), receiver_email: titleForm.receiver_email.trim() }
    const saved = editingTitle.value
      ? await updateAdminInvoiceTitle(selectedUser.value.id, editingTitle.value.id, payload)
      : await createAdminInvoiceTitle(selectedUser.value.id, payload)
    const nextTitles = editingTitle.value ? titles.value.map(title => title.id === saved.id ? saved : title) : [...titles.value, saved]
    titles.value = saved.is_default ? nextTitles.map(title => ({ ...title, is_default: title.id === saved.id })) : nextTitles
    selectedTitleID.value = saved.id
    titleEditorOpen.value = false
    appStore.showSuccess('用户抬头已保存')
  } catch (err) {
    errorMessage.value = extractApiErrorMessage(err, '用户抬头保存失败')
  } finally {
    titleSaving.value = false
  }
}

async function setDefault(titleID: number): Promise<void> {
  if (!selectedUser.value) return
  try {
    const saved = await setAdminDefaultInvoiceTitle(selectedUser.value.id, titleID)
    titles.value = titles.value.map(title => ({ ...title, is_default: title.id === saved.id }))
    selectedTitleID.value = saved.id
  } catch (err) {
    errorMessage.value = extractApiErrorMessage(err, '默认抬头设置失败')
  }
}

async function deleteTitle(): Promise<void> {
  const user = selectedUser.value
  const target = titleToDelete.value
  if (!user || !target) return
  try {
    await deleteAdminInvoiceTitle(user.id, target.id)
    titles.value = titles.value.filter(title => title.id !== target.id)
    if (selectedTitleID.value === target.id) selectedTitleID.value = titles.value[0]?.id || 0
    titleToDelete.value = null
  } catch (err) {
    errorMessage.value = extractApiErrorMessage(err, '用户抬头删除失败')
  }
}

/** Submits the selected title and orders; the backend owns final eligibility checks. */
async function submit(): Promise<void> {
  if (!selectedUser.value) return
  if (!selectedTitleID.value) {
    errorMessage.value = '请选择发票抬头'
    return
  }
  if (selectedOrderIDs.value.length === 0) {
    errorMessage.value = '请选择充值订单'
    return
  }
  submitting.value = true
  errorMessage.value = ''
  try {
    await createAdminInvoiceApplication({ user_id: selectedUser.value.id, title_id: Number(selectedTitleID.value), order_ids: selectedOrderIDs.value })
    appStore.showSuccess('代开申请已提交')
    emit('created')
    emit('close')
  } catch (err) {
    errorMessage.value = extractApiErrorMessage(err, '代开申请提交失败')
  } finally {
    submitting.value = false
  }
}

function closeDialog(): void {
  if (!submitting.value) emit('close')
}
</script>
