import type { UserAvailableChannel, UserSupportedModelPricing } from '@/api/channels'
import {
  BILLING_MODE_IMAGE,
  BILLING_MODE_PER_REQUEST,
  BILLING_MODE_TOKEN,
  type BillingMode,
} from '@/constants/channel'
import { platformLabel } from '@/utils/platformColors'
import { formatScaled } from '@/utils/pricing'
import type {
  MarketplaceFilterOption,
  MarketplaceFilterState,
  MarketplaceModel,
  MarketplaceSortMode,
  ModelEndpointType,
  PriceUnit,
} from './types'

/** 全部筛选项的稳定 ID，避免各组件各自写 magic string。 */
export const ALL_FILTER_ID = 'all'

/**
 * 把渠道视角响应铺平成模型卡片视角。
 *
 * 一个模型在不同渠道或平台下可能有不同定价/分组，因此不按模型名合并；
 * 这样能避免把不同渠道配置混成一个看似统一的模型。
 *
 * @param channels `/api/v1/channels/available` 返回的用户可见渠道列表。
 * @returns 可直接渲染到模型广场卡片的模型条目列表。
 */
export function flattenAvailableChannels(channels: UserAvailableChannel[]): MarketplaceModel[] {
  const entries: MarketplaceModel[] = []

  channels.forEach((channel, channelIndex) => {
    channel.platforms.forEach((section, sectionIndex) => {
      section.supported_models.forEach((model, modelIndex) => {
        const platform = model.platform || section.platform
        const entry: MarketplaceModel = {
          id: [
            channelIndex,
            sectionIndex,
            modelIndex,
            channel.name,
            platform,
            model.name,
          ].join(':'),
          modelName: model.name,
          channelName: channel.name,
          description: channel.description || '',
          platform,
          groups: section.groups,
          pricing: model.pricing,
          sourceModel: model,
          tags: [],
          endpointTypes: resolveEndpointTypes(model.pricing),
        }
        entries.push({
          ...entry,
          tags: deriveTags(entry),
        })
      })
    })
  })

  return entries
}

/**
 * 构建左侧筛选项及计数。
 *
 * @param entries 已铺平的模型条目。
 * @returns 分组、供应商、标签、计费类型、端点类型五组筛选项。
 */
export function buildMarketplaceFilterOptions(entries: MarketplaceModel[]) {
  return {
    groups: withAllOption('所有分组', entries.length, countBy(entries, (entry) =>
      entry.groups.map((group) => ({ id: String(group.id), label: group.name })),
    )),
    providers: withAllOption('所有供应商', entries.length, countBy(entries, (entry) => [
      { id: entry.platform, label: platformLabel(entry.platform) },
    ])),
    tags: withAllOption('所有标签', entries.length, countBy(entries, (entry) =>
      entry.tags.map((tag) => ({ id: tag, label: tag })),
    )),
    pricingTypes: [
      { id: ALL_FILTER_ID, label: '所有模型', count: entries.length },
      { id: BILLING_MODE_TOKEN, label: '按量计费', count: entries.filter((entry) => billingMode(entry.pricing) === BILLING_MODE_TOKEN).length },
      { id: BILLING_MODE_PER_REQUEST, label: '按请求', count: entries.filter((entry) => billingMode(entry.pricing) === BILLING_MODE_PER_REQUEST).length },
      { id: BILLING_MODE_IMAGE, label: '按图片', count: entries.filter((entry) => billingMode(entry.pricing) === BILLING_MODE_IMAGE).length },
    ],
    endpointTypes: [
      { id: ALL_FILTER_ID, label: '所有类型', count: entries.length },
      { id: 'chat', label: 'Chat', count: entries.filter((entry) => entry.endpointTypes.includes('chat')).length },
      { id: 'image', label: '图片', count: entries.filter((entry) => entry.endpointTypes.includes('image')).length },
    ],
  }
}

/**
 * 按当前筛选状态过滤模型。
 *
 * @param entries 已铺平的模型条目。
 * @param filters 用户当前选择的筛选状态。
 * @param searchQuery 搜索关键字，匹配模型、渠道、供应商、分组和标签。
 * @returns 命中筛选条件的模型条目。
 */
export function filterMarketplaceModels(
  entries: MarketplaceModel[],
  filters: MarketplaceFilterState,
  searchQuery: string,
): MarketplaceModel[] {
  const q = searchQuery.trim().toLowerCase()
  return entries.filter((entry) => {
    if (q && !matchesSearch(entry, q)) return false
    if (filters.groupId !== ALL_FILTER_ID && !entry.groups.some((group) => String(group.id) === filters.groupId)) return false
    if (filters.provider !== ALL_FILTER_ID && entry.platform !== filters.provider) return false
    if (filters.tag !== ALL_FILTER_ID && !entry.tags.includes(filters.tag)) return false
    if (filters.pricingType !== ALL_FILTER_ID && billingMode(entry.pricing) !== filters.pricingType) return false
    if (filters.endpointType !== ALL_FILTER_ID && !entry.endpointTypes.includes(filters.endpointType as ModelEndpointType)) return false
    return true
  })
}

/**
 * 模型广场排序：价格缺失排到最后，避免未配置定价的模型盖过可比较项。
 *
 * @param entries 已过滤的模型条目。
 * @param mode 排序方式。
 * @returns 排序后的新数组，不修改入参。
 */
export function sortMarketplaceModels(
  entries: MarketplaceModel[],
  mode: MarketplaceSortMode,
): MarketplaceModel[] {
  const sorted = [...entries]
  sorted.sort((a, b) => {
    if (mode === 'name') return a.modelName.localeCompare(b.modelName)
    const aPrice = comparablePrice(a.pricing, mode)
    const bPrice = comparablePrice(b.pricing, mode)
    if (aPrice !== bPrice) return aPrice - bPrice
    return a.modelName.localeCompare(b.modelName)
  })
  return sorted
}

/**
 * 按展示单位格式化 token 单价。
 *
 * @param value 后端返回的单 token 单价；为空时显示 `-`。
 * @param unit 页面当前价格单位。
 * @returns 带美元符号的缩放后价格文本。
 */
export function formatTokenPrice(value: number | null | undefined, unit: PriceUnit): string {
  return formatScaled(value ?? null, unit === '1m' ? 1_000_000 : 1_000)
}

/**
 * 返回当前价格单位的文案后缀。
 *
 * @param unit 页面当前价格单位。
 * @returns `/1M` 或 `/1K`。
 */
export function priceUnitLabel(unit: PriceUnit): string {
  return unit === '1m' ? '/1M' : '/1K'
}

/**
 * 定价模式展示名。
 *
 * @param mode 后端计费模式；为空时按 token 计费兜底。
 * @returns 用户可读的中文计费类型。
 */
export function billingModeLabel(mode: BillingMode | string | null | undefined): string {
  switch (mode) {
    case BILLING_MODE_PER_REQUEST:
      return '按请求'
    case BILLING_MODE_IMAGE:
      return '按图片'
    case BILLING_MODE_TOKEN:
    default:
      return '按量计费'
  }
}

function billingMode(pricing: UserSupportedModelPricing | null): BillingMode {
  return (pricing?.billing_mode || BILLING_MODE_TOKEN) as BillingMode
}

function resolveEndpointTypes(pricing: UserSupportedModelPricing | null): ModelEndpointType[] {
  return billingMode(pricing) === BILLING_MODE_IMAGE ? ['image'] : ['chat']
}

function deriveTags(entry: MarketplaceModel): string[] {
  const tags = new Set<string>()
  const mode = billingMode(entry.pricing)
  tags.add(billingModeLabel(mode))
  if (mode === BILLING_MODE_IMAGE || (entry.pricing?.image_output_price ?? 0) > 0) tags.add('图片')
  if ((entry.pricing?.intervals?.length ?? 0) > 0) tags.add('阶梯定价')
  if ((entry.pricing?.cache_write_price ?? 0) > 0 || (entry.pricing?.cache_read_price ?? 0) > 0) tags.add('缓存')
  return Array.from(tags)
}

function matchesSearch(entry: MarketplaceModel, query: string): boolean {
  const haystack = [
    entry.modelName,
    entry.channelName,
    entry.description,
    entry.platform,
    platformLabel(entry.platform),
    ...entry.groups.map((group) => group.name),
    ...entry.tags,
  ].join(' ').toLowerCase()
  return haystack.includes(query)
}

function comparablePrice(
  pricing: UserSupportedModelPricing | null,
  mode: MarketplaceSortMode,
): number {
  if (!pricing) return Number.POSITIVE_INFINITY
  if (mode === 'input_price') return pricing.input_price ?? pricing.per_request_price ?? pricing.image_output_price ?? Number.POSITIVE_INFINITY
  return pricing.output_price ?? pricing.per_request_price ?? pricing.image_output_price ?? Number.POSITIVE_INFINITY
}

function withAllOption(
  label: string,
  total: number,
  options: MarketplaceFilterOption[],
): MarketplaceFilterOption[] {
  return [{ id: ALL_FILTER_ID, label, count: total }, ...options]
}

function countBy(
  entries: MarketplaceModel[],
  getter: (entry: MarketplaceModel) => Array<{ id: string; label: string }>,
): MarketplaceFilterOption[] {
  const map = new Map<string, MarketplaceFilterOption>()
  entries.forEach((entry) => {
    getter(entry).forEach(({ id, label }) => {
      if (!id) return
      const existing = map.get(id)
      if (existing) {
        existing.count += 1
      } else {
        map.set(id, { id, label, count: 1 })
      }
    })
  })
  return Array.from(map.values()).sort((a, b) => b.count - a.count || a.label.localeCompare(b.label))
}
