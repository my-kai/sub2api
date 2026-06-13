import { apiClient } from '@/api/client'
import { normalizeItems } from './helpers'
import type {
  ImageAdminUserOption,
  ImageQueueConfigInput,
  ImageAdminUserSearchResponse,
  ImageQueueConfig,
  ImageUserLimit,
  ImageUserLimitListResponse,
} from '../types'

const ADMIN_IMAGE_API_PREFIX = '/admin/custom/images'

/**
 * 读取管理员生图配置。
 */
export async function fetchImageQueueConfig(options?: { signal?: AbortSignal }): Promise<ImageQueueConfig> {
  const { data } = await apiClient.get<ImageQueueConfig>(`${ADMIN_IMAGE_API_PREFIX}/config`, {
    signal: options?.signal,
  })
  return data
}

/**
 * 保存管理员生图配置。
 */
export async function saveImageQueueConfig(config: ImageQueueConfigInput): Promise<ImageQueueConfig> {
  const { data } = await apiClient.put<ImageQueueConfig>(`${ADMIN_IMAGE_API_PREFIX}/config`, config)
  return data
}

/**
 * 搜索可设置生图限制的用户。
 */
export async function searchImageAdminUsers(
  query: string,
  options?: { signal?: AbortSignal },
): Promise<ImageAdminUserOption[]> {
  const { data } = await apiClient.get<ImageAdminUserSearchResponse>(`${ADMIN_IMAGE_API_PREFIX}/users/search`, {
    params: {
      q: query.trim(),
      limit: 20,
    },
    signal: options?.signal,
  })
  return normalizeItems(data.items).filter((item) => Number(item.id) > 0)
}

/**
 * 读取用户生图并发限制。
 */
export async function fetchImageUserLimits(options?: { signal?: AbortSignal }): Promise<ImageUserLimit[]> {
  const { data } = await apiClient.get<ImageUserLimitListResponse>(`${ADMIN_IMAGE_API_PREFIX}/user-limits`, {
    signal: options?.signal,
  })
  return normalizeItems(data.items)
}

/**
 * 保存单个用户生图并发限制。
 */
export async function saveImageUserLimit(
  userId: number,
  input: Pick<ImageUserLimit, 'concurrency'>,
): Promise<ImageUserLimit> {
  const { data } = await apiClient.put<ImageUserLimit>(`${ADMIN_IMAGE_API_PREFIX}/user-limits/${userId}`, input)
  return data
}

/**
 * 删除单个用户生图并发限制。
 */
export async function deleteImageUserLimit(userId: number): Promise<{ deleted: boolean }> {
  const { data } = await apiClient.delete<{ deleted: boolean }>(`${ADMIN_IMAGE_API_PREFIX}/user-limits/${userId}`)
  return data
}

/**
 * 管理员生图 API 集合。
 */
export const imagegenAdminAPI = {
  fetchImageQueueConfig,
  saveImageQueueConfig,
  searchImageAdminUsers,
  fetchImageUserLimits,
  saveImageUserLimit,
  deleteImageUserLimit,
}
