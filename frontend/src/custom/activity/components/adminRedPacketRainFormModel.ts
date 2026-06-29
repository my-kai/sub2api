import type {
  AdminCustomActivityDetail,
  AdminCustomActivityUpsertRequest,
  AdminRedPacketRainConfig,
  CustomActivityStatus,
} from '../types'

/**
 * 管理员红包雨表单状态。
 *
 * 表单字段统一用字符串承载，避免金额和 datetime-local 值在输入阶段被浏览器隐式转换。
 */
export interface AdminRedPacketRainFormState {
  title: string
  description: string
  cover_url: string
  starts_at: string
  ends_at: string
  round_count: string
  round_duration_seconds: string
  round_interval_seconds: string
  total_budget: string
  per_user_round_cap: string
  per_user_total_cap: string
  base_unit_amount: string
  max_single_reward: string
  probability_step: string
  gift_validity_days: string
}

/**
 * 创建默认表单值。
 *
 * 默认时间给出可编辑的未来窗口；赠送余额有效期保持空值，要求管理员显式填写。
 */
export function createDefaultRedPacketRainForm(): AdminRedPacketRainFormState {
  const startsAt = new Date(Date.now() + 60 * 60 * 1000)
  const endsAt = new Date(Date.now() + 3 * 60 * 60 * 1000)
  return {
    title: '',
    description: '',
    cover_url: '',
    starts_at: toDateTimeLocal(startsAt.toISOString()),
    ends_at: toDateTimeLocal(endsAt.toISOString()),
    round_count: '12',
    round_duration_seconds: '60',
    round_interval_seconds: '900',
    total_budget: '1000.00000000',
    per_user_round_cap: '5.00000000',
    per_user_total_cap: '20.00000000',
    base_unit_amount: '0.10000000',
    max_single_reward: '3.00000000',
    probability_step: '0.08000000',
    gift_validity_days: '',
  }
}

/**
 * 将详情响应转换成表单字符串。
 *
 * @param activity - 管理员活动详情。
 * @returns 可直接绑定到表单的字符串状态。
 */
export function redPacketRainFormFromActivity(activity: AdminCustomActivityDetail): AdminRedPacketRainFormState {
  const config = activity.red_packet_rain
  return {
    title: activity.title || '',
    description: activity.description || '',
    cover_url: activity.cover_url || '',
    starts_at: toDateTimeLocal(activity.starts_at),
    ends_at: toDateTimeLocal(activity.ends_at),
    round_count: String(config?.round_count || ''),
    round_duration_seconds: String(config?.round_duration_seconds || ''),
    round_interval_seconds: String(config?.round_interval_seconds ?? ''),
    total_budget: config?.total_budget || activity.total_budget || '',
    per_user_round_cap: config?.per_user_round_cap || '',
    per_user_total_cap: config?.per_user_total_cap || '',
    base_unit_amount: config?.base_unit_amount || '',
    max_single_reward: config?.max_single_reward || '',
    probability_step: config?.probability_step || '',
    gift_validity_days: config?.gift_validity_days ? String(config.gift_validity_days) : '',
  }
}

/**
 * 判断活动是否仍允许完整编辑。
 *
 * @param status - 活动状态。
 * @returns 草稿或未开始活动返回 true。
 */
export function isEditableActivityStatus(status: CustomActivityStatus): boolean {
  return status === 'draft' || status === 'scheduled'
}

/**
 * 校验表单业务规则。
 *
 * @param form - 管理员红包雨表单。
 * @returns 校验错误文案列表。
 */
export function validateRedPacketRainForm(form: AdminRedPacketRainFormState): string[] {
  const nextErrors: string[] = []
  const startsAt = parseLocalDate(form.starts_at)
  const endsAt = parseLocalDate(form.ends_at)

  if (!form.title.trim()) nextErrors.push('请填写活动标题')
  if (!startsAt) nextErrors.push('请选择开始时间')
  if (!endsAt) nextErrors.push('请选择结束时间')
  if (startsAt && endsAt && endsAt.getTime() <= startsAt.getTime()) {
    nextErrors.push('结束时间需晚于开始时间')
  }

  validatePositiveInteger(form.round_count, '轮数', nextErrors)
  validatePositiveInteger(form.round_duration_seconds, '单轮时长秒', nextErrors)
  validateNonNegativeInteger(form.round_interval_seconds, '轮次间隔秒', nextErrors)
  validatePositiveMoney(form.total_budget, '活动总预算', nextErrors)
  validatePositiveMoney(form.per_user_round_cap, '单用户单轮上限', nextErrors)
  validatePositiveMoney(form.per_user_total_cap, '单用户活动总上限', nextErrors)
  validatePositiveMoney(form.base_unit_amount, '基础奖励金额', nextErrors)
  validatePositiveMoney(form.max_single_reward, '单次最高奖励', nextErrors)
  validatePositiveMoney(form.probability_step, '概率步长', nextErrors)
  validatePositiveInteger(form.gift_validity_days, '赠送余额有效时长', nextErrors)

  if (isPositiveMoney(form.per_user_total_cap) && isPositiveMoney(form.total_budget)) {
    if (compareDecimalStrings(form.per_user_total_cap, form.total_budget) > 0) {
      nextErrors.push('单用户活动总上限不能大于活动总预算')
    }
  }
  if (isPositiveMoney(form.max_single_reward) && isPositiveMoney(form.per_user_round_cap)) {
    if (compareDecimalStrings(form.max_single_reward, form.per_user_round_cap) > 0) {
      nextErrors.push('单次最高奖励不能大于单用户单轮上限')
    }
  }
  if (isPositiveMoney(form.probability_step) && compareDecimalStrings(form.probability_step, '1') > 0) {
    nextErrors.push('概率步长不能大于 1')
  }

  return nextErrors
}

/**
 * 构建管理员活动保存请求。
 *
 * @param form - 已通过校验的管理员红包雨表单。
 * @returns 管理员活动创建或更新请求。
 */
export function buildRedPacketRainPayload(form: AdminRedPacketRainFormState): AdminCustomActivityUpsertRequest {
  const redPacketRain: AdminRedPacketRainConfig = {
    round_count: Number.parseInt(form.round_count, 10),
    round_duration_seconds: Number.parseInt(form.round_duration_seconds, 10),
    round_interval_seconds: Number.parseInt(form.round_interval_seconds, 10),
    total_budget: form.total_budget.trim(),
    per_user_round_cap: form.per_user_round_cap.trim(),
    per_user_total_cap: form.per_user_total_cap.trim(),
    base_unit_amount: form.base_unit_amount.trim(),
    max_single_reward: form.max_single_reward.trim(),
    probability_step: form.probability_step.trim(),
    gift_validity_days: Number.parseInt(form.gift_validity_days, 10),
  }
  return {
    type: 'red_packet_rain',
    title: form.title.trim(),
    description: form.description.trim(),
    cover_url: form.cover_url.trim(),
    starts_at: toISOStringFromLocal(form.starts_at),
    ends_at: toISOStringFromLocal(form.ends_at),
    red_packet_rain: redPacketRain,
  }
}

/**
 * 校验正整数。
 */
function validatePositiveInteger(value: string, label: string, target: string[]): void {
  if (!/^[1-9]\d*$/.test(value.trim())) {
    target.push(`${label}需大于 0`)
  }
}

/**
 * 校验非负整数。
 */
function validateNonNegativeInteger(value: string, label: string, target: string[]): void {
  if (!/^(0|[1-9]\d*)$/.test(value.trim())) {
    target.push(`${label}不能小于 0`)
  }
}

/**
 * 校验正 decimal 字符串。
 */
function validatePositiveMoney(value: string, label: string, target: string[]): void {
  if (!isPositiveMoney(value)) {
    target.push(`${label}需大于 0`)
  }
}

/**
 * 判断 decimal 字符串是否为正数。
 */
function isPositiveMoney(value: string): boolean {
  return isDecimalString(value) && compareDecimalStrings(value, '0') > 0
}

/**
 * 判断是否为非负 decimal 字符串。
 */
function isDecimalString(value: string): boolean {
  return /^(0|[1-9]\d*)(\.\d+)?$/.test(value.trim())
}

/**
 * 比较两个非负 decimal 字符串。
 *
 * 不转换为 Number，避免金额精度被浏览器浮点数影响。
 */
function compareDecimalStrings(left: string, right: string): number {
  const [leftInt, leftFrac = ''] = normalizeDecimal(left).split('.')
  const [rightInt, rightFrac = ''] = normalizeDecimal(right).split('.')
  if (leftInt.length !== rightInt.length) return leftInt.length > rightInt.length ? 1 : -1
  if (leftInt !== rightInt) return leftInt > rightInt ? 1 : -1

  const maxFracLength = Math.max(leftFrac.length, rightFrac.length)
  const normalizedLeftFrac = leftFrac.padEnd(maxFracLength, '0')
  const normalizedRightFrac = rightFrac.padEnd(maxFracLength, '0')
  if (normalizedLeftFrac === normalizedRightFrac) return 0
  return normalizedLeftFrac > normalizedRightFrac ? 1 : -1
}

/**
 * 去除 decimal 字符串的冗余前导零。
 */
function normalizeDecimal(value: string): string {
  const [integerPart, fractionPart] = value.trim().split('.')
  const normalizedInteger = integerPart.replace(/^0+(?=\d)/, '') || '0'
  return fractionPart === undefined ? normalizedInteger : `${normalizedInteger}.${fractionPart}`
}

/**
 * 解析 datetime-local 值。
 */
function parseLocalDate(value: string): Date | null {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? null : date
}

/**
 * 将 ISO 时间转为 datetime-local 可展示值。
 */
function toDateTimeLocal(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  const pad = (part: number) => String(part).padStart(2, '0')
  return [
    date.getFullYear(),
    pad(date.getMonth() + 1),
    pad(date.getDate()),
  ].join('-') + `T${pad(date.getHours())}:${pad(date.getMinutes())}`
}

/**
 * 将 datetime-local 值转为接口使用的 ISO 时间。
 */
function toISOStringFromLocal(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toISOString()
}
