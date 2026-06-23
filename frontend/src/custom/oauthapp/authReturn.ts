import type { LocationQuery, LocationQueryRaw } from 'vue-router'

const DEFAULT_AUTH_RETURN_PATH = '/dashboard'

/**
 * 从 vue-router query 字段中读取单个字符串值。
 *
 * @param value - 路由 query 中可能为字符串、数组或空值的字段
 * @returns 去除首尾空白后的第一个字符串；不存在时返回空字符串
 */
function readSingleQueryString(value: unknown): string {
  if (typeof value === 'string') return value.trim()
  if (Array.isArray(value) && typeof value[0] === 'string') return value[0].trim()
  return ''
}

/**
 * 归一化登录/注册完成后的站内回跳地址。
 *
 * @param value - 来源于 URL query 的 redirect 值
 * @param fallback - redirect 不可用时使用的默认站内地址
 * @returns 可信的站内路径，拒绝外链、协议相对 URL 和换行注入
 */
export function resolveAuthReturnPath(value: unknown, fallback = DEFAULT_AUTH_RETURN_PATH): string {
  const redirect = readSingleQueryString(value)
  if (!redirect) return fallback
  if (!redirect.startsWith('/') || redirect.startsWith('//')) return fallback
  if (redirect.includes('\n') || redirect.includes('\r')) return fallback
  return redirect
}

/**
 * 从当前路由 query 中解析认证完成后的回跳地址。
 *
 * @param query - 当前登录/注册页的 query
 * @param fallback - 缺省回跳地址
 * @returns 可直接传给 vue-router 的站内地址
 */
export function resolveAuthReturnPathFromQuery(
  query: LocationQuery,
  fallback = DEFAULT_AUTH_RETURN_PATH,
): string {
  return resolveAuthReturnPath(query.redirect, fallback)
}

/**
 * 构建登录和注册页互相切换时需要继承的 query。
 *
 * @param query - 当前认证页 query
 * @returns 仅包含安全 redirect 的 query，避免把无关参数带入注册流程
 */
export function buildAuthSwitchQuery(query: LocationQuery): LocationQueryRaw {
  const redirect = resolveAuthReturnPath(query.redirect, '')
  return redirect ? { redirect } : {}
}
