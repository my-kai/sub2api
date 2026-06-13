<template>
  <AppLayout>
    <div class="grid gap-5 xl:grid-cols-[280px_minmax(0,1fr)]">
      <FilterSidebar
        v-model:group-id="filters.groupId"
        v-model:provider="filters.provider"
        v-model:tag="filters.tag"
        v-model:pricing-type="filters.pricingType"
        v-model:endpoint-type="filters.endpointType"
        :group-options="filterOptions.groups"
        :provider-options="filterOptions.providers"
        :tag-options="filterOptions.tags"
        :pricing-options="filterOptions.pricingTypes"
        :endpoint-options="filterOptions.endpointTypes"
        @reset="resetFilters"
      />

      <section class="min-w-0 space-y-5">
        <div class="rounded-3xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-900">
          <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
            <div class="flex items-center gap-3">
              <div class="flex h-10 w-10 items-center justify-center rounded-2xl bg-primary-500/10 text-primary-600 dark:text-primary-400">
                <Icon name="cube" size="lg" />
              </div>
              <div>
                <h1 class="text-lg font-semibold text-gray-950 dark:text-white">{{ sortedModels.length }} 个模型</h1>
                <p class="text-sm text-gray-500 dark:text-gray-400">模型广场</p>
              </div>
            </div>

            <div class="flex flex-col gap-3 sm:flex-row sm:items-center">
              <div class="relative w-full sm:w-72">
                <Icon name="search" size="sm" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
                <input
                  v-model="searchQuery"
                  type="text"
                  class="input pl-9"
                  placeholder="搜索模型、渠道或分组"
                />
              </div>

              <div class="flex flex-wrap items-center gap-2">
                <button
                  type="button"
                  class="segmented-btn"
                  :class="{ 'segmented-btn-active': priceUnit === '1m' }"
                  @click="priceUnit = '1m'"
                >
                  /1M
                </button>
                <button
                  type="button"
                  class="segmented-btn"
                  :class="{ 'segmented-btn-active': priceUnit === '1k' }"
                  @click="priceUnit = '1k'"
                >
                  /1K
                </button>
                <button type="button" class="segmented-btn" @click="toggleSortMode">
                  <Icon name="arrowsUpDown" size="xs" />
                  {{ sortLabel }}
                </button>
                <button type="button" class="segmented-btn" :disabled="loading" @click="loadChannels">
                  <Icon name="refresh" size="xs" :class="{ 'animate-spin': loading }" />
                  刷新
                </button>
                <button type="button" class="icon-toggle" :class="{ 'icon-toggle-active': viewMode === 'grid' }" @click="viewMode = 'grid'">
                  <Icon name="grid" size="sm" />
                </button>
                <button type="button" class="icon-toggle" :class="{ 'icon-toggle-active': viewMode === 'list' }" @click="viewMode = 'list'">
                  <Icon name="menu" size="sm" />
                </button>
              </div>
            </div>
          </div>
        </div>

        <div v-if="loading && sortedModels.length === 0" class="grid gap-5 lg:grid-cols-2 2xl:grid-cols-3">
          <div v-for="index in 6" :key="index" class="h-64 animate-pulse rounded-3xl bg-gray-100 dark:bg-dark-800"></div>
        </div>

        <div v-else-if="sortedModels.length === 0" class="rounded-3xl border border-dashed border-gray-300 bg-white p-12 text-center dark:border-dark-700 dark:bg-dark-900">
          <Icon name="inbox" size="xl" class="mx-auto text-gray-400" />
          <h2 class="mt-3 text-base font-semibold text-gray-900 dark:text-white">暂无可用模型</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">换个筛选条件试试。</p>
        </div>

        <div
          v-else
          class="grid gap-5"
          :class="viewMode === 'grid' ? 'lg:grid-cols-2 2xl:grid-cols-3' : 'grid-cols-1'"
        >
          <ModelCard
            v-for="model in sortedModels"
            :key="model.id"
            :model="model"
            :price-unit="priceUnit"
            :view-mode="viewMode"
          />
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import userChannelsAPI, { type UserAvailableChannel } from '@/api/channels'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import FilterSidebar from '../components/FilterSidebar.vue'
import ModelCard from '../components/ModelCard.vue'
import {
  ALL_FILTER_ID,
  buildMarketplaceFilterOptions,
  filterMarketplaceModels,
  flattenAvailableChannels,
  sortMarketplaceModels,
} from '../marketplace'
import type {
  MarketplaceFilterState,
  MarketplaceSortMode,
  MarketplaceViewMode,
  PriceUnit,
} from '../types'

const appStore = useAppStore()
const channels = ref<UserAvailableChannel[]>([])
const loading = ref(false)
const searchQuery = ref('')
const priceUnit = ref<PriceUnit>('1m')
const sortMode = ref<MarketplaceSortMode>('name')
const viewMode = ref<MarketplaceViewMode>('grid')
let controller: AbortController | null = null

const filters = reactive<MarketplaceFilterState>({
  groupId: ALL_FILTER_ID,
  provider: ALL_FILTER_ID,
  tag: ALL_FILTER_ID,
  pricingType: ALL_FILTER_ID,
  endpointType: ALL_FILTER_ID,
})

const marketplaceModels = computed(() => flattenAvailableChannels(channels.value))
const filterOptions = computed(() => buildMarketplaceFilterOptions(marketplaceModels.value))
const filteredModels = computed(() => filterMarketplaceModels(marketplaceModels.value, filters, searchQuery.value))
const sortedModels = computed(() => sortMarketplaceModels(filteredModels.value, sortMode.value))
const sortLabel = computed(() => {
  if (sortMode.value === 'input_price') return '输入价'
  if (sortMode.value === 'output_price') return '输出价'
  return '名称'
})

onMounted(() => {
  void loadChannels()
})

onBeforeUnmount(() => {
  controller?.abort()
})

/**
 * 读取用户可见渠道，并在前端转换为模型广场卡片。
 */
async function loadChannels(): Promise<void> {
  controller?.abort()
  controller = new AbortController()
  loading.value = true
  try {
    channels.value = await userChannelsAPI.getAvailable({ signal: controller.signal })
  } catch (err) {
    if ((err as { name?: string }).name !== 'AbortError') {
      appStore.showError(extractApiErrorMessage(err, '模型列表读取失败'))
    }
  } finally {
    if (!controller.signal.aborted) loading.value = false
  }
}

/**
 * 重置全部筛选条件，保留搜索词让用户可以继续缩小关键词范围。
 */
function resetFilters(): void {
  filters.groupId = ALL_FILTER_ID
  filters.provider = ALL_FILTER_ID
  filters.tag = ALL_FILTER_ID
  filters.pricingType = ALL_FILTER_ID
  filters.endpointType = ALL_FILTER_ID
}

/**
 * 排序按钮在三个常用维度间循环，避免顶部操作区塞入下拉框。
 */
function toggleSortMode(): void {
  sortMode.value = sortMode.value === 'name'
    ? 'input_price'
    : sortMode.value === 'input_price'
      ? 'output_price'
      : 'name'
}
</script>

<style scoped>
.segmented-btn {
  @apply inline-flex h-9 items-center gap-1.5 rounded-full border border-gray-200 bg-white px-3 text-sm font-medium text-gray-600 transition hover:border-gray-300 hover:text-gray-950 disabled:cursor-not-allowed disabled:opacity-60 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-300 dark:hover:text-white;
}

.segmented-btn-active {
  @apply border-gray-950 bg-gray-950 text-white hover:text-white dark:border-white dark:bg-white dark:text-gray-950;
}

.icon-toggle {
  @apply inline-flex h-9 w-9 items-center justify-center rounded-full border border-gray-200 bg-white text-gray-500 transition hover:border-gray-300 hover:text-gray-950 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-300 dark:hover:text-white;
}

.icon-toggle-active {
  @apply border-gray-950 bg-gray-950 text-white hover:text-white dark:border-white dark:bg-white dark:text-gray-950;
}
</style>
