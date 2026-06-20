import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import {
  createAdminCustomActivity,
  endAdminCustomActivity,
  fetchAdminCustomActivities,
  fetchAdminCustomActivityClaims,
  fetchAdminCustomActivityDetail,
  offlineAdminCustomActivity,
  updateAdminCustomActivity,
} from '../api'
import { isCanceledRequest } from './adminActivityViewHelpers'
import { isEditableActivityStatus } from './adminRedPacketRainFormModel'
import type {
  AdminCustomActivityClaimItem,
  AdminCustomActivityDetail,
  AdminCustomActivityListItem,
  AdminCustomActivityUpsertRequest,
} from '../types'

export type AdminActivityActionKind = 'end' | 'offline'

interface PendingAction {
  kind: AdminActivityActionKind
  activity: AdminCustomActivityListItem
}

/**
 * 管理员活动管理页状态和接口流转。
 *
 * 页面组件只负责布局；这里集中处理列表、详情、领取记录、表单保存和状态操作。
 */
export function useAdminActivityManagement() {
  const appStore = useAppStore()
  const activities = ref<AdminCustomActivityListItem[]>([])
  const selectedActivity = ref<AdminCustomActivityDetail | null>(null)
  const claims = ref<AdminCustomActivityClaimItem[]>([])
  const listLoading = ref(false)
  const detailLoading = ref(false)
  const claimsLoading = ref(false)
  const saving = ref(false)
  const actionLoading = ref(false)
  const errorMessage = ref('')
  const formOpen = ref(false)
  const editingActivity = ref<AdminCustomActivityDetail | null>(null)
  const pendingAction = ref<PendingAction | null>(null)
  const claimsPage = ref(1)
  const claimsPageSize = ref(20)
  const claimsTotal = ref(0)
  let listController: AbortController | null = null
  let detailController: AbortController | null = null
  let claimsController: AbortController | null = null

  const pendingActionTitle = computed(() => pendingAction.value?.kind === 'end' ? '结束活动' : '下架活动')
  const pendingActionMessage = computed(() => pendingAction.value?.kind === 'end' ? '确认结束该活动？' : '确认下架该活动？')
  const formSaveDisabled = computed(() => (
    saving.value || Boolean(editingActivity.value && !isEditableActivityStatus(editingActivity.value.status))
  ))

  onMounted(() => {
    void loadActivities()
  })

  onBeforeUnmount(() => {
    listController?.abort()
    detailController?.abort()
    claimsController?.abort()
  })

  /**
   * 读取活动管理列表。
   */
  async function loadActivities(): Promise<void> {
    listController?.abort()
    listController = new AbortController()
    listLoading.value = true
    errorMessage.value = ''
    try {
      const result = await fetchAdminCustomActivities({ signal: listController.signal })
      activities.value = result.items
      await syncSelectedAfterListLoad()
    } catch (err) {
      if (!isCanceledRequest(err)) {
        errorMessage.value = extractApiErrorMessage(err, '活动读取失败')
        appStore.showError(errorMessage.value)
      }
    } finally {
      if (!listController.signal.aborted) listLoading.value = false
    }
  }

  /**
   * 列表刷新后保持当前选择；当前选择已消失时切到第一条。
   */
  async function syncSelectedAfterListLoad(): Promise<void> {
    if (activities.value.length === 0) {
      selectedActivity.value = null
      claims.value = []
      claimsTotal.value = 0
      return
    }
    const currentID = selectedActivity.value?.id
    const nextActivity = activities.value.find((activity) => activity.id === currentID) || activities.value[0]
    await selectActivity(nextActivity)
  }

  /**
   * 选择活动并读取详情和领取记录。
   */
  async function selectActivity(activity: AdminCustomActivityListItem): Promise<void> {
    selectedActivity.value = { ...activity }
    claimsPage.value = 1
    await reloadSelectedActivity()
  }

  /**
   * 重新读取当前活动详情。
   */
  async function reloadSelectedActivity(): Promise<void> {
    if (!selectedActivity.value) return
    const activityID = selectedActivity.value.id
    detailController?.abort()
    detailController = new AbortController()
    detailLoading.value = true
    try {
      selectedActivity.value = await fetchAdminCustomActivityDetail(activityID, { signal: detailController.signal })
      await loadClaims()
    } catch (err) {
      if (!isCanceledRequest(err)) appStore.showError(extractApiErrorMessage(err, '活动详情读取失败'))
    } finally {
      if (!detailController.signal.aborted) detailLoading.value = false
    }
  }

  /**
   * 读取当前活动领取记录。
   */
  async function loadClaims(): Promise<void> {
    if (!selectedActivity.value) return
    claimsController?.abort()
    claimsController = new AbortController()
    claimsLoading.value = true
    try {
      const result = await fetchAdminCustomActivityClaims(
        selectedActivity.value.id,
        { page: claimsPage.value, page_size: claimsPageSize.value },
        { signal: claimsController.signal },
      )
      claims.value = result.items
      claimsTotal.value = result.total
    } catch (err) {
      if (!isCanceledRequest(err)) appStore.showError(extractApiErrorMessage(err, '领取记录读取失败'))
    } finally {
      if (!claimsController.signal.aborted) claimsLoading.value = false
    }
  }

  /**
   * 打开新建活动表单。
   */
  function openCreateForm(): void {
    editingActivity.value = null
    formOpen.value = true
  }

  /**
   * 打开编辑活动表单。
   */
  async function openEditForm(activity: AdminCustomActivityListItem): Promise<void> {
    if (!isEditableActivityStatus(activity.status)) {
      appStore.showWarning('活动已开始，不可编辑')
      return
    }
    detailLoading.value = true
    try {
      editingActivity.value = await fetchAdminCustomActivityDetail(activity.id)
      formOpen.value = true
    } catch (err) {
      appStore.showError(extractApiErrorMessage(err, '活动详情读取失败'))
    } finally {
      detailLoading.value = false
    }
  }

  /**
   * 关闭活动表单。
   */
  function closeForm(): void {
    if (saving.value) return
    formOpen.value = false
    editingActivity.value = null
  }

  /**
   * 保存新建或编辑活动。
   */
  async function handleSaveActivity(payload: AdminCustomActivityUpsertRequest): Promise<void> {
    saving.value = true
    try {
      const saved = editingActivity.value
        ? await updateAdminCustomActivity(editingActivity.value.id, payload)
        : await createAdminCustomActivity(payload)
      appStore.showSuccess('保存成功')
      formOpen.value = false
      editingActivity.value = null
      selectedActivity.value = saved
      await loadActivities()
    } catch (err) {
      appStore.showError(extractApiErrorMessage(err, '保存失败'))
    } finally {
      saving.value = false
    }
  }

  /**
   * 打开结束或下架确认框。
   */
  function openActionConfirm(kind: AdminActivityActionKind, activity: AdminCustomActivityListItem): void {
    pendingAction.value = { kind, activity }
  }

  /**
   * 执行结束或下架操作。
   */
  async function confirmPendingAction(): Promise<void> {
    if (!pendingAction.value || actionLoading.value) return
    actionLoading.value = true
    const action = pendingAction.value
    try {
      const result = action.kind === 'end'
        ? await endAdminCustomActivity(action.activity.id)
        : await offlineAdminCustomActivity(action.activity.id)
      appStore.showSuccess(result.message || '操作成功')
      pendingAction.value = null
      await loadActivities()
    } catch (err) {
      appStore.showError(extractApiErrorMessage(err, '操作失败'))
    } finally {
      actionLoading.value = false
    }
  }

  /**
   * 切换领取记录页码。
   */
  function handleClaimsPageChange(page: number): void {
    claimsPage.value = page
    void loadClaims()
  }

  /**
   * 切换领取记录每页数量。
   */
  function handleClaimsPageSizeChange(pageSize: number): void {
    claimsPageSize.value = pageSize
    claimsPage.value = 1
    void loadClaims()
  }

  return {
    activities,
    selectedActivity,
    claims,
    listLoading,
    detailLoading,
    claimsLoading,
    saving,
    actionLoading,
    errorMessage,
    formOpen,
    editingActivity,
    pendingAction,
    pendingActionTitle,
    pendingActionMessage,
    formSaveDisabled,
    claimsPage,
    claimsPageSize,
    claimsTotal,
    closeForm,
    confirmPendingAction,
    handleClaimsPageChange,
    handleClaimsPageSizeChange,
    handleSaveActivity,
    loadActivities,
    loadClaims,
    openActionConfirm,
    openCreateForm,
    openEditForm,
    reloadSelectedActivity,
    selectActivity,
  }
}
