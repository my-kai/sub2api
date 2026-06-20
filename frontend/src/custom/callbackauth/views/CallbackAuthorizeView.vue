<template>
  <AuthLayout>
    <div class="space-y-6">
      <div class="text-center">
        <h2 class="text-2xl font-bold text-gray-900 dark:text-white">
          {{ localText("授权登录", "Authorize login") }}
        </h2>
        <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">
          {{ statusText }}
        </p>
      </div>

      <div v-if="isLoading" class="flex justify-center py-4">
        <div class="h-8 w-8 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"></div>
      </div>

      <div v-else-if="errorMessage" class="space-y-4">
        <p class="rounded-lg bg-red-50 px-4 py-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">
          {{ errorMessage }}
        </p>
        <button class="btn btn-secondary w-full" type="button" @click="router.replace('/login')">
          {{ localText("返回登录", "Back to login") }}
        </button>
      </div>

      <div v-else class="space-y-4">
        <button
          class="btn btn-primary w-full"
          type="button"
          :disabled="isSubmitting"
          @click="confirmAuthorization"
        >
          {{ isSubmitting ? localText("处理中", "Processing") : localText("确认授权", "Authorize") }}
        </button>
        <button class="btn btn-secondary w-full" type="button" :disabled="isSubmitting" @click="router.replace('/dashboard')">
          {{ localText("取消", "Cancel") }}
        </button>
      </div>
    </div>
  </AuthLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { AuthLayout } from '@/components/layout'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'
import { authorizeCallback, getCallbackAuthorization } from '../api'

const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const { locale } = useI18n()

const callback = computed(() => {
  const value = route.query.callback
  return typeof value === 'string' ? value : ''
})
const domain = ref('')
const isLoading = ref(true)
const isSubmitting = ref(false)
const errorMessage = ref('')

const statusText = computed(() => {
  if (isLoading.value || isSubmitting.value) {
    return localText('正在确认授权状态', 'Checking authorization')
  }
  if (errorMessage.value) {
    return localText('无法继续授权', 'Authorization cannot continue')
  }
  // Consent target stays in the page subtitle so the confirmation screen remains compact.
  return domain.value
    ? localText(`确认授权给${domain.value}`, `Confirm authorization for ${domain.value}`)
    : localText('确认授权', 'Confirm authorization')
})

function localText(zh: string, en: string): string {
  return locale.value.startsWith('zh') ? zh : en
}

async function redirectWithCode(): Promise<void> {
  isSubmitting.value = true
  try {
    const result = await authorizeCallback(callback.value)
    window.location.assign(result.redirect_url)
  } catch (error: unknown) {
    errorMessage.value = extractApiErrorMessage(error, localText('授权失败', 'Authorization failed'))
    appStore.showError(errorMessage.value)
  } finally {
    isSubmitting.value = false
  }
}

async function confirmAuthorization(): Promise<void> {
  await redirectWithCode()
}

onMounted(async () => {
  if (!callback.value) {
    errorMessage.value = localText('回跳地址无效', 'Invalid callback URL')
    isLoading.value = false
    return
  }

  try {
    const info = await getCallbackAuthorization(callback.value)
    domain.value = info.domain
    if (info.authorized) {
      // Previously confirmed domains skip the consent click but still issue a fresh one-time code.
      await redirectWithCode()
      return
    }
  } catch (error: unknown) {
    errorMessage.value = extractApiErrorMessage(error, localText('回跳地址未被允许', 'Callback URL is not allowed'))
  } finally {
    isLoading.value = false
  }
})
</script>
