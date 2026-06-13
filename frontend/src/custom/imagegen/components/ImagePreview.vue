<template>
  <div :class="['overflow-hidden rounded-xl border border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-800', ratioClass]">
    <div class="relative flex h-full min-h-[160px] w-full items-center justify-center">
      <div
        v-if="loading"
        class="absolute inset-0 animate-pulse bg-gradient-to-br from-gray-100 to-gray-200 dark:from-dark-700 dark:to-dark-800"
        aria-hidden="true"
      ></div>

      <img
        v-if="safeSrc && !imageFailed"
        :src="safeSrc"
        :alt="alt"
        class="h-full w-full object-cover"
        :loading="lazy ? 'lazy' : 'eager'"
        @load="handleLoad"
        @error="handleError"
      />

      <div v-else class="flex flex-col items-center gap-2 px-4 text-center text-sm text-gray-500 dark:text-gray-400">
        <span>{{ emptyText }}</span>
      </div>

      <div v-if="safeSrc && !imageFailed && showActions" class="absolute bottom-3 right-3 flex items-center gap-2">
        <a
          :href="safeSrc"
          target="_blank"
          rel="noopener noreferrer"
          class="rounded-lg bg-white/90 px-3 py-1.5 text-xs font-medium text-gray-700 shadow-sm transition-colors hover:bg-white dark:bg-dark-900/90 dark:text-gray-200"
        >
          查看
        </a>
        <button
          v-if="showDownload"
          type="button"
          class="rounded-lg bg-primary-600 px-3 py-1.5 text-xs font-medium text-white shadow-sm transition-colors hover:bg-primary-700"
          @click="handleDownload"
        >
          下载
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'

type ImagePreviewRatio = 'square' | 'video' | 'auto'

/**
 * ImagePreview 展示单张图片，并提供基础查看和下载动作。
 *
 * 下载通过原始 URL 触发，跨域文件是否强制下载由浏览器和资源响应头决定。
 */
const props = withDefaults(defineProps<{
  src?: string | null
  alt?: string
  loading?: boolean
  lazy?: boolean
  ratio?: ImagePreviewRatio
  downloadName?: string
  showActions?: boolean
  showDownload?: boolean
}>(), {
  alt: '图片预览',
  loading: false,
  lazy: true,
  ratio: 'square',
  downloadName: 'image.png',
  showActions: true,
  showDownload: true,
})

const emit = defineEmits<{
  load: []
  error: []
  download: [url: string]
}>()

const imageFailed = ref(false)
const safeSrc = computed(() => props.src?.trim() || '')

const emptyText = computed(() => {
  if (imageFailed.value) {
    return '图片加载失败'
  }
  return props.loading ? '图片加载中' : '暂无图片'
})

const ratioClass = computed(() => {
  if (props.ratio === 'video') {
    return 'aspect-video'
  }
  if (props.ratio === 'auto') {
    return ''
  }
  return 'aspect-square'
})

watch(safeSrc, () => {
  imageFailed.value = false
})

/**
 * 图片加载成功后通知父级收起外层 loading。
 */
function handleLoad(): void {
  imageFailed.value = false
  emit('load')
}

/**
 * 图片失败时保留卡片尺寸，避免列表布局跳动。
 */
function handleError(): void {
  imageFailed.value = true
  emit('error')
}

/**
 * 触发浏览器下载动作。
 */
function handleDownload(): void {
  if (!safeSrc.value || typeof document === 'undefined') {
    return
  }

  emit('download', safeSrc.value)

  const link = document.createElement('a')
  link.href = safeSrc.value
  link.download = props.downloadName
  link.rel = 'noopener noreferrer'
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
}
</script>
