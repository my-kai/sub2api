import { apiClient } from '@/api/client'
import { buildPageParams, normalizePage } from './helpers'
import type {
  ImageGalleryMutationResponse,
  ImagePageParams,
  ImagePageResult,
  MyImageItem,
  PublicGalleryItem,
} from '../types'

const USER_IMAGE_API_PREFIX = '/custom/images'
const PUBLIC_GALLERY_API_PREFIX = '/custom/gallery'

/**
 * 分页读取当前用户已完成图片。
 */
export async function fetchMyImages(
  params: ImagePageParams = {},
  options?: { signal?: AbortSignal },
): Promise<ImagePageResult<MyImageItem>> {
  const { data } = await apiClient.get<ImagePageResult<MyImageItem>>(`${USER_IMAGE_API_PREFIX}/my-images`, {
    params: buildPageParams(params),
    signal: options?.signal,
  })
  return normalizePage(data)
}

/**
 * 发布单张图片到公共图库。
 */
export async function publishMyImage(taskId: number, imageIndex: number): Promise<ImageGalleryMutationResponse> {
  const { data } = await apiClient.post<ImageGalleryMutationResponse>(
    `${USER_IMAGE_API_PREFIX}/my-images/${taskId}/${imageIndex}/publish`,
  )
  return data
}

/**
 * 从公共图库隐藏单张图片。
 */
export async function hideMyImage(taskId: number, imageIndex: number): Promise<ImageGalleryMutationResponse> {
  const { data } = await apiClient.post<ImageGalleryMutationResponse>(
    `${USER_IMAGE_API_PREFIX}/my-images/${taskId}/${imageIndex}/hide`,
  )
  return data
}

/**
 * 分页读取公共图库图片。
 */
export async function fetchPublicGalleryImages(
  params: ImagePageParams = {},
  options?: { signal?: AbortSignal },
): Promise<ImagePageResult<PublicGalleryItem>> {
  const { data } = await apiClient.get<ImagePageResult<PublicGalleryItem>>(`${PUBLIC_GALLERY_API_PREFIX}/images`, {
    params: buildPageParams(params),
    signal: options?.signal,
  })
  return normalizePage(data)
}
