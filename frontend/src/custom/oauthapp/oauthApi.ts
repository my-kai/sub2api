import { apiClient } from '@/api/client'
import type {
  OAuthAuthorizationInfo,
  OAuthAuthorizeRequest,
  OAuthAuthorizeResponse,
} from './types'

/**
 * 读取当前用户的第三方 OAuth 授权预览信息。
 *
 * @param params - 当前 authorize URL 中的 OAuth query 值
 * @returns 授权确认页展示的应用和回调信息
 * @throws 当客户端或回调地址无效时，抛出后端校验错误
 */
export async function getOAuthAuthorization(params: OAuthAuthorizeRequest): Promise<OAuthAuthorizationInfo> {
  const { data } = await apiClient.get<OAuthAuthorizationInfo>('/custom/oauth/authorize', {
    params,
  })
  return data
}

/**
 * 确认用户授权，并请求后端生成回调地址。
 *
 * @param body - 从原始请求透传的 OAuth 授权参数
 * @returns 携带授权码的回调目标
 * @throws 当授权确认无法完成时，抛出后端校验错误
 */
export async function authorizeOAuthApplication(body: OAuthAuthorizeRequest): Promise<OAuthAuthorizeResponse> {
  const { data } = await apiClient.post<OAuthAuthorizeResponse>('/custom/oauth/authorize', body)
  return data
}
