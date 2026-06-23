<template>
  <div
    class="announcement-md-editor-shell relative overflow-hidden rounded-xl border border-gray-200 bg-white dark:border-dark-600 dark:bg-dark-800"
  >
    <MdEditor
      v-model="editorValue"
      class="announcement-md-editor"
      :theme="editorTheme"
      :language="editorLanguage"
      :placeholder="placeholder"
      :style="editorStyle"
      :preview-theme="previewTheme"
      :footers="editorFooters"
      :on-upload-img="handleUploadImg"
    />

    <textarea
      v-if="required"
      class="announcement-md-editor-required-proxy"
      :value="modelValue"
      required
      aria-hidden="true"
      tabindex="-1"
    ></textarea>

    <div
      class="pointer-events-none absolute bottom-12 right-3 rounded-lg bg-gray-900/70 px-2 py-1 text-xs text-white transition-opacity"
      :class="uploading ? 'opacity-100' : 'opacity-0'"
    >
      {{ t('admin.announcements.form.imageUploading') }}
    </div>

    <p class="border-t border-gray-200 px-3 py-2 text-xs text-gray-500 dark:border-dark-600 dark:text-dark-400">
      {{ t('admin.announcements.form.markdownPasteHint') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { MdEditor } from 'md-editor-v3'
import type { Footers, Themes, UploadImgCallBack, UploadImgEvent } from 'md-editor-v3'
import 'md-editor-v3/lib/style.css'
import { useAppStore } from '@/stores/app'
import { uploadAnnouncementImage } from '@/custom/announcements/api'

const props = withDefaults(defineProps<{
  modelValue: string
  required?: boolean
  rows?: number
  placeholder?: string
}>(), {
  required: false,
  rows: 12,
  placeholder: '',
})

const emit = defineEmits<{
  'update:modelValue': [value: string]
  'uploading-change': [value: boolean]
}>()

const { t, locale } = useI18n()
const appStore = useAppStore()
const uploading = ref(false)
const editorTheme = ref<Themes>(resolveEditorTheme())
const previewTheme = 'github'
const editorFooters: Footers[] = ['markdownTotal', '=', 'scrollSwitch']
let themeObserver: MutationObserver | null = null
let pendingUploadBatches = 0
let disposed = false

const editorValue = computed({
  get: () => props.modelValue,
  set: (value: string) => emit('update:modelValue', value),
})

const editorLanguage = computed(() => (locale.value.startsWith('en') ? 'en-US' : 'zh-CN'))

const editorStyle = computed(() => {
  const minHeight = Math.max(320, props.rows * 24 + 120)
  return {
    height: `${minHeight}px`,
  }
})

onMounted(() => {
  // 主题切换只更新 html.dark 类；监听该类可以让编辑器跟随系统现有暗色模式实现。
  themeObserver = new MutationObserver(() => {
    editorTheme.value = resolveEditorTheme()
  })
  themeObserver.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ['class'],
  })
})

onBeforeUnmount(() => {
  disposed = true
  pendingUploadBatches = 0
  themeObserver?.disconnect()
  themeObserver = null
  setUploading(false)
})

/**
 * 获取编辑器主题。
 *
 * @returns 当前页面应使用的 Markdown 编辑器主题。
 */
function resolveEditorTheme(): Themes {
  return document.documentElement.classList.contains('dark') ? 'dark' : 'light'
}

/**
 * 同步上传状态给父级表单。
 *
 * @param value - 当前是否仍有图片上传任务。
 */
function setUploading(value: boolean) {
  if (uploading.value === value) {
    return
  }
  uploading.value = value
  emit('uploading-change', value)
}

/**
 * 开始一批图片上传。
 */
function beginUploadBatch() {
  pendingUploadBatches += 1
  setUploading(true)
}

/**
 * 结束一批图片上传，并在所有批次完成后恢复保存按钮。
 */
function finishUploadBatch() {
  pendingUploadBatches = Math.max(0, pendingUploadBatches - 1)
  if (!disposed) {
    setUploading(pendingUploadBatches > 0)
  }
}

/**
 * 获取可展示给管理员的上传失败原因。
 *
 * @param error - 接口或运行时抛出的异常。
 * @returns 用户可见的错误提示。
 */
function uploadErrorMessage(error: any): string {
  return error?.response?.data?.detail || error?.message || t('admin.announcements.form.imageUploadFailed')
}

/**
 * 上传编辑器传入的图片，并回填标准 Markdown 图片链接。
 *
 * @param files - 编辑器从上传按钮、粘贴或截图中收集到的图片文件。
 * @param callback - md-editor-v3 用于插入 Markdown 图片语法的回调。
 */
async function uploadImages(files: File[], callback: UploadImgCallBack): Promise<void> {
  if (files.length === 0) {
    return
  }

  beginUploadBatch()
  const uploadedImages: Array<{ url: string, alt: string, title: string }> = []

  try {
    for (const file of files) {
      const result = await uploadAnnouncementImage(file)
      if (disposed) {
        return
      }

      // 只保存标准 Markdown 图片语法所需的 URL，不引入编辑器私有格式，方便后续替换组件。
      uploadedImages.push({
        url: result.url,
        alt: file.name || 'image',
        title: file.name || 'image',
      })
    }

    if (uploadedImages.length > 0) {
      callback(uploadedImages)
    }
  } catch (error: any) {
    appStore.showError(uploadErrorMessage(error))
    if (!disposed && uploadedImages.length > 0) {
      callback(uploadedImages)
    }
  } finally {
    finishUploadBatch()
  }
}

const handleUploadImg: UploadImgEvent = (files, callback) => {
  void uploadImages(files, callback)
}
</script>

<style scoped>
.announcement-md-editor-shell :deep(.md-editor) {
  border: 0;
  border-radius: 0;
}

.announcement-md-editor-shell :deep(.md-editor-toolbar-wrapper) {
  border-bottom-color: rgb(229 231 235);
}

.dark .announcement-md-editor-shell :deep(.md-editor-toolbar-wrapper) {
  border-bottom-color: rgb(55 65 81);
}

.announcement-md-editor-shell :deep(.md-editor-preview img) {
  max-width: 100%;
  height: auto;
  border-radius: 0.5rem;
}

.announcement-md-editor-required-proxy {
  position: absolute;
  bottom: 0;
  left: 0;
  width: 1px;
  height: 1px;
  opacity: 0;
  pointer-events: none;
}
</style>
