<template>
  <BaseDialog :show="show" title="发票管理权限" width="normal" @close="closeDialog">
    <div class="space-y-4">
      <p class="text-sm text-gray-500 dark:text-dark-400">选择可以进入发票管理的用户。未配置用户时，仅系统管理员可进入。</p>
      <Input v-model="searchKeyword" label="搜索用户" placeholder="输入邮箱搜索" :disabled="saving" />
      <div v-if="searchResults.length > 0" class="max-h-48 overflow-y-auto rounded-lg border border-gray-200 dark:border-dark-700">
        <button
          v-for="user in searchResults"
          :key="user.id"
          type="button"
          class="flex w-full items-center justify-between px-3 py-2 text-left text-sm hover:bg-gray-50 dark:hover:bg-dark-700"
          @click="toggleManager(user.id)"
        >
          <span class="text-gray-700 dark:text-dark-200">{{ user.email }}</span>
          <Icon v-if="selectedIDs.includes(user.id)" name="check" size="sm" class="text-primary-500" />
        </button>
      </div>
      <div>
        <div class="mb-2 text-sm font-medium text-gray-900 dark:text-white">已授权用户</div>
        <div v-if="selectedManagers.length > 0" class="flex flex-wrap gap-2">
          <span
            v-for="manager in selectedManagers"
            :key="manager.user_id"
            class="inline-flex items-center gap-1 rounded-lg bg-primary-50 px-2.5 py-1 text-sm text-primary-700 dark:bg-primary-500/10 dark:text-primary-300"
          >
            {{ manager.email }}
            <button type="button" class="rounded p-0.5 hover:bg-primary-100 dark:hover:bg-primary-500/20" title="移除授权" @click="toggleManager(manager.user_id)">
              <Icon name="x" size="sm" />
            </button>
          </span>
        </div>
        <p v-else class="text-sm text-gray-500 dark:text-dark-400">暂无授权用户</p>
      </div>
      <p v-if="errorMessage" class="text-sm text-red-600 dark:text-red-300">{{ errorMessage }}</p>
    </div>
    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" :disabled="saving" @click="closeDialog">取消</button>
        <button type="button" class="btn btn-primary" :disabled="saving || loading" @click="saveManagers">
          {{ saving ? '保存中...' : '保存权限' }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { searchUsers, type SimpleUser } from '@/api/admin/usage'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Input from '@/components/common/Input.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'
import { listInvoiceManagers, replaceInvoiceManagers } from '../api'
import type { InvoiceManager } from '../types'

const props = defineProps<{ show: boolean }>()
const emit = defineEmits<{ (event: 'close'): void }>()
const appStore = useAppStore()
const loading = ref(false)
const saving = ref(false)
const errorMessage = ref('')
const searchKeyword = ref('')
const searchResults = ref<SimpleUser[]>([])
const managers = ref<InvoiceManager[]>([])
const selectedIDs = ref<number[]>([])
let searchSequence = 0

const selectedManagers = computed(() => {
  const selected = new Set(selectedIDs.value)
  return managers.value.filter((manager) => selected.has(manager.user_id))
})

watch(() => props.show, (show) => {
  if (show) void loadManagers()
})

watch(searchKeyword, (value) => {
  const keyword = value.trim()
  const sequence = ++searchSequence
  searchResults.value = []
  if (!keyword) return
  void searchUsers(keyword).then((users) => {
    if (sequence !== searchSequence) return
    searchResults.value = users.filter((user) => !user.deleted)
  }).catch(() => {
    if (sequence === searchSequence) errorMessage.value = '用户搜索失败'
  })
})

/** Loads the persisted allowlist whenever the administrator opens the dialog. */
async function loadManagers(): Promise<void> {
  loading.value = true
  errorMessage.value = ''
  searchKeyword.value = ''
  try {
    managers.value = await listInvoiceManagers()
    selectedIDs.value = managers.value.map((manager) => manager.user_id)
  } catch (err) {
    errorMessage.value = extractApiErrorMessage(err, '权限配置读取失败')
  } finally {
    loading.value = false
  }
}

/** Adds or removes one visible user from the pending allowlist. */
function toggleManager(userID: number): void {
  if (!managers.value.some((manager) => manager.user_id === userID)) {
    const user = searchResults.value.find((candidate) => candidate.id === userID)
    if (user) managers.value = [...managers.value, { user_id: user.id, email: user.email }]
  }
  selectedIDs.value = selectedIDs.value.includes(userID)
    ? selectedIDs.value.filter((id) => id !== userID)
    : [...selectedIDs.value, userID]
}

/** Persists the complete allowlist; server-side validation remains authoritative. */
async function saveManagers(): Promise<void> {
  if (saving.value) return
  saving.value = true
  errorMessage.value = ''
  try {
    managers.value = await replaceInvoiceManagers(selectedIDs.value)
    selectedIDs.value = managers.value.map((manager) => manager.user_id)
    appStore.showSuccess('发票权限已保存')
    emit('close')
  } catch (err) {
    errorMessage.value = extractApiErrorMessage(err, '权限配置保存失败')
    appStore.showError(errorMessage.value)
  } finally {
    saving.value = false
  }
}

/** Keeps an in-flight save visible instead of implying it was cancelled. */
function closeDialog(): void {
  if (!saving.value) emit('close')
}
</script>
