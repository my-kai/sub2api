<template>
  <AppLayout>
    <div class="flex h-[calc(100vh-6rem)] min-h-0 flex-col gap-5 overflow-hidden md:h-[calc(100vh-7rem)] lg:h-[calc(100vh-8rem)]">
      <div v-if="error" class="rounded-xl border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-900/40 dark:bg-red-900/20 dark:text-red-300">
        {{ error }}
      </div>

      <div v-if="imageGenerationStatusLoading" class="flex min-h-0 flex-1 items-center justify-center rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900">
        <div class="text-center">
          <Icon name="refresh" size="xl" class="mx-auto animate-spin text-gray-400" />
          <p class="mt-3 text-sm text-gray-500 dark:text-gray-400">加载中</p>
        </div>
      </div>

      <div v-else-if="!imageGenerationEnabled" class="flex min-h-0 flex-1 items-center justify-center rounded-2xl border border-dashed border-gray-300 bg-white p-8 text-center shadow-sm dark:border-dark-700 dark:bg-dark-900">
        <div>
          <Icon name="sparkles" size="xl" class="mx-auto text-gray-400" />
          <h1 class="mt-3 text-base font-semibold text-gray-900 dark:text-white">生图功能已关闭</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">暂时不能创建新的生图任务。</p>
        </div>
      </div>

      <div v-else class="grid min-h-0 flex-1 gap-5 overflow-hidden xl:grid-cols-[18rem_minmax(0,1fr)_22rem]">
        <SessionSelector
          :sessions="sessions"
          :selected-session-id="selectedSessionId"
          :loading="sessionsLoading"
          :creating="creatingSession"
          :deleting-session-id="deletingSessionId"
          @create="handleCreateSession"
          @select="handleSelectSession"
          @delete="handleDeleteSession"
        />

        <section class="flex h-full min-h-0 flex-col overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900">
          <div class="flex flex-col gap-3 border-b border-gray-100 p-5 dark:border-dark-700 lg:flex-row lg:items-center lg:justify-between">
            <div>
              <div class="flex items-center gap-2">
                <Icon name="sparkles" size="lg" class="text-primary-500" />
                <h1 class="text-lg font-semibold text-gray-900 dark:text-white">AI 生图</h1>
                <span class="rounded-full bg-blue-50 px-2.5 py-1 text-xs font-medium text-blue-700 dark:bg-blue-900/20 dark:text-blue-300">
                  {{ selectedModel }}
                </span>
              </div>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">输入提示词创建图片，也可以上传图片做编辑。</p>
            </div>

            <div class="flex flex-wrap items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
              <span v-if="taskEventsConnected" class="rounded-full bg-emerald-50 px-2.5 py-1 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300">
                实时更新
              </span>
              <span v-else-if="taskEventsFallback" class="rounded-full bg-amber-50 px-2.5 py-1 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300">
                备用刷新
              </span>
              <button type="button" class="btn btn-secondary btn-sm" :disabled="tasksLoading || !selectedSessionId" @click="reloadCurrentTasks">
                <Icon v-if="tasksLoading" name="refresh" size="sm" class="animate-spin" />
                <Icon v-else name="refresh" size="sm" />
                <span>刷新</span>
              </button>
            </div>
          </div>

          <div ref="taskScrollRef" class="min-h-0 flex-1 overflow-y-auto p-5">
            <div v-if="tasksLoading && tasks.length === 0" class="space-y-3">
              <div v-for="index in 4" :key="index" class="h-28 animate-pulse rounded-2xl bg-gray-100 dark:bg-dark-800"></div>
            </div>

            <div v-else-if="!selectedSessionId" class="flex h-full min-h-[360px] items-center justify-center rounded-2xl border border-dashed border-gray-300 p-8 text-center dark:border-dark-700">
              <div>
                <Icon name="chat" size="xl" class="mx-auto text-gray-400" />
                <h2 class="mt-3 text-base font-semibold text-gray-900 dark:text-white">还没有会话</h2>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">新建会话后开始生成图片。</p>
              </div>
            </div>

            <div v-else-if="tasks.length === 0" class="flex h-full min-h-[360px] items-center justify-center rounded-2xl border border-dashed border-gray-300 p-8 text-center dark:border-dark-700">
              <div>
                <Icon name="sparkles" size="xl" class="mx-auto text-gray-400" />
                <h2 class="mt-3 text-base font-semibold text-gray-900 dark:text-white">暂无任务</h2>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">写下提示词，提交后会显示在这里。</p>
              </div>
            </div>

            <div v-else class="space-y-4">
              <article
                v-for="task in orderedTasks"
                :key="task.id"
                class="rounded-2xl border border-gray-200 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800/50"
              >
                <div class="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
                  <div class="min-w-0 space-y-2">
                    <div class="flex flex-wrap items-center gap-2">
                      <span class="font-semibold text-gray-900 dark:text-white">任务 #{{ task.id }}</span>
                      <TaskStatusBadge :status="task.status" :queue-position="task.queue_position" />
                      <span class="rounded-full bg-gray-100 px-2 py-1 text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300">
                        {{ taskModeLabel(task) }}
                      </span>
                    </div>
                    <p class="break-words text-sm text-gray-700 dark:text-gray-300">{{ task.prompt }}</p>
                    <p class="text-xs text-gray-500 dark:text-gray-400">
                      {{ task.model }} · {{ task.n }} 张 · {{ formatTaskCharge(task) }} · {{ formatDateTime(task.created_at) }}
                    </p>
                  </div>

                  <div class="flex flex-wrap items-center gap-2">
                    <button
                      v-if="task.status === 'failed'"
                      type="button"
                      class="btn btn-secondary btn-sm"
                      :disabled="retryingTaskId === task.id"
                      @click="handleRetryTask(task.id)"
                    >
                      <Icon v-if="retryingTaskId === task.id" name="refresh" size="sm" class="animate-spin" />
                      <Icon v-else name="refresh" size="sm" />
                      <span>重试</span>
                    </button>
                    <button
                      v-if="task.status === 'queued'"
                      type="button"
                      class="btn btn-secondary btn-sm text-red-600 hover:text-red-700 dark:text-red-400"
                      :disabled="cancelingTaskId === task.id"
                      @click="handleCancelTask(task.id)"
                    >
                      <Icon v-if="cancelingTaskId === task.id" name="refresh" size="sm" class="animate-spin" />
                      <Icon v-else name="x" size="sm" />
                      <span>撤销</span>
                    </button>
                  </div>
                </div>

                <div v-if="task.status === 'completed' && taskImages(task).length > 0" class="mt-4 grid gap-3 sm:grid-cols-2 2xl:grid-cols-3">
                  <div v-for="image in taskImages(task)" :key="taskImageKey(task.id, image.imageIndex)" class="space-y-2">
                    <ImagePreview
                      :src="image.src"
                      :alt="`任务 ${task.id} 图片 ${image.imageIndex + 1}`"
                      :download-name="`image-${task.id}-${image.imageIndex + 1}.png`"
                    />
                    <div class="flex flex-wrap items-center gap-2">
                      <button
                        type="button"
                        class="btn btn-secondary btn-sm"
                        :disabled="settingCurrentImageKey === taskImageKey(task.id, image.imageIndex)"
                        @click="handleSetCurrentImage(task, image.imageIndex)"
                      >
                        <Icon v-if="settingCurrentImageKey === taskImageKey(task.id, image.imageIndex)" name="refresh" size="sm" class="animate-spin" />
                        <Icon v-else name="edit" size="sm" />
                        <span>{{ isCurrentImage(task.id, image.imageIndex) ? '当前编辑图' : '设为编辑图' }}</span>
                      </button>
                    </div>
                  </div>
                </div>

                <div v-if="task.status === 'failed'" class="mt-4 rounded-xl border border-red-200 bg-white p-3 text-sm text-red-700 dark:border-red-900/40 dark:bg-dark-900 dark:text-red-300">
                  {{ task.error_message || '生图任务执行失败' }}
                </div>
              </article>

              <Pagination
                v-if="tasksTotal > taskPageSize"
                :page="tasksPage"
                :page-size="taskPageSize"
                :total="tasksTotal"
                :show-page-size-selector="false"
                @update:page="handleTaskPageChange"
                @update:page-size="handleTaskPageSize"
              />
            </div>
          </div>

          <form class="border-t border-gray-100 p-5 dark:border-dark-700" @submit.prevent="handleGenerate">
            <div v-if="editPreviewImage" class="mb-2 flex items-center gap-2 rounded-lg border border-blue-200 bg-blue-50 px-2.5 py-2 dark:border-blue-900/40 dark:bg-blue-900/20">
              <img :src="editPreviewImage.src" :alt="editPreviewImage.alt" class="h-11 w-11 rounded-md object-cover" />
              <div class="min-w-0 flex-1">
                <div class="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5">
                  <span class="text-sm font-medium text-blue-900 dark:text-blue-100">{{ pendingEditImage ? '已选择上传图片' : '已指定编辑图片' }}</span>
                  <span class="truncate text-xs text-blue-700 dark:text-blue-300">{{ editPreviewImage.alt }}</span>
                </div>
              </div>
              <button type="button" class="rounded-md p-1 text-blue-700 hover:bg-blue-100 dark:text-blue-200 dark:hover:bg-blue-900/40" :disabled="clearingCurrentImage" @click="clearEditImage">
                <Icon v-if="clearingCurrentImage" name="refresh" size="sm" class="animate-spin" />
                <Icon v-else name="x" size="sm" />
              </button>
            </div>

            <textarea
              v-model="prompt"
              class="input min-h-[7rem] w-full resize-y"
              placeholder="描述你想生成的图片，例如：一只漂浮在太空里的猫，电影光效，细节丰富"
              :disabled="submitting"
              @paste="handlePaste"
            ></textarea>

            <div class="mt-4 flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
              <div class="flex min-w-0 flex-wrap items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
                <span class="rounded-full bg-gray-100 px-3 py-1.5 dark:bg-dark-800">
                  {{ qualityLabel(quality) }} · {{ resolutionLabel(resolution) }} · {{ aspectRatioLabel(aspectRatio) }}
                </span>
                <span class="rounded-full bg-gray-100 px-3 py-1.5 dark:bg-dark-800">
                  {{ count }} 张
                </span>
                <span class="rounded-full bg-amber-50 px-3 py-1.5 font-medium text-amber-700 dark:bg-amber-900/20 dark:text-amber-300">
                  消耗：{{ imagePriceText }}
                </span>
              </div>

              <div class="flex shrink-0 items-center gap-3">
                <ImageConfigPopup
                  v-model:selected-model="selectedModel"
                  v-model:quality="quality"
                  v-model:resolution="resolution"
                  v-model:aspect-ratio="aspectRatio"
                  v-model:count="count"
                  v-model:publish-to-gallery="publishToGallery"
                  :default-image-model-id="defaultImageModelID"
                  :clamp-image-count="clampImageCount"
                  @resolution-change="handleResolutionChange"
                />

                <input ref="fileInputRef" type="file" accept="image/*" class="hidden" @change="handleFileInput" />
                <button type="button" class="btn btn-secondary shrink-0 whitespace-nowrap" :disabled="submitting" @click="fileInputRef?.click()">
                  <Icon name="upload" size="sm" />
                  <span>上传图片</span>
                </button>
                <button type="submit" class="btn btn-primary shrink-0 whitespace-nowrap" :disabled="submitting">
                  <Icon v-if="submitting" name="refresh" size="sm" class="animate-spin" />
                  <Icon v-else name="sparkles" size="sm" />
                  <span>{{ submitting ? '提交中' : '生成' }}</span>
                </button>
              </div>
            </div>
          </form>
        </section>

        <aside class="min-h-0 space-y-5 overflow-y-auto">
          <section class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900">
            <div class="flex items-center justify-between">
              <h2 class="text-sm font-semibold text-gray-900 dark:text-white">当前编辑图</h2>
              <button v-if="selectedSession?.current_image_task_id" type="button" class="btn btn-secondary btn-sm" :disabled="clearingCurrentImage" @click="handleResetCurrentImage">
                取消指定
              </button>
            </div>
            <div class="mt-4">
              <ImagePreview
                :src="currentTaskImage?.src"
                :alt="currentTaskImage ? `编辑图片 ${currentTaskImage.task.id}-${currentTaskImage.imageIndex + 1}` : '当前编辑图'"
                :show-actions="Boolean(currentTaskImage)"
              />
            </div>
          </section>

          <section class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900">
            <h2 class="text-sm font-semibold text-gray-900 dark:text-white">生图摘要</h2>
            <dl class="mt-4 space-y-3 text-sm">
              <div class="flex justify-between gap-3">
                <dt class="text-gray-500 dark:text-gray-400">质量</dt>
                <dd class="font-medium text-gray-900 dark:text-gray-100">{{ qualityLabel(quality) }}</dd>
              </div>
              <div class="flex justify-between gap-3">
                <dt class="text-gray-500 dark:text-gray-400">尺寸</dt>
                <dd class="font-medium text-gray-900 dark:text-gray-100">{{ resolutionLabel(resolution) }} · {{ aspectRatioLabel(aspectRatio) }}</dd>
              </div>
              <div class="flex justify-between gap-3">
                <dt class="text-gray-500 dark:text-gray-400">数量</dt>
                <dd class="font-medium text-gray-900 dark:text-gray-100">{{ count }} 张</dd>
              </div>
              <div class="flex justify-between gap-3">
                <dt class="text-gray-500 dark:text-gray-400">后端尺寸</dt>
                <dd class="font-medium text-gray-900 dark:text-gray-100">{{ imageSize }}</dd>
              </div>
            </dl>
          </section>
        </aside>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import Pagination from '@/components/common/Pagination.vue'
import ImageConfigPopup from '../components/ImageConfigPopup.vue'
import ImagePreview from '../components/ImagePreview.vue'
import SessionSelector from '../components/SessionSelector.vue'
import TaskStatusBadge from '../components/TaskStatusBadge.vue'
import { useImageGenerationWorkspace } from '../composables/useImageGenerationWorkspace'

// 页面只负责布局；生图会话/任务/SSE/上传状态集中放在 custom composable 中维护。
const {
  aspectRatio,
  aspectRatioLabel,
  cancelingTaskId,
  clearingCurrentImage,
  clampImageCount,
  count,
  currentTaskImage,
  defaultImageModelID,
  deletingSessionId,
  editPreviewImage,
  error,
  fileInputRef,
  formatDateTime,
  formatTaskCharge,
  handleCancelTask,
  handleCreateSession,
  handleDeleteSession,
  handleFileInput,
  handleGenerate,
  handlePaste,
  clearEditImage,
  handleResetCurrentImage,
  handleResolutionChange,
  handleRetryTask,
  handleSelectSession,
  handleSetCurrentImage,
  handleTaskPageChange,
  handleTaskPageSize,
  imageGenerationEnabled,
  imageGenerationStatusLoading,
  imagePriceText,
  imageSize,
  isCurrentImage,
  orderedTasks,
  pendingEditImage,
  prompt,
  publishToGallery,
  quality,
  qualityLabel,
  reloadCurrentTasks,
  resolution,
  resolutionLabel,
  retryingTaskId,
  selectedModel,
  selectedSession,
  selectedSessionId,
  sessions,
  sessionsLoading,
  creatingSession,
  settingCurrentImageKey,
  submitting,
  taskEventsConnected,
  taskEventsFallback,
  taskImageKey,
  taskImages,
  taskModeLabel,
  taskPageSize,
  tasks,
  tasksLoading,
  tasksPage,
  tasksTotal,
} = useImageGenerationWorkspace()

const taskScrollRef = ref<HTMLElement | null>(null)

/**
 * 任务列表是对话流，新增任务或 SSE 状态刷新后保持视线在最新任务附近。
 */
watch(
  () => orderedTasks.value.map((task) => `${task.id}:${task.status}:${task.result?.data?.length ?? 0}`).join('|'),
  () => {
    void scrollTasksToBottom()
  },
  { flush: 'post' },
)

async function scrollTasksToBottom(): Promise<void> {
  await nextTick()
  const el = taskScrollRef.value
  if (!el) return
  el.scrollTo({ top: el.scrollHeight, behavior: 'smooth' })
}
</script>
