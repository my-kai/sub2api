<template>
  <AppLayout>
    <div class="space-y-5">
      <header class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <div class="flex items-center gap-2">
            <Icon name="grid" size="lg" class="text-primary-500" />
            <h1 class="text-lg font-semibold text-gray-900 dark:text-white">我的图片</h1>
          </div>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">查看已完成图片，可公开或隐藏到图库。</p>
        </div>
        <button type="button" class="btn btn-secondary" :disabled="loading" @click="loadImages(page)">
          <Icon v-if="loading" name="refresh" size="sm" class="animate-spin" />
          <Icon v-else name="refresh" size="sm" />
          <span>刷新</span>
        </button>
      </header>

      <section class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900">
        <div v-if="loading && images.items.length === 0" class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4">
          <div v-for="index in 8" :key="index" class="h-72 animate-pulse rounded-2xl bg-gray-100 dark:bg-dark-800"></div>
        </div>

        <div v-else-if="images.items.length === 0" class="rounded-2xl border border-dashed border-gray-300 p-12 text-center dark:border-dark-700">
          <Icon name="grid" size="xl" class="mx-auto text-gray-400" />
          <h2 class="mt-3 text-base font-semibold text-gray-900 dark:text-white">还没有图片</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">生成完成的图片会展示在这里。</p>
        </div>

        <div v-else class="space-y-5">
          <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4">
            <ImageHistoryCard
              v-for="item in images.items"
              :key="`${item.task_id}-${item.image_index}`"
              :item="item"
              @publish="handlePublish"
              @hide="handleHide"
            />
          </div>
          <Pagination
            v-if="images.total > pageSize"
            :page="images.page"
            :page-size="images.page_size"
            :total="images.total"
            :show-page-size-selector="false"
            @update:page="handlePageChange"
            @update:page-size="handlePageSize"
          />
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import Pagination from '@/components/common/Pagination.vue'
import { useAppStore } from '@/stores/app'
import { fetchMyImages, hideMyImage, publishMyImage } from '../api'
import ImageHistoryCard from '../components/ImageHistoryCard.vue'
import type { ImagePageResult, MyImageItem } from '../types'
import { defaultGalleryPageSize, emptyImagePage, errorMessage } from '../viewHelpers'

const appStore = useAppStore()
const pageSize = defaultGalleryPageSize
const page = ref(1)
const images = ref<ImagePageResult<MyImageItem>>(emptyImagePage(pageSize))
const loading = ref(false)
const switchingKey = ref('')
let controller: AbortController | null = null

onMounted(() => {
  void loadImages(page.value)
})

onBeforeUnmount(() => {
  controller?.abort()
})

/**
 * 分页读取当前用户已完成图片。
 */
async function loadImages(nextPage: number): Promise<void> {
  controller?.abort()
  controller = new AbortController()
  loading.value = true
  try {
    const result = await fetchMyImages({ page: nextPage, page_size: pageSize }, { signal: controller.signal })
    images.value = {
      ...result,
      items: normalizeMyImages(result.items),
    }
    page.value = images.value.page
  } catch (err) {
    if ((err as { name?: string }).name !== 'AbortError') {
      appStore.showWarning(errorMessage(err, '我的图片读取失败'))
    }
  } finally {
    if (!controller.signal.aborted) {
      loading.value = false
    }
  }
}

/**
 * 过滤缺少 URL 的异常项，避免单条脏数据影响整页展示。
 */
function normalizeMyImages(items: MyImageItem[] | null | undefined): MyImageItem[] {
  return Array.isArray(items) ? items.filter((item) => Boolean(item.url?.trim())) : []
}

/**
 * 发布图片到公共图库。
 */
async function handlePublish(item: MyImageItem): Promise<void> {
  await switchGalleryState(item, true)
}

/**
 * 从公共图库隐藏图片。
 */
async function handleHide(item: MyImageItem): Promise<void> {
  await switchGalleryState(item, false)
}

/**
 * 切换图库状态并刷新当前页。
 */
async function switchGalleryState(item: MyImageItem, publish: boolean): Promise<void> {
  const key = `${item.task_id}-${item.image_index}`
  if (switchingKey.value) {
    return
  }
  switchingKey.value = key
  try {
    if (publish) {
      await publishMyImage(item.task_id, item.image_index)
      appStore.showSuccess('图片已公开')
    } else {
      await hideMyImage(item.task_id, item.image_index)
      appStore.showSuccess('图片已隐藏')
    }
    await loadImages(page.value)
  } catch (err) {
    appStore.showError(errorMessage(err, '更新图片状态失败'))
  } finally {
    switchingKey.value = ''
  }
}

/**
 * 切换分页。
 */
function handlePageChange(nextPage: number): void {
  page.value = nextPage
  void loadImages(nextPage)
}

/**
 * 当前页面固定分页大小；保留事件处理满足 Pagination 契约。
 */
function handlePageSize(): void {
  page.value = 1
  void loadImages(1)
}
</script>
