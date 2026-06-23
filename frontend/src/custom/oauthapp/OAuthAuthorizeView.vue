<template>
  <AuthLayout>
    <div class="space-y-6">
      <div class="text-center">
        <h2 class="text-2xl font-bold text-gray-900 dark:text-white">
          第三方授权
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
        <button class="btn btn-secondary w-full" type="button" @click="cancelAuthorization">
          返回控制台
        </button>
      </div>

      <div v-else class="space-y-5">
        <div class="rounded-xl border border-gray-200 bg-white/70 p-4 dark:border-dark-700 dark:bg-dark-900/50">
          <dl class="space-y-3">
            <div class="flex items-start justify-between gap-4">
              <dt class="text-sm text-gray-500 dark:text-dark-400">应用名称</dt>
              <dd class="max-w-[14rem] break-words text-right text-sm font-medium text-gray-900 dark:text-white">
                {{ applicationName }}
              </dd>
            </div>
            <div class="flex items-start justify-between gap-4">
              <dt class="text-sm text-gray-500 dark:text-dark-400">回调域名</dt>
              <dd class="max-w-[14rem] break-words text-right text-sm font-medium text-gray-900 dark:text-white">
                {{ redirectDomain }}
              </dd>
            </div>
          </dl>
        </div>

        <p class="text-center text-sm text-gray-500 dark:text-dark-400">
          确认后将继续授权流程
        </p>

        <div class="space-y-3">
          <button
            class="btn btn-primary w-full"
            type="button"
            :disabled="isSubmitting"
            @click="confirmAuthorization"
          >
            {{ isSubmitting ? '授权中' : '确认授权' }}
          </button>
          <button class="btn btn-secondary w-full" type="button" :disabled="isSubmitting" @click="cancelAuthorization">
            取消
          </button>
        </div>
      </div>
    </div>
  </AuthLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { AuthLayout } from '@/components/layout'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'
import { authorizeOAuthApplication, getOAuthAuthorization } from './oauthApi'
import type { OAuthAuthorizationInfo, OAuthAuthorizeRequest } from './types'

const route = useRoute()
const router = useRouter()
const appStore = useAppStore()

const authorizationInfo = ref<OAuthAuthorizationInfo | null>(null)
const isLoading = ref(true)
const isSubmitting = ref(false)
const errorMessage = ref('')

const authorizeRequest = computed<OAuthAuthorizeRequest>(() => ({
  response_type: readQueryParam('response_type'),
  client_id: readQueryParam('client_id'),
  redirect_uri: readQueryParam('redirect_uri'),
  state: readQueryParam('state'),
}))

const applicationName = computed(() => authorizationInfo.value?.applicationName || '第三方应用')
const redirectDomain = computed(() => authorizationInfo.value?.redirectDomain || resolveDomain(authorizeRequest.value.redirect_uri))

const statusText = computed(() => {
  if (isLoading.value) return '正在读取授权信息'
  if (isSubmitting.value) return '正在确认授权'
  if (errorMessage.value) return '无法继续授权'
  return '请确认授权信息'
})

/**
 * 读取单个 OAuth query 字段，并统一返回字符串。
 *
 * @param key - 当前路由 query 中期望存在的 OAuth 请求字段
 * @returns 字段的第一个字符串值；缺失时返回空字符串
 */
function readQueryParam(key: keyof OAuthAuthorizeRequest): string {
  const value = route.query[key]
  if (typeof value === 'string') return value
  if (Array.isArray(value) && typeof value[0] === 'string') return value[0]
  return ''
}

/**
 * 当后端没有返回域名时，仅为展示解析一个兜底域名。
 *
 * @param redirectUri - 原始请求中的 OAuth 回调地址
 * @returns 用于展示的 host，解析失败时返回短兜底文案
 */
function resolveDomain(redirectUri: string): string {
  if (!redirectUri) return '未知域名'
  try {
    return new URL(redirectUri).host || '未知域名'
  } catch {
    return '未知域名'
  }
}

/**
 * 调用后端接口前，先校验最小 OAuth 请求参数。
 *
 * @param request - 从路由 query 解析出的 OAuth 参数
 * @returns 用户可见错误文案；可用时返回空字符串
 */
function validateAuthorizeRequest(request: OAuthAuthorizeRequest): string {
  if (!request.response_type || !request.client_id || !request.redirect_uri) {
    return '授权请求无效'
  }
  if (request.response_type !== 'code') {
    return '授权类型不支持'
  }
  return ''
}

/**
 * 发送授权确认，并跳转到后端返回的回调地址。
 *
 * @returns 回调跳转开始后完成的 Promise
 */
async function confirmAuthorization(): Promise<void> {
  const validationMessage = validateAuthorizeRequest(authorizeRequest.value)
  if (validationMessage) {
    errorMessage.value = validationMessage
    return
  }

  isSubmitting.value = true
  try {
    const result = await authorizeOAuthApplication(authorizeRequest.value)
    window.location.assign(result.redirect_url)
  } catch (error: unknown) {
    errorMessage.value = extractApiErrorMessage(error, '授权失败')
    appStore.showError(errorMessage.value)
  } finally {
    isSubmitting.value = false
  }
}

/**
 * 取消 OAuth 授权流程，并返回控制台。
 */
function cancelAuthorization(): void {
  router.replace('/dashboard')
}

onMounted(async () => {
  const validationMessage = validateAuthorizeRequest(authorizeRequest.value)
  if (validationMessage) {
    errorMessage.value = validationMessage
    isLoading.value = false
    return
  }

  try {
    authorizationInfo.value = await getOAuthAuthorization(authorizeRequest.value)
  } catch (error: unknown) {
    errorMessage.value = extractApiErrorMessage(error, '授权信息读取失败')
  } finally {
    isLoading.value = false
  }
})
</script>
