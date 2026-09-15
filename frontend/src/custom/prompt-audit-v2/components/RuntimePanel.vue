<template>
  <section class="border-b border-gray-200 py-6 dark:border-dark-700">
    <div class="flex items-center justify-between gap-3">
      <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.promptAuditV2.runtime.title') }}</h2>
      <button type="button" class="btn btn-secondary btn-sm" :disabled="loading" :title="t('common.refresh')" @click="$emit('refresh')">
        <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
        <span>{{ t('common.refresh') }}</span>
      </button>
    </div>
    <div v-if="runtime" class="mt-4 grid grid-cols-2 gap-3 md:grid-cols-4 xl:grid-cols-6">
      <div v-for="item in items" :key="item.label" class="border border-gray-200 px-3 py-3 dark:border-dark-700">
        <p class="text-xs text-gray-500 dark:text-dark-400">{{ item.label }}</p>
        <p class="mt-1 text-sm font-semibold tabular-nums text-gray-900 dark:text-white">{{ item.value }}</p>
      </div>
    </div>
    <div v-if="runtime && Object.keys(runtime.endpoints).length" class="mt-4 border-t border-gray-100 pt-3 dark:border-dark-700">
      <p class="text-xs font-medium text-gray-600 dark:text-dark-300">{{ t('admin.promptAuditV2.runtime.endpointStatus') }}</p>
      <div class="mt-2 flex flex-wrap gap-2">
        <span v-for="(endpoint, id) in runtime.endpoints" :key="id" class="px-2 py-1 text-xs" :class="endpoint.ok ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300' : 'bg-red-50 text-red-700 dark:bg-red-950/40 dark:text-red-300'">
          {{ endpointName(id) }} · {{ endpoint.status }}<template v-if="endpoint.latency_ms"> · {{ endpoint.latency_ms }} ms</template><template v-if="endpoint.checked_at"> · {{ formatCheckedAt(endpoint.checked_at) }}</template>
        </span>
      </div>
    </div>
    <p v-if="runtime?.health_error_code" class="mt-3 text-sm text-red-600 dark:text-red-300">{{ t('admin.promptAuditV2.runtime.healthUnavailable') }}</p>
    <p v-else-if="runtime?.health_checked_at" class="mt-3 text-xs text-gray-500 dark:text-dark-400">
      {{ t('admin.promptAuditV2.runtime.healthCheckedAt') }}: {{ formatCheckedAt(runtime.health_checked_at) }}
    </p>
    <p v-if="!runtime" class="mt-4 text-sm text-gray-500 dark:text-dark-400">{{ loading ? t('common.loading') : '—' }}</p>
    <p v-if="runtime?.last_error_code" class="mt-3 text-sm text-red-600 dark:text-red-300">{{ runtime.last_error_code }}</p>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { PromptAuditV2Endpoint, PromptAuditV2Runtime } from '../types'

const props = defineProps<{
  runtime: PromptAuditV2Runtime | null
  endpoints: PromptAuditV2Endpoint[]
  loading: boolean
}>()
defineEmits<{ (event: 'refresh'): void }>()
const { t } = useI18n()

const items = computed(() => {
  if (!props.runtime) return []
  return [
    { label: t('admin.promptAuditV2.runtime.status'), value: runtimeStatus(props.runtime.status) },
    { label: t('admin.promptAuditV2.runtime.workers'), value: `${props.runtime.processing} / ${props.runtime.worker_count}` },
    { label: t('admin.promptAuditV2.runtime.queue'), value: `${props.runtime.queued} / ${props.runtime.queue_capacity}` },
    { label: t('admin.promptAuditV2.runtime.hits'), value: String(props.runtime.hits) },
    { label: t('admin.promptAuditV2.runtime.rejected'), value: String(props.runtime.rejected) },
    { label: t('admin.promptAuditV2.runtime.failovers'), value: String(props.runtime.failovers) },
    { label: t('admin.promptAuditV2.runtime.unavailable'), value: String(props.runtime.unavailable) },
    { label: t('admin.promptAuditV2.runtime.emailPending'), value: String(props.runtime.email_pending) },
    { label: t('admin.promptAuditV2.runtime.emailFailed'), value: String(props.runtime.email_failed) },
  ]
})

/** Resolves the runtime key to the configured display name without exposing internal IDs. */
function endpointName(id: string): string {
  return props.endpoints.find((endpoint) => endpoint.id === id)?.name.trim()
    || t('admin.promptAuditV2.runtime.unnamedEndpoint')
}

function runtimeStatus(status: string): string {
  return ['running', 'disabled', 'unavailable'].includes(status)
    ? t(`admin.promptAuditV2.runtime.statusValues.${status}`)
    : status
}

function formatCheckedAt(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}
</script>
