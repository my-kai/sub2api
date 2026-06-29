<template>
  <div :class="props.embedded ? 'space-y-4' : 'card'">
    <div
      v-if="!props.embedded"
      class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
    >
      <h2 class="text-lg font-medium text-gray-900 dark:text-white">
        {{ t('profile.changePassword') }}
      </h2>
    </div>
    <div :class="props.embedded ? '' : 'px-6 py-6'">
      <form @submit.prevent="handleChangePassword" class="space-y-4">
        <div v-if="props.embedded">
          <p class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('profile.changePassword') }}
          </p>
        </div>
        <div>
          <label :for="fieldIds.oldPassword" class="input-label">
            {{ t('profile.currentPassword') }}
          </label>
          <input
            :id="fieldIds.oldPassword"
            v-model="form.old_password"
            type="password"
            required
            autocomplete="current-password"
            class="input"
          />
        </div>

        <div>
          <label :for="fieldIds.newPassword" class="input-label">
            {{ t('profile.newPassword') }}
          </label>
          <input
            :id="fieldIds.newPassword"
            v-model="form.new_password"
            type="password"
            required
            autocomplete="new-password"
            class="input"
          />
          <p class="input-hint">
            {{ t('profile.passwordHint') }}
          </p>
        </div>

        <div>
          <label :for="fieldIds.confirmPassword" class="input-label">
            {{ t('profile.confirmNewPassword') }}
          </label>
          <input
            :id="fieldIds.confirmPassword"
            v-model="form.confirm_password"
            type="password"
            required
            autocomplete="new-password"
            class="input"
          />
        </div>

        <div class="flex justify-end pt-4">
          <button type="submit" :disabled="loading" class="btn btn-primary">
            {{ loading ? t('profile.changingPassword') : t('profile.changePasswordButton') }}
          </button>
        </div>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { userAPI } from '@/api'

const { t } = useI18n()
const appStore = useAppStore()
const props = withDefaults(defineProps<{
  embedded?: boolean
  idPrefix?: string
}>(), {
  embedded: false,
  idPrefix: '',
})
const emit = defineEmits<{
  (event: 'success'): void
}>()

const loading = ref(false)
const form = ref({
  old_password: '',
  new_password: '',
  confirm_password: ''
})

// 同一个页面可能同时存在资料页表单和弹窗表单，字段 ID 必须可区分，避免 label 指向错实例。
const fieldIds = computed(() => ({
  oldPassword: props.idPrefix ? `${props.idPrefix}-old-password` : 'old_password',
  newPassword: props.idPrefix ? `${props.idPrefix}-new-password` : 'new_password',
  confirmPassword: props.idPrefix ? `${props.idPrefix}-confirm-password` : 'confirm_password',
}))

const handleChangePassword = async () => {
  if (form.value.new_password !== form.value.confirm_password) {
    appStore.showError(t('profile.passwordsNotMatch'))
    return
  }

  if (form.value.new_password.length < 8) {
    appStore.showError(t('profile.passwordTooShort'))
    return
  }

  loading.value = true
  try {
    await userAPI.changePassword(form.value.old_password, form.value.new_password)
    form.value = { old_password: '', new_password: '', confirm_password: '' }
    appStore.showSuccess(t('profile.passwordChangeSuccess'))
    emit('success')
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('profile.passwordChangeFailed'))
  } finally {
    loading.value = false
  }
}
</script>
