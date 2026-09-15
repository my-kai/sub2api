<template>
  <section class="border-b border-gray-200 py-6 dark:border-dark-700">
    <div class="flex items-center justify-between gap-3">
      <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.promptAuditV2.endpoints.title') }}</h2>
      <button type="button" class="btn btn-primary btn-sm" @click="openCreate">
        <Icon name="plus" size="sm" />
        <span>{{ t('common.add') }}</span>
      </button>
    </div>

    <div class="mt-4 grid gap-3 lg:grid-cols-2">
      <article v-for="endpoint in modelValue" :key="endpoint.id" class="border border-gray-200 p-4 dark:border-dark-700">
        <div class="flex items-start justify-between gap-3">
          <div class="min-w-0">
            <h3 class="truncate text-sm font-semibold text-gray-900 dark:text-white">{{ endpoint.name || endpoint.id }}</h3>
            <p class="mt-1 truncate text-xs text-gray-500 dark:text-dark-400">{{ endpoint.model }} · {{ endpoint.priority }}</p>
          </div>
          <Toggle :model-value="endpoint.enabled" @update:model-value="toggle(endpoint.id, $event)" />
        </div>
        <div class="mt-4 flex items-center justify-between gap-3">
          <span class="text-xs" :class="probeClass(probeResults[endpoint.id])">{{ probeLabel(probeResults[endpoint.id]) }}</span>
          <div class="flex gap-1">
            <button
              type="button"
              class="btn btn-ghost btn-sm"
              :title="t('admin.promptAuditV2.actions.probe')"
              :disabled="probingIds.includes(endpoint.id)"
              @click="probeEndpoint(endpoint)"
            >
              <Icon name="refresh" size="sm" :class="{ 'animate-spin': probingIds.includes(endpoint.id) }" />
              <span>{{ t('admin.promptAuditV2.actions.probe') }}</span>
            </button>
            <button type="button" class="btn btn-ghost btn-sm" :title="t('common.edit')" @click="openEdit(endpoint)"><Icon name="edit" size="sm" /></button>
            <button type="button" class="btn btn-ghost btn-sm text-red-600" :title="t('common.delete')" @click="deleting = endpoint"><Icon name="trash" size="sm" /></button>
          </div>
        </div>
      </article>
      <p v-if="modelValue.length === 0" class="py-6 text-sm text-gray-500 dark:text-dark-400">{{ t('admin.promptAuditV2.endpoints.empty') }}</p>
    </div>

    <BaseDialog :show="Boolean(editing)" :title="editingIndex < 0 ? t('admin.promptAuditV2.endpoints.add') : t('admin.promptAuditV2.endpoints.edit')" width="wide" @close="closeEditor">
      <form v-if="editing" id="prompt-audit-v2-endpoint-form" class="grid gap-4 sm:grid-cols-2" @submit.prevent="saveEditor">
        <Input v-model="editing.name" :label="t('admin.promptAuditV2.endpoints.name')" required />
        <Input v-model="editing.model" :label="t('admin.promptAuditV2.endpoints.model')" required />
        <div class="sm:col-span-2"><Input v-model="editing.base_url" :label="t('admin.promptAuditV2.endpoints.baseUrl')" required /></div>
        <div class="sm:col-span-2">
          <Input v-model="editing.api_key" type="password" autocomplete="new-password" :label="t('admin.promptAuditV2.endpoints.apiKey')" :placeholder="editing.has_api_key ? t('admin.promptAuditV2.endpoints.keepKey') : ''" />
        </div>
        <Input :model-value="editing.priority" type="number" :label="t('admin.promptAuditV2.endpoints.priority')" required @update:model-value="editing.priority = Number($event)" />
        <Input :model-value="editing.timeout_ms" type="number" :label="t('admin.promptAuditV2.endpoints.timeout')" required @update:model-value="editing.timeout_ms = Number($event)" />
      </form>
      <template #footer>
        <button type="button" class="btn btn-secondary" @click="closeEditor">{{ t('common.cancel') }}</button>
        <span
          v-if="editing"
          class="mr-auto text-xs"
          :class="probeClass(probeResults[editing.id])"
        >
          {{ probeLabel(probeResults[editing.id]) }}
        </span>
        <button
          type="button"
          class="btn btn-secondary"
          :disabled="!canProbeEditing || probingIds.includes(editing?.id || '')"
          @click="probeEditing"
        >
          <Icon
            name="refresh"
            size="sm"
            class="mr-1"
            :class="{ 'animate-spin': probingIds.includes(editing?.id || '') }"
          />
          {{ t('admin.promptAuditV2.actions.probe') }}
        </button>
        <button type="submit" form="prompt-audit-v2-endpoint-form" class="btn btn-primary">{{ t('common.save') }}</button>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="Boolean(deleting)"
      :title="t('admin.promptAuditV2.endpoints.deleteTitle')"
      :message="t('admin.promptAuditV2.endpoints.deleteConfirm', { name: deleting?.name || '' })"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      danger
      @confirm="removeEndpoint"
      @cancel="deleting = null"
    />
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Input from '@/components/common/Input.vue'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'
import type { PromptAuditV2Endpoint, PromptAuditV2ProbeResult } from '../types'
import { createPromptAuditV2Endpoint } from '../viewModel'

const props = defineProps<{
  modelValue: PromptAuditV2Endpoint[]
  probeResults: Record<string, PromptAuditV2ProbeResult>
  probingIds: string[]
}>()
const emit = defineEmits<{
  (event: 'update:modelValue', value: PromptAuditV2Endpoint[]): void
  (event: 'probe', value: PromptAuditV2Endpoint): void
}>()
const { t } = useI18n()
const editing = ref<PromptAuditV2Endpoint | null>(null)
const editingIndex = ref(-1)
const deleting = ref<PromptAuditV2Endpoint | null>(null)

const canProbeEditing = computed(() => {
  if (!editing.value) return false
  return Boolean(
    editing.value.name.trim() &&
    editing.value.model.trim() &&
    editing.value.base_url.trim() &&
    (editing.value.api_key?.trim() || editing.value.has_api_key),
  )
})

/** Opens a complete new endpoint draft. */
function openCreate(): void {
  editingIndex.value = -1
  editing.value = createPromptAuditV2Endpoint(props.modelValue.length)
}

/** Opens an isolated copy so cancel never mutates the saved page draft. */
function openEdit(endpoint: PromptAuditV2Endpoint): void {
  editingIndex.value = props.modelValue.findIndex((item) => item.id === endpoint.id)
  // Endpoint fields are flat primitives; spread creates a detached plain object
  // without passing Vue's reactive proxy to structuredClone.
  editing.value = { ...endpoint, api_key: '' }
}

/** Closes the editor and discards its isolated draft. */
function closeEditor(): void {
  editing.value = null
  editingIndex.value = -1
}

/** Tests the current draft without saving it first; existing secrets remain write-only. */
function probeEditing(): void {
  if (!editing.value || !canProbeEditing.value) return
  probeEndpoint(editing.value)
}

/** Emits a detached endpoint snapshot so the parent never receives the reactive draft proxy. */
function probeEndpoint(endpoint: PromptAuditV2Endpoint): void {
  emit('probe', { ...endpoint })
}

/** Replaces or appends one endpoint while preserving configured ordering. */
function saveEditor(): void {
  if (!editing.value?.name.trim() || !editing.value.model.trim() || !editing.value.base_url.trim()) return
  const items = props.modelValue.map((item) => ({ ...item }))
  const savedEndpoint = { ...editing.value }
  if (editingIndex.value < 0) items.push(savedEndpoint)
  else items.splice(editingIndex.value, 1, savedEndpoint)
  emit('update:modelValue', items.map((item, order) => ({ ...item, order })))
  closeEditor()
}

/** Toggles one service without modifying any other draft field. */
function toggle(id: string, enabled: boolean): void {
  emit('update:modelValue', props.modelValue.map((item) => item.id === id ? { ...item, enabled } : { ...item }))
}

/** Removes the explicitly confirmed endpoint. */
function removeEndpoint(): void {
  if (!deleting.value) return
  emit('update:modelValue', props.modelValue.filter((item) => item.id !== deleting.value?.id).map((item, order) => ({ ...item, order })))
  deleting.value = null
}

function probeLabel(result?: PromptAuditV2ProbeResult): string {
  if (!result) return t('admin.promptAuditV2.endpoints.notProbed')
  return result.ok ? `${t('admin.promptAuditV2.endpoints.available')} · ${result.latency_ms} ms` : (result.error_code || result.status)
}

function probeClass(result?: PromptAuditV2ProbeResult): string {
  if (!result) return 'text-gray-500 dark:text-dark-400'
  return result.ok ? 'text-emerald-600 dark:text-emerald-300' : 'text-red-600 dark:text-red-300'
}
</script>
