<template>
  <BaseDialog
    :show="show"
    :title="t('admin.promptAuditV2.promptTest.title')"
    width="normal"
    :close-on-escape="!loading"
    :show-close-button="!loading"
    @close="closeDialog"
  >
    <form id="prompt-audit-v2-test-form" class="space-y-4" @submit.prevent="submitTest">
      <TextArea
        :model-value="userInput"
        :label="t('admin.promptAuditV2.promptTest.input')"
        :placeholder="t('admin.promptAuditV2.promptTest.inputPlaceholder')"
        :disabled="loading"
        rows="6"
        required
        @update:model-value="updateInput"
      />

      <p v-if="blockingMessage" role="alert" class="text-sm text-amber-700 dark:text-amber-300">
        {{ blockingMessage }}
      </p>
      <p v-if="errorText" role="alert" class="border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-900 dark:bg-red-950/30 dark:text-red-300">
        {{ errorText }}
      </p>

      <section v-if="result" class="border border-gray-200 p-4 dark:border-dark-700">
        <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.promptAuditV2.promptTest.result') }}</h4>
        <dl class="mt-3 grid gap-3 sm:grid-cols-2">
          <div>
            <dt class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.promptAuditV2.promptTest.endpoint') }}</dt>
            <dd class="mt-1 break-words text-sm text-gray-900 dark:text-white">{{ result.endpoint_name }}</dd>
          </div>
          <div>
            <dt class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.promptAuditV2.promptTest.model') }}</dt>
            <dd class="mt-1 break-words text-sm text-gray-900 dark:text-white">{{ result.audit_model }}</dd>
          </div>
          <div>
            <dt class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.promptAuditV2.promptTest.confidence') }}</dt>
            <dd class="mt-1 text-sm tabular-nums text-gray-900 dark:text-white">{{ result.confidence.toFixed(4) }}</dd>
          </div>
          <div>
            <dt class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.promptAuditV2.promptTest.latency') }}</dt>
            <dd class="mt-1 text-sm tabular-nums text-gray-900 dark:text-white">{{ result.latency_ms }}</dd>
          </div>
          <div class="sm:col-span-2">
            <dt class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.promptAuditV2.promptTest.reason') }}</dt>
            <dd class="mt-1 whitespace-pre-wrap break-words text-sm text-gray-900 dark:text-white">{{ result.reason }}</dd>
          </div>
        </dl>
      </section>
    </form>

    <template #footer>
      <button type="button" class="btn btn-secondary" :disabled="loading" @click="closeDialog">
        {{ t('common.cancel') }}
      </button>
      <button type="submit" form="prompt-audit-v2-test-form" class="btn btn-primary" :disabled="!canSubmit">
        <Icon name="play" size="sm" :class="{ 'animate-pulse': loading }" />
        <span>{{ loading ? t('admin.promptAuditV2.promptTest.testing') : t('admin.promptAuditV2.promptTest.action') }}</span>
      </button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import TextArea from '@/components/common/TextArea.vue'
import Icon from '@/components/icons/Icon.vue'
import type { PromptAuditV2PromptTestResult } from '../types'

const props = defineProps<{
  show: boolean
  loading: boolean
  result: PromptAuditV2PromptTestResult | null
  errorText: string
  validationError: string
}>()
const emit = defineEmits<{
  (event: 'close'): void
  (event: 'reset'): void
  (event: 'submit', userInput: string): void
}>()
const { t } = useI18n()
const userInput = ref('')
const inputError = computed(() => userInput.value.trim() ? '' : t('admin.promptAuditV2.promptTest.inputRequired'))
const blockingMessage = computed(() => props.validationError || inputError.value)
const canSubmit = computed(() => !props.loading && !blockingMessage.value)

watch(() => props.show, (show) => {
  if (show) userInput.value = ''
})

/** Updates the draft message and clears any outcome produced for the previous input. */
function updateInput(value: string): void {
  userInput.value = value
  emit('reset')
}

/** Closes the dialog only after the in-flight request has completed. */
function closeDialog(): void {
  if (!props.loading) emit('close')
}

/** Emits the exact entered message after local and draft validation succeeds. */
function submitTest(): void {
  if (!canSubmit.value) return
  emit('submit', userInput.value)
}
</script>
