<template>
  <section class="rounded-3xl border border-red-100 bg-gradient-to-br from-red-50 via-orange-50 to-yellow-50 p-5 shadow-sm dark:border-red-900/40 dark:from-red-950/40 dark:via-orange-950/30 dark:to-yellow-950/20">
    <div class="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <div>
        <h2 class="text-lg font-semibold text-gray-950 dark:text-white">红包雨</h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ stageHint }}</p>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <span class="rounded-full bg-white px-3 py-1 text-sm font-medium text-red-600 shadow-sm dark:bg-dark-800">
          已点中 {{ hitCount }} 个
        </span>
        <span class="rounded-full bg-white px-3 py-1 text-sm font-medium text-gray-700 shadow-sm dark:bg-dark-800 dark:text-gray-200">
          {{ countdownText }}
        </span>
      </div>
    </div>

    <div class="relative h-[420px] overflow-hidden rounded-3xl border border-red-100 bg-red-600 shadow-inner dark:border-red-900/50">
      <div class="absolute inset-0 bg-[radial-gradient(circle_at_top,_rgba(255,255,255,0.22),_transparent_42%)]"></div>

      <button
        v-for="packet in packets"
        :key="packet.id"
        type="button"
        class="red-packet"
        :class="{ 'red-packet-hit': packet.hit }"
        :style="packetStyle(packet)"
        :disabled="!canHitPacket || packet.hit"
        @click="hitPacket(packet.id, $event)"
      >
        <span>红包</span>
      </button>

      <div v-if="!isRoundActive || disabled" class="absolute inset-0 flex items-center justify-center bg-red-950/35 p-6 text-center backdrop-blur-sm">
        <div class="rounded-3xl bg-white/95 px-6 py-5 shadow-lg dark:bg-dark-900/95">
          <Icon name="gift" size="xl" class="mx-auto text-red-500" />
          <p class="mt-3 text-base font-semibold text-gray-950 dark:text-white">{{ overlayText }}</p>
        </div>
      </div>
    </div>

    <div class="mt-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <p class="text-sm text-gray-500 dark:text-gray-400">点中的红包越多，奖励机会越高。</p>
      <button type="button" class="btn btn-primary" :disabled="!canSettle" @click="emitSettle">
        <Icon v-if="submitting" name="refresh" size="sm" class="animate-spin" />
        <Icon v-else name="gift" size="sm" />
        <span>{{ submitting ? '结算中' : '结算本轮' }}</span>
      </button>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, watch, ref } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import type { RedPacketRainRound } from '../types'

interface StagePacket {
  id: number
  left: number
  size: number
  duration: number
  delay: number
  hit: boolean
}

const props = withDefaults(defineProps<{
  round: RedPacketRainRound | null
  secondsUntilEnd: number
  disabled?: boolean
  disabledReason?: string
  submitting?: boolean
  resetKey?: number
}>(), {
  disabled: false,
  disabledReason: '',
  submitting: false,
  resetKey: 0,
})

const emit = defineEmits<{
  settle: [hitCount: number]
  hitChange: [hitCount: number]
  hitPacket: [packetID: number, event: MouseEvent]
  resetTrace: []
}>()

const hitCount = ref(0)
const packets = ref<StagePacket[]>([])
let packetSequence = 0
let spawnTimer: number | undefined

const isRoundActive = computed(() => props.round?.status === 'active' && props.secondsUntilEnd > 0)
const canHitPacket = computed(() => isRoundActive.value && !props.disabled && !props.submitting)
const canSettle = computed(() => isRoundActive.value && !props.disabled && !props.submitting)
const countdownText = computed(() => (isRoundActive.value ? `${props.secondsUntilEnd}s` : '未开始'))
const stageHint = computed(() => (isRoundActive.value ? '本轮进行中' : '等待下一轮'))
const overlayText = computed(() => props.disabledReason || '当前不可领取')

watch(
  () => [props.round?.id, isRoundActive.value] as const,
  () => resetStageForRound(),
  { immediate: true },
)

watch(
  () => props.resetKey,
  () => resetHitsAfterClaim(),
)

onBeforeUnmount(() => {
  stopSpawn()
})

/**
 * 轮次切换时重置舞台。
 *
 * 舞台随机数只控制红包位置和动画，不参与奖励金额计算。
 */
function resetStageForRound(): void {
  hitCount.value = 0
  packets.value = []
  emit('hitChange', hitCount.value)
  emit('resetTrace')
  if (isRoundActive.value) {
    startSpawn()
  } else {
    stopSpawn()
  }
}

/**
 * 成功结算后只清空本次命中数，允许同一轮继续点击并再次结算。
 */
function resetHitsAfterClaim(): void {
  hitCount.value = 0
  packets.value = packets.value.filter((packet) => !packet.hit)
  emit('hitChange', hitCount.value)
  emit('resetTrace')
}

/**
 * 启动红包生成器，保持页面有可点击目标。
 */
function startSpawn(): void {
  stopSpawn()
  for (let index = 0; index < 14; index += 1) {
    addPacket()
  }
  spawnTimer = window.setInterval(addPacket, 700)
}

/**
 * 停止红包生成器，避免离开页面后仍保留计时器。
 */
function stopSpawn(): void {
  if (spawnTimer !== undefined) {
    window.clearInterval(spawnTimer)
    spawnTimer = undefined
  }
}

/**
 * 添加一个红包动画节点。
 */
function addPacket(): void {
  if (!isRoundActive.value) {
    return
  }
  const nextPacket: StagePacket = {
    id: packetSequence += 1,
    left: Math.round(Math.random() * 86) + 4,
    size: Math.round(Math.random() * 18) + 54,
    duration: Math.round(Math.random() * 2600) + 5200,
    delay: Math.round(Math.random() * 900),
    hit: false,
  }
  packets.value = [...packets.value.filter((packet) => !packet.hit).slice(-22), nextPacket]
}

/**
 * 记录一次用户点中红包。
 */
function hitPacket(packetID: number, event: MouseEvent): void {
  if (!canHitPacket.value) {
    return
  }
  packets.value = packets.value.map((packet) => (
    packet.id === packetID ? { ...packet, hit: true } : packet
  ))
  hitCount.value += 1
  emit('hitPacket', packetID, event)
  emit('hitChange', hitCount.value)
}

/**
 * 主动结算当前命中数，金额仍交给后端计算。
 */
function emitSettle(): void {
  if (!canSettle.value) {
    return
  }
  emit('settle', hitCount.value)
}

/**
 * 将红包动画参数转成 CSS 变量，避免模板里拼复杂样式。
 */
function packetStyle(packet: StagePacket): Record<string, string> {
  return {
    '--packet-left': `${packet.left}%`,
    '--packet-size': `${packet.size}px`,
    '--packet-duration': `${packet.duration}ms`,
    '--packet-delay': `${packet.delay}ms`,
  }
}
</script>

<style scoped>
.red-packet {
  @apply absolute top-[-80px] flex items-center justify-center rounded-2xl border border-yellow-200 bg-gradient-to-b from-red-500 to-red-700 text-xs font-bold text-yellow-100 shadow-lg transition;
  animation: packet-fall var(--packet-duration) linear var(--packet-delay) infinite;
  height: var(--packet-size);
  left: var(--packet-left);
  width: calc(var(--packet-size) * 0.76);
}

.red-packet::before {
  @apply absolute left-1/2 top-2 h-3 w-3 -translate-x-1/2 rounded-full bg-yellow-300 content-[''];
}

.red-packet-hit {
  @apply scale-75 opacity-0;
  animation-play-state: paused;
}

@keyframes packet-fall {
  from {
    transform: translateY(-100px) rotate(-8deg);
  }
  to {
    transform: translateY(560px) rotate(12deg);
  }
}
</style>
