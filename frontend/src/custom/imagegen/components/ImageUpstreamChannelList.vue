<template>
  <div class="rounded-2xl border border-gray-200 p-4 dark:border-dark-700">
    <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <div>
        <h3 class="text-sm font-semibold text-gray-900 dark:text-white">上游渠道</h3>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">优先级越低越先使用。</p>
      </div>
      <button type="button" class="btn btn-secondary btn-sm" :disabled="saving" @click="emit('add-channel')">
        <Icon name="plus" size="sm" />
        <span>新增渠道</span>
      </button>
    </div>

    <div v-if="channels.length === 0" class="mt-4 rounded-2xl border border-dashed border-gray-300 p-6 text-center dark:border-dark-700">
      <Icon name="sparkles" size="lg" class="mx-auto text-gray-400" />
      <h4 class="mt-2 text-sm font-semibold text-gray-900 dark:text-white">暂无上游渠道</h4>
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">新增后可用于生图请求。</p>
    </div>

    <div v-else class="mt-4 space-y-4">
      <article
        v-for="(channel, index) in channels"
        :key="channel.id"
        class="rounded-2xl border border-gray-200 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800/60"
      >
        <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <div class="flex items-center gap-3">
            <span class="rounded-full bg-white px-3 py-1 text-xs font-medium text-gray-600 shadow-sm dark:bg-dark-900 dark:text-gray-300">
              优先级 {{ channel.priority }}
            </span>
            <div>
              <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ channel.name || channelTypeLabel(channel.type) }}</h4>
              <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ channelTypeLabel(channel.type) }}</p>
            </div>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <div class="inline-flex items-center gap-2 rounded-xl bg-white px-3 py-2 text-sm text-gray-700 dark:bg-dark-900 dark:text-gray-300">
              <span>启用</span>
              <Toggle :model-value="channel.enabled" @update:model-value="(value) => updateChannel(index, { enabled: value })" />
            </div>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="saving || index === 0" @click="emit('move-channel', index, -1)">上移</button>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="saving || index === channels.length - 1" @click="emit('move-channel', index, 1)">下移</button>
            <button
              type="button"
              class="btn btn-secondary btn-sm text-red-600 hover:text-red-700 dark:text-red-400"
              :disabled="saving || channels.length <= 1"
              @click="emit('remove-channel', index)"
            >
              <Icon name="trash" size="sm" />
              <span>删除</span>
            </button>
          </div>
        </div>

        <div class="mt-4 grid gap-4 md:grid-cols-2">
          <label class="space-y-1 text-sm">
            <span class="text-gray-600 dark:text-gray-300">类型</span>
            <Select
              :model-value="channel.type"
              :options="channelTypeOptions"
              :disabled="saving"
              @update:model-value="handleTypeChange(index, $event)"
            />
          </label>

          <label class="space-y-1 text-sm">
            <span class="text-gray-600 dark:text-gray-300">名称</span>
            <input class="input" :value="channel.name" :disabled="saving" type="text" placeholder="渠道名称" @input="updateChannel(index, { name: eventValue($event) })" />
          </label>

          <label class="space-y-1 text-sm">
            <span class="text-gray-600 dark:text-gray-300">优先级</span>
            <input class="input" :value="channel.priority" :disabled="saving" type="number" step="1" @input="updateChannel(index, { priority: eventInteger($event, channel.priority) })" />
          </label>

          <label class="space-y-1 text-sm">
            <span class="text-gray-600 dark:text-gray-300">Base URL</span>
            <input class="input" :value="channel.base_url" :disabled="saving" type="url" :placeholder="baseURLPlaceholder(channel.type)" @input="updateChannel(index, { base_url: eventValue($event) })" />
          </label>

          <label class="space-y-1 text-sm">
            <span class="text-gray-600 dark:text-gray-300">Auth Key</span>
            <input
              class="input"
              :value="channel.auth_key"
              :disabled="saving || channel.clear_auth_key"
              type="password"
              autocomplete="new-password"
              :placeholder="channel.auth_key_configured ? '已配置，留空保持不变' : '请输入 Auth Key'"
              @input="handleAuthKeyInput(index, $event)"
            />
          </label>

          <label class="space-y-1 text-sm">
            <span class="text-gray-600 dark:text-gray-300">重试次数</span>
            <input class="input" :value="channel.retry_count" :disabled="saving" type="number" min="0" @input="updateChannel(index, { retry_count: eventNonNegativeInteger($event) })" />
          </label>
        </div>

        <label
          v-if="channel.auth_key_configured"
          class="mt-3 inline-flex items-center gap-2 rounded-xl bg-white px-3 py-2 text-sm text-gray-700 dark:bg-dark-900 dark:text-gray-300"
        >
          <input
            class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
            type="checkbox"
            :checked="channel.clear_auth_key"
            :disabled="saving"
            @change="handleClearAuthKeyChange(index, $event)"
          />
          <span>保存时清空 Auth Key</span>
        </label>
      </article>
    </div>
  </div>
</template>

<script setup lang="ts">
import Icon from '@/components/icons/Icon.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import type { ImageUpstreamChannelForm, ImageUpstreamChannelType } from '../types'

const props = withDefaults(defineProps<{
  channels: ImageUpstreamChannelForm[]
  saving?: boolean
}>(), {
  saving: false,
})

const emit = defineEmits<{
  (event: 'add-channel'): void
  (event: 'remove-channel', index: number): void
  (event: 'move-channel', index: number, direction: -1 | 1): void
  (event: 'update-channel', index: number, patch: Partial<ImageUpstreamChannelForm>): void
}>()

const channelTypeOptions: SelectOption[] = [
  { value: 'chatgpt2api', label: 'chatgpt2api' },
  { value: 'openai', label: 'OpenAI' },
]

/**
 * 向父级提交局部更新，避免子组件直接修改渠道对象。
 */
function updateChannel(index: number, patch: Partial<ImageUpstreamChannelForm>): void {
  emit('update-channel', index, patch)
}

/**
 * 切换渠道类型时同步默认名称，只在名称为空或仍是旧默认名时替换。
 */
function handleTypeChange(index: number, value: SelectOption['value']): void {
  const type = normalizeType(String(value ?? ''))
  const current = props.channels[index]
  const patch: Partial<ImageUpstreamChannelForm> = { type }
  if (current?.type !== type) {
    // 渠道类型变更后不能沿用旧密钥；换新 ID 可避免后端按旧 ID 继承其他渠道密钥。
    patch.id = createUpstreamChannelID(type)
    patch.auth_key = ''
    patch.clear_auth_key = false
    patch.auth_key_configured = false
  }
  if (!current?.name || current.name === channelTypeDefaultName(current.type)) {
    patch.name = channelTypeDefaultName(type)
  }
  updateChannel(index, patch)
}

/**
 * 输入新密钥时取消清空标记，避免用户误把新密钥和清空动作一起提交。
 */
function handleAuthKeyInput(index: number, event: Event): void {
  const value = eventValue(event)
  updateChannel(index, {
    auth_key: value,
    clear_auth_key: value ? false : props.channels[index]?.clear_auth_key ?? false,
  })
}

/**
 * 勾选清空密钥时同步清空本次输入框，保持保存语义唯一。
 */
function handleClearAuthKeyChange(index: number, event: Event): void {
  const clearAuthKey = eventChecked(event)
  updateChannel(index, {
    clear_auth_key: clearAuthKey,
    auth_key: clearAuthKey ? '' : props.channels[index]?.auth_key ?? '',
  })
}

/**
 * 渠道类型展示文案。
 */
function channelTypeLabel(type: ImageUpstreamChannelType): string {
  return type === 'openai' ? 'OpenAI' : 'chatgpt2api'
}

/**
 * 不同渠道的默认 Base URL 占位。
 */
function baseURLPlaceholder(type: ImageUpstreamChannelType): string {
  return type === 'openai' ? 'https://api.openai.com/v1' : 'http://127.0.0.1:8000'
}

/**
 * 从 DOM 事件读取字符串值。
 */
function eventValue(event: Event): string {
  return event.target instanceof HTMLInputElement ? event.target.value : ''
}

/**
 * 从 DOM 事件读取勾选状态。
 */
function eventChecked(event: Event): boolean {
  return event.target instanceof HTMLInputElement ? event.target.checked : false
}

/**
 * 从 DOM 事件读取非负整数；空值先交给保存层兜底。
 */
function eventNonNegativeInteger(event: Event): number {
  const value = Number(eventValue(event))
  return Number.isFinite(value) && value >= 0 ? Math.trunc(value) : 0
}

/**
 * priority 允许 0 和负数；空输入时沿用旧值，避免用户编辑过程中跳回默认值。
 */
function eventInteger(event: Event, fallback: number): number {
  const value = Number(eventValue(event))
  return Number.isFinite(value) ? Math.trunc(value) : fallback
}

/**
 * 将未知类型收敛到当前后端支持的渠道类型。
 */
function normalizeType(type: string): ImageUpstreamChannelType {
  return type === 'openai' ? 'openai' : 'chatgpt2api'
}

/**
 * 生成新的渠道 ID，用于新增渠道或类型变更后的密钥隔离。
 */
function createUpstreamChannelID(type: ImageUpstreamChannelType): string {
  const randomPart = Math.random().toString(36).slice(2, 8)
  return `${type}-${Date.now().toString(36)}-${randomPart}`
}

/**
 * 渠道类型默认名称。
 */
function channelTypeDefaultName(type: ImageUpstreamChannelType): string {
  return type === 'openai' ? 'OpenAI' : 'chatgpt2api'
}
</script>
