<template>
  <AppLayout>
    <div class="mx-auto max-w-6xl space-y-5">
      <header class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <div class="flex items-center gap-2">
            <Icon name="cog" size="lg" class="text-primary-500" />
            <h1 class="text-lg font-semibold text-gray-900 dark:text-white">生图配置</h1>
          </div>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">管理生图并发、价格和用户覆盖。</p>
        </div>
        <button type="button" class="btn btn-secondary" :disabled="loading" @click="loadAdminImages">
          <Icon v-if="loading" name="refresh" size="sm" class="animate-spin" />
          <Icon v-else name="refresh" size="sm" />
          <span>刷新</span>
        </button>
      </header>

      <div v-if="error" class="rounded-xl border border-amber-200 bg-amber-50 p-4 text-sm text-amber-700 dark:border-amber-900/40 dark:bg-amber-900/20 dark:text-amber-300">
        {{ error }}
      </div>

      <section class="rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900">
        <div class="border-b border-gray-100 px-5 py-4 dark:border-dark-700">
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">平台配置</h2>
        </div>

        <form class="space-y-5 p-5" @submit.prevent="handleSaveConfig">
          <div v-if="loading" class="grid gap-4 md:grid-cols-3">
            <div v-for="index in 3" :key="index" class="h-24 animate-pulse rounded-xl bg-gray-100 dark:bg-dark-800"></div>
          </div>

          <div class="rounded-2xl border border-gray-200 p-4 dark:border-dark-700">
            <div class="flex items-center justify-between gap-4">
              <div>
                <h3 class="text-sm font-semibold text-gray-900 dark:text-white">启用生图</h3>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">关闭后用户不能创建新的生图任务。</p>
              </div>
              <Toggle v-model="configForm.enabled" />
            </div>
          </div>

          <ImageUpstreamChannelList
            :channels="configForm.upstream_channels"
            :saving="savingConfig"
            @add-channel="addUpstreamChannel"
            @move-channel="moveUpstreamChannel"
            @remove-channel="removeUpstreamChannel"
            @update-channel="updateUpstreamChannel"
          />

          <div class="grid gap-4 md:grid-cols-3">
            <label class="space-y-1 text-sm">
              <span class="text-gray-600 dark:text-gray-300">平台总并发</span>
              <input v-model.number="configForm.platform_concurrency" type="number" min="1" class="input" required />
            </label>
            <label class="space-y-1 text-sm">
              <span class="text-gray-600 dark:text-gray-300">默认用户并发</span>
              <input v-model.number="configForm.default_user_concurrency" type="number" min="1" class="input" required />
            </label>
            <label class="space-y-1 text-sm">
              <span class="text-gray-600 dark:text-gray-300">保留天数</span>
              <input v-model.number="configForm.retention_days" type="number" min="1" class="input" required />
            </label>
          </div>

          <div class="rounded-2xl border border-gray-200 p-4 dark:border-dark-700">
            <h3 class="text-sm font-semibold text-gray-900 dark:text-white">图片价格</h3>
            <div class="mt-4 grid gap-4 md:grid-cols-3">
              <label class="space-y-1 text-sm">
                <span class="text-gray-600 dark:text-gray-300">1K</span>
                <input v-model.number="configForm.unit_price_1k" type="number" min="0" step="0.00001" class="input" required />
              </label>
              <label class="space-y-1 text-sm">
                <span class="text-gray-600 dark:text-gray-300">2K</span>
                <input v-model.number="configForm.unit_price_2k" type="number" min="0" step="0.00001" class="input" required />
              </label>
              <label class="space-y-1 text-sm">
                <span class="text-gray-600 dark:text-gray-300">4K</span>
                <input v-model.number="configForm.unit_price_4k" type="number" min="0" step="0.00001" class="input" required />
              </label>
            </div>
          </div>

          <div class="flex justify-end">
            <button type="submit" class="btn btn-primary" :disabled="savingConfig">
              <Icon v-if="savingConfig" name="refresh" size="sm" class="animate-spin" />
              <Icon v-else name="check" size="sm" />
              <span>保存配置</span>
            </button>
          </div>
        </form>
      </section>

      <section class="rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900">
        <div class="flex flex-col gap-3 border-b border-gray-100 px-5 py-4 dark:border-dark-700 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">用户并发覆盖</h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">为指定用户设置独立并发；删除后恢复默认用户并发。</p>
          </div>
          <button type="button" class="btn btn-primary" @click="openCreateLimitForm">
            <Icon name="plus" size="sm" />
            <span>新增用户覆盖</span>
          </button>
        </div>

        <div class="p-5">
          <div v-if="loading && limits.length === 0" class="space-y-3">
            <div v-for="index in 4" :key="index" class="h-16 animate-pulse rounded-xl bg-gray-100 dark:bg-dark-800"></div>
          </div>

          <div v-else-if="limits.length === 0" class="rounded-2xl border border-dashed border-gray-300 p-10 text-center dark:border-dark-700">
            <Icon name="users" size="xl" class="mx-auto text-gray-400" />
            <h3 class="mt-3 text-base font-semibold text-gray-900 dark:text-white">暂无用户覆盖</h3>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">添加后会显示在这里。</p>
          </div>

          <div v-else class="overflow-x-auto">
            <table class="w-full min-w-[720px] text-left text-sm">
              <thead>
                <tr class="border-b border-gray-200 text-gray-500 dark:border-dark-700 dark:text-gray-400">
                  <th class="px-3 py-2 font-medium">用户 ID</th>
                  <th class="px-3 py-2 font-medium">用户</th>
                  <th class="px-3 py-2 font-medium">并发数</th>
                  <th class="px-3 py-2 font-medium">更新时间</th>
                  <th class="px-3 py-2 text-right font-medium">操作</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="limit in limits" :key="limit.user_id" class="border-b border-gray-100 last:border-b-0 dark:border-dark-800">
                  <td class="px-3 py-3 text-gray-900 dark:text-white">{{ limit.user_id }}</td>
                  <td class="px-3 py-3">
                    <div class="font-medium text-gray-900 dark:text-white">{{ limit.username || `用户 ${limit.user_id}` }}</div>
                    <div class="text-xs text-gray-500 dark:text-gray-400">{{ limit.email || '暂无邮箱' }}</div>
                  </td>
                  <td class="px-3 py-3 text-gray-700 dark:text-gray-300">{{ limit.concurrency }}</td>
                  <td class="px-3 py-3 text-gray-700 dark:text-gray-300">{{ formatDateTime(limit.updated_at) }}</td>
                  <td class="px-3 py-3">
                    <div class="flex justify-end gap-2">
                      <button type="button" class="btn btn-secondary btn-sm" @click="openEditLimitForm(limit)">编辑</button>
                      <button
                        type="button"
                        class="btn btn-secondary btn-sm text-red-600 hover:text-red-700 dark:text-red-400"
                        :disabled="deletingUserId === limit.user_id"
                        @click="handleDeleteLimit(limit.user_id)"
                      >
                        <Icon v-if="deletingUserId === limit.user_id" name="refresh" size="sm" class="animate-spin" />
                        <Icon v-else name="trash" size="sm" />
                        <span>删除</span>
                      </button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </section>

      <section v-if="limitFormOpen" class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900">
        <div class="flex items-center justify-between gap-3">
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ editingLimit ? '编辑用户覆盖' : '新增用户覆盖' }}</h2>
          <button type="button" class="rounded-lg p-1 text-gray-500 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-800" @click="closeLimitForm">
            <Icon name="x" size="sm" />
          </button>
        </div>

        <form class="mt-4 grid gap-4 lg:grid-cols-[minmax(0,1fr)_12rem_auto]" @submit.prevent="handleSaveLimit">
          <div class="space-y-2">
            <label class="block text-sm text-gray-600 dark:text-gray-300" for="image-user-search">用户</label>
            <input
              id="image-user-search"
              v-model="userQuery"
              class="input"
              type="search"
              :disabled="Boolean(editingLimit)"
              placeholder="输入 ID、邮箱或用户名搜索"
              @input="scheduleUserSearch"
              @focus="ensureUserOptions"
            />
            <div v-if="selectedUserLabel" class="text-xs text-gray-500 dark:text-gray-400">已选择：{{ selectedUserLabel }}</div>
            <div v-if="!editingLimit" class="max-h-44 overflow-y-auto rounded-xl border border-gray-200 dark:border-dark-700">
              <button
                v-for="user in userOptions"
                :key="user.id"
                type="button"
                class="block w-full px-3 py-2 text-left text-sm hover:bg-gray-50 dark:hover:bg-dark-800"
                :class="user.id === limitForm.user_id ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300' : 'text-gray-700 dark:text-gray-300'"
                @click="selectUserOption(user)"
              >
                {{ formatUserOptionLabel(user) }}
              </button>
              <div v-if="userSearchLoading" class="px-3 py-2 text-sm text-gray-500 dark:text-gray-400">搜索中</div>
              <div v-else-if="userOptions.length === 0" class="px-3 py-2 text-sm text-gray-500 dark:text-gray-400">暂无匹配用户</div>
            </div>
          </div>

          <label class="space-y-2 text-sm">
            <span class="text-gray-600 dark:text-gray-300">并发数</span>
            <input v-model.number="limitForm.concurrency" type="number" min="1" class="input" required />
          </label>

          <div class="flex items-end gap-2">
            <button type="submit" class="btn btn-primary" :disabled="savingLimit">
              <Icon v-if="savingLimit" name="refresh" size="sm" class="animate-spin" />
              <Icon v-else name="check" size="sm" />
              <span>保存</span>
            </button>
            <button type="button" class="btn btn-secondary" @click="closeLimitForm">取消</button>
          </div>
        </form>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import Toggle from '@/components/common/Toggle.vue'
import ImageUpstreamChannelList from '../components/ImageUpstreamChannelList.vue'
import { useImageQueueAdmin } from '../composables/useImageQueueAdmin'

// 页面只做表单和表格布局；配置保存、用户搜索等状态集中在 custom composable。
const {
  addUpstreamChannel,
  closeLimitForm,
  configForm,
  deletingUserId,
  editingLimit,
  ensureUserOptions,
  error,
  formatDateTime,
  formatUserOptionLabel,
  handleDeleteLimit,
  handleSaveConfig,
  handleSaveLimit,
  limitForm,
  limitFormOpen,
  limits,
  loadAdminImages,
  loading,
  moveUpstreamChannel,
  openCreateLimitForm,
  openEditLimitForm,
  removeUpstreamChannel,
  savingConfig,
  savingLimit,
  scheduleUserSearch,
  selectUserOption,
  selectedUserLabel,
  updateUpstreamChannel,
  userOptions,
  userQuery,
  userSearchLoading,
} = useImageQueueAdmin()
</script>
