import { apiClient } from '@/api/client'
import type { AxiosRequestConfig } from 'axios'
import type {
  AdminCustomActivityActionResponse,
  AdminCustomActivityClaimsParams,
  AdminCustomActivityClaimsResponse,
  AdminCustomActivityDetail,
  AdminCustomActivityListItem,
  AdminCustomActivityListResponse,
  AdminCustomActivityUpsertRequest,
  CustomActivityDetail,
  CustomActivityListItem,
  CustomActivityListResponse,
  RedPacketRainState,
  RedPacketRainWSTicketRequest,
  RedPacketRainWSTicketResponse,
} from '../types'

const USER_ACTIVITY_API_PREFIX = '/custom/activities'
const ADMIN_ACTIVITY_API_PREFIX = '/admin/custom/activities'

/**
 * 兜底归一化列表字段。
 *
 * 后端契约要求返回数组；这里仍做防御，避免异常响应让活动大厅直接崩溃。
 */
function normalizeItems(items: CustomActivityListItem[] | null | undefined): CustomActivityListItem[] {
  return Array.isArray(items) ? items : []
}

/**
 * 归一化管理员列表字段。
 *
 * 管理端列表和领取记录都依赖数组渲染，异常响应时兜底为空列表，避免表格直接报错。
 */
function normalizeAdminItems<T>(items: T[] | null | undefined): T[] {
  return Array.isArray(items) ? items : []
}

/**
 * 读取当前登录用户可见活动列表。
 *
 * @param config - Axios 请求配置，可传入 AbortSignal 取消过期请求。
 * @returns 活动列表响应，`items` 已归一化为数组。
 */
export async function fetchCustomActivities(config?: AxiosRequestConfig): Promise<CustomActivityListResponse> {
  const { data } = await apiClient.get<CustomActivityListResponse>(USER_ACTIVITY_API_PREFIX, config)
  return {
    ...data,
    items: normalizeItems(data.items),
  }
}

/**
 * 读取活动详情。
 *
 * @param activityID - 活动 ID。
 * @param config - Axios 请求配置，可传入 AbortSignal 取消过期请求。
 * @returns 活动详情。
 */
export async function fetchCustomActivityDetail(
  activityID: number,
  config?: AxiosRequestConfig,
): Promise<CustomActivityDetail> {
  const { data } = await apiClient.get<CustomActivityDetail>(`${USER_ACTIVITY_API_PREFIX}/${activityID}`, config)
  return data
}

/**
 * 读取红包雨当前轮次状态。
 *
 * @param activityID - 活动 ID。
 * @param config - Axios 请求配置，可传入 AbortSignal 取消过期请求。
 * @returns 红包雨状态。
 */
export async function fetchRedPacketRainState(
  activityID: number,
  config?: AxiosRequestConfig,
): Promise<RedPacketRainState> {
  const { data } = await apiClient.get<RedPacketRainState>(
    `${USER_ACTIVITY_API_PREFIX}/${activityID}/red-packet-rain/state`,
    config,
  )
  return data
}

/**
 * 签发红包雨 WebSocket 入场票据。
 *
 * @param activityID - 活动 ID。
 * @param payload - 当前轮次、设备摘要和客户端随机值。
 * @returns 短期 WebSocket ticket。
 */
export async function issueRedPacketRainWSTicket(
  activityID: number,
  payload: RedPacketRainWSTicketRequest,
): Promise<RedPacketRainWSTicketResponse> {
  const { data } = await apiClient.post<RedPacketRainWSTicketResponse>(
    `${USER_ACTIVITY_API_PREFIX}/${activityID}/red-packet-rain/ws-ticket`,
    payload,
  )
  return data
}

/**
 * 建立红包雨领取 WebSocket。
 *
 * WebSocket 使用已登录接口签发的一次性 ticket 鉴权，不在子协议里携带访问令牌。
 */
export function createRedPacketRainWS(activityID: number, ticket: string): WebSocket {
  const baseURL = import.meta.env.VITE_API_BASE_URL || '/api/v1'
  const wsURL = buildWSURL(`${baseURL}${USER_ACTIVITY_API_PREFIX}/${activityID}/red-packet-rain/ws`)
  wsURL.searchParams.set('ticket', ticket)
  return new WebSocket(wsURL.toString())
}

function buildWSURL(pathOrURL: string): URL {
  const base = new URL(pathOrURL, window.location.origin)
  base.protocol = base.protocol === 'https:' ? 'wss:' : 'ws:'
  return base
}

/**
 * 读取管理员活动列表。
 *
 * @param config - Axios 请求配置，可传入 AbortSignal 取消过期请求。
 * @returns 管理员活动列表，`items` 已归一化为数组。
 */
export async function fetchAdminCustomActivities(
  config?: AxiosRequestConfig,
): Promise<AdminCustomActivityListResponse> {
  const { data } = await apiClient.get<AdminCustomActivityListResponse>(ADMIN_ACTIVITY_API_PREFIX, config)
  return {
    ...data,
    items: normalizeAdminItems<AdminCustomActivityListItem>(data.items),
    total: Number.isFinite(data.total) ? data.total : 0,
  }
}

/**
 * 创建管理员红包雨活动。
 *
 * @param payload - 活动配置。
 * @returns 创建后的活动详情。
 */
export async function createAdminCustomActivity(
  payload: AdminCustomActivityUpsertRequest,
): Promise<AdminCustomActivityDetail> {
  const { data } = await apiClient.post<AdminCustomActivityDetail>(ADMIN_ACTIVITY_API_PREFIX, payload)
  return data
}

/**
 * 读取管理员活动详情。
 *
 * @param activityID - 活动 ID。
 * @param config - Axios 请求配置，可传入 AbortSignal 取消过期请求。
 * @returns 管理员活动详情。
 */
export async function fetchAdminCustomActivityDetail(
  activityID: number,
  config?: AxiosRequestConfig,
): Promise<AdminCustomActivityDetail> {
  const { data } = await apiClient.get<AdminCustomActivityDetail>(
    `${ADMIN_ACTIVITY_API_PREFIX}/${activityID}`,
    config,
  )
  return {
    ...data,
    rounds: normalizeAdminItems(data.rounds),
  }
}

/**
 * 更新管理员红包雨活动。
 *
 * @param activityID - 活动 ID。
 * @param payload - 活动配置。
 * @returns 更新后的活动详情。
 */
export async function updateAdminCustomActivity(
  activityID: number,
  payload: AdminCustomActivityUpsertRequest,
): Promise<AdminCustomActivityDetail> {
  const { data } = await apiClient.put<AdminCustomActivityDetail>(
    `${ADMIN_ACTIVITY_API_PREFIX}/${activityID}`,
    payload,
  )
  return data
}

/**
 * 提前结束活动。
 *
 * @param activityID - 活动 ID。
 * @returns 操作结果。
 */
export async function endAdminCustomActivity(activityID: number): Promise<AdminCustomActivityActionResponse> {
  const { data } = await apiClient.post<AdminCustomActivityActionResponse>(
    `${ADMIN_ACTIVITY_API_PREFIX}/${activityID}/end`,
  )
  return data
}

/**
 * 下架活动。
 *
 * @param activityID - 活动 ID。
 * @returns 操作结果。
 */
export async function offlineAdminCustomActivity(activityID: number): Promise<AdminCustomActivityActionResponse> {
  const { data } = await apiClient.post<AdminCustomActivityActionResponse>(
    `${ADMIN_ACTIVITY_API_PREFIX}/${activityID}/offline`,
  )
  return data
}

/**
 * 读取活动领取记录。
 *
 * @param activityID - 活动 ID。
 * @param params - 分页参数。
 * @param config - Axios 请求配置，可传入 AbortSignal 取消过期请求。
 * @returns 领取记录，`items` 已归一化为数组。
 */
export async function fetchAdminCustomActivityClaims(
  activityID: number,
  params: AdminCustomActivityClaimsParams = {},
  config?: AxiosRequestConfig,
): Promise<AdminCustomActivityClaimsResponse> {
  const { data } = await apiClient.get<AdminCustomActivityClaimsResponse>(
    `${ADMIN_ACTIVITY_API_PREFIX}/${activityID}/claims`,
    {
      ...config,
      params: {
        ...config?.params,
        ...params,
      },
    },
  )
  return {
    ...data,
    items: normalizeAdminItems(data.items),
    total: Number.isFinite(data.total) ? data.total : 0,
  }
}
