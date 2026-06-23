/**
 * OAuth 授权预览和授权确认接口共享的 query/body 结构。
 *
 * 后端契约这里刻意保留 snake_case 字段，前端可以直接透传，不额外增加字段映射层。
 */
export interface OAuthAuthorizeRequest {
  /** OAuth response type；当前后端只接受 code 授权。 */
  response_type: string
  /** OAuth 请求中的第三方应用 client id。 */
  client_id: string
  /** 接收授权码的第三方回调地址。 */
  redirect_uri: string
  /** 原样回传给第三方回调地址的 OAuth state。 */
  state: string
}

/**
 * 用户确认授权前返回的授权预览信息。
 */
export interface OAuthAuthorizationInfo {
  /** 客户端注册的用户可见应用名称。 */
  applicationName: string
  /** 当前应用绑定的 access key；保留用于契约完整性。 */
  accessKey: string
  /** 后端校验后的回调地址。 */
  redirectUri: string
  /** 后端解析后可安全展示的回调域名。 */
  redirectDomain: string
  /** 后端原样回传的 OAuth state。 */
  state: string
}

/**
 * 用户确认授权后返回的授权结果。
 */
export interface OAuthAuthorizeResponse {
  /** 携带生成授权码的最终回调地址。 */
  redirect_url: string
  /** 后端生成的一次性授权码。 */
  code: string
  /** 后端返回的授权码过期时间。 */
  expires_at: string
}

export type OAuthApplicationStatus = 'enabled' | 'disabled'

/**
 * 管理员可见的应用结构；列表/详情接口刻意不返回密钥。
 */
export interface OAuthApplication {
  id: number
  name: string
  accessKey: string
  allowedDomains: string[]
  status: OAuthApplicationStatus
  createdAt: string
  updatedAt: string
}

export interface OAuthApplicationFormPayload {
  name: string
  allowedDomains: string[]
  status: OAuthApplicationStatus
}

export interface OAuthApplicationSecretResponse {
  application: OAuthApplication
  accessSecret: string
}

/**
 * 管理端 OAuth 应用状态。
 */
export type AdminOAuthApplicationStatus = 'enabled' | 'disabled'

/**
 * 返回给管理页的非敏感 OAuth 应用记录。
 */
export interface AdminOAuthApplication {
  /** 管理端变更接口使用的数字应用 ID。 */
  id: number
  /** OAuth 授权确认时展示给用户的应用名称。 */
  name: string
  /** 第三方 OAuth 请求使用的公开客户端标识。 */
  accessKey: string
  /** 回调域名白名单；通配项使用 "*.example.com" 形式。 */
  allowedDomains: string[]
  /** 应用是否允许发起 OAuth 授权。 */
  status: AdminOAuthApplicationStatus
  /** 后端返回的创建时间。 */
  createdAt: string
  /** 后端返回的最后更新时间。 */
  updatedAt: string
}

/**
 * 管理端创建和更新接口接收的 payload。
 */
export interface AdminOAuthApplicationUpsertRequest {
  /** 去除首尾空白后的应用名称。 */
  name: string
  /** 规范化后的回调域名白名单。 */
  allowedDomains: string[]
  /** 期望写入的启用状态。 */
  status: AdminOAuthApplicationStatus
}

/**
 * 创建/重置响应；这是唯一展示明文密钥的窗口。
 */
export interface AdminOAuthApplicationSecretResponse {
  /** 变更后的应用记录。 */
  application: AdminOAuthApplication
  /** 后端只返回一次的明文客户端密钥。 */
  accessSecret: string
}
