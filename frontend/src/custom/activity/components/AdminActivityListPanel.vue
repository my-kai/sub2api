<template>
  <section class="rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900">
    <div class="border-b border-gray-100 px-5 py-4 dark:border-dark-700">
      <h2 class="text-base font-semibold text-gray-900 dark:text-white">活动列表</h2>
    </div>

    <DataTable :columns="activityColumns" :data="activities" :loading="loading">
      <template #cell-title="{ value, row }">
        <button type="button" class="text-left text-sm font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400" @click="emit('select', row)">
          {{ value }}
        </button>
      </template>

      <template #cell-status="{ value }">
        <ActivityStatusBadge :status="value" />
      </template>

      <template #cell-starts_at="{ row }">
        <span class="text-sm text-gray-600 dark:text-gray-300">
          {{ formatActivityDateTime(row.starts_at) }} - {{ formatActivityDateTime(row.ends_at) }}
        </span>
      </template>

      <template #cell-issued_amount="{ row }">
        <div class="text-sm text-gray-700 dark:text-gray-300">
          <span class="font-medium text-gray-900 dark:text-white">${{ displayActivityMoney(row.issued_amount) }}</span>
          <span class="mx-1 text-gray-400">/</span>
          <span>${{ displayActivityMoney(row.total_budget) }}</span>
        </div>
      </template>

      <template #cell-participant_count="{ value }">
        <span class="text-sm text-gray-700 dark:text-gray-300">{{ value }}</span>
      </template>

      <template #cell-actions="{ row }">
        <div class="flex flex-wrap items-center gap-2">
          <button type="button" class="btn btn-secondary btn-sm" @click="emit('select', row)">查看</button>
          <button v-if="canEditActivity(row.status)" type="button" class="btn btn-secondary btn-sm" @click="emit('edit', row)">
            编辑
          </button>
          <button
            v-if="canEndActivity(row.status)"
            type="button"
            class="btn btn-secondary btn-sm"
            :disabled="actionLoading"
            @click="emit('action', 'end', row)"
          >
            结束
          </button>
          <button
            v-if="canOfflineActivity(row.status)"
            type="button"
            class="btn btn-secondary btn-sm text-red-600 hover:text-red-700 dark:text-red-400"
            :disabled="actionLoading"
            @click="emit('action', 'offline', row)"
          >
            下架
          </button>
        </div>
      </template>

      <template #empty>
        <EmptyState title="暂无活动" description="新建后会显示在这里。" />
      </template>
    </DataTable>
  </section>
</template>

<script setup lang="ts">
import DataTable from '@/components/common/DataTable.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import type { Column } from '@/components/common/types'
import ActivityStatusBadge from './ActivityStatusBadge.vue'
import { displayActivityMoney, formatActivityDateTime } from './adminActivityViewHelpers'
import type { AdminCustomActivityListItem, CustomActivityStatus } from '../types'

defineProps<{
  activities: AdminCustomActivityListItem[]
  loading: boolean
  actionLoading: boolean
}>()

const emit = defineEmits<{
  select: [activity: AdminCustomActivityListItem]
  edit: [activity: AdminCustomActivityListItem]
  action: [kind: 'end' | 'offline', activity: AdminCustomActivityListItem]
}>()

const activityColumns: Column[] = [
  { key: 'id', label: 'ID' },
  { key: 'title', label: '活动' },
  { key: 'status', label: '状态' },
  { key: 'starts_at', label: '时间' },
  { key: 'issued_amount', label: '预算消耗' },
  { key: 'participant_count', label: '参与人数' },
  { key: 'actions', label: '操作' },
]

/**
 * 判断活动是否允许编辑资金和时间规则。
 */
function canEditActivity(status: CustomActivityStatus): boolean {
  return status === 'draft' || status === 'scheduled'
}

/**
 * 判断活动是否允许提前结束。
 */
function canEndActivity(status: CustomActivityStatus): boolean {
  return status === 'active'
}

/**
 * 判断活动是否允许下架。
 */
function canOfflineActivity(status: CustomActivityStatus): boolean {
  return status !== 'offline'
}
</script>
