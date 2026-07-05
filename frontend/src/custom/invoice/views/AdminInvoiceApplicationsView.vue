<template>
  <AppLayout>
    <div class="mx-auto max-w-7xl space-y-5">
      <header class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <div class="flex items-center gap-2">
            <Icon name="download" size="lg" class="text-primary-500" />
            <h1 class="text-lg font-semibold text-gray-900 dark:text-white">开票管理</h1>
          </div>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">处理用户开票申请。</p>
        </div>
        <div class="flex flex-wrap gap-2">
          <Select
            v-model="filters.status"
            :options="statusOptions"
            class="w-36"
            :searchable="false"
            @change="handleStatusChange"
          />
          <button type="button" class="btn btn-secondary" :disabled="loading" @click="loadApplications">
            <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
            <span>刷新</span>
          </button>
          <button type="button" class="btn btn-secondary" :disabled="testSending" @click="openTestEmailDialog">测试发件</button>
        </div>
      </header>

      <div v-if="errorMessage" class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-300">
        {{ errorMessage }}
      </div>

      <section class="overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div v-if="loading && applications.length === 0" class="flex justify-center py-12">
          <LoadingSpinner size="lg" />
        </div>

        <div v-else-if="applications.length === 0" class="py-12 text-center">
          <Icon name="inbox" size="xl" class="mx-auto text-gray-300 dark:text-dark-500" />
          <p class="mt-3 text-sm text-gray-500 dark:text-dark-400">暂无开票申请</p>
        </div>

        <div v-else class="overflow-x-auto">
          <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
            <thead class="bg-gray-50 dark:bg-dark-700/60">
              <tr>
                <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-300">申请</th>
                <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-300">用户</th>
                <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-300">状态</th>
                <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-300">金额</th>
                <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-300">抬头</th>
                <th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-300">创建时间</th>
                <th class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-300">操作</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="app in applications" :key="app.id" class="hover:bg-gray-50 dark:hover:bg-dark-700/40">
                <td class="px-4 py-3 text-sm font-medium text-gray-900 dark:text-white">{{ app.application_no }}</td>
                <td class="px-4 py-3 text-sm text-gray-600 dark:text-dark-300">#{{ app.user_id }}</td>
                <td class="px-4 py-3 text-sm">
                  <span :class="invoiceStatusClass(app.status)" class="rounded-full px-2.5 py-1 text-xs font-medium">
                    {{ invoiceStatusLabel(app.status) }}
                  </span>
                </td>
                <td class="px-4 py-3 text-sm text-gray-900 dark:text-white">{{ formatInvoiceAmount(app.total_amount, app.currency) }}</td>
                <td class="px-4 py-3 text-sm text-gray-600 dark:text-dark-300">{{ app.company_title }}</td>
                <td class="px-4 py-3 text-sm text-gray-500 dark:text-dark-400">{{ formatInvoiceDate(app.created_at) }}</td>
                <td class="px-4 py-3">
                  <div class="flex justify-end gap-1">
                    <button type="button" class="btn btn-secondary btn-sm" @click="openDetail(app)">查看</button>
                    <button
                      v-if="app.status === 'pending'"
                      type="button"
                      class="btn btn-primary btn-sm"
                      @click="openIssueDialog(app)"
                    >
                      标记开票
                    </button>
                    <button
                      v-if="app.status === 'pending'"
                      type="button"
                      class="btn btn-danger btn-sm"
                      @click="openRejectDialog(app)"
                    >
                      驳回
                    </button>
                    <button
                      v-if="app.status === 'issued'"
                      class="btn btn-secondary btn-sm"
                      type="button"
                      :disabled="downloadingID === app.id"
                      @click="downloadInvoice(app)"
                    >
                      {{ downloadingID === app.id ? '下载中...' : '下载' }}
                    </button>
                  </div>
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
          :show-jump="true"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </section>
    </div>

    <BaseDialog :show="Boolean(detail)" title="开票申请详情" width="wide" @close="detail = null">
      <div v-if="detail" class="space-y-5">
        <div class="grid gap-3 text-sm sm:grid-cols-2">
          <InfoItem label="状态" :value="invoiceStatusLabel(detail.status)" />
          <InfoItem label="申请编号" :value="detail.application_no" />
          <InfoItem label="用户" :value="`#${detail.user_id}`" />
          <InfoItem label="金额" :value="formatInvoiceAmount(detail.total_amount, detail.currency)" />
          <InfoItem label="订单数" :value="String(detail.order_count)" />
          <InfoItem label="创建时间" :value="formatInvoiceDate(detail.created_at)" />
          <InfoItem v-if="detail.invoice_number" label="发票号码" :value="detail.invoice_number" />
          <InfoItem v-if="detail.admin_remark" label="备注" :value="detail.admin_remark" />
          <InfoItem v-if="detail.reject_reason" label="驳回原因" :value="detail.reject_reason" />
          <InfoItem v-if="detail.file_original_name" label="文件" :value="detail.file_original_name" />
          <InfoItem v-if="detail.file_size" label="文件大小" :value="formatInvoiceFileSize(detail.file_size)" />
        </div>

        <!-- 抬头快照独立展示，方便管理员核对开票信息时和审核状态分开阅读。 -->
        <section class="space-y-3">
          <div class="flex items-center justify-between gap-3">
            <h3 class="text-sm font-medium text-gray-900 dark:text-white">抬头信息</h3>
            <button type="button" class="btn btn-secondary btn-sm" title="复制抬头信息" @click="copyTitleInfo">
              <Icon name="copy" size="sm" />
              <span>复制</span>
            </button>
          </div>
          <div class="grid gap-3 text-sm sm:grid-cols-3">
            <InfoItem label="公司抬头" :value="detail.company_title" />
            <InfoItem label="税号" :value="detail.tax_number" />
            <InfoItem label="接收邮箱" :value="detail.receiver_email" />
          </div>
        </section>

        <div>
          <h3 class="mb-2 text-sm font-medium text-gray-900 dark:text-white">订单明细</h3>
          <div class="overflow-x-auto rounded-xl border border-gray-200 dark:border-dark-700">
            <table class="w-full min-w-[640px] divide-y divide-gray-200 dark:divide-dark-700">
              <thead class="bg-gray-50 dark:bg-dark-800">
                <tr>
                  <th class="px-4 py-2 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">订单</th>
                  <th class="px-4 py-2 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">金额</th>
                  <th class="px-4 py-2 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">支付方式</th>
                  <th class="px-4 py-2 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">状态</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 text-sm dark:divide-dark-800">
                <tr v-for="order in detail.orders || []" :key="order.order_id">
                  <td class="px-4 py-3 text-gray-900 dark:text-white">{{ order.out_trade_no || `订单 #${order.order_id}` }}</td>
                  <td class="px-4 py-3 text-gray-600 dark:text-dark-300">{{ formatInvoiceAmount(order.amount, order.currency) }}</td>
                  <td class="px-4 py-3 text-gray-600 dark:text-dark-300">{{ order.payment_type || '-' }}</td>
                  <td class="px-4 py-3 text-gray-500 dark:text-dark-400">{{ order.status || '-' }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        <div v-if="detail.status === 'issued'" class="flex flex-wrap gap-2">
          <button
            class="btn btn-primary"
            type="button"
            :disabled="downloadingID === detail.id"
            @click="downloadInvoice(detail)"
          >
            {{ downloadingID === detail.id ? '下载中...' : '下载发票' }}
          </button>
        </div>
      </div>
    </BaseDialog>

    <BaseDialog :show="testEmailDialogOpen" title="测试发件" width="normal" @close="closeTestEmailDialog">
      <form id="invoice-test-email-form" class="space-y-4" @submit.prevent="submitTestEmail">
        <Input
          v-model="testEmailReceiverEmail"
          type="email"
          label="接收邮箱"
          placeholder="name@example.com"
          :disabled="testSending"
          required
        />
        <p class="text-sm text-gray-500 dark:text-dark-400">系统会生成测试开票信息。</p>
        <p v-if="testEmailError" class="text-sm text-red-600 dark:text-red-300">{{ testEmailError }}</p>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" :disabled="testSending" @click="closeTestEmailDialog">取消</button>
          <button type="submit" form="invoice-test-email-form" class="btn btn-primary" :disabled="testSending">
            {{ testSending ? '发送中...' : '发送测试邮件' }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <BaseDialog :show="Boolean(issueTarget)" title="标记已开票" width="normal" @close="closeIssueDialog">
      <form id="invoice-issue-form" class="space-y-4" @submit.prevent="submitIssue">
        <Input v-model="issueForm.invoice_number" label="发票号码" required />
        <TextArea v-model="issueForm.admin_remark" label="备注" rows="3" required />
        <div>
          <label class="input-label mb-1.5 block">发票文件 <span class="text-red-500">*</span></label>
          <input
            type="file"
            accept="application/pdf,.pdf"
            class="block w-full text-sm text-gray-600 file:mr-4 file:rounded-lg file:border-0 file:bg-primary-50 file:px-4 file:py-2 file:text-sm file:font-medium file:text-primary-700 hover:file:bg-primary-100 dark:text-dark-300 dark:file:bg-primary-500/10 dark:file:text-primary-300"
            @change="handleIssueFileChange"
          />
          <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">仅支持 PDF，最大 10MB。</p>
        </div>
        <p v-if="issueError" class="text-sm text-red-600 dark:text-red-300">{{ issueError }}</p>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="closeIssueDialog">取消</button>
          <button type="submit" form="invoice-issue-form" class="btn btn-primary" :disabled="actionLoading">
            {{ actionLoading ? '处理中...' : '确认开票' }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <BaseDialog :show="Boolean(rejectTarget)" title="驳回申请" width="normal" @close="closeRejectDialog">
      <form id="invoice-reject-form" class="space-y-4" @submit.prevent="submitReject">
        <TextArea v-model="rejectReason" label="驳回原因" rows="4" required />
        <p v-if="rejectError" class="text-sm text-red-600 dark:text-red-300">{{ rejectError }}</p>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="closeRejectDialog">取消</button>
          <button type="submit" form="invoice-reject-form" class="btn btn-danger" :disabled="actionLoading">
            {{ actionLoading ? '处理中...' : '确认驳回' }}
          </button>
        </div>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, reactive, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Input from '@/components/common/Input.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import TextArea from '@/components/common/TextArea.vue'
import { useClipboard } from '@/composables/useClipboard'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'
import {
  downloadAdminInvoiceFile,
  getAdminInvoiceApplication,
  issueInvoiceApplication,
  listAdminInvoiceApplications,
  rejectInvoiceApplication,
  testSendGeneratedAdminInvoiceEmail,
} from '../api'
import type { InvoiceApplication, InvoiceApplicationStatus } from '../types'
import {
  formatInvoiceAmount,
  formatInvoiceDate,
  formatInvoiceFileSize,
  invoiceStatusClass,
  invoiceStatusLabel,
  saveInvoiceBlob,
} from '../utils'

const InfoItem = defineComponent({
  props: { label: { type: String, required: true }, value: { type: String, required: true } },
  setup(props) {
    return () => h('div', { class: 'rounded-xl bg-gray-50 p-3 dark:bg-dark-800' }, [
      h('div', { class: 'text-xs text-gray-500 dark:text-dark-400' }, props.label),
      h('div', { class: 'mt-1 break-words text-gray-900 dark:text-white' }, props.value),
    ])
  },
})

const appStore = useAppStore()
const { copyToClipboard } = useClipboard()
const applications = ref<InvoiceApplication[]>([])
const loading = ref(false)
const actionLoading = ref(false)
const errorMessage = ref('')
const detail = ref<InvoiceApplication | null>(null)
const issueTarget = ref<InvoiceApplication | null>(null)
const rejectTarget = ref<InvoiceApplication | null>(null)
const issueFile = ref<File | null>(null)
const downloadingID = ref<number | null>(null)
const testSending = ref(false)
const testEmailDialogOpen = ref(false)
const testEmailError = ref('')
const testEmailReceiverEmail = ref('')
const issueError = ref('')
const rejectError = ref('')
const rejectReason = ref('')

const filters = reactive({
  status: '',
})

const pagination = reactive({
  total: 0,
  page: 1,
  page_size: 20,
})

const issueForm = reactive({
  invoice_number: '',
  admin_remark: '',
})

const statusOptions = computed(() => [
  { value: '', label: '全部状态' },
  { value: 'pending', label: invoiceStatusLabel('pending') },
  { value: 'issued', label: invoiceStatusLabel('issued') },
  { value: 'rejected', label: invoiceStatusLabel('rejected') },
])

onMounted(() => {
  void loadApplications()
})

/**
 * 读取管理员开票申请列表，并保留分页状态。
 */
async function loadApplications(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  try {
    const page = await listAdminInvoiceApplications({
      page: pagination.page,
      page_size: pagination.page_size,
      status: filters.status,
    })
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
 * 状态筛选变化时回到第一页，避免旧页码导致空列表。
 */
function handleStatusChange(value: string | number | boolean | null): void {
  filters.status = typeof value === 'string' ? value : ''
  pagination.page = 1
  void loadApplications()
}

/**
 * 切换列表页码。
 *
 * @param page - 目标页码。
 */
function handlePageChange(page: number): void {
  pagination.page = page
  void loadApplications()
}

/**
 * 切换每页数量并回到第一页。
 *
 * @param pageSize - 每页数量。
 */
function handlePageSizeChange(pageSize: number): void {
  pagination.page_size = pageSize
  pagination.page = 1
  void loadApplications()
}

/**
 * 打开测试发件弹窗。
 */
function openTestEmailDialog(): void {
  if (testSending.value) return
  testEmailDialogOpen.value = true
  testEmailError.value = ''
}

/**
 * 关闭测试发件弹窗；发送中不关闭，避免用户误判发件状态。
 */
function closeTestEmailDialog(): void {
  if (testSending.value) return
  testEmailDialogOpen.value = false
  testEmailError.value = ''
}

/**
 * 读取申请详情。
 *
 * @param app - 当前列表行。
 */
async function openDetail(app: InvoiceApplication): Promise<void> {
  try {
    detail.value = await getAdminInvoiceApplication(app.id)
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, '开票申请详情读取失败'))
  }
}

/**
 * 打开标记开票弹窗。
 *
 * @param app - 待处理申请。
 */
function openIssueDialog(app: InvoiceApplication): void {
  issueTarget.value = app
  issueForm.invoice_number = ''
  issueForm.admin_remark = ''
  issueFile.value = null
  issueError.value = ''
}

/**
 * 关闭标记开票弹窗。
 */
function closeIssueDialog(): void {
  issueTarget.value = null
  issueFile.value = null
  issueError.value = ''
}

/**
 * 记录管理员选择的 PDF 文件。
 *
 * @param event - 文件输入变更事件。
 */
function handleIssueFileChange(event: Event): void {
  const input = event.target as HTMLInputElement
  issueFile.value = input.files?.[0] || null
}

/**
 * 提交开票结果；发票号码、备注和 PDF 都是必填。
 */
async function submitIssue(): Promise<void> {
  if (!issueTarget.value) return
  if (!issueForm.invoice_number.trim() || !issueForm.admin_remark.trim() || !issueFile.value) {
    issueError.value = '请填写开票信息并上传 PDF'
    return
  }
  actionLoading.value = true
  issueError.value = ''
  try {
    const updated = await issueInvoiceApplication(issueTarget.value.id, {
      invoice_number: issueForm.invoice_number,
      admin_remark: issueForm.admin_remark,
      file: issueFile.value,
    })
    replaceApplication(updated)
    issueTarget.value = null
    detail.value = updated
    appStore.showSuccess('开票状态已更新')
  } catch (err) {
    issueError.value = extractApiErrorMessage(err, '开票状态更新失败')
    appStore.showError(issueError.value)
  } finally {
    actionLoading.value = false
  }
}

/**
 * 打开驳回弹窗。
 *
 * @param app - 待处理申请。
 */
function openRejectDialog(app: InvoiceApplication): void {
  rejectTarget.value = app
  rejectReason.value = ''
  rejectError.value = ''
}

/**
 * 关闭驳回弹窗。
 */
function closeRejectDialog(): void {
  rejectTarget.value = null
  rejectReason.value = ''
  rejectError.value = ''
}

/**
 * 提交驳回原因；后端会释放订单占用。
 */
async function submitReject(): Promise<void> {
  if (!rejectTarget.value) return
  if (!rejectReason.value.trim()) {
    rejectError.value = '请填写驳回原因'
    return
  }
  actionLoading.value = true
  rejectError.value = ''
  try {
    const updated = await rejectInvoiceApplication(rejectTarget.value.id, { reason: rejectReason.value })
    replaceApplication(updated)
    rejectTarget.value = null
    detail.value = updated
    appStore.showSuccess('申请已驳回')
  } catch (err) {
    rejectError.value = extractApiErrorMessage(err, '申请驳回失败')
    appStore.showError(rejectError.value)
  } finally {
    actionLoading.value = false
  }
}

/**
 * 用接口返回的新状态更新当前页列表。
 *
 * @param updated - 后端返回的申请记录。
 */
function replaceApplication(updated: InvoiceApplication): void {
  applications.value = applications.value.map(app => (app.id === updated.id ? updated : app))
  if (filters.status && updated.status !== (filters.status as InvoiceApplicationStatus)) {
    applications.value = applications.value.filter(app => app.id !== updated.id)
    pagination.total = Math.max(0, pagination.total - 1)
  }
}

/**
 * 复制当前详情中的抬头快照，便于管理员粘贴到开票系统核对。
 */
async function copyTitleInfo(): Promise<void> {
  if (!detail.value) {
    appStore.showError('开票申请详情不存在')
    return
  }
  const text = [
    `公司抬头：${detail.value.company_title}`,
    `税号：${detail.value.tax_number}`,
    `接收邮箱：${detail.value.receiver_email}`,
  ].join('\n')
  await copyToClipboard(text, '抬头信息已复制')
}

/**
 * 通过统一 API client 下载发票 PDF，复用 token 刷新和错误处理。
 *
 * @param app - 已开票申请。
 */
async function downloadInvoice(app: InvoiceApplication): Promise<void> {
  downloadingID.value = app.id
  try {
    const blob = await downloadAdminInvoiceFile(app.id)
    saveInvoiceBlob(blob, app.file_original_name || `invoice-${app.id}.pdf`)
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, '发票下载失败'))
  } finally {
    downloadingID.value = null
  }
}

/**
 * 发送生成信息的测试邮件，不需要真实开票申请。
 */
async function submitTestEmail(): Promise<void> {
  const receiverEmail = testEmailReceiverEmail.value.trim()
  if (!receiverEmail) {
    testEmailError.value = '请填写接收邮箱'
    return
  }
  testSending.value = true
  testEmailError.value = ''
  try {
    await testSendGeneratedAdminInvoiceEmail({ receiver_email: receiverEmail })
    appStore.showSuccess('测试邮件已发送')
    testEmailDialogOpen.value = false
  } catch (err) {
    testEmailError.value = extractApiErrorMessage(err, '测试邮件发送失败')
    appStore.showError(testEmailError.value)
  } finally {
    testSending.value = false
  }
}
</script>
