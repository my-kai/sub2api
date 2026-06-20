<template>
  <div class="overflow-hidden rounded-xl border border-gray-200 bg-white dark:border-dark-600 dark:bg-dark-800">
    <div class="flex flex-wrap items-center justify-between gap-2 border-b border-gray-200 px-3 py-2 dark:border-dark-600">
      <div class="flex flex-wrap items-center gap-1.5">
        <button
          v-for="action in toolbarActions"
          :key="action.key"
          type="button"
          class="btn btn-secondary btn-sm"
          @click="applyToolbarAction(action.key)"
        >
          {{ action.label }}
        </button>
      </div>
      <div class="flex items-center gap-2">
        <span v-if="uploading" class="text-xs text-gray-500 dark:text-dark-400">
          {{ t('admin.announcements.form.imageUploading') }}
        </span>
        <button type="button" class="btn btn-secondary btn-sm" @click="showPreview = !showPreview">
          {{ showPreview ? t('admin.announcements.form.hidePreview') : t('admin.announcements.form.showPreview') }}
        </button>
      </div>
    </div>

    <div class="grid grid-cols-1 md:grid-cols-2">
      <div class="relative" :class="showPreview ? '' : 'md:col-span-2'">
        <textarea
          ref="textareaRef"
          :value="modelValue"
          :required="required"
          :rows="rows"
          class="min-h-[18rem] w-full resize-y border-0 bg-white px-4 py-3 font-mono text-sm leading-6 text-gray-900 outline-none placeholder:text-gray-400 focus:ring-0 dark:bg-dark-800 dark:text-gray-100 dark:placeholder:text-dark-400"
          :placeholder="placeholder"
          @input="handleInput"
          @paste="handlePaste"
        ></textarea>
        <div
          class="pointer-events-none absolute bottom-3 right-3 rounded-lg bg-gray-900/70 px-2 py-1 text-xs text-white transition-opacity"
          :class="uploading ? 'opacity-100' : 'opacity-0'"
        >
          {{ t('admin.announcements.form.imageUploading') }}
        </div>
      </div>

      <div
        v-if="showPreview"
        class="markdown-editor-preview min-h-[18rem] overflow-auto border-t border-gray-200 bg-gray-50 px-4 py-3 text-sm text-gray-800 dark:border-dark-600 dark:bg-dark-900/50 dark:text-dark-100 md:border-l md:border-t-0"
        v-html="renderedPreview"
      ></div>
    </div>

    <p class="border-t border-gray-200 px-3 py-2 text-xs text-gray-500 dark:border-dark-600 dark:text-dark-400">
      {{ t('admin.announcements.form.markdownPasteHint') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import { useAppStore } from '@/stores/app'
import { uploadAnnouncementImage } from '@/custom/announcements/api'

type ToolbarAction = 'bold' | 'italic' | 'link' | 'quote' | 'code' | 'list'

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

const { t } = useI18n()
const appStore = useAppStore()
const textareaRef = ref<HTMLTextAreaElement | null>(null)
const showPreview = ref(true)
const uploading = ref(false)
let pendingUploadBatches = 0
let disposed = false

onBeforeUnmount(() => {
  disposed = true
  pendingUploadBatches = 0
  uploading.value = false
})

function setUploading(value: boolean) {
  uploading.value = value
  emit('uploading-change', value)
}

const toolbarActions = computed<Array<{ key: ToolbarAction, label: string }>>(() => [
  { key: 'bold', label: t('admin.announcements.form.toolbar.bold') },
  { key: 'italic', label: t('admin.announcements.form.toolbar.italic') },
  { key: 'link', label: t('admin.announcements.form.toolbar.link') },
  { key: 'quote', label: t('admin.announcements.form.toolbar.quote') },
  { key: 'code', label: t('admin.announcements.form.toolbar.code') },
  { key: 'list', label: t('admin.announcements.form.toolbar.list') },
])

marked.setOptions({
  breaks: true,
  gfm: true,
})

const renderedPreview = computed(() => {
  const raw = props.modelValue?.trim() || ''
  if (!raw) return ''
  // Preview uses the same Markdown + sanitizer pair as announcement display components,
  // so admins see the closest safe rendering before saving.
  const html = marked.parse(raw) as string
  return DOMPurify.sanitize(html)
})

function emitValue(value: string) {
  emit('update:modelValue', value)
}

function handleInput(event: Event) {
  const input = event.target as HTMLTextAreaElement | null
  emitValue(input?.value ?? '')
}

function insertTextAtCursor(text: string) {
  const input = textareaRef.value
  if (!input) {
    emitValue(`${props.modelValue}${text}`)
    return
  }

  const currentValue = input.value
  const start = input.selectionStart ?? currentValue.length
  const end = input.selectionEnd ?? currentValue.length
  const nextValue = `${currentValue.slice(0, start)}${text}${currentValue.slice(end)}`
  emitValue(nextValue)

  // Keep the DOM value and cursor in sync immediately; a single paste can upload
  // multiple images before Vue flushes the next render.
  input.value = nextValue
  const cursor = start + text.length
  input.setSelectionRange(cursor, cursor)

  // Restore focus after Vue has reconciled the v-model value.
  void nextTick(() => {
    input.focus()
    input.setSelectionRange(cursor, cursor)
  })
}

function wrapSelection(prefix: string, suffix = prefix, fallback = '') {
  const input = textareaRef.value
  if (!input) return

  const start = input.selectionStart ?? 0
  const end = input.selectionEnd ?? 0
  const selected = props.modelValue.slice(start, end) || fallback
  const replacement = `${prefix}${selected}${suffix}`
  emitValue(`${props.modelValue.slice(0, start)}${replacement}${props.modelValue.slice(end)}`)

  void nextTick(() => {
    input.focus()
    input.setSelectionRange(start + prefix.length, start + prefix.length + selected.length)
  })
}

function prefixSelectionLines(prefix: string, fallback: string) {
  const input = textareaRef.value
  if (!input) return

  const start = input.selectionStart ?? 0
  const end = input.selectionEnd ?? 0
  const selected = props.modelValue.slice(start, end) || fallback
  const replacement = selected
    .split('\n')
    .map((line) => `${prefix}${line}`)
    .join('\n')
  emitValue(`${props.modelValue.slice(0, start)}${replacement}${props.modelValue.slice(end)}`)

  void nextTick(() => {
    input.focus()
    input.setSelectionRange(start, start + replacement.length)
  })
}

function applyToolbarAction(action: ToolbarAction) {
  switch (action) {
    case 'bold':
      wrapSelection('**', '**', t('admin.announcements.form.toolbar.defaultText'))
      break
    case 'italic':
      wrapSelection('*', '*', t('admin.announcements.form.toolbar.defaultText'))
      break
    case 'link':
      wrapSelection('[', '](https://)', t('admin.announcements.form.toolbar.defaultText'))
      break
    case 'quote':
      prefixSelectionLines('> ', t('admin.announcements.form.toolbar.defaultText'))
      break
    case 'code':
      wrapSelection('```\n', '\n```', t('admin.announcements.form.toolbar.defaultCode'))
      break
    case 'list':
      prefixSelectionLines('- ', t('admin.announcements.form.toolbar.defaultText'))
      break
  }
}

function pastedImageFiles(event: ClipboardEvent): File[] {
  const items = Array.from(event.clipboardData?.items || [])
  const files = items
    .filter((item) => item.kind === 'file' && item.type.startsWith('image/'))
    .map((item) => item.getAsFile())
    .filter((file): file is File => !!file)

  // Some browsers expose clipboard images only through files, not items.
  const fallback = Array.from(event.clipboardData?.files || []).filter((file) => file.type.startsWith('image/'))
  return files.length > 0 ? files : fallback
}

async function handlePaste(event: ClipboardEvent) {
  const images = pastedImageFiles(event)
  if (images.length === 0) return

  event.preventDefault()
  pendingUploadBatches += 1
  setUploading(true)
  try {
    for (const image of images) {
      const result = await uploadAnnouncementImage(image)
      if (disposed) {
        return
      }
      // Insert a normal Markdown image token instead of editor-specific markup, keeping
      // stored announcement content portable across viewers and future editors.
      insertTextAtCursor(`\n![image](${result.url})\n`)
    }
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || error.message || t('admin.announcements.form.imageUploadFailed'))
  } finally {
    pendingUploadBatches = Math.max(0, pendingUploadBatches - 1)
    if (!disposed) {
      setUploading(pendingUploadBatches > 0)
    }
  }
}
</script>

<style scoped>
.markdown-editor-preview :deep(h1) {
  @apply mb-3 mt-4 border-b border-gray-200 pb-2 text-2xl font-bold dark:border-dark-600;
}

.markdown-editor-preview :deep(h2) {
  @apply mb-2 mt-4 text-xl font-bold;
}

.markdown-editor-preview :deep(h3) {
  @apply mb-2 mt-3 text-lg font-semibold;
}

.markdown-editor-preview :deep(p) {
  @apply mb-3 leading-6;
}

.markdown-editor-preview :deep(a) {
  @apply text-primary-600 underline underline-offset-4 dark:text-primary-300;
}

.markdown-editor-preview :deep(ul) {
  @apply mb-3 list-disc pl-5;
}

.markdown-editor-preview :deep(ol) {
  @apply mb-3 list-decimal pl-5;
}

.markdown-editor-preview :deep(blockquote) {
  @apply my-3 border-l-4 border-gray-300 pl-3 text-gray-600 dark:border-dark-500 dark:text-dark-300;
}

.markdown-editor-preview :deep(code) {
  @apply rounded bg-gray-100 px-1.5 py-0.5 font-mono text-xs dark:bg-dark-700;
}

.markdown-editor-preview :deep(pre) {
  @apply my-3 overflow-x-auto rounded-lg bg-gray-900 p-3 text-gray-100;
}

.markdown-editor-preview :deep(pre code) {
  @apply bg-transparent p-0 text-inherit;
}

.markdown-editor-preview :deep(img) {
  @apply my-3 h-auto max-w-full rounded-lg;
}
</style>
