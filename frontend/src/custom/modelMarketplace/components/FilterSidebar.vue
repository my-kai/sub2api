<template>
  <aside class="rounded-3xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900">
    <div class="mb-5 space-y-2">
      <div class="flex items-center justify-between gap-3">
        <h2 class="text-base font-semibold text-gray-900 dark:text-white">筛选</h2>
        <button
          type="button"
          class="inline-flex shrink-0 items-center gap-1 whitespace-nowrap rounded-full px-2.5 py-1 text-xs font-medium text-gray-500 hover:bg-gray-100 hover:text-gray-900 dark:text-gray-400 dark:hover:bg-dark-800 dark:hover:text-white"
          @click="emit('reset')"
        >
          <Icon name="refresh" size="xs" />
          重置
        </button>
      </div>
      <p class="text-sm leading-6 text-gray-500 dark:text-gray-400">按供应商、分组、类型细化模型。</p>
    </div>

    <div class="space-y-5">
      <FilterSection
        title="分组"
        :options="groupOptions"
        :model-value="groupId"
        @update:model-value="emit('update:groupId', $event)"
      />
      <FilterSection
        title="所有供应商"
        :options="providerOptions"
        :model-value="provider"
        @update:model-value="emit('update:provider', $event)"
      />
      <FilterSection
        title="模型标签"
        :options="tagOptions"
        :model-value="tag"
        @update:model-value="emit('update:tag', $event)"
      />
      <FilterSection
        title="定价类型"
        :options="pricingOptions"
        :model-value="pricingType"
        @update:model-value="emit('update:pricingType', $event)"
      />
      <FilterSection
        title="端点类型"
        :options="endpointOptions"
        :model-value="endpointType"
        @update:model-value="emit('update:endpointType', $event)"
      />
    </div>
  </aside>
</template>

<script setup lang="ts">
import { defineComponent, h, type PropType } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import type { MarketplaceFilterOption } from '../types'

defineProps<{
  groupId: string
  provider: string
  tag: string
  pricingType: string
  endpointType: string
  groupOptions: MarketplaceFilterOption[]
  providerOptions: MarketplaceFilterOption[]
  tagOptions: MarketplaceFilterOption[]
  pricingOptions: MarketplaceFilterOption[]
  endpointOptions: MarketplaceFilterOption[]
}>()

const emit = defineEmits<{
  'update:groupId': [value: string]
  'update:provider': [value: string]
  'update:tag': [value: string]
  'update:pricingType': [value: string]
  'update:endpointType': [value: string]
  reset: []
}>()

/**
 * 单个筛选分区。
 *
 * 写成局部组件是为了让页面保留紧凑结构，同时避免每组筛选都复制一套按钮渲染逻辑。
 */
const FilterSection = defineComponent({
  name: 'FilterSection',
  props: {
    title: { type: String, required: true },
    options: { type: Array as PropType<MarketplaceFilterOption[]>, required: true },
    modelValue: { type: String, required: true },
  },
  emits: ['update:modelValue'],
  setup(props, { emit }) {
    return () => h('section', { class: 'border-t border-gray-100 pt-4 first:border-t-0 first:pt-0 dark:border-dark-800' }, [
      h('div', { class: 'mb-3 flex items-center justify-between' }, [
        h('h3', { class: 'text-sm font-semibold text-gray-900 dark:text-white' }, props.title),
        h('span', { class: 'text-gray-400' }, '⌃'),
      ]),
      h('div', { class: 'flex flex-wrap gap-2' }, props.options.map((option) =>
        h(
          'button',
          {
            type: 'button',
            class: [
              'inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-xs font-medium transition',
              option.id === props.modelValue
                ? 'border-gray-300 bg-white text-gray-950 shadow-sm dark:border-dark-500 dark:bg-dark-800 dark:text-white'
                : 'border-gray-200 bg-gray-50 text-gray-600 hover:border-gray-300 hover:text-gray-900 dark:border-dark-700 dark:bg-dark-800/50 dark:text-gray-400 dark:hover:text-white',
            ],
            onClick: () => emit('update:modelValue', option.id),
          },
          [
            h('span', option.label),
            h('span', { class: 'rounded-full bg-gray-200/70 px-1.5 py-0.5 text-[10px] text-gray-500 dark:bg-dark-700 dark:text-gray-400' }, String(option.count)),
          ],
        ),
      )),
    ])
  },
})
</script>
