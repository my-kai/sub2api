<template>
  <section class="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
    <div class="info-card">
      <span class="info-label">活动状态</span>
      <ActivityStatusBadge :status="activity.status" />
    </div>
    <div class="info-card">
      <span class="info-label">当前轮次</span>
      <strong class="info-value">{{ roundText }}</strong>
    </div>
    <div class="info-card">
      <span class="info-label">本轮倒计时</span>
      <strong class="info-value">{{ countdownText }}</strong>
    </div>
    <div class="info-card">
      <span class="info-label">我的累计奖励</span>
      <strong class="info-value">${{ displayMoney(activityReward) }}</strong>
    </div>
  </section>
</template>

<script setup lang="ts">
import ActivityStatusBadge from './ActivityStatusBadge.vue'
import type { CustomActivityDetail } from '../types'

defineProps<{
  activity: CustomActivityDetail
  roundText: string
  countdownText: string
  activityReward?: string
}>()

/**
 * 金额只按后端字符串展示。
 */
function displayMoney(value: string | undefined): string {
  return value?.trim() || '0.00000000'
}
</script>

<style scoped>
.info-card {
  @apply rounded-3xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900;
}

.info-label {
  @apply mb-2 block text-sm text-gray-500 dark:text-gray-400;
}

.info-value {
  @apply text-lg font-semibold text-gray-950 dark:text-white;
}
</style>
