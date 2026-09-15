<template>
  <section class="border-b border-gray-200 py-6 dark:border-dark-700">
    <div class="flex items-center justify-between gap-4">
      <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.promptAuditV2.config.title') }}</h2>
      <Toggle :model-value="modelValue.enabled" @update:model-value="patch({ enabled: $event })" />
    </div>

    <div class="mt-5 space-y-5">
      <TextArea
        :model-value="modelValue.prompt_template"
        :label="t('admin.promptAuditV2.config.prompt')"
        :placeholder="`${t('admin.promptAuditV2.config.promptPlaceholder')} {{user_input}}`"
        rows="8"
        required
        @update:model-value="patch({ prompt_template: $event })"
      />
      <div class="flex justify-end">
        <button type="button" class="btn btn-secondary btn-sm" @click="emit('test')">
          <Icon name="play" size="sm" />
          <span>{{ t('admin.promptAuditV2.promptTest.action') }}</span>
        </button>
      </div>

      <fieldset>
        <legend class="input-label">{{ t('admin.promptAuditV2.config.protocols') }} <span class="text-red-500">*</span></legend>
        <div class="mt-2 grid gap-2 sm:grid-cols-2 xl:grid-cols-4">
          <label
            v-for="protocol in modelValue.protocols"
            :key="protocol.value"
            class="flex min-h-11 cursor-pointer items-center gap-2 border border-gray-200 px-3 py-2 text-sm text-gray-700 dark:border-dark-700 dark:text-dark-200"
          >
            <input
              type="checkbox"
              :checked="modelValue.enabled_protocols.includes(protocol.value)"
              @change="toggleProtocol(protocol.value)"
            />
            <span>{{ protocol.label }}</span>
          </label>
        </div>
      </fieldset>

      <div class="grid gap-4 sm:grid-cols-3">
        <Input
          :model-value="modelValue.worker_count"
          type="number"
          :label="t('admin.promptAuditV2.config.workers')"
          required
          @update:model-value="patchNumber('worker_count', $event)"
        />
        <Input
          :model-value="modelValue.queue_capacity"
          type="number"
          :label="t('admin.promptAuditV2.config.queue')"
          required
          @update:model-value="patchNumber('queue_capacity', $event)"
        />
        <Input
          :model-value="modelValue.log_retention_days"
          type="number"
          :label="t('admin.promptAuditV2.config.retention')"
          required
          @update:model-value="patchNumber('log_retention_days', $event)"
        />
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Input from '@/components/common/Input.vue'
import TextArea from '@/components/common/TextArea.vue'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'
import type { PromptAuditV2Config } from '../types'

const props = defineProps<{ modelValue: PromptAuditV2Config }>()
const emit = defineEmits<{
  (event: 'update:modelValue', value: PromptAuditV2Config): void
  (event: 'test'): void
}>()
const { t } = useI18n()

/** Emits a new configuration object so page-level dirty state stays reliable. */
function patch(value: Partial<PromptAuditV2Config>): void {
  emit('update:modelValue', { ...props.modelValue, ...value })
}

/** Converts common Input string output into an explicit numeric field. */
function patchNumber(field: 'worker_count' | 'queue_capacity' | 'log_retention_days', value: string): void {
  patch({ [field]: Number(value) })
}

/** Adds or removes one backend-provided protocol identifier. */
function toggleProtocol(protocol: string): void {
  const selected = new Set(props.modelValue.enabled_protocols)
  if (selected.has(protocol)) selected.delete(protocol)
  else selected.add(protocol)
  patch({ enabled_protocols: [...selected] })
}
</script>
