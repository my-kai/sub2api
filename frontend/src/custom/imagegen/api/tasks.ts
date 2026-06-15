import { apiClient } from '@/api/client'
import type {
  CreateImageEditTaskRequest,
  CreateImageTaskRequest,
  ImageEditSourceReference,
  ImageGenerationTask,
  ImageTaskResponse,
} from '../types'

const USER_IMAGE_API_PREFIX = '/custom/images'
const FIXED_IMAGE_MODEL: CreateImageTaskRequest['model'] = 'gpt-image-2'
const DEFAULT_OUTPUT_FORMAT: NonNullable<CreateImageTaskRequest['output_format']> = 'png'
const DEFAULT_OUTPUT_COMPRESSION = 100

/**
 * OpenAI output_compression 范围是 0-100；API 层兜底可阻断异常调用传入越界值。
 */
function normalizeOutputCompression(value: number | undefined): number {
  if (!Number.isFinite(value)) {
    return DEFAULT_OUTPUT_COMPRESSION
  }
  return Math.min(100, Math.max(0, Math.trunc(value as number)))
}

/**
 * 归一化用户侧 OpenAI Image API 字段。
 *
 * 模型在 API 层强制固定，避免旧浏览器状态或异常调用把其他模型透传到 custom 队列。
 */
function normalizeImageTaskRequest(input: CreateImageTaskRequest): CreateImageTaskRequest {
  return {
    ...input,
    model: FIXED_IMAGE_MODEL,
    output_format: input.output_format || DEFAULT_OUTPUT_FORMAT,
    output_compression: normalizeOutputCompression(input.output_compression),
  }
}

/**
 * 把页面层的当前编辑图对象压成 OpenAI `image` 字段值。
 *
 * 后端会把这个引用映射回任务快照；前端不再把 source_image_* 私有字段作为请求协议。
 */
function serializeEditImageReference(reference: ImageEditSourceReference): string {
  return `task:${reference.task_id}:${reference.image_index}`
}

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
  const { data } = await apiClient.post<ImageTaskResponse>(
    `${USER_IMAGE_API_PREFIX}/tasks`,
    normalizeImageTaskRequest(input),
  )
  return data
}

/**
 * 创建图片编辑任务。
 *
 * 编辑来源只通过 OpenAI `image` 字段表达；上传文件和当前编辑图引用都在这里收敛为 multipart 字段。
 */
export async function createImageEditTask(input: CreateImageEditTaskRequest): Promise<ImageTaskResponse> {
  const normalized = normalizeImageTaskRequest(input)
  const body = new FormData()
  body.set('session_id', String(normalized.session_id))
  body.set('model', normalized.model)
  body.set('prompt', normalized.prompt)
  body.set('n', String(normalized.n))
  if (normalized.size) body.set('size', normalized.size)
  if (normalized.quality) body.set('quality', normalized.quality)
  if (normalized.output_format) body.set('output_format', normalized.output_format)
  if (normalized.output_compression !== undefined) {
    body.set('output_compression', String(normalized.output_compression))
  }
  if (normalized.publish_to_gallery) body.set('publish_to_gallery', 'true')
  // `image` 只表达 OpenAI images.edit 的来源图语义；具体文件或引用由后端映射到任务快照。
  body.set('image', input.image instanceof File ? input.image : serializeEditImageReference(input.image))

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
