import { onBeforeUnmount, ref, watch, type Ref } from 'vue'
import { useAppStore } from '@/stores/app'
import { fetchImageSessionTasks, subscribeImageSessionTasks } from '../api'
import type { ImageGenerationTask, ImageTaskEventSubscription } from '../types'
import { defaultTaskPageSize, errorMessage, mergeImageTasks } from '../viewHelpers'

interface ImageSessionTasksOptions {
  selectedSessionId: Ref<number | undefined>
  onSnapshot?: () => void
}

/**
 * useImageSessionTasks 管理会话任务分页、SSE 和轮询兜底。
 *
 * EventSource 失败时会自动改用短轮询，页面侧只需要消费统一的任务列表状态。
 */
export function useImageSessionTasks(options: ImageSessionTasksOptions) {
  const appStore = useAppStore()
  const taskPageSize = defaultTaskPageSize
  const tasks = ref<ImageGenerationTask[]>([])
  const tasksPage = ref(1)
  const tasksTotal = ref(0)
  const tasksLoading = ref(false)
  const taskEventsConnected = ref(false)
  const taskEventsFallback = ref(false)
  const taskEventsErrorShown = ref(false)
  let taskSubscription: ImageTaskEventSubscription | null = null
  let fallbackTimer: number | undefined

  watch(options.selectedSessionId, (sessionId) => {
    tasksPage.value = 1
    reconnectTaskEvents(sessionId)
  })

  watch(tasksPage, () => {
    if (options.selectedSessionId.value) {
      reconnectTaskEvents(options.selectedSessionId.value)
    }
  })

  onBeforeUnmount(() => {
    closeTaskSubscription()
    stopFallbackPolling()
  })

  /**
   * 分页读取当前会话任务，作为 SSE 不可用时的兜底刷新。
   */
  async function loadSessionTasks(sessionId: number, page = tasksPage.value, signal?: AbortSignal): Promise<void> {
    tasksLoading.value = true
    try {
      const result = await fetchImageSessionTasks(sessionId, { page, page_size: taskPageSize }, { signal })
      tasks.value = Array.isArray(result.items) ? result.items : []
      tasksTotal.value = result.total || 0
      tasksPage.value = result.page || page
    } catch (err) {
      if ((err as { name?: string }).name !== 'AbortError') {
        appStore.showWarning(errorMessage(err, '任务读取失败'))
      }
    } finally {
      if (!signal?.aborted) {
        tasksLoading.value = false
      }
    }
  }

  /**
   * 合并单个或多个任务更新。
   */
  function mergeTaskUpdates(incoming: ImageGenerationTask[], incrementTotal = false): void {
    tasks.value = mergeImageTasks(tasks.value, incoming)
    if (incrementTotal) {
      tasksTotal.value = Math.max(tasksTotal.value + incoming.length, incoming.length)
    }
  }

  /**
   * 清空任务列表，通常用于切换或删除会话。
   */
  function clearTasks(): void {
    tasks.value = []
    tasksTotal.value = 0
  }

  /**
   * 重新读取当前页任务。
   */
  function reloadCurrentTasks(): void {
    if (options.selectedSessionId.value) {
      void loadSessionTasks(options.selectedSessionId.value, tasksPage.value)
    }
  }

  /**
   * 切换任务分页。
   */
  function handleTaskPageChange(page: number): void {
    tasksPage.value = page
  }

  /**
   * 当前任务列表固定 20 条；Pagination 仍要求监听 pageSize 事件。
   */
  function handleTaskPageSize(): void {
    tasksPage.value = 1
  }

  /**
   * 重新建立 SSE；失败后交给轮询兜底。
   */
  function reconnectTaskEvents(sessionId?: number): void {
    closeTaskSubscription()
    stopFallbackPolling()
    taskEventsConnected.value = false
    taskEventsFallback.value = false
    taskEventsErrorShown.value = false

    if (!sessionId) {
      clearTasks()
      return
    }

    tasksLoading.value = true
    taskSubscription = subscribeImageSessionTasks(sessionId, { page: tasksPage.value, page_size: taskPageSize }, {
      onTasks: (event) => {
        const result = event.tasks
        tasks.value = Array.isArray(result.items) ? result.items : []
        tasksTotal.value = result.total || 0
        tasksPage.value = result.page || tasksPage.value
        tasksLoading.value = false
        taskEventsConnected.value = true
        taskEventsFallback.value = false
        taskEventsErrorShown.value = false
        options.onSnapshot?.()
      },
      onError: (err) => {
        taskEventsConnected.value = false
        taskEventsFallback.value = true
        if (!taskEventsErrorShown.value) {
          appStore.showWarning(err.message || '任务更新中断，已切换为备用刷新')
          taskEventsErrorShown.value = true
        }
        startFallbackPolling(sessionId)
      },
    })
  }

  /**
   * 关闭 SSE 订阅。
   */
  function closeTaskSubscription(): void {
    taskSubscription?.close()
    taskSubscription = null
  }

  /**
   * 启动备用轮询，避免 SSE 断开后页面停止更新。
   */
  function startFallbackPolling(sessionId: number): void {
    if (fallbackTimer) {
      return
    }
    void loadSessionTasks(sessionId, tasksPage.value)
    fallbackTimer = window.setInterval(() => {
      void loadSessionTasks(sessionId, tasksPage.value)
    }, 5000)
  }

  /**
   * 停止备用轮询。
   */
  function stopFallbackPolling(): void {
    if (fallbackTimer) {
      window.clearInterval(fallbackTimer)
      fallbackTimer = undefined
    }
  }

  return {
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
  }
}
