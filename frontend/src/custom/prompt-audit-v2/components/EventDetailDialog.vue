<template>
  <BaseDialog :show="show" :title="t('admin.promptAuditV2.events.detailTitle')" width="wide" @close="$emit('close')">
    <div v-if="loading" class="py-10 text-center text-sm text-gray-500">{{ t('common.loading') }}</div>
    <div v-else-if="event" class="space-y-5">
      <dl class="grid gap-3 sm:grid-cols-2">
        <div v-for="item in summary" :key="item.label" class="border-b border-gray-100 pb-2 dark:border-dark-700">
          <dt class="text-xs text-gray-500 dark:text-dark-400">{{ item.label }}</dt>
          <dd class="mt-1 break-words text-sm text-gray-900 dark:text-white">{{ item.value }}</dd>
        </div>
      </dl>
      <div>
        <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.promptAuditV2.events.message') }}</h3>
        <pre class="mt-2 max-h-80 overflow-auto whitespace-pre-wrap break-words border border-gray-200 bg-gray-50 p-4 text-sm text-gray-800 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-100">{{ event.message }}</pre>
      </div>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import type { PromptAuditV2Event } from '../types'

const props = defineProps<{ show: boolean; loading: boolean; event: PromptAuditV2Event | null }>()
defineEmits<{ (event: 'close'): void }>()
const { t } = useI18n()

const summary = computed(() => {
  if (!props.event) return []
  return [
    { label: t('admin.promptAuditV2.events.user'), value: `${props.event.username || props.event.user_id} · ${props.event.user_email}` },
    { label: t('admin.promptAuditV2.events.score'), value: props.event.confidence.toFixed(4) },
    { label: t('admin.promptAuditV2.events.reason'), value: props.event.reason },
    { label: t('admin.promptAuditV2.events.endpoint'), value: `${props.event.endpoint_name} · ${props.event.audit_model}` },
    { label: t('admin.promptAuditV2.events.count'), value: `${props.event.window_hit_count} / ${props.event.rule_trigger_count}` },
    { label: t('admin.promptAuditV2.events.action'), value: t(`admin.promptAuditV2.actions.${props.event.final_action}`) },
  ]
})
</script>
