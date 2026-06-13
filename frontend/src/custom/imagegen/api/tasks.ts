import { apiClient } from '@/api/client'
import type {
  CreateImageEditTaskRequest,
  CreateImageTaskRequest,
  ImageGenerationTask,
  ImageTaskResponse,
} from '../types'

const USER_IMAGE_API_PREFIX = '/custom/images'

/**
 * 创建生图任务。
 */
export async function createImageTask(input: CreateImageTaskRequest): Promise<ImageGenerationTask> {
  const response = await createImageTaskWithBalance(input)
  return response.task
}

/**
 * 创建生图任务并返回兼容余额字段。
 */
export async function createImageTaskWithBalance(input: CreateImageTaskRequest): Promise<ImageTaskResponse> {
  const { data } = await apiClient.post<ImageTaskResponse>(`${USER_IMAGE_API_PREFIX}/tasks`, input)
  return data
}

/**
 * 携带本地图片创建编辑任务。
 */
export async function createImageEditTask(input: CreateImageEditTaskRequest): Promise<ImageTaskResponse> {
  const body = new FormData()
  body.set('session_id', String(input.session_id))
  body.set('model', input.model)
  body.set('prompt', input.prompt)
  body.set('n', String(input.n))
  if (input.quality) body.set('quality', input.quality)
  if (input.size) body.set('size', input.size)
  if (input.publish_to_gallery) body.set('publish_to_gallery', 'true')
  body.set('image', input.image)

  // apiClient 默认 JSON；显式标记 multipart，避免 FormData 被序列化成普通 JSON。
  const { data } = await apiClient.post<ImageTaskResponse>(`${USER_IMAGE_API_PREFIX}/tasks`, body, {
    headers: {
      'Content-Type': 'multipart/form-data',
    },
  })
  return data
}

/**
 * 查询生图任务状态。
 */
export async function fetchImageTask(taskId: number, options?: { signal?: AbortSignal }): Promise<ImageGenerationTask> {
  const response = await fetchImageTaskWithBalance(taskId, options)
  return response.task
}

/**
 * 查询生图任务状态并返回兼容余额字段。
 */
export async function fetchImageTaskWithBalance(
  taskId: number,
  options?: { signal?: AbortSignal },
): Promise<ImageTaskResponse> {
  const { data } = await apiClient.get<ImageTaskResponse>(`${USER_IMAGE_API_PREFIX}/tasks/${taskId}`, {
    signal: options?.signal,
  })
  return data
}

/**
 * 取消还在排队中的任务。
 */
export async function cancelImageTask(taskId: number): Promise<ImageTaskResponse> {
  const { data } = await apiClient.post<ImageTaskResponse>(`${USER_IMAGE_API_PREFIX}/tasks/${taskId}/cancel`)
  return data
}

/**
 * 重试失败任务。
 */
export async function retryImageTask(taskId: number): Promise<ImageTaskResponse> {
  const { data } = await apiClient.post<ImageTaskResponse>(`${USER_IMAGE_API_PREFIX}/tasks/${taskId}/retry`)
  return data
}
