import { apiClient } from '@/api/client'
import type {
  OAuthApplication,
  OAuthApplicationFormPayload,
  OAuthApplicationSecretResponse,
} from './types'

const ADMIN_OAUTH_APPLICATIONS_PREFIX = '/admin/custom/oauth-applications'

/**
 * 读取管理页展示的所有第三方 OAuth 应用。
 *
 * @returns 不包含客户端密钥的应用列表
 */
export async function listOAuthApplications(): Promise<OAuthApplication[]> {
  const { data } = await apiClient.get<OAuthApplication[]>(ADMIN_OAUTH_APPLICATIONS_PREFIX)
  return data
}

/**
 * 创建第三方 OAuth 应用。
 *
 * @param payload - 应用名称、域名白名单和初始状态
 * @returns 创建后的应用和一次性明文密钥
 */
export async function createOAuthApplication(payload: OAuthApplicationFormPayload): Promise<OAuthApplicationSecretResponse> {
  const { data } = await apiClient.post<OAuthApplicationSecretResponse>(ADMIN_OAUTH_APPLICATIONS_PREFIX, payload)
  return data
}

/**
 * 更新应用可变配置。
 *
 * @param id - 应用 ID
 * @param payload - 可变应用字段
 * @returns 不包含密钥的更新后应用
 */
export async function updateOAuthApplication(id: number, payload: OAuthApplicationFormPayload): Promise<OAuthApplication> {
  const { data } = await apiClient.put<OAuthApplication>(`${ADMIN_OAUTH_APPLICATIONS_PREFIX}/${id}`, payload)
  return data
}

/**
 * 轮换应用密钥。
 *
 * @param id - 应用 ID
 * @returns 更新后的应用和一次性明文密钥
 */
export async function resetOAuthApplicationSecret(id: number): Promise<OAuthApplicationSecretResponse> {
  const { data } = await apiClient.post<OAuthApplicationSecretResponse>(`${ADMIN_OAUTH_APPLICATIONS_PREFIX}/${id}/reset-secret`)
  return data
}

/**
 * 删除应用。
 *
 * @param id - 应用 ID
 */
export async function deleteOAuthApplication(id: number): Promise<void> {
  await apiClient.delete(`${ADMIN_OAUTH_APPLICATIONS_PREFIX}/${id}`)
}
