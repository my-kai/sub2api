<template>
  <aside class="space-y-5">
    <section class="rounded-3xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900">
      <h2 class="text-base font-semibold text-gray-950 dark:text-white">活动信息</h2>
      <dl class="mt-4 space-y-3 text-sm">
        <div class="flex items-center justify-between gap-4">
          <dt class="text-gray-500 dark:text-gray-400">开始时间</dt>
          <dd class="font-medium text-gray-900 dark:text-white">{{ formatDateTime(activity.starts_at) }}</dd>
        </div>
        <div class="flex items-center justify-between gap-4">
          <dt class="text-gray-500 dark:text-gray-400">结束时间</dt>
          <dd class="font-medium text-gray-900 dark:text-white">{{ formatDateTime(activity.ends_at) }}</dd>
        </div>
        <div class="flex items-center justify-between gap-4">
          <dt class="text-gray-500 dark:text-gray-400">轮数</dt>
          <dd class="font-medium text-gray-900 dark:text-white">{{ activity.red_packet_rain?.round_count || '-' }}</dd>
        </div>
        <div class="flex items-center justify-between gap-4">
          <dt class="text-gray-500 dark:text-gray-400">单轮时长</dt>
          <dd class="font-medium text-gray-900 dark:text-white">{{ activity.red_packet_rain?.round_duration_seconds || '-' }}s</dd>
        </div>
      </dl>
    </section>

    <section class="rounded-3xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900">
      <h2 class="text-base font-semibold text-gray-950 dark:text-white">领取进度</h2>
      <dl class="mt-4 space-y-3 text-sm">
        <div class="flex items-center justify-between gap-4">
          <dt class="text-gray-500 dark:text-gray-400">本轮已得</dt>
          <dd class="font-medium text-gray-900 dark:text-white">${{ displayMoney(state?.user_reward.round_total) }}</dd>
        </div>
        <div class="flex items-center justify-between gap-4">
          <dt class="text-gray-500 dark:text-gray-400">本轮剩余</dt>
          <dd class="font-medium text-gray-900 dark:text-white">${{ displayMoney(state?.user_reward.round_remaining) }}</dd>
        </div>
        <div class="flex items-center justify-between gap-4">
          <dt class="text-gray-500 dark:text-gray-400">累计剩余</dt>
          <dd class="font-medium text-gray-900 dark:text-white">${{ displayMoney(state?.user_reward.activity_remaining) }}</dd>
        </div>
        <div class="flex items-center justify-between gap-4">
          <dt class="text-gray-500 dark:text-gray-400">活动剩余</dt>
          <dd class="font-medium text-gray-900 dark:text-white">${{ displayMoney(state?.budget.remaining) }}</dd>
        </div>
      </dl>
    </section>

    <RedPacketRainClaimResult v-if="claimResult" :result="claimResult" />
  </aside>
</template>

<script setup lang="ts">
import type { CustomActivityDetail, RedPacketRainClaimResponse, RedPacketRainState } from '../types'
import RedPacketRainClaimResult from './RedPacketRainClaimResult.vue'

defineProps<{
  activity: CustomActivityDetail
  state: RedPacketRainState | null
  claimResult: RedPacketRainClaimResponse | null
}>()

/**
 * 金额只按后端字符串展示。
 */
function displayMoney(value: string | undefined): string {
  return value?.trim() || '0.00000000'
}

/**
 * 按本地时区展示活动时间。
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
