<template>
  <div
    ref="editorShellRef"
    class="announcement-md-editor-shell relative overflow-hidden rounded-xl border border-gray-200 bg-white dark:border-dark-600 dark:bg-dark-800"
    @click.capture="handleEditorClick"
  >
    <MdEditor
      ref="editorRef"
      v-model="editorValue"
      class="announcement-md-editor"
      :theme="editorTheme"
      :language="editorLanguage"
      :placeholder="placeholder"
      :style="editorStyle"
      :preview-theme="previewTheme"
      :footers="editorFooters"
      :toolbars="editorToolbars"
      :on-upload-img="handleUploadImg"
    >
      <template #defToolbars>
        <NormalToolbar
          :title="t('admin.announcements.form.insertLink')"
          @on-click="openInsertLinkDialog"
        >
          <Icon name="link" size="md" />
        </NormalToolbar>
      </template>
    </MdEditor>

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

    <BaseDialog
      :show="linkDialog.visible"
      :title="linkDialog.mode === 'edit' ? t('admin.announcements.form.editLink') : t('admin.announcements.form.insertLink')"
      width="normal"
      :z-index="LINK_DIALOG_Z_INDEX"
      @close="closeLinkDialog"
    >
      <form id="announcement-link-form" class="space-y-4" @submit.prevent="saveLinkDialog">
        <div>
          <label class="input-label">{{ t('admin.announcements.form.linkUrl') }}</label>
          <input
            v-model="linkDialog.href"
            type="text"
            class="input"
            required
          />
        </div>
        <div>
          <label class="input-label">{{ t('admin.announcements.form.linkText') }}</label>
          <input
            v-model="linkDialog.text"
            type="text"
            class="input"
          />
        </div>
        <div>
          <label class="input-label">{{ t('admin.announcements.form.linkOpenMode') }}</label>
          <Select
            v-model="linkDialog.openMode"
            :options="linkOpenModeOptions"
          />
        </div>
      </form>

      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="closeLinkDialog">
            {{ t('common.cancel') }}
          </button>
          <button type="submit" form="announcement-link-form" class="btn btn-primary">
            {{ t('common.save') }}
          </button>
        </div>
      </template>
    </BaseDialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { MdEditor, NormalToolbar, allToolbar } from 'md-editor-v3'
import type { ExposeParam, Footers, Themes, ToolbarNames, UploadImgCallBack, UploadImgEvent } from 'md-editor-v3'
import 'md-editor-v3/lib/style.css'
import { useAppStore } from '@/stores/app'
import { uploadAnnouncementImage } from '@/custom/announcements/api'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import {
  ANNOUNCEMENT_LINK_OPEN_MODE_CURRENT,
  ANNOUNCEMENT_LINK_OPEN_MODE_NEW_TAB,
  type AnnouncementLinkOpenMode,
  type EditableAnnouncementLink,
  buildAnnouncementLinkMarkdown,
  findEditableAnnouncementLink,
  replaceAnnouncementLink,
} from '@/custom/announcements/linkOpenMode'

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
const editorToolbars = allToolbar.map((toolbar) => toolbar === 'link' ? 0 : toolbar) as ToolbarNames[]
const editorRef = ref<ExposeParam | null>(null)
const editorShellRef = ref<HTMLElement | null>(null)
let themeObserver: MutationObserver | null = null
let pendingUploadBatches = 0
let disposed = false

// Select.vue 的下拉层会 Teleport 到 body 且使用 100000020；链接弹窗必须低于它，否则“打开方式”选项会被弹窗层遮住。
const LINK_DIALOG_Z_INDEX = 100000010

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

const linkOpenModeOptions = computed(() => [
  { value: ANNOUNCEMENT_LINK_OPEN_MODE_CURRENT, label: t('admin.announcements.form.linkOpenModeCurrent') },
  { value: ANNOUNCEMENT_LINK_OPEN_MODE_NEW_TAB, label: t('admin.announcements.form.linkOpenModeNewTab') },
])

const linkDialog = reactive<{
  visible: boolean
  mode: 'insert' | 'edit'
  text: string
  href: string
  openMode: AnnouncementLinkOpenMode
  sourceLink: EditableAnnouncementLink | null
}>({
  visible: false,
  mode: 'insert',
  text: '',
  href: '',
  openMode: ANNOUNCEMENT_LINK_OPEN_MODE_CURRENT,
  sourceLink: null,
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
 * 打开链接插入弹窗。
 *
 * 选中文本会作为链接文字带入；链接文字允许为空，保存时按产品规则使用链接地址作为展示文本。
 */
function openInsertLinkDialog() {
  linkDialog.mode = 'insert'
  linkDialog.sourceLink = null
  linkDialog.text = editorRef.value?.getSelectedText()?.trim() ?? ''
  linkDialog.href = ''
  linkDialog.openMode = ANNOUNCEMENT_LINK_OPEN_MODE_CURRENT
  linkDialog.visible = true
}

/**
 * 关闭链接弹窗并清理临时状态。
 */
function closeLinkDialog() {
  linkDialog.visible = false
  linkDialog.sourceLink = null
}

/**
 * 拦截编辑器预览里的链接点击，用于回到 Markdown 源内容修改链接配置。
 *
 * @param event - 编辑器区域的点击事件。
 */
function handleEditorClick(event: MouseEvent) {
  const target = event.target as HTMLElement | null
  const link = target?.closest('a')
  if (!link || !editorShellRef.value?.contains(link)) {
    return
  }

  if (!link.closest('.md-editor-preview')) {
    return
  }

  event.preventDefault()
  event.stopPropagation()
  openEditLinkDialog(link)
}

/**
 * 根据预览 DOM 中的链接定位 Markdown 源片段并打开编辑弹窗。
 *
 * @param link - 预览区被点击的链接元素。
 */
function openEditLinkDialog(link: HTMLAnchorElement) {
  const href = link.getAttribute('href')?.trim() ?? ''
  const sourceLink = findEditableAnnouncementLink(editorValue.value, {
    href,
    text: link.textContent?.trim() ?? '',
    occurrence: previewLinkOccurrence(link),
  })

  if (!sourceLink) {
    appStore.showError(t('admin.announcements.form.linkLocateFailed'))
    return
  }

  linkDialog.mode = 'edit'
  linkDialog.sourceLink = sourceLink
  linkDialog.text = sourceLink.text
  linkDialog.href = sourceLink.href
  linkDialog.openMode = sourceLink.openMode
  linkDialog.visible = true
}

/**
 * 获取被点击链接在预览区相同链接中的序号。
 *
 * Markdown 源内容里可能出现相同文字和地址的链接；带上序号可以避免点击第二个链接却修改第一个链接。
 *
 * @param link - 预览区被点击的链接元素。
 * @returns 相同链接在预览区从 0 开始的序号。
 */
function previewLinkOccurrence(link: HTMLAnchorElement): number {
  const previewRoot = link.closest('.md-editor-preview')
  if (!previewRoot) {
    return 0
  }

  const href = link.getAttribute('href')?.trim() ?? ''
  const text = link.textContent?.trim() ?? ''
  let occurrence = 0
  for (const candidate of Array.from(previewRoot.querySelectorAll('a'))) {
    if (candidate === link) {
      return occurrence
    }
    if (
      candidate.getAttribute('href')?.trim() === href &&
      (candidate.textContent?.trim() ?? '') === text
    ) {
      occurrence += 1
    }
  }

  return 0
}

/**
 * 保存链接弹窗配置。
 */
function saveLinkDialog() {
  const href = linkDialog.href.trim()
  const text = linkDialog.text.trim() || href

  if (!href) {
    appStore.showError(t('admin.announcements.form.linkRequired'))
    return
  }

  if (linkDialog.mode === 'edit') {
    if (!linkDialog.sourceLink) {
      appStore.showError(t('admin.announcements.form.linkLocateFailed'))
      return
    }
    editorValue.value = replaceAnnouncementLink(editorValue.value, linkDialog.sourceLink, {
      text,
      href,
      openMode: linkDialog.openMode,
    })
    closeLinkDialog()
    return
  }

  if (!editorRef.value) {
    appStore.showError(t('admin.announcements.form.editorNotReady'))
    return
  }

  const linkMarkdown = buildAnnouncementLinkMarkdown({
    text,
    href,
    openMode: linkDialog.openMode,
  })
  editorRef.value.insert(() => ({
    targetValue: linkMarkdown,
  }))
  closeLinkDialog()
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

/* md-editor-v3 未暴露图片下拉菜单项配置；这里仅隐藏“添加链接”，保留上传入口。 */
.announcement-md-editor-shell :deep(.md-editor-menu-item-image:first-child) {
  display: none;
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
