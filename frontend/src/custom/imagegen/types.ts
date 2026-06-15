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
 * 用户侧固定使用的 OpenAI Image API 模型。
 */
export type ImageGenerationModel = 'gpt-image-2'

/**
 * OpenAI Image API 输出图片质量。
 */
export type ImageTaskQuality = 'auto' | 'low' | 'medium' | 'high'

/**
 * OpenAI Image API 输出格式。
 */
export type ImageOutputFormat = 'png' | 'jpeg' | 'webp'

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
  model: string
  prompt: string
  n: number
  quality?: ImageTaskQuality | string
  size?: string
  output_format?: ImageOutputFormat | string
  output_compression?: number
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
 * OpenAI `gpt-image-2` 图片生成请求字段。
 */
export interface ImageGenerationCreateRequest {
  model: ImageGenerationModel
  prompt: string
  n: number
  size?: string
  quality?: ImageTaskQuality
  output_format?: ImageOutputFormat
  output_compression?: number
}

/**
 * 创建生图任务请求。
 *
 * `model/prompt/n/size/quality/output_format/output_compression` 对齐 OpenAI
 * `gpt-image-2` Image API；`session_id` 和 `publish_to_gallery` 只属于本项目异步任务外壳。
 */
export interface CreateImageTaskRequest extends ImageGenerationCreateRequest {
  session_id: number
  /**
   * 是否发布到公共图库；该字段不参与上游 Image API 请求。
   */
  publish_to_gallery?: boolean
}

/**
 * 当前编辑图引用。
 *
 * 页面只表达“当前编辑图片”，序列化成 OpenAI `image` 字段的细节由 API 层处理。
 */
export interface ImageEditSourceReference {
  kind: 'current_task_image'
  task_id: number
  image_index: number
}

/**
 * 创建图片编辑任务请求。
 *
 * `image` 使用 OpenAI `images.edit` 语义：可以是浏览器上传的图片文件，也可以是后端可解析的当前编辑图引用。
 */
export interface CreateImageEditTaskRequest extends CreateImageTaskRequest {
  image: File | ImageEditSourceReference
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
 * 用户侧生图 OpenAI 兼容 Key。
 *
 * `key` 只会在创建成功响应里出现；列表页必须使用脱敏字段展示，避免再次暴露完整 Key。
 */
export interface ImageAPIKey {
  id: number | string
  name: string
  key_prefix?: string
  key_suffix?: string
  masked_key?: string
  key?: string
  enabled: boolean
  last_used_at?: string | null
  created_at: string
}

/**
 * 用户侧生图 Key 列表响应。
 */
export interface ImageAPIKeyListResponse {
  items?: ImageAPIKey[] | null
}

/**
 * 创建用户侧生图 Key 请求。
 */
export interface CreateImageAPIKeyRequest {
  name: string
}

/**
 * 创建用户侧生图 Key 响应。
 */
export interface CreateImageAPIKeyResponse {
  api_key: ImageAPIKey
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
  upstream_channels: ImageUpstreamChannel[]
  /**
   * Deprecated: 仅兼容旧后端响应；管理页保存时必须提交 upstream_channels。
   */
  chatgpt2api?: ImageUpstreamConfig
  updated_by_user_id?: number
  updated_at?: string
}

/**
 * 旧 chatgpt2api 响应兼容配置。
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
  upstream_channels: ImageUpstreamChannelInput[]
}

/**
 * custom 生图支持的上游渠道类型。
 */
export type ImageUpstreamChannelType = 'chatgpt2api' | 'openai'

/**
 * 管理页读取到的单个上游渠道。
 *
 * auth-key 不会回显，页面只能根据 auth_key_configured 展示保留或清空入口。
 */
export interface ImageUpstreamChannel {
  id: string
  name: string
  type: ImageUpstreamChannelType
  enabled: boolean
  priority: number
  base_url: string
  auth_key_configured: boolean
  retry_count: number
}

/**
 * 管理页保存单个上游渠道的请求结构。
 *
 * auth_key 留空且 clear_auth_key 为 false 时，后端会按渠道 ID 保留当前密钥。
 */
export interface ImageUpstreamChannelInput {
  id: string
  name: string
  type: ImageUpstreamChannelType
  enabled: boolean
  priority: number
  base_url: string
  auth_key: string
  clear_auth_key: boolean
  retry_count: number
}

/**
 * 管理页渠道表单比保存结构多 auth_key_configured，用于安全展示密钥状态。
 */
export interface ImageUpstreamChannelForm extends ImageUpstreamChannelInput {
  auth_key_configured: boolean
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
