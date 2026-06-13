/**
 * 生图模型基础信息。
 *
 * 后端只要求 `id` 可用于创建任务，其余字段保持透传，避免前端绑定上游模型细节。
 */
export interface ImageModel {
  id: string
  object?: string
  owned_by?: string
}

/**
 * 生图模型列表响应。
 */
export interface ImageModelsResponse {
  data: ImageModel[]
}

/**
 * 单张生成图片结果。
 *
 * `revised_prompt` 由上游按需返回，页面只能作为辅助展示，不能依赖它判断任务成功。
 */
export interface GeneratedImage {
  url?: string
  revised_prompt?: string
}

/**
 * 生图结果集合。
 */
export interface ImageGenerationResponse {
  created?: number
  data: GeneratedImage[]
}

/**
 * 生图任务状态。
 */
export type ImageTaskStatus = 'queued' | 'running' | 'completed' | 'failed' | 'canceled'

/**
 * 费用状态兼容字段。
 *
 * 当前生图不接主仓余额扣费核心，但保留字段用于兼容旧页面展示。
 */
export type ImageChargeStatus = 'none' | 'pending' | 'success' | 'failed' | 'refunded' | 'refund_failed'

/**
 * 生图模式。
 */
export type ImageGenerationMode = 'generate' | 'edit'

/**
 * 生图任务主体。
 */
export interface ImageGenerationTask {
  id: number
  user_id: number
  username?: string
  email?: string
  status: ImageTaskStatus
  session_id?: number
  generation_mode?: ImageGenerationMode
  source_image_task_id?: number
  source_image_index?: number
  model: string
  prompt: string
  n: number
  quality?: string
  size?: string
  publish_to_gallery?: boolean
  charge_amount?: string
  charge_status?: ImageChargeStatus
  balance_idempotency_key?: string
  charge_message?: string
  queue_position?: number
  result?: ImageGenerationResponse
  error_message?: string
  created_at: string
  started_at?: string
  finished_at?: string
}

/**
 * 创建或查询任务时的响应。
 */
export interface ImageTaskResponse {
  task: ImageGenerationTask
  balance: number | string
}

/**
 * 分页响应。
 */
export interface ImagePageResult<T> {
  page: number
  page_size: number
  total: number
  pages: number
  items: T[]
}

/**
 * 通用分页请求参数。
 */
export interface ImagePageParams {
  page?: number
  page_size?: number
}

/**
 * 会话任务 SSE 快照。
 */
export interface ImageSessionTasksEvent {
  tasks: ImagePageResult<ImageGenerationTask>
  balance: number | string
}

/**
 * SSE 订阅句柄。
 */
export interface ImageTaskEventSubscription {
  close: () => void
}

/**
 * 生图会话。
 */
export interface ImageSession {
  id: number
  title: string
  current_image_task_id?: number
  current_image_index?: number
  last_task_id?: number
  created_at: string
  updated_at: string
}

/**
 * 会话列表响应。
 */
export interface ImageSessionListResponse {
  items?: ImageSession[] | null
}

/**
 * 会话详情响应。
 */
export interface ImageSessionResponse {
  session: ImageSession
}

/**
 * 创建生图任务请求。
 */
export interface CreateImageTaskRequest {
  session_id: number
  model: string
  prompt: string
  n: number
  quality?: string
  size?: string
  publish_to_gallery?: boolean
}

/**
 * 创建图片编辑任务请求。
 */
export interface CreateImageEditTaskRequest extends CreateImageTaskRequest {
  image: File
}

/**
 * 当前会话编辑来源图。
 */
export interface SetCurrentImageRequest {
  task_id: number
  image_index: number
}

/**
 * 我的图片列表项。
 */
export interface MyImageItem {
  task_id: number
  image_index: number
  url: string
  created_at: string
  prompt?: string
  gallery_item_id?: number
  in_gallery?: boolean
}

/**
 * 公共图库图片。
 */
export interface PublicGalleryItem {
  id: number
  image_url: string
  published_at: string
  prompt?: string
}

/**
 * 图库发布状态响应。
 */
export interface ImageGalleryMutationResponse {
  gallery_item: {
    id: number
    in_gallery: boolean
  }
}

/**
 * 图片规格价格配置。
 */
export interface ImageUnitPrices {
  one_k: string
  two_k: string
  four_k: string
}

/**
 * 生图价格预览。
 */
export interface ImagePriceQuote {
  model: string
  resolution: string
  count: number
  unit_price: string
  total_price: string
  currency: string
}

/**
 * 生图价格预览参数。
 */
export interface ImagePriceQuoteParams {
  model: string
  resolution: string
  size?: string
  n: number
}

/**
 * 用户侧生图公开状态。
 */
export interface ImageGenerationStatus {
  enabled: boolean
}

/**
 * 管理员生图配置。
 */
export interface ImageQueueConfig {
  enabled: boolean
  platform_concurrency: number
  default_user_concurrency: number
  retention_days: number
  unit_prices: ImageUnitPrices
  chatgpt2api: ImageUpstreamConfig
  updated_by_user_id?: number
  updated_at?: string
}

/**
 * chatgpt2api 管理页可见配置。
 *
 * auth-key 只返回是否已配置，不返回密钥明文。
 */
export interface ImageUpstreamConfig {
  base_url: string
  auth_key_configured: boolean
}

/**
 * 管理员保存生图配置请求。
 */
export interface ImageQueueConfigInput {
  enabled: boolean
  platform_concurrency: number
  default_user_concurrency: number
  retention_days: number
  unit_prices: ImageUnitPrices
  chatgpt2api: ImageUpstreamConfigInput
}

/**
 * chatgpt2api 管理页保存输入。
 *
 * auth_key 留空且 clear_auth_key 为 false 时，后端会保留当前密钥。
 */
export interface ImageUpstreamConfigInput {
  base_url: string
  auth_key: string
  clear_auth_key: boolean
}

/**
 * 用户生图并发限制。
 */
export interface ImageUserLimit {
  user_id: number
  username?: string
  email?: string
  concurrency: number
  updated_at?: string
}

/**
 * 用户限制列表响应。
 */
export interface ImageUserLimitListResponse {
  items?: ImageUserLimit[] | null
}

/**
 * 管理员搜索用户候选项。
 */
export interface ImageAdminUserOption {
  id: number
  username?: string
  email?: string
}

/**
 * 管理员用户搜索响应。
 */
export interface ImageAdminUserSearchResponse {
  items?: ImageAdminUserOption[] | null
}
