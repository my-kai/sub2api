<template>
  <BaseDialog :show="show" :title="t('admin.groups.modelRateMultipliersTitle')" width="normal" @close="close">
    <div v-if="group" class="space-y-4">
      <p class="text-sm text-gray-500 dark:text-gray-400">{{ group.name }}: {{ group.rate_multiplier }}x</p>
      <div class="space-y-2">
        <div v-for="(entry, index) in entries" :key="index" class="flex items-center gap-2">
          <input v-model="entry.model" type="text" class="input min-w-0 flex-1" placeholder="model" />
          <input v-model.number="entry.rate_multiplier" type="number" min="0" step="0.001" class="input w-28" />
          <button type="button" class="btn btn-secondary" @click="entries.splice(index, 1)">
            <Icon name="trash" size="sm" />
          </button>
        </div>
        <button type="button" class="btn btn-secondary" @click="entries.push({ model: '', rate_multiplier: group.rate_multiplier })">
          {{ t('common.add') }}
        </button>
      </div>
      <div class="flex justify-end gap-2 border-t border-gray-200 pt-4 dark:border-dark-600">
        <button type="button" class="btn btn-secondary" @click="close">{{ t('common.close') }}</button>
        <button type="button" class="btn btn-primary" :disabled="saving" @click="save">{{ t('common.save') }}</button>
      </div>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI } from '@/api/admin'
import type { AdminGroup } from '@/types'

const props = defineProps<{ show: boolean; group: AdminGroup | null }>()
const emit = defineEmits<{ close: []; success: [] }>()
const { t } = useI18n()
const saving = ref(false)
const entries = ref<Array<{ model: string; rate_multiplier: number }>>([])

watch(() => [props.show, props.group?.id], async ([show]) => {
  if (!show || !props.group) return
  const loaded = await adminAPI.groups.getGroupModelRateMultipliers(props.group.id)
  entries.value = loaded.map(({ model, rate_multiplier }) => ({ model, rate_multiplier }))
}, { immediate: true })

const close = () => emit('close')
const save = async () => {
  if (!props.group) return
  saving.value = true
  try {
    await adminAPI.groups.batchSetGroupModelRateMultipliers(props.group.id, entries.value)
    emit('success')
    close()
  } finally {
    saving.value = false
  }
}
</script>
