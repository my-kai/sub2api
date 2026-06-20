<template>
  <StatusBadge :status="badgeStatus" :label="statusLabel" />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import type { CustomActivityStatus, RedPacketRainRoundStatus } from '../types'

const props = defineProps<{
  status: CustomActivityStatus | RedPacketRainRoundStatus
}>()

/**
 * 状态文案只暴露业务含义，不泄露活动调度或后台规则。
 */
const statusLabel = computed(() => {
  const labels: Record<CustomActivityStatus | RedPacketRainRoundStatus, string> = {
    draft: '未开放',
    scheduled: '未开始',
    active: '进行中',
    ended: '已结束',
    offline: '已下架',
    waiting: '等待中',
    finished: '已结束',
  }
  return labels[props.status] ?? '未知状态'
})

/**
 * 复用项目通用状态点样式，避免 custom 页面另造一套视觉规则。
 */
const badgeStatus = computed(() => {
  switch (props.status) {
    case 'active':
      return 'success'
    case 'scheduled':
    case 'waiting':
      return 'warning'
    case 'offline':
      return 'danger'
    case 'draft':
    case 'ended':
    case 'finished':
      return 'disabled'
    default:
      return 'inactive'
  }
})
</script>
