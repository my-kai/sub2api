<template>
  <div class="space-y-6">
    <!-- 代理默认带入 -->
    <div class="card">
      <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
          {{ t('admin.settings.accountDefaults.proxy.title') }}
        </h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.settings.accountDefaults.proxy.hint') }}
        </p>
      </div>
      <div class="space-y-5 p-6">
        <div class="max-w-md">
          <label class="input-label">{{ t('admin.settings.accountDefaults.proxy.mode') }}</label>
          <Select
            :modelValue="form.proxy_mode"
            :options="proxyModeOptions"
            @update:modelValue="onProxyModeChange"
          />
        </div>

        <div v-if="form.proxy_mode === 'fixed'">
          <label class="input-label">{{ t('admin.settings.accountDefaults.proxy.fixedProxies') }}</label>
          <ProxySelector v-model="fixedSelection" :proxies="availableProxies" multiple />
          <div class="mt-3 flex items-center gap-2">
            <Toggle v-model="form.allow_local_egress" />
            <span class="text-sm text-gray-700 dark:text-gray-300">
              {{ t('admin.settings.accountDefaults.proxy.allowLocal') }}
            </span>
          </div>
        </div>

        <div v-else-if="form.proxy_mode === 'random'">
          <p class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.settings.accountDefaults.proxy.randomHint') }}
          </p>
        </div>
      </div>
    </div>

    <!-- 模型默认带入 -->
    <div class="card">
      <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
          {{ t('admin.settings.accountDefaults.models.title') }}
        </h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.settings.accountDefaults.models.hint') }}
        </p>
      </div>
      <div class="space-y-5 p-6">
        <div v-for="platform in platforms" :key="platform">
          <label class="input-label">{{ platformLabel(platform) }}</label>
          <ModelWhitelistSelector
            :modelValue="form.models_by_platform[platform] ?? []"
            :platform="platform"
            @update:modelValue="(value: string[]) => setPlatformModels(platform, value)"
          />
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{
              (form.models_by_platform[platform] ?? []).length === 0
                ? t('admin.settings.accountDefaults.models.supportAll')
                : t('admin.settings.accountDefaults.models.selected', {
                    count: (form.models_by_platform[platform] ?? []).length
                  })
            }}
          </p>
        </div>
      </div>
    </div>

    <!-- 独立保存（与备份 Tab 相同模式，不参与主表单提交） -->
    <div class="flex justify-end">
      <button type="button" class="btn btn-primary" :disabled="saving || loading" @click="save">
        <svg
          v-if="saving"
          class="h-4 w-4 animate-spin"
          xmlns="http://www.w3.org/2000/svg"
          fill="none"
          viewBox="0 0 24 24"
        >
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v4a4 4 0 00-4 4H4z"></path>
        </svg>
        {{ saving ? t('common.saving') : t('common.save') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import ProxySelector from '@/components/common/ProxySelector.vue'
import ModelWhitelistSelector from '@/components/account/ModelWhitelistSelector.vue'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import type { AccountPlatform, Proxy } from '@/types'
import {
  getAccountDefaults,
  updateAccountDefaults,
  type AccountDefaultsConfig,
  type ProxyMode
} from './api'

const { t } = useI18n()
const appStore = useAppStore()

/**
 * 账号添加默认配置面板：
 * - 模型按平台预选白名单（空 = 不限制）；
 * - 代理二选一：固定 N 个（手动勾选）或随机 1 个（建表单时服务端抽）。
 */
const platforms: AccountPlatform[] = [
  'anthropic',
  'openai',
  'gemini',
  'antigravity',
  'grok',
  'kimi',
  'zhipu',
  'deepseek',
  'minimax',
  'opencode_go'
]

const loading = ref(true)
const saving = ref(false)
const availableProxies = ref<Proxy[]>([])

const form = reactive<AccountDefaultsConfig>({
  models_by_platform: {},
  proxy_mode: '',
  proxy_fixed_ids: [],
  allow_local_egress: false
})

const proxyModeOptions = computed(() => [
  { value: '', label: t('admin.settings.accountDefaults.proxy.modeNone') },
  { value: 'fixed', label: t('admin.settings.accountDefaults.proxy.modeFixed') },
  { value: 'random', label: t('admin.settings.accountDefaults.proxy.modeRandom') }
])

/** ProxySelector 的选中值是 number[]；配置中的固定代理 ID 与其双向绑定。 */
const fixedSelection = computed<number[]>({
  get: () => form.proxy_fixed_ids,
  set: (values) => {
    form.proxy_fixed_ids = values.filter((id) => id > 0)
  }
})

const platformLabel = (platform: string) => {
  const key = `admin.settings.accountDefaults.models.platforms.${platform}`
  const label = t(key)
  // 翻译缺失时回退平台 ID，避免显示完整 key。
  return label === key ? platform : label
}

const setPlatformModels = (platform: string, value: string[]) => {
  form.models_by_platform = { ...form.models_by_platform, [platform]: value }
}

const onProxyModeChange = (value: string | number | boolean | null) => {
  form.proxy_mode = (value ?? '') as ProxyMode | ''
}

/** 代理池：仅状态正常且未过期的代理可作为默认带入候选。 */
const isAvailable = (proxy: Proxy) =>
  proxy.status === 'active' &&
  (!proxy.expires_at || new Date(proxy.expires_at).getTime() > Date.now())

const load = async () => {
  loading.value = true
  try {
    const [config, proxies] = await Promise.all([
      getAccountDefaults(),
      adminAPI.proxies.getAll()
    ])
    form.models_by_platform = config.models_by_platform ?? {}
    form.proxy_mode = config.proxy_mode ?? ''
    form.proxy_fixed_ids = config.proxy_fixed_ids ?? []
    form.allow_local_egress = !!config.allow_local_egress
    availableProxies.value = (proxies ?? []).filter(isAvailable)
  } catch {
    appStore.showError(t('admin.settings.accountDefaults.loadFailed'))
  } finally {
    loading.value = false
  }
}

const save = async () => {
  saving.value = true
  try {
    const saved = await updateAccountDefaults({ ...form })
    form.models_by_platform = saved.models_by_platform ?? {}
    form.proxy_mode = saved.proxy_mode ?? ''
    form.proxy_fixed_ids = saved.proxy_fixed_ids ?? []
    form.allow_local_egress = !!saved.allow_local_egress
    appStore.showSuccess(t('admin.settings.accountDefaults.saveSuccess'))
  } catch (err) {
    const message =
      (err as { message?: string })?.message ||
      t('admin.settings.accountDefaults.saveFailed')
    appStore.showError(message)
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>
