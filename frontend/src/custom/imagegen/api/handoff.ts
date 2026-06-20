import { apiClient } from '@/api/client'

const HANDOFF_API_PREFIX = '/custom/image-gen'

export interface ImageGenLoginCodeResponse {
  redirect_url: string
  expires_at: string
}

/**
 * 向 sub2api-ex 后端申请一次性登录 code。
 *
 * 后端只返回可跳转地址和过期时间；浏览器不会接触 service secret，
 * 也不会读取旧生图渠道配置。
 */
export async function createImageGenLoginCode(): Promise<ImageGenLoginCodeResponse> {
  const { data } = await apiClient.post<ImageGenLoginCodeResponse>(`${HANDOFF_API_PREFIX}/login-code`, {})
  return data
}
