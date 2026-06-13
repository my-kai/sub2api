<template>
  <article
    class="group rounded-3xl border border-gray-200 bg-white p-5 shadow-sm transition hover:-translate-y-0.5 hover:shadow-md dark:border-dark-700 dark:bg-dark-900"
    :class="{ 'lg:flex lg:items-start lg:gap-5': viewMode === 'list' }"
  >
    <div class="flex min-w-0 flex-1 items-start gap-4">
      <div
        class="flex h-12 w-12 flex-shrink-0 items-center justify-center rounded-2xl"
        :class="platformBadgeClass(model.platform)"
      >
        <PlatformIcon :platform="model.platform as GroupPlatform" size="lg" />
      </div>

      <div class="min-w-0 flex-1">
        <div class="flex items-start justify-between gap-3">
          <div class="min-w-0">
            <h3 class="truncate text-lg font-semibold text-gray-950 dark:text-white" :title="model.modelName">
              {{ model.modelName }}
            </h3>
            <div class="mt-1 flex flex-wrap items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
              <span>{{ platformLabel(model.platform) }}</span>
              <span class="h-1 w-1 rounded-full bg-gray-300 dark:bg-dark-600"></span>
              <span>{{ model.channelName }}</span>
            </div>
          </div>

          <div class="flex flex-shrink-0 items-center gap-2">
            <button
              type="button"
              class="rounded-full border border-gray-200 px-3 py-1.5 text-xs font-medium text-gray-600 transition hover:border-gray-300 hover:text-gray-950 dark:border-dark-700 dark:text-gray-300 dark:hover:text-white"
              @click="detailsOpen = !detailsOpen"
            >
              详情
            </button>
            <button
              type="button"
              class="rounded-full border border-gray-200 p-2 text-gray-500 transition hover:border-gray-300 hover:text-gray-950 dark:border-dark-700 dark:text-gray-300 dark:hover:text-white"
              title="复制模型名称"
              @click="copyModelName"
            >
              <Icon name="copy" size="sm" />
            </button>
          </div>
        </div>

        <div class="mt-3 flex flex-wrap gap-x-4 gap-y-1.5 text-sm">
          <template v-for="row in priceRows" :key="row.label">
            <span class="text-gray-500 dark:text-gray-400">{{ row.label }}</span>
            <span class="font-semibold text-gray-950 dark:text-white">{{ row.value }}</span>
          </template>
        </div>

        <p class="mt-5 line-clamp-2 min-h-[2.5rem] text-sm leading-6 text-gray-600 dark:text-gray-300">
          {{ model.description || '暂无描述。' }}
        </p>

        <div class="mt-5 flex flex-wrap items-center gap-2">
          <span
            v-for="group in visibleGroups"
            :key="group.id"
            class="rounded-full bg-gray-100 px-2.5 py-1 text-xs font-medium text-gray-600 dark:bg-dark-800 dark:text-gray-300"
          >
            {{ group.name }}{{ group.rate_multiplier && group.rate_multiplier !== 1 ? ` x${group.rate_multiplier}` : '' }}
          </span>
          <span v-if="hiddenGroupCount > 0" class="rounded-full bg-gray-100 px-2.5 py-1 text-xs text-gray-500 dark:bg-dark-800 dark:text-gray-400">
            +{{ hiddenGroupCount }}
          </span>
          <span
            v-for="tag in model.tags"
            :key="tag"
            class="rounded-full border border-gray-200 px-2.5 py-1 text-xs text-gray-500 dark:border-dark-700 dark:text-gray-400"
          >
            {{ tag }}
          </span>
        </div>

        <div v-if="detailsOpen" class="mt-5 rounded-2xl bg-gray-50 p-4 text-sm dark:bg-dark-800/70">
          <div class="grid gap-3 sm:grid-cols-2">
            <DetailItem label="供应商" :value="platformLabel(model.platform)" />
            <DetailItem label="计费类型" :value="billingModeLabel(model.pricing?.billing_mode)" />
            <DetailItem label="渠道" :value="model.channelName" />
            <DetailItem label="分组" :value="model.groups.map((group) => group.name).join('、') || '-'" />
          </div>
        </div>
      </div>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, ref } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import type { GroupPlatform } from '@/types'
import { useClipboard } from '@/composables/useClipboard'
import {
  BILLING_MODE_IMAGE,
  BILLING_MODE_PER_REQUEST,
} from '@/constants/channel'
import { platformBadgeClass, platformLabel } from '@/utils/platformColors'
import { formatScaled } from '@/utils/pricing'
import {
  billingModeLabel,
  formatTokenPrice,
  priceUnitLabel,
} from '../marketplace'
import type { MarketplaceModel, MarketplaceViewMode, PriceUnit } from '../types'

const props = defineProps<{
  model: MarketplaceModel
  priceUnit: PriceUnit
  viewMode: MarketplaceViewMode
}>()

const { copyToClipboard } = useClipboard()
const detailsOpen = ref(false)

const visibleGroups = computed(() => props.model.groups.slice(0, 2))
const hiddenGroupCount = computed(() => Math.max(0, props.model.groups.length - visibleGroups.value.length))

const priceRows = computed(() => {
  const pricing = props.model.pricing
  if (!pricing) return [{ label: '定价', value: '未配置' }]
  if (pricing.billing_mode === BILLING_MODE_PER_REQUEST) {
    return [{ label: '请求', value: `${formatScaled(pricing.per_request_price, 1)}/次` }]
  }
  if (pricing.billing_mode === BILLING_MODE_IMAGE) {
    return [{ label: '图片', value: `${formatScaled(pricing.image_output_price, 1)}/张` }]
  }
  return [
    { label: '输入', value: `${formatTokenPrice(pricing.input_price, props.priceUnit)}${priceUnitLabel(props.priceUnit)}` },
    { label: '输出', value: `${formatTokenPrice(pricing.output_price, props.priceUnit)}${priceUnitLabel(props.priceUnit)}` },
    ...(pricing.cache_read_price != null
      ? [{ label: '缓存', value: `${formatTokenPrice(pricing.cache_read_price, props.priceUnit)}${priceUnitLabel(props.priceUnit)}` }]
      : []),
  ]
})

/**
 * 复制模型名，便于用户直接粘贴到 API 请求或客户端配置里。
 */
async function copyModelName(): Promise<void> {
  await copyToClipboard(props.model.modelName, '模型名称已复制')
}

/**
 * 详情键值项。
 */
const DetailItem = defineComponent({
  name: 'DetailItem',
  props: {
    label: { type: String, required: true },
    value: { type: String, required: true },
  },
  setup(itemProps) {
    return () => h('div', [
      h('div', { class: 'text-xs text-gray-500 dark:text-gray-400' }, itemProps.label),
      h('div', { class: 'mt-1 break-words font-medium text-gray-900 dark:text-white' }, itemProps.value),
    ])
  },
})
</script>
