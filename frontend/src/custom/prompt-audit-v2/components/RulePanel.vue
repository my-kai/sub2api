<template>
  <section class="py-6">
    <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.promptAuditV2.rules.title') }}</h2>

    <div class="mt-5 grid gap-4 sm:grid-cols-2">
      <Input
        :model-value="modelValue.confidence_threshold"
        type="number"
        :label="t('admin.promptAuditV2.rules.confidence')"
        required
        @update:model-value="patchNumber('confidence_threshold', $event)"
      />
      <Input
        :model-value="modelValue.window_minutes"
        type="number"
        :label="t('admin.promptAuditV2.rules.window')"
        required
        @update:model-value="patchNumber('window_minutes', $event)"
      />
      <Input
        :model-value="modelValue.trigger_count"
        type="number"
        :label="t('admin.promptAuditV2.rules.count')"
        required
        @update:model-value="patchNumber('trigger_count', $event)"
      />
      <div>
        <label class="input-label mb-1.5 block">{{ t('admin.promptAuditV2.rules.action') }} <span class="text-red-500">*</span></label>
        <Select :model-value="modelValue.action" :options="actionOptions" :searchable="false" @update:model-value="changeAction(String($event))" />
      </div>
      <Input
        v-if="modelValue.action === 'warning'"
        :model-value="modelValue.restriction_minutes || 0"
        type="number"
        :label="t('admin.promptAuditV2.rules.restriction')"
        required
        @update:model-value="patchNumber('restriction_minutes', $event)"
      />
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Input from '@/components/common/Input.vue'
import Select from '@/components/common/Select.vue'
import type { PromptAuditV2Action, PromptAuditV2Rule } from '../types'

const props = defineProps<{ modelValue: PromptAuditV2Rule }>()
const emit = defineEmits<{ (event: 'update:modelValue', value: PromptAuditV2Rule): void }>()
const { t } = useI18n()
const actionOptions = computed(() => [
  { value: 'warning', label: t('admin.promptAuditV2.actions.warning') },
  { value: 'ban', label: t('admin.promptAuditV2.actions.ban') },
])

/** Emits a new rule object so the page-level dirty state stays reliable. */
function patch(value: Partial<PromptAuditV2Rule>): void {
  emit('update:modelValue', { ...props.modelValue, ...value })
}

/** Converts common Input string output into an explicit numeric rule field. */
function patchNumber(field: 'confidence_threshold' | 'window_minutes' | 'trigger_count' | 'restriction_minutes', value: string): void {
  patch({ [field]: Number(value) })
}

/** Keeps the warning-only restriction field valid for the selected action. */
function changeAction(value: string): void {
  if (value !== 'warning' && value !== 'ban') return
  const action = value as PromptAuditV2Action
  patch({ action, restriction_minutes: action === 'warning' ? (props.modelValue.restriction_minutes || 30) : null })
}
</script>
