<template>
  <div class="rounded-2xl border border-gray-200 dark:border-dark-700">
    <div class="border-b border-gray-100 px-4 py-3 dark:border-dark-700">
      <h3 class="text-sm font-semibold text-gray-900 dark:text-white">轮次摘要</h3>
    </div>
    <div v-if="rounds.length === 0" class="p-5 text-sm text-gray-500 dark:text-gray-400">暂无轮次</div>
    <div v-else class="overflow-x-auto">
      <table class="w-full min-w-[680px] text-left text-sm">
        <thead class="bg-gray-50 text-gray-500 dark:bg-dark-800 dark:text-gray-400">
          <tr>
            <th class="px-4 py-3 font-medium">轮次</th>
            <th class="px-4 py-3 font-medium">状态</th>
            <th class="px-4 py-3 font-medium">时间</th>
            <th class="px-4 py-3 font-medium">已发放</th>
            <th class="px-4 py-3 font-medium">领取数</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
          <tr v-for="round in rounds" :key="round.id || round.round_no">
            <td class="px-4 py-3 text-gray-900 dark:text-white">第 {{ round.round_no }} 轮</td>
            <td class="px-4 py-3"><ActivityStatusBadge :status="round.status" /></td>
            <td class="px-4 py-3 text-gray-600 dark:text-gray-300">
              {{ formatOptionalActivityDateTime(round.starts_at) }} - {{ formatOptionalActivityDateTime(round.ends_at) }}
            </td>
            <td class="px-4 py-3 text-gray-600 dark:text-gray-300">${{ displayActivityMoney(round.issued_amount) }}</td>
            <td class="px-4 py-3 text-gray-600 dark:text-gray-300">{{ round.claim_count ?? '-' }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup lang="ts">
import ActivityStatusBadge from './ActivityStatusBadge.vue'
import { displayActivityMoney, formatOptionalActivityDateTime } from './adminActivityViewHelpers'
import type { AdminRedPacketRainRoundSummary } from '../types'

defineProps<{
  rounds: AdminRedPacketRainRoundSummary[]
}>()
</script>
