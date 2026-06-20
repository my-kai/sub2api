import { apiClient } from '@/api/client'

export interface CallbackAuthorizationInfo {
  callback: string
  domain: string
  authorized: boolean
}

export interface CallbackAuthorizeResponse {
  redirect_url: string
  code: string
  expires_at: string
}

/**
 * Loads callback authorization state for the current logged-in user.
 */
export async function getCallbackAuthorization(callback: string): Promise<CallbackAuthorizationInfo> {
  const { data } = await apiClient.get<CallbackAuthorizationInfo>('/custom/callback-auth/authorize', {
    params: { callback },
  })
  return data
}

/**
 * Confirms durable consent for the callback domain and issues a one-time code.
 */
export async function authorizeCallback(callback: string): Promise<CallbackAuthorizeResponse> {
  const { data } = await apiClient.post<CallbackAuthorizeResponse>('/custom/callback-auth/authorize', {
    callback,
  })
  return data
}
