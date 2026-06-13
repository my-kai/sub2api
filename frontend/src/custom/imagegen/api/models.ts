import { apiClient } from '@/api/client'
import { normalizeItems } from './helpers'
import type {
  ImageModel,
  ImageModelsResponse,
  ImageGenerationStatus,
  ImagePriceQuote,
  ImagePriceQuoteParams,
} from '../types'

const USER_IMAGE_API_PREFIX = '/custom/images'

/**
 * 读取当前可用生图模型。
 *
 * 模型权限由后端统一判断，前端只过滤掉不可展示的空 id。
 */
export async function fetchImageModels(options?: { signal?: AbortSignal }): Promise<ImageModel[]> {
  const { data } = await apiClient.get<ImageModelsResponse>(`${USER_IMAGE_API_PREFIX}/models`, {
    signal: options?.signal,
  })
  return normalizeItems(data.data).filter((model) => model.id.trim() !== '')
}

/**
 * 读取用户侧生图开关状态。
 *
 * 该接口只返回 enabled，避免普通用户拿到管理员上游配置。
 */
export async function fetchImageGenerationStatus(options?: { signal?: AbortSignal }): Promise<ImageGenerationStatus> {
  const { data } = await apiClient.get<ImageGenerationStatus>(`${USER_IMAGE_API_PREFIX}/status`, {
    signal: options?.signal,
  })
  return data
}

/**
 * 读取当前参数的价格预览。
 *
 * 价格由后端配置计算，浏览器不保存价格规则，避免配置更新后展示漂移。
 */
export async function fetchImagePriceQuote(
  input: ImagePriceQuoteParams,
  options?: { signal?: AbortSignal },
): Promise<ImagePriceQuote> {
  const { data } = await apiClient.get<ImagePriceQuote>(`${USER_IMAGE_API_PREFIX}/price-quote`, {
    params: {
      model: input.model,
      resolution: input.resolution,
      size: input.size || 'auto',
      n: input.n || 1,
    },
    signal: options?.signal,
  })
  return data
}
