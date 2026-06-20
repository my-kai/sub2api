/**
 * 金额只按后端字符串展示。
 *
 * @param value - 后端返回的 decimal 金额字符串。
 * @returns 可展示金额。
 */
export function displayActivityMoney(value: string | undefined): string {
  return value?.trim() || '0.00000000'
}

/**
 * 格式化可选时间。
 *
 * @param value - ISO 时间字符串。
 * @returns 本地时间短格式或占位符。
 */
export function formatOptionalActivityDateTime(value: string | undefined): string {
  return value ? formatActivityDateTime(value) : '-'
}

/**
 * 按本地时区展示活动时间。
 *
 * @param value - ISO 时间字符串。
 * @returns 本地时间短格式或占位符。
 */
export function formatActivityDateTime(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date)
}

/**
 * 识别请求取消错误。
 *
 * @param err - 捕获到的错误对象。
 * @returns 取消请求返回 true。
 */
export function isCanceledRequest(err: unknown): boolean {
  return Boolean(
    err
      && typeof err === 'object'
      && ('code' in err || 'name' in err)
      && (((err as { code?: string }).code === 'ERR_CANCELED') || ((err as { name?: string }).name === 'AbortError')),
  )
}
