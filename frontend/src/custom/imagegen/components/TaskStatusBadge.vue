<template>
  <span
    :class="[
      'inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium',
      toneClass
    ]"
  >
    <span :class="['h-1.5 w-1.5 rounded-full', dotClass]" aria-hidden="true"></span>
    <span>{{ statusLabel }}</span>
    <span v-if="queueText" class="text-[11px] opacity-80">{{ queueText }}</span>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { ImageTaskStatus } from '../types'

/**
 * TaskStatusBadge 负责把任务状态压成短业务文案。
 *
 * 只有还没被 worker claim 的 queued 状态展示排队位次；进入 running 后只展示生成中。
 */
const props = defineProps<{
  status: ImageTaskStatus
  queuePosition?: number
}>()

const statusLabelMap: Record<ImageTaskStatus, string> = {
  queued: '排队中',
  running: '生成中',
  completed: '已完成',
  failed: '失败',
  canceled: '已取消',
}

const statusLabel = computed(() => statusLabelMap[props.status])

const queueText = computed(() => {
  if (props.status !== 'queued' || !props.queuePosition || props.queuePosition < 1) {
    return ''
  }
  return `第 ${props.queuePosition} 位`
})

const toneClass = computed(() => {
  switch (props.status) {
    case 'queued':
      return 'bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300'
    case 'running':
      return 'bg-blue-50 text-blue-700 dark:bg-blue-900/20 dark:text-blue-300'
    case 'completed':
      return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300'
    case 'failed':
      return 'bg-red-50 text-red-700 dark:bg-red-900/20 dark:text-red-300'
    case 'canceled':
      return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
  }
})

const dotClass = computed(() => {
  switch (props.status) {
    case 'queued':
      return 'bg-amber-500'
    case 'running':
      return 'bg-blue-500'
    case 'completed':
      return 'bg-emerald-500'
    case 'failed':
      return 'bg-red-500'
    case 'canceled':
      return 'bg-gray-400'
  }
})
</script>
