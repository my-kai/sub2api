import { computed, onMounted, ref, watch } from 'vue'
import { useAppStore } from '@/stores/app'
import {
  cancelImageTask,
  createImageEditTask,
  createImageSession,
  createImageTaskWithBalance,
  deleteImageSession,
  fetchImagePriceQuote,
  fetchImageSessions,
  resetImageSessionCurrentImage,
  retryImageTask,
  setImageSessionCurrentImage,
} from '../api'
import type { ImageGenerationTask, ImagePriceQuote, ImageSession } from '../types'
import {
  aspectRatioLabel,
  clampImageCount,
  compareTaskCreatedAsc,
  defaultImageModelID,
  errorMessage,
  formatDateTime,
  formatPriceQuote,
  formatTaskCharge,
  imageAspectRatioOptions,
  imageQualityOptions,
  imageResolutionOptions,
  imageSizeFor,
  isAspectRatioSupported,
  maxCustomImageCount,
  mergeImageSessions,
  normalizeAspectRatioForResolution,
  qualityLabel,
  resolutionLabel,
  resolveCurrentTaskImage,
  taskImageKey,
  taskImages,
  taskModeLabel,
  type ImageQuality,
  type ImageResolution,
} from '../viewHelpers'
import { useImageGenerationStatus } from './useImageGenerationStatus'
import { useImageSessionTasks } from './useImageSessionTasks'
import { usePendingEditImage } from './usePendingEditImage'

/**
 * useImageGenerationWorkspace 承载用户生图页状态机。
 *
 * 视图文件只保留布局；会话、任务、SSE、上传图和报价等流程集中在这里，减少大页面后续维护成本。
 */
export function useImageGenerationWorkspace() {
  const appStore = useAppStore()
  const imageGenerationStatus = useImageGenerationStatus()

  const selectedModel = ref(defaultImageModelID)
  const quality = ref<ImageQuality>('low')
  const resolution = ref<ImageResolution>('1k')
  const aspectRatio = ref('2:3')
  const count = ref(1)
  const publishToGallery = ref(false)
  const prompt = ref('')
  const {
    clearPendingEditImage,
    fileInputRef,
    handleFileInput,
    handlePaste,
    pendingEditImage,
  } = usePendingEditImage()

  const sessions = ref<ImageSession[]>([])
  const selectedSessionId = ref<number | undefined>()
  const sessionsLoading = ref(false)
  const creatingSession = ref(false)
  const deletingSessionId = ref<number | undefined>()
  const submitting = ref(false)
  const cancelingTaskId = ref<number | null>(null)
  const retryingTaskId = ref<number | null>(null)
  const settingCurrentImageKey = ref('')
  const clearingCurrentImage = ref(false)

  const priceQuote = ref<ImagePriceQuote | null>(null)
  const priceLoading = ref(false)
  const error = ref('')

  const {
    clearTasks,
    handleTaskPageChange,
    handleTaskPageSize,
    mergeTaskUpdates,
    reloadCurrentTasks,
    taskEventsConnected,
    taskEventsFallback,
    taskPageSize,
    tasks,
    tasksLoading,
    tasksPage,
    tasksTotal,
  } = useImageSessionTasks({
    selectedSessionId,
    onSnapshot: () => {
      void loadSessions()
    },
  })

  const selectedSession = computed(() => sessions.value.find((session) => session.id === selectedSessionId.value))
  const imageGenerationEnabled = computed(() => imageGenerationStatus.enabled.value)
  const imageGenerationStatusLoading = computed(() => imageGenerationStatus.loading.value && !imageGenerationStatus.loaded.value)
  const orderedTasks = computed(() => [...tasks.value].sort(compareTaskCreatedAsc))
  const imageSize = computed(() => imageSizeFor(resolution.value, aspectRatio.value))
  const imagePriceText = computed(() => formatPriceQuote(priceQuote.value, priceLoading.value))
  const currentTaskImage = computed(() => resolveCurrentTaskImage(selectedSession.value, tasks.value))
  const editPreviewImage = computed(() => {
    if (pendingEditImage.value) {
      return { src: pendingEditImage.value.src, alt: pendingEditImage.value.file.name || '待编辑图片' }
    }
    if (!currentTaskImage.value) {
      return null
    }
    return {
      src: currentTaskImage.value.src,
      alt: `任务 ${currentTaskImage.value.task.id} 图片 ${currentTaskImage.value.imageIndex + 1}`,
    }
  })

  watch([selectedModel, resolution, imageSize, count], () => {
    if (!imageGenerationStatus.enabled.value) {
      priceQuote.value = null
      return
    }
    void loadPriceQuote()
  })

  watch(imageGenerationEnabled, (enabled) => {
    if (!enabled) {
      clearTasks()
      sessions.value = []
      selectedSessionId.value = undefined
      priceQuote.value = null
      return
    }
    void initializeWorkspace()
  })

  watch(selectedModel, (value) => {
    if (value !== defaultImageModelID) {
      selectedModel.value = defaultImageModelID
    }
  })

  onMounted(() => {
    void initializeWorkspace()
  })

  /**
   * 先读取生图公开开关；关闭时保留页面关闭态，不再额外拉会话列表。
   */
  async function initializeWorkspace(): Promise<void> {
    try {
      const enabled = await imageGenerationStatus.load()
      if (!enabled) {
        clearTasks()
        sessions.value = []
        selectedSessionId.value = undefined
        priceQuote.value = null
        return
      }
    } catch {
      // 状态接口失败时继续按后端创建任务兜底处理，避免临时网络问题把页面整体锁死。
    }
    await loadSessions()
    await loadPriceQuote()
  }

  /**
   * 生图模型固定为 gpt-image-2；提交前统一读取这个值，避免浏览器旧状态残留其他模型。
   */
  function selectedImageModel(): string {
    if (selectedModel.value !== defaultImageModelID) {
      selectedModel.value = defaultImageModelID
    }
    return defaultImageModelID
  }

  /**
   * 按当前表单读取价格预览，提交前不在浏览器本地计算价格规则。
   */
  async function loadPriceQuote(): Promise<void> {
    if (!imageGenerationStatus.enabled.value) {
      priceQuote.value = null
      return
    }
    priceLoading.value = true
    try {
      priceQuote.value = await fetchImagePriceQuote({
        model: selectedImageModel(),
        resolution: resolution.value,
        size: imageSize.value,
        n: clampImageCount(count.value),
      })
    } catch {
      priceQuote.value = null
    } finally {
      priceLoading.value = false
    }
  }

  /**
   * 读取会话列表并自动选中最近会话。
   */
  async function loadSessions(): Promise<void> {
    if (!imageGenerationStatus.enabled.value) {
      sessionsLoading.value = false
      return
    }
    sessionsLoading.value = true
    try {
      const items = await fetchImageSessions()
      sessions.value = mergeImageSessions([], items)
      if (!selectedSessionId.value || !items.some((session) => session.id === selectedSessionId.value)) {
        selectedSessionId.value = sessions.value[0]?.id
      }
    } catch (err) {
      appStore.showWarning(errorMessage(err, '会话读取失败'))
    } finally {
      sessionsLoading.value = false
    }
  }

  /**
   * 创建会话后立即切换选中项。
   */
  async function handleCreateSession(): Promise<void> {
    if (!imageGenerationStatus.enabled.value) {
      appStore.showWarning('生图功能已关闭')
      creatingSession.value = false
      return
    }
    creatingSession.value = true
    try {
      const session = await createImageSession()
      sessions.value = mergeImageSessions(sessions.value, [session])
      selectedSessionId.value = session.id
      clearTasks()
      appStore.showSuccess('会话已创建')
    } catch (err) {
      appStore.showError(errorMessage(err, '创建会话失败'))
    } finally {
      creatingSession.value = false
    }
  }

  /**
   * 切换会话时先清空任务，避免旧会话数据短暂显示在新会话下。
   */
  function handleSelectSession(sessionId: number): void {
    if (selectedSessionId.value === sessionId) {
      return
    }
    selectedSessionId.value = sessionId
    clearTasks()
  }

  /**
   * 删除会话只删除入口，不删除历史任务结果。
   */
  async function handleDeleteSession(sessionId: number): Promise<void> {
    deletingSessionId.value = sessionId
    try {
      await deleteImageSession(sessionId)
      sessions.value = sessions.value.filter((session) => session.id !== sessionId)
      if (selectedSessionId.value === sessionId) {
        selectedSessionId.value = sessions.value[0]?.id
        clearTasks()
      }
      appStore.showSuccess('会话已删除')
    } catch (err) {
      appStore.showError(errorMessage(err, '删除会话失败'))
    } finally {
      deletingSessionId.value = undefined
    }
  }

  /**
   * 提交任务前保证有会话；用户首次进入页面不需要先手动点新建。
   */
  async function ensureSession(): Promise<ImageSession> {
    if (selectedSession.value) {
      return selectedSession.value
    }
    creatingSession.value = true
    try {
      const session = await createImageSession()
      sessions.value = mergeImageSessions(sessions.value, [session])
      selectedSessionId.value = session.id
      return session
    } finally {
      creatingSession.value = false
    }
  }

  /**
   * 创建文生图或图片编辑任务。
   */
  async function handleGenerate(): Promise<void> {
    if (!imageGenerationStatus.enabled.value) {
      appStore.showWarning('生图功能已关闭')
      return
    }
    const trimmedPrompt = prompt.value.trim()
    if (!trimmedPrompt) {
      appStore.showWarning('先写提示词，再开始生成')
      return
    }
    if (submitting.value) {
      return
    }

    submitting.value = true
    error.value = ''
    try {
      const session = await ensureSession()
      const baseInput = {
        session_id: session.id,
        model: selectedImageModel(),
        prompt: trimmedPrompt,
        n: clampImageCount(count.value),
        quality: quality.value,
        size: imageSize.value,
        publish_to_gallery: publishToGallery.value,
      }
      const response = pendingEditImage.value
        ? await createImageEditTask({ ...baseInput, image: pendingEditImage.value.file })
        : await createImageTaskWithBalance(baseInput)
      mergeTaskUpdates([response.task], true)
      prompt.value = ''
      clearPendingEditImage()
      await loadSessions()
    } catch (err) {
      error.value = errorMessage(err, '创建生图任务失败')
      appStore.showError(error.value)
    } finally {
      submitting.value = false
    }
  }

  /**
   * 取消仍在排队中的任务。
   */
  async function handleCancelTask(taskId: number): Promise<void> {
    cancelingTaskId.value = taskId
    try {
      const response = await cancelImageTask(taskId)
      mergeTaskUpdates([response.task])
      appStore.showSuccess(`任务 #${taskId} 已撤销`)
    } catch (err) {
      appStore.showError(errorMessage(err, '撤销任务失败'))
    } finally {
      cancelingTaskId.value = null
    }
  }

  /**
   * 重试失败任务会创建新任务，原任务保持失败记录。
   */
  async function handleRetryTask(taskId: number): Promise<void> {
    if (!imageGenerationStatus.enabled.value) {
      appStore.showWarning('生图功能已关闭')
      retryingTaskId.value = null
      return
    }
    retryingTaskId.value = taskId
    error.value = ''
    try {
      const response = await retryImageTask(taskId)
      mergeTaskUpdates([response.task], true)
      await loadSessions()
      appStore.showSuccess(`任务 #${taskId} 已重新排队`)
    } catch (err) {
      error.value = errorMessage(err, '重试任务失败')
      appStore.showError(error.value)
    } finally {
      retryingTaskId.value = null
    }
  }

  /**
   * 将完成图片指定为会话后续编辑来源。
   */
  async function handleSetCurrentImage(task: ImageGenerationTask, imageIndex: number): Promise<void> {
    if (!selectedSessionId.value) {
      return
    }
    const key = taskImageKey(task.id, imageIndex)
    settingCurrentImageKey.value = key
    try {
      const session = await setImageSessionCurrentImage(selectedSessionId.value, { task_id: task.id, image_index: imageIndex })
      sessions.value = mergeImageSessions(sessions.value, [session])
      appStore.showSuccess('编辑图片已更新')
    } catch (err) {
      appStore.showError(errorMessage(err, '设置编辑图片失败'))
    } finally {
      settingCurrentImageKey.value = ''
    }
  }

  /**
   * 清除后端当前编辑图。
   */
  async function handleResetCurrentImage(): Promise<void> {
    if (!selectedSessionId.value || clearingCurrentImage.value) {
      return
    }
    clearingCurrentImage.value = true
    try {
      const session = await resetImageSessionCurrentImage(selectedSessionId.value)
      sessions.value = mergeImageSessions(sessions.value, [session])
      appStore.showSuccess('已取消指定编辑图片')
    } catch (err) {
      appStore.showError(errorMessage(err, '取消指定编辑图片失败'))
    } finally {
      clearingCurrentImage.value = false
    }
  }

  /**
   * 根据用户点击或上传状态清除编辑图。
   */
  function clearEditImage(): void {
    if (pendingEditImage.value) {
      clearPendingEditImage()
      return
    }
    void handleResetCurrentImage()
  }

  /**
   * 分辨率变化时同步修正宽高比。
   */
  function handleResolutionChange(): void {
    aspectRatio.value = normalizeAspectRatioForResolution(resolution.value, aspectRatio.value)
  }

  /**
   * 判断任务图片是否是当前会话编辑图。
   */
  function isCurrentImage(taskId: number, imageIndex: number): boolean {
    return selectedSession.value?.current_image_task_id === taskId
      && selectedSession.value.current_image_index === imageIndex
  }

  return {
    aspectRatio,
    aspectRatioLabel,
    cancelingTaskId,
    clearingCurrentImage,
    clampImageCount,
    count,
    currentTaskImage,
    defaultImageModelID,
    deletingSessionId,
    editPreviewImage,
    error,
    fileInputRef,
    formatDateTime,
    formatTaskCharge,
    handleCancelTask,
    handleCreateSession,
    handleDeleteSession,
    handleFileInput,
    handleGenerate,
    handlePaste,
    clearEditImage,
    handleResetCurrentImage,
    handleResolutionChange,
    handleRetryTask,
    handleSelectSession,
    handleSetCurrentImage,
    handleTaskPageChange,
    handleTaskPageSize,
    imageAspectRatioOptions,
    imagePriceText,
    imageGenerationEnabled,
    imageGenerationStatusLoading,
    imageQualityOptions,
    imageResolutionOptions,
    imageSize,
    isAspectRatioSupported,
    isCurrentImage,
    maxCustomImageCount,
    orderedTasks,
    pendingEditImage,
    prompt,
    publishToGallery,
    quality,
    qualityLabel,
    reloadCurrentTasks,
    resolution,
    resolutionLabel,
    retryingTaskId,
    selectedModel,
    selectedSession,
    selectedSessionId,
    sessions,
    sessionsLoading,
    creatingSession,
    settingCurrentImageKey,
    submitting,
    taskEventsConnected,
    taskEventsFallback,
    taskImageKey,
    taskImages,
    taskModeLabel,
    taskPageSize,
    tasks,
    tasksLoading,
    tasksPage,
    tasksTotal,
  }
}
