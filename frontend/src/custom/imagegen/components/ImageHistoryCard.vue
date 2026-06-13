<template>
  <article class="overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900">
    <ImagePreview
      :src="imageURL"
      :alt="imageAlt"
      :download-name="downloadName"
      @download="$emit('download', item)"
    />

    <div class="space-y-3 p-4">
      <div class="flex items-center justify-between gap-3">
        <span v-if="showGalleryStatus && myImageItem" :class="galleryStatusClass">
          {{ galleryStatusText }}
        </span>
        <span v-else class="text-xs text-gray-500 dark:text-gray-400">{{ createdAtText }}</span>

        <span v-if="showGalleryStatus && myImageItem" class="text-xs text-gray-500 dark:text-gray-400">
          {{ createdAtText }}
        </span>
      </div>

      <p v-if="showPrompt && item.prompt" class="line-clamp-2 text-sm text-gray-700 dark:text-gray-300">
        {{ item.prompt }}
      </p>

      <div v-if="myImageItem && showGalleryActions" class="flex items-center gap-2">
        <button
          v-if="!myImageItem.in_gallery"
          type="button"
          class="rounded-lg bg-primary-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-primary-700"
          @click="handlePublish"
        >
          公开
        </button>
        <button
          v-else
          type="button"
          class="rounded-lg border border-gray-300 px-3 py-1.5 text-xs font-medium text-gray-700 transition-colors hover:bg-gray-50 dark:border-dark-600 dark:text-gray-200 dark:hover:bg-dark-800"
          @click="handleHide"
        >
          隐藏
        </button>
      </div>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import ImagePreview from './ImagePreview.vue'
import type { MyImageItem, PublicGalleryItem } from '../types'

type HistoryCardItem = MyImageItem | PublicGalleryItem

/**
 * ImageHistoryCard 展示“我的图片”或公共图库图片。
 *
 * 两类数据的图片字段不同，组件内部统一读取，页面层不用重复写兼容逻辑。
 */
const props = withDefaults(defineProps<{
  item: HistoryCardItem
  showPrompt?: boolean
  showGalleryStatus?: boolean
  showGalleryActions?: boolean
}>(), {
  showPrompt: true,
  showGalleryStatus: true,
  showGalleryActions: true,
})

const emit = defineEmits<{
  publish: [item: MyImageItem]
  hide: [item: MyImageItem]
  download: [item: HistoryCardItem]
}>()

const myImageItem = computed(() => (isMyImageItem(props.item) ? props.item : null))
const imageURL = computed(() => {
  if (isMyImageItem(props.item)) {
    return props.item.url
  }
  return props.item.image_url
})
const imageAlt = computed(() => {
  if (isMyImageItem(props.item)) {
    return `我的图片 ${props.item.task_id}-${props.item.image_index + 1}`
  }
  return `图库图片 ${props.item.id}`
})
const downloadName = computed(() => {
  if (isMyImageItem(props.item)) {
    return `my-image-${props.item.task_id}-${props.item.image_index + 1}.png`
  }
  return `gallery-image-${props.item.id}.png`
})

const createdAtText = computed(() => {
  const raw = isMyImageItem(props.item) ? props.item.created_at : props.item.published_at
  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) {
    return '时间未知'
  }
  return date.toLocaleString()
})

const galleryStatusText = computed(() => {
  if (!myImageItem.value) {
    return ''
  }
  return myImageItem.value.in_gallery ? '已公开' : '未公开'
})

const galleryStatusClass = computed(() => {
  if (!myImageItem.value?.in_gallery) {
    return 'rounded-full bg-gray-100 px-2 py-1 text-xs font-medium text-gray-600 dark:bg-dark-700 dark:text-gray-300'
  }
  return 'rounded-full bg-blue-50 px-2 py-1 text-xs font-medium text-blue-700 dark:bg-blue-900/20 dark:text-blue-300'
})

/**
 * 判断图片卡片是否来自“我的图片”。
 */
function isMyImageItem(item: HistoryCardItem): item is MyImageItem {
  return 'url' in item
}

/**
 * 触发公开动作。
 */
function handlePublish(): void {
  if (myImageItem.value) {
    emit('publish', myImageItem.value)
  }
}

/**
 * 触发隐藏动作。
 */
function handleHide(): void {
  if (myImageItem.value) {
    emit('hide', myImageItem.value)
  }
}
</script>
