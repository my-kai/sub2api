import type {
  UserAvailableGroup,
  UserSupportedModel,
  UserSupportedModelPricing,
} from '@/api/channels'

/**
 * 模型广场的卡片数据。
 *
 * 后端 `/channels/available` 是渠道视角，这里只在前端重排为模型视角；
 * 不新增字段来源，避免把用户侧页面绑定到管理端内部数据。
 */
export interface MarketplaceModel {
  id: string
  modelName: string
  channelName: string
  description: string
  platform: string
  groups: UserAvailableGroup[]
  pricing: UserSupportedModelPricing | null
  sourceModel: UserSupportedModel
  tags: string[]
  endpointTypes: ModelEndpointType[]
}

/**
 * 筛选项通用结构。
 */
export interface MarketplaceFilterOption {
  id: string
  label: string
  count: number
}

/**
 * 端点类型只从用户可见定价模式推导；当前接口不返回 Responses / Rerank /
 * Embeddings 等更细粒度能力，所以这里不臆造未确认分类。
 */
export type ModelEndpointType = 'chat' | 'image'

/**
 * 模型广场筛选状态。
 */
export interface MarketplaceFilterState {
  groupId: string
  provider: string
  tag: string
  pricingType: string
  endpointType: string
}

/**
 * 顶部价格单位切换。
 */
export type PriceUnit = '1m' | '1k'

/**
 * 列表排序方式。
 */
export type MarketplaceSortMode = 'name' | 'input_price' | 'output_price'

/**
 * 卡片展示密度。
 */
export type MarketplaceViewMode = 'grid' | 'list'
