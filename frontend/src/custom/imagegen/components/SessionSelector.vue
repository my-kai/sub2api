<template>
  <section class="flex min-h-0 flex-col overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900">
    <div class="flex items-center justify-between border-b border-gray-100 px-4 py-3 dark:border-dark-700">
      <h2 class="text-sm font-semibold text-gray-900 dark:text-gray-100">会话</h2>
      <button
        type="button"
        class="rounded-lg bg-primary-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-primary-700 disabled:cursor-not-allowed disabled:opacity-60"
        :disabled="creating"
        @click="$emit('create')"
      >
        {{ creating ? '创建中' : '新建' }}
      </button>
    </div>

    <div v-if="loading" class="space-y-2 p-3" aria-label="会话加载中">
      <div v-for="index in 4" :key="index" class="h-14 animate-pulse rounded-xl bg-gray-100 dark:bg-dark-800"></div>
    </div>

    <div v-else-if="sessions.length === 0" class="px-4 py-8 text-center text-sm text-gray-500 dark:text-gray-400">
      暂无会话
    </div>

    <ul v-else class="min-h-0 flex-1 space-y-1 overflow-y-auto p-2">
      <li
        v-for="session in sessions"
        :key="session.id"
        :class="[
          'group flex items-center gap-2 rounded-xl transition-colors',
          session.id === selectedSessionId
            ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300'
            : 'text-gray-700 hover:bg-gray-50 dark:text-gray-300 dark:hover:bg-dark-800'
        ]"
      >
        <button
          type="button"
          class="min-w-0 flex-1 px-3 py-2 text-left"
          :aria-pressed="session.id === selectedSessionId"
          @click="$emit('select', session.id)"
        >
          <span class="block truncate text-sm font-medium">{{ session.title || '未命名会话' }}</span>
          <span class="block truncate text-xs text-gray-500 dark:text-gray-400">{{ formatSessionTime(session.updated_at) }}</span>
        </button>
        <button
          type="button"
          class="mr-2 rounded-md px-2 py-1 text-xs text-gray-400 opacity-0 transition hover:bg-red-50 hover:text-red-600 group-hover:opacity-100 disabled:opacity-60 dark:hover:bg-red-900/20 dark:hover:text-red-300"
          :disabled="deletingSessionId === session.id"
          @click.stop="$emit('delete', session.id)"
        >
          {{ deletingSessionId === session.id ? '删除中' : '删除' }}
        </button>
      </li>
    </ul>
  </section>
</template>

<script setup lang="ts">
import type { ImageSession } from '../types'

/**
 * SessionSelector 渲染会话列表和基础操作。
 *
 * 删除确认由页面层决定，组件只发出用户意图，避免在基础组件里绑定交互策略。
 */
withDefaults(defineProps<{
  sessions: ImageSession[]
  selectedSessionId?: number
  loading?: boolean
  creating?: boolean
  deletingSessionId?: number
}>(), {
  loading: false,
  creating: false,
})

defineEmits<{
  create: []
  select: [sessionId: number]
  delete: [sessionId: number]
}>()

/**
 * 格式化会话更新时间。
 */
function formatSessionTime(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return '时间未知'
  }
  return date.toLocaleString()
}
</script>
