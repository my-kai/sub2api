<template>
  <section class="rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900">
    <div class="flex items-center justify-between gap-3 border-b border-gray-100 px-5 py-4 dark:border-dark-700">
      <h2 class="text-base font-semibold text-gray-900 dark:text-white">活动详情</h2>
      <button
        v-if="activity"
        type="button"
        class="btn btn-secondary btn-sm"
        :disabled="loading"
        @click="emit('reload')"
      >
        <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
        <span>刷新</span>
      </button>
    </div>

    <div v-if="loading && !activity" class="grid gap-4 p-5 md:grid-cols-2">
      <div v-for="index in 4" :key="index" class="h-24 animate-pulse rounded-xl bg-gray-100 dark:bg-dark-800"></div>
    </div>

    <div v-else-if="!activity" class="p-5">
      <EmptyState title="未选择活动" description="选择活动后查看详情。" />
    </div>

    <div v-else class="space-y-5 p-5">
      <div class="grid gap-4 md:grid-cols-4">
        <div class="detail-card">
          <span class="detail-label">活动状态</span>
          <ActivityStatusBadge :status="activity.status" />
        </div>
        <div class="detail-card">
          <span class="detail-label">活动预算</span>
          <strong class="detail-value">${{ displayActivityMoney(detailBudget) }}</strong>
        </div>
        <div class="detail-card">
          <span class="detail-label">已发放</span>
          <strong class="detail-value">${{ displayActivityMoney(activity.issued_amount) }}</strong>
        </div>
        <div class="detail-card">
          <span class="detail-label">参与人数</span>
          <strong class="detail-value">{{ activity.participant_count }}</strong>
        </div>
      </div>

      <div class="grid gap-4 md:grid-cols-2">
        <dl class="info-list">
          <div>
            <dt>活动标题</dt>
            <dd>{{ activity.title }}</dd>
          </div>
          <div>
            <dt>开始时间</dt>
            <dd>{{ formatActivityDateTime(activity.starts_at) }}</dd>
          </div>
          <div>
            <dt>结束时间</dt>
            <dd>{{ formatActivityDateTime(activity.ends_at) }}</dd>
          </div>
          <div>
            <dt>活动描述</dt>
            <dd>{{ activity.description || '-' }}</dd>
          </div>
        </dl>

        <dl class="info-list">
          <div>
            <dt>轮数</dt>
            <dd>{{ activity.red_packet_rain?.round_count || '-' }}</dd>
          </div>
          <div>
            <dt>单轮时长</dt>
            <dd>{{ activity.red_packet_rain?.round_duration_seconds || '-' }} 秒</dd>
          </div>
          <div>
            <dt>轮次间隔</dt>
            <dd>{{ activity.red_packet_rain?.round_interval_seconds ?? '-' }} 秒</dd>
          </div>
          <div>
            <dt>单用户上限</dt>
            <dd>${{ displayActivityMoney(activity.red_packet_rain?.per_user_total_cap) }}</dd>
          </div>
        </dl>
      </div>

      <AdminActivityRoundsTable :rounds="rounds" />
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import ActivityStatusBadge from './ActivityStatusBadge.vue'
import AdminActivityRoundsTable from './AdminActivityRoundsTable.vue'
import { displayActivityMoney, formatActivityDateTime } from './adminActivityViewHelpers'
import type { AdminCustomActivityDetail, AdminRedPacketRainRoundSummary } from '../types'

const props = defineProps<{
  activity: AdminCustomActivityDetail | null
  loading: boolean
}>()

const emit = defineEmits<{
  reload: []
}>()

const detailBudget = computed(() => props.activity?.red_packet_rain?.total_budget || props.activity?.total_budget)
const rounds = computed<AdminRedPacketRainRoundSummary[]>(() => props.activity?.rounds || [])
</script>

<style scoped>
.detail-card {
  @apply rounded-2xl border border-gray-200 p-4 dark:border-dark-700;
}

.detail-label {
  @apply mb-2 block text-sm text-gray-500 dark:text-gray-400;
}

.detail-value {
  @apply text-lg font-semibold text-gray-950 dark:text-white;
}

.info-list {
  @apply space-y-3 rounded-2xl border border-gray-200 p-4 text-sm dark:border-dark-700;
}

.info-list div {
  @apply flex items-center justify-between gap-4;
}

.info-list dt {
  @apply text-gray-500 dark:text-gray-400;
}

.info-list dd {
  @apply text-right font-medium text-gray-900 dark:text-white;
}
</style>
