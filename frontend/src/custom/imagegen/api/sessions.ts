import { apiClient } from '@/api/client'
import {
  buildEventSourceURL,
  buildPageParams,
  normalizeItems,
  normalizePage,
} from './helpers'
import type {
  ImageGenerationTask,
  ImagePageParams,
  ImagePageResult,
  ImageSession,
  ImageSessionListResponse,
  ImageSessionResponse,
  ImageSessionTasksEvent,
  ImageTaskEventSubscription,
  SetCurrentImageRequest,
} from '../types'

const USER_IMAGE_API_PREFIX = '/custom/images'

/**
 * 创建服务端生图会话。
 */
export async function createImageSession(title = '', options?: { signal?: AbortSignal }): Promise<ImageSession> {
  const { data } = await apiClient.post<ImageSessionResponse>(
    `${USER_IMAGE_API_PREFIX}/sessions`,
    { title },
    { signal: options?.signal },
  )
  return data.session
}

/**
 * 读取当前用户会话列表。
 */
export async function fetchImageSessions(options?: { signal?: AbortSignal }): Promise<ImageSession[]> {
  const { data } = await apiClient.get<ImageSessionListResponse>(`${USER_IMAGE_API_PREFIX}/sessions`, {
    signal: options?.signal,
  })
  return normalizeItems(data.items)
}

/**
 * 修改当前用户自己的会话标题。
 */
export async function updateImageSession(sessionId: number, title: string): Promise<ImageSession> {
  const { data } = await apiClient.patch<ImageSessionResponse>(
    `${USER_IMAGE_API_PREFIX}/sessions/${sessionId}`,
    { title },
  )
  return data.session
}

/**
 * 删除当前用户自己的会话。
 */
export async function deleteImageSession(sessionId: number): Promise<{ deleted: boolean }> {
  const { data } = await apiClient.delete<{ deleted: boolean }>(`${USER_IMAGE_API_PREFIX}/sessions/${sessionId}`)
  return data
}

/**
 * 设置当前会话的编辑来源图。
 */
export async function setImageSessionCurrentImage(
  sessionId: number,
  input: SetCurrentImageRequest,
): Promise<ImageSession> {
  const { data } = await apiClient.post<ImageSessionResponse>(
    `${USER_IMAGE_API_PREFIX}/sessions/${sessionId}/current-image`,
    input,
  )
  return data.session
}

/**
 * 重置当前会话的编辑来源图。
 */
export async function resetImageSessionCurrentImage(sessionId: number): Promise<ImageSession> {
  const { data } = await apiClient.delete<ImageSessionResponse>(
    `${USER_IMAGE_API_PREFIX}/sessions/${sessionId}/current-image`,
  )
  return data.session
}

/**
 * 分页读取会话任务。
 */
export async function fetchImageSessionTasks(
  sessionId: number,
  params: ImagePageParams = {},
  options?: { signal?: AbortSignal },
): Promise<ImagePageResult<ImageGenerationTask>> {
  const { data } = await apiClient.get<ImagePageResult<ImageGenerationTask>>(
    `${USER_IMAGE_API_PREFIX}/sessions/${sessionId}/tasks`,
    {
      params: buildPageParams(params),
      signal: options?.signal,
    },
  )
  return normalizePage(data)
}

/**
 * 订阅会话任务快照。
 *
 * 任务列表页面可优先使用 SSE；断开后由页面自行切回轮询，避免组件层隐式刷新。
 */
export function subscribeImageSessionTasks(
  sessionId: number,
  params: ImagePageParams,
  handlers: {
    onTasks: (event: ImageSessionTasksEvent) => void
    onError?: (error: Error) => void
  },
): ImageTaskEventSubscription {
  const pageParams = buildPageParams(params)
  const source = new EventSource(
    buildEventSourceURL(`${USER_IMAGE_API_PREFIX}/sessions/${sessionId}/tasks/events`, pageParams),
    { withCredentials: true },
  )

  source.addEventListener('tasks', (event) => {
    try {
      handlers.onTasks(JSON.parse((event as MessageEvent).data) as ImageSessionTasksEvent)
    } catch {
      handlers.onError?.(new Error('任务更新失败'))
    }
  })

  source.addEventListener('snapshot_error', (event) => {
    handlers.onError?.(new Error(parseSSEErrorMessage((event as MessageEvent).data)))
  })

  source.onerror = () => {
    handlers.onError?.(new Error('任务更新中断'))
  }

  return {
    close: () => source.close(),
  }
}

/**
 * 解析 SSE 错误消息。
 */
function parseSSEErrorMessage(data: string): string {
  if (!data) {
    return '任务更新失败'
  }

  try {
    const parsed = JSON.parse(data) as { message?: string }
    return parsed.message || '任务更新失败'
  } catch {
    return '任务更新失败'
  }
}
