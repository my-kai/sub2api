<template>
  <AppLayout>
    <div class="space-y-5">
      <header class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <RouterLink to="/custom/activities" class="mb-2 inline-flex items-center gap-1 text-sm text-gray-500 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white">
            <Icon name="chevronLeft" size="sm" />
            返回活动中心
          </RouterLink>
          <div class="flex items-center gap-2">
            <Icon name="gift" size="lg" class="text-red-500" />
            <h1 class="text-lg font-semibold text-gray-900 dark:text-white">{{ activity?.title || '红包雨' }}</h1>
          </div>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ activity?.description || '限时活动' }}</p>
        </div>
        <button type="button" class="btn btn-secondary" :disabled="loading || stateLoading" @click="reloadPage">
          <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading || stateLoading }" />
          <span>刷新</span>
        </button>
      </header>

      <div v-if="loading && !activity" class="grid gap-5 lg:grid-cols-[minmax(0,1fr)_320px]">
        <div class="h-[560px] animate-pulse rounded-3xl bg-gray-100 dark:bg-dark-800"></div>
        <div class="h-80 animate-pulse rounded-3xl bg-gray-100 dark:bg-dark-800"></div>
      </div>

      <section v-else-if="errorMessage" class="rounded-3xl border border-dashed border-red-200 bg-white p-12 text-center dark:border-red-900/50 dark:bg-dark-900">
        <Icon name="exclamationCircle" size="xl" class="mx-auto text-red-500" />
        <h2 class="mt-3 text-base font-semibold text-gray-900 dark:text-white">活动读取失败</h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ errorMessage }}</p>
      </section>

      <template v-else-if="activity">
        <RedPacketRainSummaryCards
          :activity="activity"
          :round-text="roundText"
          :countdown-text="countdownText"
          :activity-reward="rainState?.user_reward.activity_total"
        />

        <div class="grid gap-5 xl:grid-cols-[minmax(0,1fr)_340px]">
          <RedPacketRainStage
            :round="rainState?.round || null"
            :seconds-until-end="secondsUntilEnd"
            :disabled="stageDisabled"
            :disabled-reason="stageDisabledReason"
            :submitting="claiming"
            :reset-key="stageResetKey"
            @hit-change="pendingHitCount = $event"
            @hit-packet="recordPacketHit"
            @reset-trace="resetTraceRecorder"
            @settle="handleClaim"
          />

          <RedPacketRainInfoPanel
            :activity="activity"
            :state="rainState"
            :claim-result="claimResult"
          />
        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { createRedPacketRainWS, fetchCustomActivityDetail, fetchRedPacketRainState, issueRedPacketRainWSTicket } from '../api'
import RedPacketRainInfoPanel from '../components/RedPacketRainInfoPanel.vue'
import RedPacketRainStage from '../components/RedPacketRainStage.vue'
import RedPacketRainSummaryCards from '../components/RedPacketRainSummaryCards.vue'
import { buildEncryptedRedPacketRainClaim, createRedPacketRainIdempotencyKey, createRedPacketRainNonce } from '../security/redPacketRainCrypto'
import { buildRedPacketRainFingerprint } from '../security/redPacketRainFingerprint'
import { RedPacketRainTraceRecorder } from '../security/redPacketRainTrace'
import type {
  CustomActivityDetail,
  RedPacketRainClaimResponse,
  RedPacketRainState,
  RedPacketRainWSChallengeMessage,
  RedPacketRainWSClaimResultMessage,
  RedPacketRainWSErrorMessage,
  RedPacketRainWSMessage,
} from '../types'

const route = useRoute()
const appStore = useAppStore()
const activity = ref<CustomActivityDetail | null>(null)
const rainState = ref<RedPacketRainState | null>(null)
const claimResult = ref<RedPacketRainClaimResponse | null>(null)
const loading = ref(false)
const stateLoading = ref(false)
const claiming = ref(false)
const errorMessage = ref('')
const stateLoadedAt = ref(Date.now())
const now = ref(Date.now())
const pendingHitCount = ref(0)
const stageResetKey = ref(0)
const wsReady = ref(false)
const autoSettledRoundIDs = new Set<number>()
const traceRecorder = new RedPacketRainTraceRecorder()
let pageController: AbortController | null = null
let stateController: AbortController | null = null
let clockTimer: number | undefined
let pollTimer: number | undefined
let redPacketWS: WebSocket | null = null
let redPacketWSOpening: Promise<void> | null = null
let wsConnectVersion = 0
let wsRoundID = 0
let wsTicket = ''
let clientNonce = ''
let deviceFingerprint = ''
let wsChallenge: RedPacketRainWSChallengeMessage | null = null

const activityID = computed(() => Number(route.params.id))
const currentRound = computed(() => rainState.value?.round || null)
const secondsUntilStart = computed(() => countdownFromServer(currentRound.value?.seconds_until_start || 0))
const secondsUntilEnd = computed(() => countdownFromServer(currentRound.value?.seconds_until_end || 0))
const roundText = computed(() => currentRound.value ? `第 ${currentRound.value.round_no} 轮` : '暂无轮次')
const countdownText = computed(() => {
  if (currentRound.value?.status === 'active') {
    return formatCountdown(secondsUntilEnd.value)
  }
  if (currentRound.value?.status === 'waiting') {
    return formatCountdown(secondsUntilStart.value)
  }
  return '已结束'
})
const stageDisabled = computed(() => Boolean(stageDisabledReason.value))
const stageDisabledReason = computed(() => {
  if (!rainState.value) return '活动读取中'
  if (rainState.value.budget.exhausted) return '活动奖励已发完'
  if (rainState.value.user_reward.activity_cap_reached) return '活动领取已达上限'
  if (rainState.value.user_reward.round_cap_reached) return '本轮领取已达上限'
  if (currentRound.value?.status === 'waiting') return '等待下一轮'
  if (currentRound.value?.status === 'active' && secondsUntilEnd.value > 0) return wsReady.value ? '' : '请刷新后重试'
  if (rainState.value.status === 'offline') return '活动已下架'
  return '本轮已结束'
})

watch(
  activityID,
  () => {
    void reloadPage()
  },
  { immediate: true },
)

watch(
  [() => currentRound.value?.id, secondsUntilEnd],
  ([roundID, seconds]) => {
    if (!roundID || seconds > 0 || autoSettledRoundIDs.has(roundID)) {
      return
    }
    autoSettledRoundIDs.add(roundID)
    void handleClaim(pendingHitCount.value)
  },
)

watch(
  () => currentRound.value?.id,
  () => {
    closeRedPacketWS()
    resetTraceRecorder()
    if (currentRound.value?.status === 'active') {
      ensureRedPacketWSSafely()
    }
  },
)

watch(
  () => currentRound.value?.status,
  (status) => {
    if (status === 'active') {
      ensureRedPacketWSSafely()
    } else {
      closeRedPacketWS()
    }
  },
)

onMounted(() => {
  clockTimer = window.setInterval(() => {
    now.value = Date.now()
  }, 1000)
  pollTimer = window.setInterval(() => {
    void reloadState()
  }, 15000)
})

onBeforeUnmount(() => {
  pageController?.abort()
  stateController?.abort()
  closeRedPacketWS()
  if (clockTimer !== undefined) window.clearInterval(clockTimer)
  if (pollTimer !== undefined) window.clearInterval(pollTimer)
})

/**
 * 重新读取活动详情和红包雨状态。
 */
async function reloadPage(): Promise<void> {
  if (!Number.isInteger(activityID.value) || activityID.value <= 0) {
    errorMessage.value = '活动不存在'
    return
  }
  pageController?.abort()
  pageController = new AbortController()
  loading.value = true
  errorMessage.value = ''
  try {
    activity.value = await fetchCustomActivityDetail(activityID.value, { signal: pageController.signal })
    await reloadState()
  } catch (err) {
    if ((err as { name?: string }).name !== 'AbortError') {
      errorMessage.value = extractApiErrorMessage(err, '活动读取失败')
      appStore.showError(errorMessage.value)
    }
  } finally {
    if (!pageController.signal.aborted) {
      loading.value = false
    }
  }
}

/**
 * 只刷新红包雨状态，用于倒计时和领取结果后的状态同步。
 */
async function reloadState(): Promise<void> {
  if (!Number.isInteger(activityID.value) || activityID.value <= 0 || !activity.value) {
    return
  }
  stateController?.abort()
  stateController = new AbortController()
  stateLoading.value = true
  try {
    rainState.value = await fetchRedPacketRainState(activityID.value, { signal: stateController.signal })
    stateLoadedAt.value = Date.now()
    now.value = Date.now()
    if (rainState.value.round?.status === 'active') {
      ensureRedPacketWSSafely()
    }
  } catch (err) {
    if ((err as { name?: string }).name !== 'AbortError') {
      appStore.showWarning(extractApiErrorMessage(err, '活动状态读取失败'))
    }
  } finally {
    if (!stateController.signal.aborted) {
      stateLoading.value = false
    }
  }
}

/**
 * 提交当前命中数并展示后端结算结果。
 */
async function handleClaim(hitCount: number): Promise<void> {
  const roundID = currentRound.value?.id
  if (!roundID || claiming.value) {
    return
  }
  claiming.value = true
  try {
    await ensureRedPacketWS()
    if (!redPacketWS || redPacketWS.readyState !== WebSocket.OPEN || !wsChallenge) {
      appStore.showError('请刷新后重试')
      return
    }
    if (wsChallenge.round_id !== roundID) {
      closeRedPacketWS()
      appStore.showError('请刷新后重试')
      return
    }
    const traceDigest = await traceRecorder.digest()
    const idempotencyKey = createRedPacketRainIdempotencyKey(activityID.value, roundID)
    const message = await buildEncryptedRedPacketRainClaim({
      activityID: activityID.value,
      roundID,
      ticket: wsTicket,
      challenge: wsChallenge,
      idempotencyKey,
      payload: {
        hit_count: Math.max(0, Math.trunc(hitCount)),
        started_at: traceRecorder.startedAtISO(),
        ended_at: traceRecorder.endedAtISO(),
        click_trace_digest: traceDigest,
        device_fingerprint: deviceFingerprint,
        client_nonce: clientNonce,
      },
    })
    redPacketWS.send(JSON.stringify(message))
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, '领取失败'))
  } finally {
    claiming.value = false
  }
}

/**
 * 后台预连接失败不阻断页面刷新流程，只提示用户刷新重试。
 */
function ensureRedPacketWSSafely(): void {
  void ensureRedPacketWS().catch((err) => {
    if ((err as { name?: string }).name !== 'AbortError') {
      appStore.showWarning(extractApiErrorMessage(err, '请刷新后重试'))
    }
  })
}

/**
 * 确保当前轮次已经建立可领取 WebSocket。
 */
async function ensureRedPacketWS(): Promise<void> {
  const roundID = currentRound.value?.id
  if (!roundID || currentRound.value?.status !== 'active') {
    return
  }
  if (redPacketWS && wsRoundID === roundID && (redPacketWS.readyState === WebSocket.OPEN || redPacketWS.readyState === WebSocket.CONNECTING)) {
    return
  }
  if (redPacketWSOpening && wsRoundID === roundID) {
    await redPacketWSOpening
    return
  }
  closeRedPacketWS()
  wsRoundID = roundID
  const version = wsConnectVersion
  const opening = openRedPacketWS(roundID, version)
  redPacketWSOpening = opening
  try {
    await opening
  } finally {
    if (redPacketWSOpening === opening) {
      redPacketWSOpening = null
    }
  }
}

/**
 * 为指定轮次建立一个新的 WebSocket 会话。
 *
 * 刷新页面时多个 watcher 可能同时触发连接，这里用 version 防止旧异步流程回写当前连接。
 */
async function openRedPacketWS(roundID: number, version: number): Promise<void> {
  deviceFingerprint = await buildRedPacketRainFingerprint()
  clientNonce = createRedPacketRainNonce(18)
  if (version !== wsConnectVersion || currentRound.value?.id !== roundID || currentRound.value?.status !== 'active') {
    return
  }
  const ticketResult = await issueRedPacketRainWSTicket(activityID.value, {
    round_id: roundID,
    device_fingerprint: deviceFingerprint,
    client_nonce: clientNonce,
  })
  if (version !== wsConnectVersion || currentRound.value?.id !== roundID || currentRound.value?.status !== 'active') {
    return
  }
  wsTicket = ticketResult.ticket
  const socket = createRedPacketRainWS(activityID.value, wsTicket)
  redPacketWS = socket
  socket.onopen = () => {
    wsReady.value = false
  }
  socket.onmessage = (event) => {
    if (redPacketWS === socket) {
      handleWSMessage(event.data)
    }
  }
  socket.onerror = () => {
    if (redPacketWS === socket) {
      appStore.showWarning('请刷新后重试')
    }
  }
  socket.onclose = () => {
    if (redPacketWS === socket) {
      redPacketWS = null
      wsRoundID = 0
      wsReady.value = false
      wsChallenge = null
    }
  }
}

/**
 * 处理红包雨 WebSocket 消息。
 */
function handleWSMessage(raw: string): void {
  try {
    const message = JSON.parse(raw) as RedPacketRainWSMessage
    if (message.type === 'challenge') {
      if (message.round_id !== currentRound.value?.id || message.round_id !== wsRoundID) {
        closeRedPacketWS()
        appStore.showError('请刷新后重试')
        return
      }
      wsChallenge = message
      wsReady.value = true
      return
    }
    if (message.type === 'claim_result') {
      applyClaimResult((message as RedPacketRainWSClaimResultMessage).data)
      return
    }
    if (message.type === 'error') {
      appStore.showError((message as RedPacketRainWSErrorMessage).message || '领取失败')
    }
  } catch {
    appStore.showError('领取失败')
  }
}

/**
 * 应用服务端领取结果并刷新活动状态。
 */
function applyClaimResult(result: RedPacketRainClaimResponse): void {
  claimResult.value = result
  if (rainState.value && result.activity_id > 0) {
    rainState.value = {
      activity_id: result.activity_id,
      status: rainState.value.status,
      round: rainState.value.round,
      user_reward: result.user_reward,
      budget: result.budget,
    }
  }
  stageResetKey.value += 1
  appStore.showSuccess(result.message || '领取完成')
  void reloadState()
}

/**
 * 记录舞台命中事件，只保存摘要所需数据。
 */
function recordPacketHit(packetID: number, event: MouseEvent): void {
  traceRecorder.recordHit(packetID, event)
}

/**
 * 清理当前轮次轨迹摘要材料。
 */
function resetTraceRecorder(): void {
  traceRecorder.reset()
  pendingHitCount.value = 0
}

/**
 * 关闭当前 WebSocket 并清空会话材料。
 */
function closeRedPacketWS(): void {
  wsConnectVersion += 1
  redPacketWSOpening = null
  if (redPacketWS) {
    redPacketWS.close()
  }
  redPacketWS = null
  wsRoundID = 0
  wsReady.value = false
  wsTicket = ''
  wsChallenge = null
}

/**
 * 基于最近一次服务端秒数做本地倒计时。
 */
function countdownFromServer(serverSeconds: number): number {
  const elapsed = Math.floor((now.value - stateLoadedAt.value) / 1000)
  return Math.max(0, serverSeconds - elapsed)
}

/**
 * 秒数格式化为短倒计时。
 */
function formatCountdown(seconds: number): string {
  if (seconds <= 0) return '0s'
  const minutes = Math.floor(seconds / 60)
  const remainSeconds = seconds % 60
  return minutes > 0 ? `${minutes}m ${remainSeconds}s` : `${remainSeconds}s`
}

</script>
