<template>
  <section class="rounded-3xl border border-green-200 bg-green-50 p-5 text-sm dark:border-green-900/50 dark:bg-green-950/30">
    <h2 class="text-base font-semibold text-green-700 dark:text-green-300">{{ result.message || '领取完成' }}</h2>
    <p class="mt-2 text-green-700 dark:text-green-200">本次奖励：${{ displayMoney(result.reward_amount) }}</p>
    <p class="mt-1 text-green-700 dark:text-green-200">点中红包：{{ result.hit_count }} 个</p>
  </section>
</template>

<script setup lang="ts">
import type { RedPacketRainClaimResponse } from '../types'

defineProps<{
  result: RedPacketRainClaimResponse
}>()

/**
 * 金额只按后端字符串展示。
 */
function displayMoney(value: string | undefined): string {
  return value?.trim() || '0.00000000'
}
</script>
