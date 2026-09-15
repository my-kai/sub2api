<template>
  <section class="py-6">
    <div class="flex items-center justify-between gap-3">
      <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.promptAuditV2.events.title') }}</h2>
      <button type="button" class="btn btn-secondary btn-sm" :disabled="loading" @click="$emit('search')">
        <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
        <span>{{ t('common.refresh') }}</span>
      </button>
    </div>

    <div class="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
      <Input :model-value="modelValue.user_id" :label="t('admin.promptAuditV2.events.userId')" type="number" @update:model-value="patch('user_id', $event)" />
      <div>
        <label class="input-label mb-1.5 block">{{ t('admin.promptAuditV2.events.action') }}</label>
        <Select :model-value="modelValue.action" :options="actionOptions" :searchable="false" @update:model-value="patch('action', String($event ?? ''))" />
      </div>
      <div>
        <label class="input-label mb-1.5 block">{{ t('admin.promptAuditV2.events.protocol') }}</label>
        <Select :model-value="modelValue.protocol" :options="protocolOptions" :searchable="false" @update:model-value="patch('protocol', String($event ?? ''))" />
      </div>
      <Input :model-value="modelValue.min_confidence" :label="t('admin.promptAuditV2.events.minConfidence')" type="number" @update:model-value="patch('min_confidence', $event)" />
      <Input :model-value="modelValue.max_confidence" :label="t('admin.promptAuditV2.events.maxConfidence')" type="number" @update:model-value="patch('max_confidence', $event)" />
      <Input :model-value="modelValue.start_at" :label="t('admin.promptAuditV2.events.startAt')" type="datetime-local" @update:model-value="patch('start_at', $event)" />
      <Input :model-value="modelValue.end_at" :label="t('admin.promptAuditV2.events.endAt')" type="datetime-local" @update:model-value="patch('end_at', $event)" />
    </div>
    <div class="mt-3 flex gap-2">
      <button type="button" class="btn btn-primary btn-sm" @click="$emit('search')">{{ t('common.search') }}</button>
      <button type="button" class="btn btn-secondary btn-sm" @click="$emit('reset')">{{ t('common.reset') }}</button>
    </div>

    <div class="mt-5 overflow-x-auto border border-gray-200 dark:border-dark-700">
      <table class="w-full min-w-[1080px] text-left text-sm">
        <thead class="bg-gray-50 text-xs text-gray-500 dark:bg-dark-800 dark:text-dark-400">
          <tr>
            <th class="px-4 py-3 font-medium">{{ t('admin.promptAuditV2.events.time') }}</th>
            <th class="px-4 py-3 font-medium">{{ t('admin.promptAuditV2.events.user') }}</th>
            <th class="px-4 py-3 font-medium">{{ t('admin.promptAuditV2.events.route') }}</th>
            <th class="px-4 py-3 font-medium">{{ t('admin.promptAuditV2.events.score') }}</th>
            <th class="px-4 py-3 font-medium">{{ t('admin.promptAuditV2.events.reason') }}</th>
            <th class="px-4 py-3 font-medium">{{ t('admin.promptAuditV2.events.count') }}</th>
            <th class="px-4 py-3 font-medium">{{ t('admin.promptAuditV2.events.action') }}</th>
            <th class="px-4 py-3 text-right font-medium">{{ t('common.actions') }}</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
          <tr v-if="loading"><td colspan="8" class="px-4 py-10 text-center text-gray-500">{{ t('common.loading') }}</td></tr>
          <tr v-else-if="events.length === 0"><td colspan="8" class="px-4 py-10 text-center text-gray-500">{{ t('admin.promptAuditV2.events.empty') }}</td></tr>
          <template v-else>
            <tr v-for="event in events" :key="event.id">
              <td class="whitespace-nowrap px-4 py-3 text-xs text-gray-600 dark:text-dark-300">{{ formatDate(event.created_at) }}</td>
              <td class="px-4 py-3"><p class="font-medium text-gray-900 dark:text-white">{{ event.username || event.user_id }}</p><p class="text-xs text-gray-500">{{ event.user_email }}</p></td>
              <td class="px-4 py-3"><p>{{ event.protocol }}</p><p class="text-xs text-gray-500">{{ event.request_model || '—' }}</p></td>
              <td class="px-4 py-3 font-medium tabular-nums">{{ event.confidence.toFixed(4) }}</td>
              <td class="max-w-xs px-4 py-3"><p class="line-clamp-2 break-words">{{ event.reason }}</p></td>
              <td class="px-4 py-3 tabular-nums">{{ event.window_hit_count }} / {{ event.rule_trigger_count }}</td>
              <td class="px-4 py-3">{{ actionLabel(event.final_action) }}</td>
              <td class="px-4 py-3 text-right"><button type="button" class="btn btn-ghost btn-sm" :title="t('common.view')" @click="$emit('view', event.id)"><Icon name="eye" size="sm" /></button></td>
            </tr>
          </template>
        </tbody>
      </table>
      <Pagination :total="total" :page="page" :page-size="pageSize" @update:page="$emit('page', $event)" @update:page-size="$emit('page-size', $event)" />
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Input from '@/components/common/Input.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import type { PromptAuditV2Event, PromptAuditV2EventFilters, PromptAuditV2Protocol } from '../types'

const props = defineProps<{
  modelValue: PromptAuditV2EventFilters
  protocols: PromptAuditV2Protocol[]
  events: PromptAuditV2Event[]
  total: number
  page: number
  pageSize: number
  loading: boolean
}>()
const emit = defineEmits<{
  (event: 'update:modelValue', value: PromptAuditV2EventFilters): void
  (event: 'search'): void
  (event: 'reset'): void
  (event: 'page', value: number): void
  (event: 'page-size', value: number): void
  (event: 'view', value: number): void
}>()
const { t, locale } = useI18n()
const actionOptions = computed(() => [
  { value: '', label: t('common.all') },
  { value: 'none', label: t('admin.promptAuditV2.actions.none') },
  { value: 'warning', label: t('admin.promptAuditV2.actions.warning') },
  { value: 'ban', label: t('admin.promptAuditV2.actions.ban') },
])
const protocolOptions = computed(() => [
  { value: '', label: t('common.all') },
  ...props.protocols.map((item) => ({ value: item.value, label: item.label })),
])
/** Emits one filter field without mutating the parent object. */
function patch(field: keyof PromptAuditV2EventFilters, value: string): void {
  emit('update:modelValue', { ...props.modelValue, [field]: value })
}

function formatDate(value: string): string {
  return new Intl.DateTimeFormat(locale.value, { dateStyle: 'short', timeStyle: 'medium' }).format(new Date(value))
}

function actionLabel(action: string): string {
  return ['none', 'warning', 'ban'].includes(action) ? t(`admin.promptAuditV2.actions.${action}`) : action
}
</script>
