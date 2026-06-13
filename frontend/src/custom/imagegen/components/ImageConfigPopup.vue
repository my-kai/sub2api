<template>
  <div ref="popupRef" class="relative shrink-0">
    <button
      type="button"
      class="btn btn-secondary btn-icon"
      :class="{ 'border-primary-300 bg-primary-50 text-primary-700 dark:border-primary-800 dark:bg-primary-900/20 dark:text-primary-300': open }"
      aria-label="生图配置"
      title="生图配置"
      :aria-expanded="open"
      @click="toggle"
    >
      <Icon name="cog" size="sm" />
    </button>

    <div
      v-if="open"
      class="absolute bottom-full right-0 z-50 mb-3 w-[min(22rem,calc(100vw-2rem))] rounded-2xl border border-gray-200 bg-white p-4 text-left shadow-xl dark:border-dark-700 dark:bg-dark-900"
      role="dialog"
      aria-label="生图配置"
    >
      <div class="mb-3 flex items-center justify-between">
        <h3 class="text-sm font-semibold text-gray-900 dark:text-white">生图配置</h3>
        <button type="button" class="rounded-lg p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-dark-800 dark:hover:text-gray-200" aria-label="关闭配置" @click="close">
          <Icon name="x" size="sm" />
        </button>
      </div>

      <div class="grid gap-3">
        <label class="space-y-1 text-sm">
          <span class="block whitespace-nowrap text-gray-600 dark:text-gray-300">模型</span>
          <Select :model-value="selectedModel" :options="modelOptions" disabled @update:model-value="emitSelectedModel" />
        </label>

        <div class="grid gap-3 sm:grid-cols-2">
          <label class="space-y-1 text-sm">
            <span class="block whitespace-nowrap text-gray-600 dark:text-gray-300">质量</span>
            <Select :model-value="quality" :options="qualityOptions" @update:model-value="emitQuality" />
          </label>

          <label class="space-y-1 text-sm">
            <span class="block whitespace-nowrap text-gray-600 dark:text-gray-300">分辨率</span>
            <Select :model-value="resolution" :options="resolutionOptions" @update:model-value="emitResolution" />
          </label>

          <div class="space-y-1 text-sm sm:col-span-2">
            <span class="block whitespace-nowrap text-gray-600 dark:text-gray-300">宽高比</span>
            <div class="grid grid-cols-7 gap-1.5">
              <button
                v-for="option in imageAspectRatioOptions"
                :key="option.value"
                type="button"
                :disabled="isAspectRatioOptionDisabled(option.value)"
                :aria-pressed="option.value === aspectRatio"
                :class="[
                  'flex h-12 min-w-0 flex-col items-center justify-center gap-1 rounded-lg border text-[11px] font-semibold leading-none transition',
                  option.value === aspectRatio
                    ? 'border-primary-500 bg-primary-50 text-primary-700 dark:border-primary-500 dark:bg-primary-900/30 dark:text-primary-200'
                    : 'border-gray-200 bg-white text-gray-700 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-300',
                  isAspectRatioOptionDisabled(option.value)
                    ? 'cursor-not-allowed opacity-40'
                    : 'hover:border-primary-400 hover:text-primary-600 dark:hover:border-primary-500 dark:hover:text-primary-300'
                ]"
                @click="selectAspectRatio(option)"
              >
                <span class="flex h-6 w-8 items-center justify-center" aria-hidden="true">
                  <span class="block rounded-sm border-2 border-current" :style="aspectRatioPreviewStyle(option)" />
                </span>
                <span class="truncate">{{ option.label }}</span>
              </button>
            </div>
          </div>

          <div class="space-y-1.5 text-sm sm:col-span-2">
            <div class="flex items-center justify-between gap-2">
              <span class="block whitespace-nowrap text-gray-600 dark:text-gray-300">数量</span>
              <input :value="count" class="input h-8 w-24" type="number" min="1" :max="maxCustomImageCount" @input="emitCount" @blur="emitClampedCount" />
            </div>
            <div class="grid grid-cols-10 gap-1">
              <button
                v-for="value in quickImageCountOptions"
                :key="value"
                type="button"
                :aria-pressed="value === count"
                :class="[
                  'h-8 rounded-lg border text-xs font-semibold transition',
                  value === count
                    ? 'border-primary-500 bg-primary-50 text-primary-700 dark:border-primary-500 dark:bg-primary-900/30 dark:text-primary-200'
                    : 'border-gray-200 bg-white text-gray-700 hover:border-primary-400 hover:text-primary-600 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-300 dark:hover:border-primary-500 dark:hover:text-primary-300'
                ]"
                @click="selectQuickCount(value)"
              >
                {{ value }}
              </button>
            </div>
          </div>
        </div>

        <label class="inline-flex items-center gap-2 rounded-xl bg-gray-50 px-3 py-2 text-sm text-gray-700 dark:bg-dark-800 dark:text-gray-300">
          <input :checked="publishToGallery" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" @change="emitPublishToGallery" />
          <span class="whitespace-nowrap">公开到图库</span>
        </label>

      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import {
  imageAspectRatioOptions,
  imageQualityOptions,
  imageResolutionOptions,
  isAspectRatioSupported,
  maxCustomImageCount,
  type ImageAspectRatioOption,
  type ImageQuality,
  type ImageResolution,
} from '../viewHelpers'

const props = defineProps<{
  defaultImageModelId: string
  selectedModel: string
  quality: ImageQuality
  resolution: ImageResolution
  aspectRatio: string
  count: number
  publishToGallery: boolean
  clampImageCount: (value: number) => number
}>()

const emit = defineEmits<{
  'update:selectedModel': [value: string]
  'update:quality': [value: ImageQuality]
  'update:resolution': [value: ImageResolution]
  'update:aspectRatio': [value: string]
  'update:count': [value: number]
  'update:publishToGallery': [value: boolean]
  resolutionChange: []
}>()

type SelectValue = string | number | boolean | null

const open = ref(false)
const popupRef = ref<HTMLElement | null>(null)

/**
 * 数量快捷项沿用原来的 1-10，超过 10 的特殊数量仍交给自定义输入处理。
 */
const quickImageCountOptions = Array.from({ length: 10 }, (_, index) => index + 1)

const modelOptions = computed<SelectOption[]>(() => {
  /**
   * custom 生图当前只开放 gpt-image-2；这里保留 Select 组件展示，但不再透传上游模型列表。
   */
  return [{ value: props.defaultImageModelId, label: props.defaultImageModelId }]
})

const qualityOptions = computed<SelectOption[]>(() =>
  imageQualityOptions.map((option) => ({
    value: option.value,
    label: option.label,
  })),
)

const resolutionOptions = computed<SelectOption[]>(() =>
  imageResolutionOptions.map((option) => ({
    value: option.value,
    label: option.label,
  })),
)

/**
 * 切换生图配置弹层。
 */
function toggle(): void {
  open.value = !open.value
}

/**
 * 关闭生图配置弹层。
 */
function close(): void {
  open.value = false
}

/**
 * 处理弹层外点击和 Escape 关闭。
 */
function handleDocumentEvent(event: MouseEvent | KeyboardEvent): void {
  if (!open.value) {
    return
  }
  if (event instanceof KeyboardEvent) {
    if (event.key === 'Escape') {
      close()
    }
    return
  }
  const target = event.target
  if (!(target instanceof Node)) {
    return
  }
  if (!popupRef.value?.contains(target)) {
    close()
  }
}

/**
 * 模型已固定为 gpt-image-2；忽略组件传回值，避免旧状态或异常事件改出其他模型。
 */
function emitSelectedModel(_value: SelectValue): void {
  emit('update:selectedModel', props.defaultImageModelId)
}

/**
 * 更新图片质量配置。
 */
function emitQuality(value: SelectValue): void {
  emit('update:quality', String(value || 'auto') as ImageQuality)
}

/**
 * 更新分辨率后通知父组件修正宽高比，避免保留当前分辨率不支持的尺寸组合。
 */
function emitResolution(value: SelectValue): void {
  emit('update:resolution', String(value || '1k') as ImageResolution)
  emit('resolutionChange')
}

/**
 * 更新宽高比配置。
 */
function emitAspectRatio(value: SelectValue): void {
  emit('update:aspectRatio', String(value || '1:1'))
}

/**
 * 判断宽高比按钮是否可用；禁用态跟后端 size 映射保持一致，避免提交当前分辨率不支持的尺寸。
 */
function isAspectRatioOptionDisabled(value: string): boolean {
  return !isAspectRatioSupported(props.resolution, value)
}

/**
 * 更新宽高比时二次拦截禁用项，避免键盘或事件冒泡绕过按钮 disabled 状态。
 */
function selectAspectRatio(option: ImageAspectRatioOption): void {
  if (isAspectRatioOptionDisabled(option.value)) {
    return
  }
  emitAspectRatio(option.value)
}

/**
 * 按比例绘制预览框，只表达横竖形状，不绑定具体像素尺寸。
 */
function aspectRatioPreviewStyle(option: ImageAspectRatioOption): Record<string, string> {
  const width = option.ratioWidth || 1
  const height = option.ratioHeight || 1
  const maxPreviewSize = 24
  const maxRatio = Math.max(width, height)

  return {
    width: `${Math.round((width / maxRatio) * maxPreviewSize)}px`,
    height: `${Math.round((height / maxRatio) * maxPreviewSize)}px`,
  }
}

function emitCount(event: Event): void {
  emit('update:count', Number((event.target as HTMLInputElement).value))
}

/**
 * 快捷数量也走统一 clamp，避免以后调整最大数量时出现入口不一致。
 */
function selectQuickCount(value: number): void {
  emit('update:count', props.clampImageCount(value))
}

function emitClampedCount(): void {
  emit('update:count', props.clampImageCount(props.count))
}

function emitPublishToGallery(event: Event): void {
  emit('update:publishToGallery', (event.target as HTMLInputElement).checked)
}

onMounted(() => {
  document.addEventListener('click', handleDocumentEvent)
  document.addEventListener('keydown', handleDocumentEvent)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', handleDocumentEvent)
  document.removeEventListener('keydown', handleDocumentEvent)
})
</script>
