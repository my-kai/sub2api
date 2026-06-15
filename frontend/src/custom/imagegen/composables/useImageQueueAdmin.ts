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
import type {
  ImageAdminUserOption,
  ImageQueueConfig,
  ImageUpstreamChannel,
  ImageUpstreamChannelForm,
  ImageUpstreamChannelInput,
  ImageUpstreamChannelType,
  ImageUserLimit,
} from '../types'
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
  upstream_channels: ImageUpstreamChannelForm[]
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
    upstream_channels: [],
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
        upstream_channels: buildUpstreamChannelInputs(configForm.upstream_channels),
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

  /**
   * 新增上游渠道。
   */
  function addUpstreamChannel(type: ImageUpstreamChannelType = 'chatgpt2api'): void {
    configForm.upstream_channels.push(createBlankUpstreamChannel(type, configForm.upstream_channels.length))
    sortChannelsByPriorityInPlace(configForm.upstream_channels)
  }

  /**
   * 更新单个渠道字段，统一放在 composable 里避免组件直接改 props。
   */
  function updateUpstreamChannel(index: number, patch: Partial<ImageUpstreamChannelForm>): void {
    const channel = configForm.upstream_channels[index]
    if (!channel) return
    Object.assign(channel, patch)
    if (Object.prototype.hasOwnProperty.call(patch, 'priority')) {
      sortChannelsByPriorityInPlace(configForm.upstream_channels)
    }
  }

  /**
   * 删除单个上游渠道；后端当前用空列表表示“保持不变”，所以前端至少保留一条。
   */
  function removeUpstreamChannel(index: number): void {
    if (configForm.upstream_channels.length <= 1 || index < 0 || index >= configForm.upstream_channels.length) return
    configForm.upstream_channels.splice(index, 1)
  }

  /**
   * 快捷调整渠道优先级；后端始终按 priority 从小到大选择渠道。
   */
  function moveUpstreamChannel(index: number, direction: -1 | 1): void {
    const targetIndex = index + direction
    if (index < 0 || targetIndex < 0 || index >= configForm.upstream_channels.length || targetIndex >= configForm.upstream_channels.length) {
      return
    }
    const current = configForm.upstream_channels[index]
    const target = configForm.upstream_channels[targetIndex]
    const currentPriority = normalizePriority(current.priority, index)
    current.priority = normalizePriority(target.priority, targetIndex)
    target.priority = currentPriority
    // 前端列表立即按后端同一规则重排，保存前就能看到真实生效顺序。
    sortChannelsByPriorityInPlace(configForm.upstream_channels)
  }

  return {
    addUpstreamChannel,
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
    moveUpstreamChannel,
    removeUpstreamChannel,
    savingConfig,
    savingLimit,
    scheduleUserSearch,
    selectUserOption,
    selectedUserLabel,
    updateUpstreamChannel,
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
  form.upstream_channels.splice(0, form.upstream_channels.length, ...sortChannelsByPriority(normalizeConfigChannels(config)))
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
 * 将后端渠道响应转换为表单状态；旧响应只带 chatgpt2api 时也转成渠道列表展示。
 */
function normalizeConfigChannels(config: ImageQueueConfig): ImageUpstreamChannelForm[] {
  const channels = Array.isArray(config.upstream_channels) ? config.upstream_channels : []
  if (channels.length > 0) {
    return channels.map((channel, index) => channelToFormValue(channel, index))
  }
  if (config.chatgpt2api?.base_url || config.chatgpt2api?.auth_key_configured) {
    return [channelToFormValue({
      id: 'chatgpt2api',
      name: 'chatgpt2api',
      type: 'chatgpt2api',
      enabled: true,
      priority: defaultUpstreamChannelPriority(0),
      base_url: config.chatgpt2api.base_url,
      auth_key_configured: config.chatgpt2api.auth_key_configured,
      retry_count: 10,
    }, 0)]
  }
  // 后端当前将空 upstream_channels 解释为“不更新”，管理页用默认空渠道避免保存时产生歧义。
  return [createBlankUpstreamChannel('chatgpt2api', 0)]
}

/**
 * 构造空白渠道，ID 在前端先稳定生成，便于后端按 ID 保留密钥。
 */
function createBlankUpstreamChannel(type: ImageUpstreamChannelType, index: number): ImageUpstreamChannelForm {
  return {
    id: createUpstreamChannelID(type),
    name: defaultUpstreamChannelName(type),
    type,
    enabled: true,
    priority: defaultUpstreamChannelPriority(index),
    base_url: '',
    auth_key: '',
    clear_auth_key: false,
    auth_key_configured: false,
    retry_count: 10,
  }
}

/**
 * 将管理页表单压成保存请求，避免把只读的密钥配置状态提交给后端。
 */
function buildUpstreamChannelInputs(channels: ImageUpstreamChannelForm[]): ImageUpstreamChannelInput[] {
  return sortChannelsByPriority([...channels]).map((channel, index) => ({
    id: channel.id.trim(),
    name: channel.name.trim(),
    type: normalizeUpstreamChannelType(channel.type),
    enabled: channel.enabled !== false,
    priority: normalizePriority(channel.priority, index),
    base_url: channel.base_url.trim(),
    auth_key: channel.auth_key.trim(),
    clear_auth_key: channel.clear_auth_key,
    retry_count: normalizeNonNegativeInt(channel.retry_count, 10),
  }))
}

/**
 * 归一化单个渠道到表单形态，密钥字段始终留空以保持“留空保留”的安全语义。
 */
function channelToFormValue(channel: ImageUpstreamChannel, index: number): ImageUpstreamChannelForm {
  const type = normalizeUpstreamChannelType(channel.type)
  return {
    id: channel.id?.trim() || `${type}-${index + 1}`,
    name: channel.name?.trim() || defaultUpstreamChannelName(type),
    type,
    enabled: channel.enabled !== false,
    priority: normalizePriority(channel.priority, index),
    base_url: channel.base_url?.trim() ?? '',
    auth_key: '',
    clear_auth_key: false,
    auth_key_configured: Boolean(channel.auth_key_configured),
    retry_count: normalizeNonNegativeInt(channel.retry_count, 10),
  }
}

/**
 * 生成前端新增渠道 ID；ID 不展示给用户，只用于保存时匹配旧密钥。
 */
function createUpstreamChannelID(type: ImageUpstreamChannelType): string {
  const randomPart = Math.random().toString(36).slice(2, 8)
  return `${type}-${Date.now().toString(36)}-${randomPart}`
}

/**
 * 未知渠道类型按 chatgpt2api 兜底，避免旧脏数据让页面表单不可保存。
 */
function normalizeUpstreamChannelType(type: string): ImageUpstreamChannelType {
  return type === 'openai' ? 'openai' : 'chatgpt2api'
}

/**
 * 按渠道类型生成默认名称。
 */
function defaultUpstreamChannelName(type: ImageUpstreamChannelType): string {
  return type === 'openai' ? 'OpenAI' : 'chatgpt2api'
}

/**
 * 默认优先级给旧数据和新增渠道留出插队空间。
 */
function defaultUpstreamChannelPriority(index: number): number {
  return (Math.max(0, index) + 1) * 100
}

/**
 * priority 支持 0 和负数；只有非数字时才按当前位置兜底。
 */
function normalizePriority(value: number, index: number): number {
  if (!Number.isFinite(value)) return defaultUpstreamChannelPriority(index)
  return Math.trunc(value)
}

/**
 * 归一化非负整数配置。
 */
function normalizeNonNegativeInt(value: number, fallback: number): number {
  if (!Number.isFinite(value) || value < 0) return fallback
  return Math.trunc(value)
}

/**
 * 与后端保持一致：值越低越优先；相同 priority 保持原相对顺序。
 */
function sortChannelsByPriority<T extends Pick<ImageUpstreamChannelForm, 'priority'>>(channels: T[]): T[] {
  return channels
    .map((channel, index) => ({ channel, index }))
    .sort((left, right) => {
      const priorityDiff = normalizePriority(left.channel.priority, left.index) - normalizePriority(right.channel.priority, right.index)
      return priorityDiff !== 0 ? priorityDiff : left.index - right.index
    })
    .map(({ channel }) => channel)
}

/**
 * 对响应式数组原地重排，避免只生成排序结果但页面仍显示旧顺序。
 */
function sortChannelsByPriorityInPlace(channels: ImageUpstreamChannelForm[]): void {
  channels.splice(0, channels.length, ...sortChannelsByPriority(channels))
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
