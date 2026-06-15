import { apiClient } from '@/api/client'
import { normalizeItems } from './helpers'
import type {
  CreateImageAPIKeyRequest,
  CreateImageAPIKeyResponse,
  ImageAPIKey,
  ImageAPIKeyListResponse,
} from '../types'

const USER_IMAGE_API_PREFIX = '/custom/images'

/**
 * 读取当前用户的生图 Key 列表。
 *
 * 列表响应只用于展示脱敏信息；即使异常响应带回完整 Key，页面层也会再次过滤。
 */
export async function fetchImageAPIKeys(options?: { signal?: AbortSignal }): Promise<ImageAPIKey[]> {
  const { data } = await apiClient.get<ImageAPIKeyListResponse>(`${USER_IMAGE_API_PREFIX}/keys`, {
    signal: options?.signal,
  })
  return normalizeItems(data.items)
}

/**
 * 创建新的生图 Key。
 *
 * 完整 Key 只在本次响应中返回，调用方必须只把它放进一次性展示区域。
 */
export async function createImageAPIKey(input: CreateImageAPIKeyRequest): Promise<ImageAPIKey> {
  const { data } = await apiClient.post<CreateImageAPIKeyResponse>(`${USER_IMAGE_API_PREFIX}/keys`, input)
  return data.api_key
}

/**
 * 删除指定生图 Key。
 */
export async function deleteImageAPIKey(id: ImageAPIKey['id']): Promise<{ deleted: true }> {
  const { data } = await apiClient.delete<{ deleted: true }>(`${USER_IMAGE_API_PREFIX}/keys/${id}`)
  return data
}

