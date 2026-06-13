import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useAppStore } from '@/stores/app'
import {
  deleteImageUserLimit,
  fetchImageQueueConfig,
  fetchImageUserLimits,
  saveImageQueueConfig,
  saveImageUserLimit,
  searchImageAdminUsers,
} from '../api/admin'
import type { ImageAdminUserOption, ImageQueueConfig, ImageUserLimit } from '../types'
import { errorMessage, formatDateTime } from '../viewHelpers'
import { useImageGenerationStatus } from './useImageGenerationStatus'

interface ConfigFormValues {
  enabled: boolean
  platform_concurrency: number
  default_user_concurrency: number
  retention_days: number
  unit_price_1k: number
  unit_price_2k: number
  unit_price_4k: number
  chatgpt2api_base_url: string
  chatgpt2api_auth_key: string
  chatgpt2api_clear_auth_key: boolean
  chatgpt2api_auth_key_configured: boolean
}

interface UserLimitFormValues {
  user_id: number
  concurrency: number
}

/**
 * useImageQueueAdmin 承载生图管理员页状态。
 *
 * 管理页面同时维护配置表单、用户覆盖表格和用户搜索，集中在 composable 中避免页面组件过大。
 */
export function useImageQueueAdmin() {
  const appStore = useAppStore()
  const imageGenerationStatus = useImageGenerationStatus()
  const configForm = reactive<ConfigFormValues>({
    enabled: true,
    platform_concurrency: 2,
    default_user_concurrency: 1,
    retention_days: 7,
    unit_price_1k: 0.134,
    unit_price_2k: 0.268,
    unit_price_4k: 0.4,
    chatgpt2api_base_url: '',
    chatgpt2api_auth_key: '',
    chatgpt2api_clear_auth_key: false,
    chatgpt2api_auth_key_configured: false,
  })
  const limitForm = reactive<UserLimitFormValues>({ user_id: 0, concurrency: 1 })
  const limits = ref<ImageUserLimit[]>([])
  const loading = ref(true)
  const savingConfig = ref(false)
  const savingLimit = ref(false)
  const deletingUserId = ref<number | null>(null)
  const limitFormOpen = ref(false)
  const editingLimit = ref<ImageUserLimit | null>(null)
  const error = ref('')

  const userQuery = ref('')
  const userOptions = ref<ImageAdminUserOption[]>([])
  const userSearchLoading = ref(false)
  let userSearchTimer: number | undefined
  let userSearchAbort: AbortController | null = null

  const selectedUserLabel = computed(() => {
    if (editingLimit.value) return formatUserOptionLabel(limitToUserOption(editingLimit.value))
    const selected = userOptions.value.find((user) => user.id === limitForm.user_id)
    return selected ? formatUserOptionLabel(selected) : ''
  })

  onMounted(() => {
    void loadAdminImages()
  })

  onBeforeUnmount(() => {
    if (userSearchTimer) window.clearTimeout(userSearchTimer)
    userSearchAbort?.abort()
  })

  /**
   * 同时读取平台配置和用户覆盖，保持页面状态一致。
   */
  async function loadAdminImages(): Promise<void> {
    loading.value = true
    error.value = ''
    try {
      const [config, userLimits] = await Promise.all([fetchImageQueueConfig(), fetchImageUserLimits()])
      applyConfigToForm(config, configForm)
      limits.value = userLimits
    } catch (err) {
      error.value = errorMessage(err, '生图配置加载失败')
    } finally {
      loading.value = false
    }
  }

  /**
   * 保存平台配置。
   */
  async function handleSaveConfig(): Promise<void> {
    if (savingConfig.value) return
    savingConfig.value = true
    try {
      await saveImageQueueConfig({
        enabled: configForm.enabled,
        platform_concurrency: normalizePositiveInt(configForm.platform_concurrency, 2),
        default_user_concurrency: normalizePositiveInt(configForm.default_user_concurrency, 1),
        retention_days: normalizePositiveInt(configForm.retention_days, 7),
        unit_prices: {
          one_k: toFixedDecimal(configForm.unit_price_1k, 5),
          two_k: toFixedDecimal(configForm.unit_price_2k, 5),
          four_k: toFixedDecimal(configForm.unit_price_4k, 5),
        },
        chatgpt2api: {
          base_url: configForm.chatgpt2api_base_url.trim(),
          auth_key: configForm.chatgpt2api_auth_key.trim(),
          clear_auth_key: configForm.chatgpt2api_clear_auth_key,
        },
      })
      appStore.showSuccess('生图配置已保存')
      await imageGenerationStatus.load({ force: true })
      await loadAdminImages()
    } catch (err) {
      appStore.showError(errorMessage(err, '保存生图配置失败'))
    } finally {
      savingConfig.value = false
    }
  }

  /**
   * 打开新增用户覆盖表单。
   */
  function openCreateLimitForm(): void {
    editingLimit.value = null
    limitForm.user_id = 0
    limitForm.concurrency = 1
    userQuery.value = ''
    userOptions.value = []
    limitFormOpen.value = true
  }

  /**
   * 打开编辑用户覆盖表单。
   */
  function openEditLimitForm(limit: ImageUserLimit): void {
    editingLimit.value = limit
    limitForm.user_id = limit.user_id
    limitForm.concurrency = limit.concurrency
    userQuery.value = formatUserOptionLabel(limitToUserOption(limit))
    userOptions.value = [limitToUserOption(limit)]
    limitFormOpen.value = true
  }

  /**
   * 关闭并重置用户覆盖表单。
   */
  function closeLimitForm(): void {
    limitFormOpen.value = false
    editingLimit.value = null
    limitForm.user_id = 0
    limitForm.concurrency = 1
    userQuery.value = ''
    userOptions.value = []
  }

  /**
   * 保存单个用户并发覆盖。
   */
  async function handleSaveLimit(): Promise<void> {
    if (savingLimit.value) return
    if (!limitForm.user_id) {
      appStore.showWarning('请选择用户')
      return
    }

    savingLimit.value = true
    try {
      await saveImageUserLimit(limitForm.user_id, { concurrency: normalizePositiveInt(limitForm.concurrency, 1) })
      closeLimitForm()
      appStore.showSuccess('用户并发覆盖已保存')
      await loadAdminImages()
    } catch (err) {
      appStore.showError(errorMessage(err, '保存用户并发覆盖失败'))
    } finally {
      savingLimit.value = false
    }
  }

  /**
   * 删除用户并发覆盖。
   */
  async function handleDeleteLimit(userId: number): Promise<void> {
    if (deletingUserId.value !== null || !window.confirm('删除该用户覆盖？')) return
    deletingUserId.value = userId
    try {
      await deleteImageUserLimit(userId)
      appStore.showSuccess('用户并发覆盖已删除')
      await loadAdminImages()
    } catch (err) {
      appStore.showError(errorMessage(err, '删除用户并发覆盖失败'))
    } finally {
      deletingUserId.value = null
    }
  }

  /**
   * 输入搜索时做简单防抖，避免每个字符都请求后端。
   */
  function scheduleUserSearch(): void {
    if (userSearchTimer) window.clearTimeout(userSearchTimer)
    userSearchTimer = window.setTimeout(() => {
      void loadUserOptions(userQuery.value)
    }, 300)
  }

  /**
   * 聚焦空列表时加载默认候选项。
   */
  function ensureUserOptions(): void {
    if (editingLimit.value || userOptions.value.length > 0) return
    void loadUserOptions(userQuery.value)
  }

  /**
   * 搜索可设置覆盖的用户。
   */
  async function loadUserOptions(query: string): Promise<void> {
    userSearchAbort?.abort()
    const controller = new AbortController()
    userSearchAbort = controller
    userSearchLoading.value = true
    try {
      const options = await searchImageAdminUsers(query, { signal: controller.signal })
      userOptions.value = mergeUserOptions(options, editingLimit.value ? [limitToUserOption(editingLimit.value)] : [])
    } catch (err) {
      if ((err as { name?: string }).name !== 'AbortError') {
        appStore.showError(errorMessage(err, '用户搜索失败'))
      }
    } finally {
      if (!controller.signal.aborted) userSearchLoading.value = false
    }
  }

  /**
   * 选择搜索结果中的用户。
   */
  function selectUserOption(user: ImageAdminUserOption): void {
    limitForm.user_id = user.id
    userQuery.value = formatUserOptionLabel(user)
  }

  return {
    closeLimitForm,
    configForm,
    deletingUserId,
    editingLimit,
    ensureUserOptions,
    error,
    formatDateTime,
    formatUserOptionLabel,
    handleDeleteLimit,
    handleSaveConfig,
    handleSaveLimit,
    limitForm,
    limitFormOpen,
    limits,
    loadAdminImages,
    loading,
    openCreateLimitForm,
    openEditLimitForm,
    savingConfig,
    savingLimit,
    scheduleUserSearch,
    selectUserOption,
    selectedUserLabel,
    userOptions,
    userQuery,
    userSearchLoading,
  }
}

/**
 * 将后端配置写入表单。
 */
function applyConfigToForm(config: ImageQueueConfig, form: ConfigFormValues): void {
  form.enabled = config.enabled !== false
  form.platform_concurrency = normalizePositiveInt(config.platform_concurrency, 2)
  form.default_user_concurrency = normalizePositiveInt(config.default_user_concurrency, 1)
  form.retention_days = normalizePositiveInt(config.retention_days, 7)
  form.unit_price_1k = Number(config.unit_prices?.one_k ?? 0.134)
  form.unit_price_2k = Number(config.unit_prices?.two_k ?? 0.268)
  form.unit_price_4k = Number(config.unit_prices?.four_k ?? 0.4)
  form.chatgpt2api_base_url = config.chatgpt2api?.base_url ?? ''
  form.chatgpt2api_auth_key = ''
  form.chatgpt2api_clear_auth_key = false
  form.chatgpt2api_auth_key_configured = Boolean(config.chatgpt2api?.auth_key_configured)
}

/**
 * 归一化正整数配置。
 */
function normalizePositiveInt(value: number, fallback: number): number {
  if (!Number.isFinite(value) || value < 1) return fallback
  return Math.trunc(value)
}

/**
 * 固定小数位输出价格字符串，避免浏览器浮点值直接进入后端配置。
 */
function toFixedDecimal(value: number, precision: number): string {
  const numeric = Number.isFinite(value) && value >= 0 ? value : 0
  return numeric.toFixed(precision)
}

/**
 * 从用户限制记录构造搜索候选项。
 */
function limitToUserOption(limit: ImageUserLimit): ImageAdminUserOption {
  return { id: limit.user_id, username: limit.username, email: limit.email }
}

/**
 * 合并搜索结果，确保编辑中的用户不会从选项里消失。
 */
function mergeUserOptions(primary: ImageAdminUserOption[], fallback: ImageAdminUserOption[]): ImageAdminUserOption[] {
  const seen = new Set<number>()
  const merged: ImageAdminUserOption[] = []
  for (const option of [...fallback, ...primary]) {
    if (!option.id || seen.has(option.id)) continue
    seen.add(option.id)
    merged.push(option)
  }
  return merged
}

/**
 * 用户选项展示短文案。
 */
function formatUserOptionLabel(user: ImageAdminUserOption): string {
  const parts = [`ID ${user.id}`]
  if (user.email) parts.push(user.email)
  if (user.username) parts.push(user.username)
  return parts.join(' · ')
}
