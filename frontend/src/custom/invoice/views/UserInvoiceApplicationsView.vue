<template>
  <AppLayout>
    <div class="space-y-6">
    <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <div>
        <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">开票申请</h1>
        <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">根据充值订单申请企业普票</p>
      </div>
      <div class="flex gap-2">
        <button class="btn btn-secondary" type="button" :disabled="loading" @click="loadApplications">
          <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
          <span>刷新</span>
        </button>
        <button class="btn btn-primary" type="button" @click="openCreateDialog">
          <Icon name="plus" size="sm" />
          <span>新增申请</span>
        </button>
      </div>
    </div>

    <div v-if="errorMessage" class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-300">
      {{ errorMessage }}
    </div>

    <div class="rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900">
      <div v-if="loading" class="p-8 text-sm text-gray-500 dark:text-dark-400">加载中...</div>
      <div v-else-if="applications.length === 0" class="p-12 text-center text-sm text-gray-500 dark:text-dark-400">
        暂无开票申请
      </div>
      <div v-else class="overflow-x-auto">
        <table class="w-full min-w-[860px] divide-y divide-gray-200 dark:divide-dark-700">
          <thead class="bg-gray-50 dark:bg-dark-800">
            <tr>
              <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">申请编号</th>
              <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">状态</th>
              <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">金额</th>
              <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">订单数</th>
              <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">抬头</th>
              <th class="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">创建时间</th>
              <th class="px-5 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
            <tr v-for="app in applications" :key="app.id">
              <td class="px-5 py-4 text-sm text-gray-900 dark:text-white">{{ app.application_no }}</td>
              <td class="px-5 py-4 text-sm">
                <span :class="invoiceStatusClass(app.status)" class="rounded-full px-2.5 py-1 text-xs font-medium">{{ invoiceStatusLabel(app.status) }}</span>
              </td>
              <td class="px-5 py-4 text-sm text-gray-900 dark:text-white">{{ formatInvoiceAmount(app.total_amount, app.currency) }}</td>
              <td class="px-5 py-4 text-sm text-gray-600 dark:text-dark-300">{{ app.order_count }}</td>
              <td class="px-5 py-4 text-sm text-gray-600 dark:text-dark-300">{{ app.company_title }}</td>
              <td class="px-5 py-4 text-sm text-gray-600 dark:text-dark-300">{{ formatInvoiceDate(app.created_at) }}</td>
              <td class="px-5 py-4 text-right text-sm">
                <button class="btn btn-secondary btn-sm" type="button" @click="openDetail(app)">查看</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <Pagination
        v-if="pagination.total > 0"
        :page="pagination.page"
        :page-size="pagination.page_size"
        :total="pagination.total"
        @update:page="handlePageChange"
        @update:pageSize="handlePageSizeChange"
      />
    </div>

    <BaseDialog :show="createOpen" title="新增开票申请" width="extra-wide" @close="closeCreateDialog">
      <div class="space-y-5">
        <div class="grid gap-4 lg:grid-cols-[1fr_320px]">
          <section class="rounded-xl border border-gray-200 dark:border-dark-700">
            <div class="border-b border-gray-200 px-4 py-3 text-sm font-medium text-gray-900 dark:border-dark-700 dark:text-white">选择充值订单</div>
            <div v-if="createLoading" class="p-6 text-sm text-gray-500 dark:text-dark-400">加载中...</div>
            <div v-else-if="eligibleOrders.length === 0" class="p-6 text-sm text-gray-500 dark:text-dark-400">暂无可开票订单</div>
            <div v-else class="max-h-96 overflow-auto">
              <table class="w-full min-w-[640px] divide-y divide-gray-200 text-sm dark:divide-dark-700">
                <thead class="sticky top-0 bg-gray-50 dark:bg-dark-800">
                  <tr>
                    <th class="w-[60px] px-3 py-2 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">选择</th>
                    <th class="px-3 py-2 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">充值订单</th>
                    <th class="px-3 py-2 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">金额</th>
                    <th class="px-3 py-2 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">完成时间</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
                  <tr
                    v-for="order in eligibleOrders"
                    :key="order.id"
                    class="cursor-pointer hover:bg-gray-50 dark:hover:bg-dark-800/70"
                    @click="toggleOrderSelection(order.id)"
                  >
                    <td class="w-[60px] px-3 py-2">
                      <input
                        v-model="selectedOrderIDs"
                        :value="order.id"
                        type="checkbox"
                        class="h-4 w-4 rounded border-gray-300 text-primary-600"
                        :aria-label="`选择订单 ${order.out_trade_no || order.id}`"
                        @click.stop
                      />
                    </td>
                    <td class="max-w-[280px] truncate px-3 py-2 text-gray-700 dark:text-dark-200">
                      {{ order.out_trade_no || `订单 #${order.id}` }}
                    </td>
                    <td class="whitespace-nowrap px-3 py-2 font-medium text-gray-900 dark:text-white">
                      {{ formatInvoiceAmount(order.pay_amount, order.currency) }}
                    </td>
                    <td class="whitespace-nowrap px-3 py-2 text-gray-500 dark:text-dark-400">
                      {{ formatInvoiceDate(order.completed_at || order.paid_at || order.created_at) }}
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </section>

          <section class="space-y-4">
            <div>
              <label class="input-label mb-1.5 block">发票抬头</label>
              <Select
                v-model="selectedTitleID"
                :options="titleOptions"
                placeholder="请选择抬头"
                :searchable="false"
              />
            </div>
            <div class="rounded-xl bg-gray-50 p-4 text-sm dark:bg-dark-800">
              <div class="flex justify-between text-gray-500 dark:text-dark-400">
                <span>已选订单</span>
                <span>{{ selectedOrderIDs.length }} 笔</span>
              </div>
              <div class="mt-2 flex justify-between text-base font-semibold text-gray-900 dark:text-white">
                <span>合计金额</span>
                <span>{{ formatInvoiceAmount(selectedTotal, selectedCurrency) }}</span>
              </div>
            </div>
            <p v-if="createError" class="text-sm text-red-600 dark:text-red-300">{{ createError }}</p>
          </section>
        </div>
      </div>
      <template #footer>
        <button class="btn btn-secondary" type="button" @click="closeCreateDialog">取消</button>
        <button class="btn btn-primary" type="button" :disabled="submitting" @click="submitApplication">{{ submitting ? '提交中...' : '提交申请' }}</button>
      </template>
    </BaseDialog>

    <BaseDialog :show="!!detail" title="开票申请详情" width="wide" @close="detail = null">
      <div v-if="detail" class="space-y-5">
        <div class="grid gap-3 text-sm sm:grid-cols-2">
          <InfoItem label="状态" :value="invoiceStatusLabel(detail.status)" />
          <InfoItem label="申请编号" :value="detail.application_no" />
          <InfoItem label="金额" :value="formatInvoiceAmount(detail.total_amount, detail.currency)" />
          <InfoItem label="公司抬头" :value="detail.company_title" />
          <InfoItem label="税号" :value="detail.tax_number" />
          <InfoItem label="接收邮箱" :value="detail.receiver_email" />
          <InfoItem label="创建时间" :value="formatInvoiceDate(detail.created_at)" />
          <InfoItem v-if="detail.invoice_number" label="发票号码" :value="detail.invoice_number" />
          <InfoItem v-if="detail.admin_remark" label="备注" :value="detail.admin_remark" />
          <InfoItem v-if="detail.reject_reason" label="驳回原因" :value="detail.reject_reason" />
        </div>
        <div>
          <h3 class="mb-2 text-sm font-medium text-gray-900 dark:text-white">订单明细</h3>
          <div class="overflow-x-auto rounded-xl border border-gray-200 dark:border-dark-700">
            <table class="w-full min-w-[560px] divide-y divide-gray-200 dark:divide-dark-700">
              <tbody class="divide-y divide-gray-100 text-sm dark:divide-dark-800">
                <tr v-for="order in detail.orders || []" :key="order.order_id">
                  <td class="px-4 py-3 text-gray-900 dark:text-white">{{ order.out_trade_no || `订单 #${order.order_id}` }}</td>
                  <td class="px-4 py-3 text-gray-600 dark:text-dark-300">{{ formatInvoiceAmount(order.amount, order.currency) }}</td>
                  <td class="px-4 py-3 text-gray-500 dark:text-dark-400">{{ order.status || '-' }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
        <button
          v-if="detail.status === 'issued'"
          class="btn btn-primary"
          type="button"
          :disabled="downloadingID === detail.id"
          @click="downloadInvoice(detail)"
        >
          {{ downloadingID === detail.id ? '下载中...' : '下载发票' }}
        </button>
      </div>
    </BaseDialog>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, reactive, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'
import {
  createInvoiceApplication,
  downloadUserInvoiceFile,
  getMyInvoiceApplication,
  listEligibleInvoiceOrders,
  listInvoiceTitles,
  listMyInvoiceApplications,
} from '../api'
import type { EligibleInvoiceOrder, InvoiceApplication, InvoiceTitle } from '../types'
import { formatInvoiceAmount, formatInvoiceDate, invoiceStatusClass, invoiceStatusLabel, saveInvoiceBlob } from '../utils'

const InfoItem = defineComponent({
  props: { label: { type: String, required: true }, value: { type: String, required: true } },
  setup(props) {
    return () => h('div', { class: 'rounded-xl bg-gray-50 p-3 dark:bg-dark-800' }, [
      h('div', { class: 'text-xs text-gray-500 dark:text-dark-400' }, props.label),
      h('div', { class: 'mt-1 break-words text-gray-900 dark:text-white' }, props.value),
    ])
  },
})

const applications = ref<InvoiceApplication[]>([])
const appStore = useAppStore()
const loading = ref(false)
const createOpen = ref(false)
const createLoading = ref(false)
const submitting = ref(false)
const createError = ref('')
const errorMessage = ref('')
const titles = ref<InvoiceTitle[]>([])
const eligibleOrders = ref<EligibleInvoiceOrder[]>([])
const selectedOrderIDs = ref<number[]>([])
const selectedTitleID = ref<string | number | boolean | null>(0)
const detail = ref<InvoiceApplication | null>(null)
const downloadingID = ref<number | null>(null)

const pagination = reactive({
  total: 0,
  page: 1,
  page_size: 20,
})

const selectedOrders = computed(() => eligibleOrders.value.filter(order => selectedOrderIDs.value.includes(order.id)))
const selectedTotal = computed(() => selectedOrders.value.reduce((sum, order) => sum + Number(order.pay_amount || 0), 0).toFixed(2))
const selectedCurrency = computed(() => selectedOrders.value[0]?.currency || 'CNY')
const titleOptions = computed(() => titles.value.map(title => ({ value: title.id, label: title.company_title })))

onMounted(loadApplications)

/**
 * 切换订单选择状态；表格行点击复用同一逻辑，checkbox 自身阻止冒泡避免双重切换。
 *
 * @param orderID - 待切换的充值订单 ID。
 */
function toggleOrderSelection(orderID: number): void {
  const current = new Set(selectedOrderIDs.value)
  if (current.has(orderID)) {
    current.delete(orderID)
  } else {
    current.add(orderID)
  }
  selectedOrderIDs.value = Array.from(current)
}

/**
 * 读取当前用户的开票申请列表。
 */
async function loadApplications(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    const page = await listMyInvoiceApplications({ page: pagination.page, page_size: pagination.page_size })
    applications.value = page.items || []
    pagination.total = page.total
    pagination.page = page.page
    pagination.page_size = page.page_size
  } catch (err) {
    errorMessage.value = extractApiErrorMessage(err, '开票申请读取失败')
    appStore.showError(errorMessage.value)
  } finally {
    loading.value = false
  }
}

/**
 * 切换开票申请列表页码。
 *
 * @param page - 目标页码。
 */
function handlePageChange(page: number): void {
  pagination.page = page
  void loadApplications()
}

/**
 * 切换每页数量并回到第一页，避免当前页码超过新页数。
 *
 * @param pageSize - 每页数量。
 */
function handlePageSizeChange(pageSize: number): void {
  pagination.page_size = pageSize
  pagination.page = 1
  void loadApplications()
}

/**
 * 打开新建申请弹窗，并同时读取抬头和可开票订单。
 */
async function openCreateDialog(): Promise<void> {
  createOpen.value = true
  createLoading.value = true
  createError.value = ''
  selectedOrderIDs.value = []
  selectedTitleID.value = 0
  try {
    const [titleList, orderList] = await Promise.all([listInvoiceTitles(), listEligibleInvoiceOrders()])
    titles.value = titleList
    eligibleOrders.value = orderList
    selectedTitleID.value = titleList.find(title => title.is_default)?.id || titleList[0]?.id || 0
  } catch (err) {
    createError.value = extractApiErrorMessage(err, '开票信息读取失败')
  } finally {
    createLoading.value = false
  }
}

/**
 * 关闭新建申请弹窗。
 */
function closeCreateDialog(): void {
  createOpen.value = false
}

/**
 * 提交开票申请。订单占用最终由后端事务保证。
 */
async function submitApplication(): Promise<void> {
  if (selectedOrderIDs.value.length === 0) {
    createError.value = '请选择充值订单'
    return
  }
  if (!selectedTitleID.value) {
    createError.value = '请选择发票抬头'
    return
  }
  submitting.value = true
  createError.value = ''
  try {
    await createInvoiceApplication({ order_ids: selectedOrderIDs.value, title_id: Number(selectedTitleID.value) })
    createOpen.value = false
    pagination.page = 1
    await loadApplications()
    appStore.showSuccess('申请已提交')
  } catch (err) {
    createError.value = extractApiErrorMessage(err, '开票申请提交失败')
  } finally {
    submitting.value = false
  }
}

/**
 * 读取申请详情，订单明细和快照字段都以后端返回为准。
 *
 * @param app - 列表中的开票申请。
 */
async function openDetail(app: InvoiceApplication): Promise<void> {
  try {
    detail.value = await getMyInvoiceApplication(app.id)
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, '开票申请详情读取失败'))
  }
}

/**
 * 通过 API client 下载 PDF，避免在链接里绕过统一鉴权处理。
 *
 * @param app - 已开票申请。
 */
async function downloadInvoice(app: InvoiceApplication): Promise<void> {
  downloadingID.value = app.id
  try {
    const blob = await downloadUserInvoiceFile(app.id)
    saveInvoiceBlob(blob, app.file_original_name || `invoice-${app.id}.pdf`)
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, '发票下载失败'))
  } finally {
    downloadingID.value = null
  }
}
</script>
