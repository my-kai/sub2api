<template>
  <AppLayout>
    <div class="flex min-h-[420px] items-center justify-center rounded-2xl border border-gray-200 bg-white p-8 text-center shadow-sm dark:border-dark-700 dark:bg-dark-900">
      <div class="max-w-md">
        <Icon
          v-if="!errorMessage"
          name="refresh"
          size="xl"
          class="mx-auto animate-spin text-primary-500"
        />
        <Icon
          v-else
          name="sparkles"
          size="xl"
          class="mx-auto text-gray-400"
        />

        <h1 class="mt-4 text-lg font-semibold text-gray-900 dark:text-white">
          {{ errorMessage ? '暂时无法进入生图页面' : '正在进入生图页面' }}
        </h1>
        <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">
          {{ errorMessage || '请稍候，系统正在为你打开独立生图服务。' }}
        </p>

        <button
          v-if="errorMessage"
          type="button"
          class="btn btn-primary mt-6"
          @click="retry"
        >
          重试
        </button>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import { useAppStore } from '@/stores'
import { createImageGenLoginCode } from '../api/handoff'

const appStore = useAppStore()
const loading = ref(false)
const errorMessage = ref('')

/**
 * 生成一次性 code 后直接跳转到 image-gen 后端 callback。
 *
 * 错误文案只保留用户可理解的业务结果；接口内部错误、配置名、
 * 上游渠道和服务间 secret 都不能出现在页面上。
 */
async function enterImageGen(): Promise<void> {
  if (loading.value) {
    return
  }
  loading.value = true
  errorMessage.value = ''

  try {
    const result = await createImageGenLoginCode()
    if (!result.redirect_url) {
      throw new Error('missing redirect url')
    }
    window.location.assign(result.redirect_url)
  } catch {
    errorMessage.value = '生图服务暂不可用，请稍后重试'
    appStore.showError(errorMessage.value)
  } finally {
    loading.value = false
  }
}

function retry(): void {
  void enterImageGen()
}

onMounted(() => {
  void enterImageGen()
})
</script>
