<template>
  <AppLayout>
    <div class="space-y-5">
      <header class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <div class="flex items-center gap-2">
            <Icon name="gift" size="lg" class="text-red-500" />
            <h1 class="text-lg font-semibold text-gray-900 dark:text-white">活动中心</h1>
          </div>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">查看当前可参加的活动。</p>
        </div>
        <button type="button" class="btn btn-secondary" :disabled="loading" @click="loadActivities">
          <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
          <span>刷新</span>
        </button>
      </header>

      <section class="rounded-3xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900">
        <div v-if="loading && activities.length === 0" class="grid gap-5 md:grid-cols-2 xl:grid-cols-3">
          <div v-for="index in 6" :key="index" class="h-80 animate-pulse rounded-3xl bg-gray-100 dark:bg-dark-800"></div>
        </div>

        <div v-else-if="errorMessage" class="rounded-2xl border border-dashed border-red-200 p-12 text-center dark:border-red-900/50">
          <Icon name="exclamationCircle" size="xl" class="mx-auto text-red-500" />
          <h2 class="mt-3 text-base font-semibold text-gray-900 dark:text-white">活动读取失败</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ errorMessage }}</p>
        </div>

        <div v-else-if="activities.length === 0" class="rounded-2xl border border-dashed border-gray-300 p-12 text-center dark:border-dark-700">
          <Icon name="gift" size="xl" class="mx-auto text-gray-400" />
          <h2 class="mt-3 text-base font-semibold text-gray-900 dark:text-white">暂无活动</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">有新活动时会显示在这里。</p>
        </div>

        <div v-else class="grid gap-5 md:grid-cols-2 xl:grid-cols-3">
          <ActivityCard v-for="activity in activities" :key="activity.id" :activity="activity" />
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { fetchCustomActivities } from '../api'
import ActivityCard from '../components/ActivityCard.vue'
import type { CustomActivityListItem } from '../types'

const appStore = useAppStore()
const activities = ref<CustomActivityListItem[]>([])
const loading = ref(false)
const errorMessage = ref('')
let controller: AbortController | null = null

onMounted(() => {
  void loadActivities()
})

onBeforeUnmount(() => {
  controller?.abort()
})

/**
 * 读取用户可见活动列表。
 */
async function loadActivities(): Promise<void> {
  controller?.abort()
  controller = new AbortController()
  loading.value = true
  errorMessage.value = ''
  try {
    const result = await fetchCustomActivities({ signal: controller.signal })
    activities.value = result.items
  } catch (err) {
    if ((err as { name?: string }).name !== 'AbortError') {
      errorMessage.value = extractApiErrorMessage(err, '活动读取失败')
      appStore.showError(errorMessage.value)
    }
  } finally {
    if (!controller.signal.aborted) {
      loading.value = false
    }
  }
}
</script>
