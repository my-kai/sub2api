<template>
  <RouterLink
    :to="activityRoute"
    class="group block overflow-hidden rounded-3xl border border-gray-200 bg-white shadow-sm transition hover:-translate-y-0.5 hover:shadow-md dark:border-dark-700 dark:bg-dark-900"
  >
    <div class="relative h-36 overflow-hidden bg-gradient-to-br from-red-500 via-orange-400 to-yellow-300">
      <img
        v-if="activity.cover_url"
        :src="activity.cover_url"
        :alt="activity.title"
        class="h-full w-full object-cover transition duration-300 group-hover:scale-105"
      />
      <div v-else class="flex h-full items-center justify-center text-white">
        <Icon name="gift" size="xl" />
      </div>
      <div class="absolute left-4 top-4 rounded-full bg-white/90 px-3 py-1 text-sm font-medium text-red-600 shadow-sm">
        红包雨
      </div>
    </div>

    <div class="space-y-4 p-5">
      <div class="flex items-start justify-between gap-3">
        <div class="min-w-0">
          <h2 class="truncate text-base font-semibold text-gray-950 dark:text-white">{{ activity.title }}</h2>
          <p class="mt-1 line-clamp-2 text-sm text-gray-500 dark:text-gray-400">{{ descriptionText }}</p>
        </div>
        <ActivityStatusBadge :status="activity.status" />
      </div>

      <dl class="grid gap-3 text-sm text-gray-600 dark:text-gray-300 sm:grid-cols-2">
        <div>
          <dt class="text-gray-400 dark:text-gray-500">开始时间</dt>
          <dd class="mt-1 font-medium">{{ formatDateTime(activity.starts_at) }}</dd>
        </div>
        <div>
          <dt class="text-gray-400 dark:text-gray-500">结束时间</dt>
          <dd class="mt-1 font-medium">{{ formatDateTime(activity.ends_at) }}</dd>
        </div>
      </dl>

      <div class="flex items-center justify-between rounded-2xl bg-gray-50 px-4 py-3 dark:bg-dark-800">
        <span class="text-sm text-gray-500 dark:text-gray-400">我的奖励</span>
        <span class="text-base font-semibold text-gray-950 dark:text-white">
          ${{ displayMoney(activity.summary?.user_total_reward) }}
        </span>
      </div>
    </div>
  </RouterLink>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import { activityDetailRouteFor } from '../routes'
import type { CustomActivityListItem } from '../types'
import ActivityStatusBadge from './ActivityStatusBadge.vue'

const props = defineProps<{
  activity: CustomActivityListItem
}>()

const activityRoute = computed(() => activityDetailRouteFor(props.activity))
const descriptionText = computed(() => props.activity.description?.trim() || '限时活动')

/**
 * 金额只做字符串展示，不进行前端计算或四舍五入。
 */
function displayMoney(value: string | undefined): string {
  return value?.trim() || '0.00000000'
}

/**
 * 按本地时区展示活动时间，解析失败时显示占位。
 */
function formatDateTime(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return '-'
  }
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date)
}
</script>
