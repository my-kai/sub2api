import { apiClient } from '@/api/client'
import type { ImagePageParams, ImagePageResult } from '../types'

/**
 * 将分页参数转换成后端契约字段。
 *
 * 后端按 `page` / `page_size` 读取，前端统一在调用层归一化，避免页面各自拼参数。
 */
export function buildPageParams(params: ImagePageParams = {}): Required<ImagePageParams> {
  return {
    page: params.page && params.page > 0 ? params.page : 1,
    page_size: params.page_size && params.page_size > 0 ? params.page_size : 20,
  }
}

/**
 * 兜底归一化数组字段。
 *
 * 契约要求空列表返回 `[]`，这里仍做防御，避免历史接口或异常数据让页面崩溃。
 */
export function normalizeItems<T>(items: T[] | null | undefined): T[] {
  return Array.isArray(items) ? items : []
}

/**
 * 兜底归一化分页响应。
 */
export function normalizePage<T>(page: ImagePageResult<T>): ImagePageResult<T> {
  return {
    ...page,
    items: normalizeItems(page.items),
  }
}

/**
 * 生成 EventSource URL。
 *
 * EventSource 不能设置 Axios Authorization header，因此只复用 apiClient 的 baseURL；
 * 如果本地存在访问令牌，会作为查询参数传给后端做同源鉴权兜底。
 */
export function buildEventSourceURL(path: string, params: Record<string, string | number>): string {
  const baseURL = String(apiClient.defaults.baseURL || '').replace(/\/$/, '')
  const query = new URLSearchParams()

  for (const [key, value] of Object.entries(params)) {
    query.set(key, String(value))
  }

  const token = readAccessToken()
  if (token) {
    query.set('token', token)
  }

  return `${baseURL}${path}?${query.toString()}`
}

/**
 * 读取目标前端 apiClient 当前使用的访问令牌。
 */
function readAccessToken(): string {
  if (typeof window === 'undefined') {
    return ''
  }
  return window.localStorage.getItem('auth_token') || ''
}
