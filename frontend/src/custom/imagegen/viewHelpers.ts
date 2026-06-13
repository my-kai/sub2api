import type {
  GeneratedImage,
  ImageGenerationTask,
  ImagePageResult,
  ImagePriceQuote,
  ImageSession,
} from './types'

export type ImageQuality = 'auto' | 'low' | 'medium' | 'high'
export type ImageResolution = '1k' | '2k' | '4k'

export interface ImageQualityOption {
  value: ImageQuality
  label: string
}

export interface ImageResolutionOption {
  value: ImageResolution
  label: string
  detail: string
}

export interface ImageAspectRatioOption {
  value: string
  label: string
  ratioWidth?: number
  ratioHeight?: number
}

export interface TaskImageReference {
  task: ImageGenerationTask
  image: GeneratedImage
  imageIndex: number
  src: string
}

export const defaultImageModelID = 'gpt-image-2'
export const defaultTaskPageSize = 20
export const defaultGalleryPageSize = 20
export const maxCustomImageCount = 100
export const maxUploadImageBytes = 64 * 1024 * 1024

export const imageQualityOptions: ImageQualityOption[] = [
  { value: 'auto', label: '自动' },
  { value: 'low', label: '低' },
  { value: 'medium', label: '中' },
  { value: 'high', label: '高' },
]

export const imageResolutionOptions: ImageResolutionOption[] = [
  { value: '1k', label: '1K', detail: '1024 基准' },
  { value: '2k', label: '2K', detail: '2048 基准' },
  { value: '4k', label: '4K', detail: '3840 基准' },
]

export const imageAspectRatioOptions: ImageAspectRatioOption[] = [
  { value: '1:1', label: '1:1', ratioWidth: 1, ratioHeight: 1 },
  { value: '2:3', label: '2:3', ratioWidth: 2, ratioHeight: 3 },
  { value: '3:2', label: '3:2', ratioWidth: 3, ratioHeight: 2 },
  { value: '3:4', label: '3:4', ratioWidth: 3, ratioHeight: 4 },
  { value: '4:3', label: '4:3', ratioWidth: 4, ratioHeight: 3 },
  { value: '9:16', label: '9:16', ratioWidth: 9, ratioHeight: 16 },
  { value: '16:9', label: '16:9', ratioWidth: 16, ratioHeight: 9 },
]

const imageSizePresets: Record<ImageResolution, Record<string, string>> = {
  '1k': {
    '1:1': '1024x1024',
    '2:3': '1024x1536',
    '3:2': '1536x1024',
    '3:4': '1024x1365',
    '4:3': '1365x1024',
    '9:16': '1088x1920',
    '16:9': '1920x1088',
  },
  '2k': {
    '1:1': '2048x2048',
    '9:16': '1440x2560',
    '16:9': '2560x1440',
  },
  '4k': {
    '9:16': '2160x3840',
    '16:9': '3840x2160',
  },
}

const fallbackAspectRatioByResolution: Record<ImageResolution, string> = {
  '1k': '2:3',
  '2k': '1:1',
  '4k': '16:9',
}

/**
 * 构造空分页对象，避免页面初始化时各自拼默认字段。
 */
export function emptyImagePage<T>(pageSize: number): ImagePageResult<T> {
  return {
    page: 1,
    page_size: pageSize,
    total: 0,
    pages: 0,
    items: [],
  }
}

/**
 * 读取当前分辨率和宽高比对应的后端 size 字段。
 *
 * 页面展示分辨率和比例两层选择，后端仍接收单个 size 字段，所以转换集中在这里。
 */
export function imageSizeFor(resolution: ImageResolution, aspectRatio: string): string {
  return imageSizePresets[resolution]?.[aspectRatio] || 'auto'
}

/**
 * 判断宽高比是否能在当前分辨率下生成明确尺寸。
 */
export function isAspectRatioSupported(resolution: ImageResolution, aspectRatio: string): boolean {
  return Boolean(imageSizePresets[resolution]?.[aspectRatio])
}

/**
 * 分辨率切换后修正不可用比例，避免把 UI 上不可选的尺寸提交给后端。
 */
export function normalizeAspectRatioForResolution(resolution: ImageResolution, aspectRatio: string): string {
  if (isAspectRatioSupported(resolution, aspectRatio)) {
    return aspectRatio
  }
  return fallbackAspectRatioByResolution[resolution]
}

/**
 * 限制图片数量范围，防止手动输入绕过下拉选项。
 */
export function clampImageCount(value: number): number {
  if (!Number.isFinite(value)) {
    return 1
  }
  return Math.min(maxCustomImageCount, Math.max(1, Math.trunc(value)))
}

/**
 * 把接口错误收敛为页面可展示短文案。
 */
export function errorMessage(error: unknown, fallback: string): string {
  if (error && typeof error === 'object' && 'message' in error) {
    const message = String((error as { message?: unknown }).message || '').trim()
    if (message) {
      return message
    }
  }
  return fallback
}

/**
 * 将任务按 id 合并，适合任务创建、取消、重试和 SSE 快照共同更新同一列表。
 */
export function mergeImageTasks(current: ImageGenerationTask[], incoming: ImageGenerationTask[]): ImageGenerationTask[] {
  const byID = new Map<number, ImageGenerationTask>()
  for (const task of current) {
    byID.set(task.id, task)
  }
  for (const task of incoming) {
    byID.set(task.id, task)
  }
  return Array.from(byID.values()).sort(compareTaskCreatedDesc)
}

/**
 * 将会话按 id 合并，并按更新时间展示最近活跃项。
 */
export function mergeImageSessions(current: ImageSession[], incoming: ImageSession[]): ImageSession[] {
  const byID = new Map<number, ImageSession>()
  for (const session of current) {
    byID.set(session.id, session)
  }
  for (const session of incoming) {
    byID.set(session.id, session)
  }
  return Array.from(byID.values()).sort(compareSessionUpdatedDesc)
}

/**
 * 读取任务里可展示的图片结果，过滤空链接以保护图片组件。
 */
export function taskImages(task: ImageGenerationTask): TaskImageReference[] {
  const images = Array.isArray(task.result?.data) ? task.result.data : []
  return images
    .map((image, imageIndex) => ({ task, image, imageIndex, src: image.url?.trim() || '' }))
    .filter((item) => item.src !== '')
}

/**
 * 根据会话当前图片引用，从当前页任务里解析预览图。
 */
export function resolveCurrentTaskImage(
  session: ImageSession | undefined,
  tasks: ImageGenerationTask[],
): TaskImageReference | null {
  if (!session?.current_image_task_id || session.current_image_index === undefined) {
    return null
  }

  const task = tasks.find((item) => item.id === session.current_image_task_id)
  if (!task) {
    return null
  }

  return taskImages(task).find((item) => item.imageIndex === session.current_image_index) ?? null
}

/**
 * 任务创建时间降序，无法解析时按 id 降序兜底。
 */
export function compareTaskCreatedDesc(a: ImageGenerationTask, b: ImageGenerationTask): number {
  const left = Date.parse(a.created_at)
  const right = Date.parse(b.created_at)
  if (Number.isFinite(left) && Number.isFinite(right) && left !== right) {
    return right - left
  }
  return b.id - a.id
}

/**
 * 任务创建时间正序，适合对话流从上到下阅读，最新任务自然落在底部。
 */
export function compareTaskCreatedAsc(a: ImageGenerationTask, b: ImageGenerationTask): number {
  const left = Date.parse(a.created_at)
  const right = Date.parse(b.created_at)
  if (Number.isFinite(left) && Number.isFinite(right) && left !== right) {
    return left - right
  }
  return a.id - b.id
}

/**
 * 会话更新时间降序，无法解析时按 id 降序兜底。
 */
export function compareSessionUpdatedDesc(a: ImageSession, b: ImageSession): number {
  const left = Date.parse(a.updated_at)
  const right = Date.parse(b.updated_at)
  if (Number.isFinite(left) && Number.isFinite(right) && left !== right) {
    return right - left
  }
  return b.id - a.id
}

/**
 * 格式化后端时间；异常时间不阻断列表渲染。
 */
export function formatDateTime(value?: string): string {
  if (!value) {
    return '-'
  }
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return '-'
  }
  return date.toLocaleString()
}

/**
 * 展示任务费用兼容字段；当前 custom 不接主仓余额核心，但保留快照供排查和展示。
 */
export function formatTaskCharge(task: ImageGenerationTask): string {
  return formatCurrencyAmount(task.charge_amount, '$')
}

/**
 * 展示价格预览，加载中和失败态都压成短句。
 */
export function formatPriceQuote(quote: ImagePriceQuote | null | undefined, loading: boolean): string {
  if (loading) {
    return '计算中'
  }
  if (!quote) {
    return '$0'
  }
  return formatCurrencyAmount(quote.total_price, quote.currency || '$')
}

/**
 * 金额展示最多保留 5 位小数，并去掉末尾 0。
 *
 * 后端价格字段以字符串传递；这里仅处理展示，不参与任何计价逻辑。
 */
function formatCurrencyAmount(raw: string | null | undefined, currency: string): string {
  const normalizedCurrency = currency || '$'
  const trimmed = raw?.trim()
  if (!trimmed) {
    return `${normalizedCurrency}0`
  }
  const parsed = Number(trimmed)
  if (!Number.isFinite(parsed)) {
    return `${normalizedCurrency}${trimmed}`
  }
  const amount = parsed.toFixed(5).replace(/\.?0+$/, '')
  return `${normalizedCurrency}${amount || '0'}`
}

/**
 * 生成任务图片在本地状态中的稳定 key。
 */
export function taskImageKey(taskID: number, imageIndex: number): string {
  return `${taskID}-${imageIndex}`
}

/**
 * 生图任务模式短文案。
 */
export function taskModeLabel(task: ImageGenerationTask): string {
  return task.generation_mode === 'edit' ? '编辑' : '文生图'
}

/**
 * 图片质量短文案。
 */
export function qualityLabel(value: ImageQuality): string {
  return imageQualityOptions.find((option) => option.value === value)?.label || '自动'
}

/**
 * 分辨率短文案。
 */
export function resolutionLabel(value: ImageResolution): string {
  return imageResolutionOptions.find((option) => option.value === value)?.label || '1K'
}

/**
 * 宽高比短文案。
 */
export function aspectRatioLabel(value: string): string {
  return imageAspectRatioOptions.find((option) => option.value === value)?.label || value
}
