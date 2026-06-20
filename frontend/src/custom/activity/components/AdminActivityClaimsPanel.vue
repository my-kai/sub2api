<template>
  <section class="rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900">
    <div class="flex items-center justify-between gap-3 border-b border-gray-100 px-5 py-4 dark:border-dark-700">
      <h2 class="text-base font-semibold text-gray-900 dark:text-white">领取记录</h2>
      <button type="button" class="btn btn-secondary btn-sm" :disabled="loading || !activity" @click="emit('reload')">
        <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
        <span>刷新</span>
      </button>
    </div>

    <div v-if="!activity" class="p-5">
      <EmptyState title="暂无记录" description="选择活动后查看记录。" />
    </div>

    <div v-else class="space-y-4 p-5">
      <DataTable :columns="claimColumns" :data="claims" :loading="loading">
        <template #cell-round_no="{ value }">
          <span>第 {{ value }} 轮</span>
        </template>
        <template #cell-reward_amount="{ value }">
          <span>${{ displayActivityMoney(value) }}</span>
        </template>
        <template #cell-created_at="{ value }">
          <span>{{ formatActivityDateTime(value) }}</span>
        </template>
        <template #empty>
          <EmptyState title="暂无记录" description="有领取后会显示在这里。" />
        </template>
      </DataTable>

      <Pagination
        v-if="total > 0"
        :page="page"
        :page-size="pageSize"
        :total="total"
        @update:page="emit('update:page', $event)"
        @update:pageSize="emit('update:pageSize', $event)"
      />
    </div>
  </section>
</template>

<script setup lang="ts">
import Icon from '@/components/icons/Icon.vue'
import DataTable from '@/components/common/DataTable.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Pagination from '@/components/common/Pagination.vue'
import type { Column } from '@/components/common/types'
import { displayActivityMoney, formatActivityDateTime } from './adminActivityViewHelpers'
import type { AdminCustomActivityClaimItem, AdminCustomActivityDetail } from '../types'

defineProps<{
  activity: AdminCustomActivityDetail | null
  claims: AdminCustomActivityClaimItem[]
  loading: boolean
  page: number
  pageSize: number
  total: number
}>()

const emit = defineEmits<{
  reload: []
  'update:page': [page: number]
  'update:pageSize': [pageSize: number]
}>()

const claimColumns: Column[] = [
  { key: 'id', label: 'ID' },
  { key: 'round_no', label: '轮次' },
  { key: 'user_id', label: '用户 ID' },
  { key: 'hit_count', label: '命中数' },
  { key: 'reward_amount', label: '奖励' },
  { key: 'created_at', label: '时间' },
]
</script>
