/**
 * custom 活动中心用户侧类型定义。
 *
 * 金额字段全部保持后端返回的字符串，前端只展示、不计算，避免把资金规则下沉到浏览器。
 */

/**
 * 当前活动中心支持的活动类型。
 */
export type CustomActivityType = 'red_packet_rain'

/**
 * 用户侧可见活动状态。
 */
export type CustomActivityStatus = 'draft' | 'scheduled' | 'active' | 'ended' | 'offline'

/**
 * 红包雨轮次状态。
 */
export type RedPacketRainRoundStatus = 'waiting' | 'active' | 'ended' | 'finished' | 'offline'

/**
 * decimal 金额字符串。
 */
export type MoneyString = string

/**
 * 活动大厅展示摘要。
 */
export interface CustomActivitySummary {
  total_budget?: MoneyString
  user_total_reward?: MoneyString
}

/**
 * 活动大厅列表项。
 */
export interface CustomActivityListItem {
  id: number
  type: CustomActivityType
  title: string
  description?: string
  cover_url?: string
  status: CustomActivityStatus
  starts_at: string
  ends_at: string
  summary?: CustomActivitySummary
}

/**
 * 活动大厅列表响应。
 */
export interface CustomActivityListResponse {
  items: CustomActivityListItem[]
}

/**
 * 红包雨配置摘要。
 */
export interface RedPacketRainConfig {
  round_count: number
  round_duration_seconds: number
  round_interval_seconds: number
  per_user_round_cap: MoneyString
  per_user_total_cap: MoneyString
}

/**
 * 活动详情。
 */
export interface CustomActivityDetail extends CustomActivityListItem {
  red_packet_rain?: RedPacketRainConfig
}

/**
 * 当前或下一轮红包雨信息。
 */
export interface RedPacketRainRound {
  id: number
  round_no: number
  status: RedPacketRainRoundStatus
  starts_at: string
  ends_at: string
  server_now: string
  seconds_until_start: number
  seconds_until_end: number
}

/**
 * 当前用户在红包雨中的奖励进度。
 */
export interface RedPacketRainUserReward {
  round_total: MoneyString
  activity_total: MoneyString
  round_remaining: MoneyString
  activity_remaining: MoneyString
  round_cap_reached: boolean
  activity_cap_reached: boolean
}

/**
 * 红包雨活动预算状态。
 */
export interface RedPacketRainBudgetState {
  remaining: MoneyString
  exhausted: boolean
}

/**
 * 红包雨实时状态。
 */
export interface RedPacketRainState {
  activity_id: number
  status: RedPacketRainRoundStatus
  round: RedPacketRainRound | null
  user_reward: RedPacketRainUserReward
  budget: RedPacketRainBudgetState
}

/**
 * 红包雨领取响应。
 */
export interface RedPacketRainClaimResponse {
  claim_id: number
  activity_id: number
  round_id: number
  hit_count: number
  reward_amount: MoneyString
  credited: boolean
  duplicate: boolean
  message: string
  user_reward: RedPacketRainUserReward
  budget: RedPacketRainBudgetState
}

/**
 * 红包雨 WebSocket 入场票据请求。
 */
export interface RedPacketRainWSTicketRequest {
  round_id: number
  device_fingerprint: string
  client_nonce: string
}

/**
 * 红包雨 WebSocket 入场票据响应。
 */
export interface RedPacketRainWSTicketResponse {
  ticket: string
  expires_at: string
  ws_url: string
}

/**
 * 服务端下发的 WebSocket 挑战。
 */
export interface RedPacketRainWSChallengeMessage {
  type: 'challenge'
  session_id: string
  server_nonce: string
  challenge: string
  expires_at: string
  round_id: number
  round_ends_at: string
}

/**
 * WebSocket 加密领取消息。
 */
export interface RedPacketRainWSClaimMessage {
  type: 'claim'
  session_id: string
  round_id: number
  idempotency_key: string
  nonce: string
  ciphertext: string
  signature: string
}

/**
 * WebSocket 领取结果消息。
 */
export interface RedPacketRainWSClaimResultMessage {
  type: 'claim_result'
  data: RedPacketRainClaimResponse
}

/**
 * WebSocket 业务错误消息。
 */
export interface RedPacketRainWSErrorMessage {
  type: 'error'
  message: string
}

/**
 * 红包雨 WebSocket 消息联合类型。
 */
export type RedPacketRainWSMessage =
  | RedPacketRainWSChallengeMessage
  | RedPacketRainWSClaimResultMessage
  | RedPacketRainWSErrorMessage
  | { type: 'state' }

/**
 * 红包雨加密领取载荷。
 */
export interface RedPacketRainEncryptedClaimPayload {
  hit_count: number
  started_at: string
  ended_at: string
  click_trace_digest: string
  device_fingerprint: string
  client_nonce: string
}

/**
 * 管理员红包雨完整配置。
 *
 * 管理端需要展示和编辑资金规则；金额仍保持 decimal 字符串，避免浏览器浮点数改写金额精度。
 */
export interface AdminRedPacketRainConfig extends RedPacketRainConfig {
  total_budget: MoneyString
  base_unit_amount: MoneyString
  max_single_reward: MoneyString
  probability_step: MoneyString
}

/**
 * 管理员活动列表项。
 */
export interface AdminCustomActivityListItem {
  id: number
  type: CustomActivityType
  title: string
  status: CustomActivityStatus
  starts_at: string
  ends_at: string
  total_budget: MoneyString
  issued_amount: MoneyString
  participant_count: number
}

/**
 * 管理员活动列表响应。
 */
export interface AdminCustomActivityListResponse {
  items: AdminCustomActivityListItem[]
  total: number
}

/**
 * 红包雨轮次摘要。
 *
 * 后端详情接口可返回该摘要用于管理端查看排期与领取进度；字段都按可选处理，避免旧响应缺字段时页面崩溃。
 */
export interface AdminRedPacketRainRoundSummary {
  id?: number
  round_no: number
  status: RedPacketRainRoundStatus
  starts_at?: string
  ends_at?: string
  issued_amount?: MoneyString
  participant_count?: number
  claim_count?: number
}

/**
 * 管理员活动详情。
 */
export interface AdminCustomActivityDetail extends Omit<AdminCustomActivityListItem, 'total_budget'> {
  description?: string
  cover_url?: string
  total_budget?: MoneyString
  issued_amount: MoneyString
  participant_count: number
  red_packet_rain?: AdminRedPacketRainConfig
  rounds?: AdminRedPacketRainRoundSummary[]
}

/**
 * 管理员创建或更新活动请求。
 */
export interface AdminCustomActivityUpsertRequest {
  type: CustomActivityType
  title: string
  description?: string
  cover_url?: string
  starts_at: string
  ends_at: string
  red_packet_rain: AdminRedPacketRainConfig
}

/**
 * 管理员结束或下架操作响应。
 */
export interface AdminCustomActivityActionResponse {
  id: number
  status: CustomActivityStatus
  message: string
}

/**
 * 管理员活动领取记录。
 */
export interface AdminCustomActivityClaimItem {
  id: number
  round_no: number
  user_id: number
  hit_count: number
  reward_amount: MoneyString
  created_at: string
}

/**
 * 管理员活动领取记录响应。
 */
export interface AdminCustomActivityClaimsResponse {
  items: AdminCustomActivityClaimItem[]
  total: number
}

/**
 * 管理员领取记录查询参数。
 */
export interface AdminCustomActivityClaimsParams {
  page?: number
  page_size?: number
}
