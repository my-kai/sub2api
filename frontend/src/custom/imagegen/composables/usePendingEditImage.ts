import { onBeforeUnmount, ref } from 'vue'
import { useAppStore } from '@/stores/app'
import { maxUploadImageBytes } from '../viewHelpers'

interface PendingEditImage {
  file: File
  src: string
}

/**
 * usePendingEditImage 管理浏览器侧待编辑图片。
 *
 * 上传图片只在本次提交中使用，必须在替换和离开页面时释放 object URL。
 */
export function usePendingEditImage() {
  const appStore = useAppStore()
  const pendingEditImage = ref<PendingEditImage | null>(null)
  const fileInputRef = ref<HTMLInputElement | null>(null)

  onBeforeUnmount(() => {
    revokePendingEditImage()
  })

  /**
   * 处理文件选择。
   */
  function handleFileInput(event: Event): void {
    const input = event.target as HTMLInputElement
    setPendingEditImage(input.files?.[0])
    input.value = ''
  }

  /**
   * 粘贴图片时直接设为本次编辑图。
   */
  function handlePaste(event: ClipboardEvent): void {
    const image = Array.from(event.clipboardData?.files ?? []).find((file) => file.type.startsWith('image/'))
    if (image) {
      setPendingEditImage(image)
    }
  }

  /**
   * 校验并暂存上传图片。
   */
  function setPendingEditImage(file?: File): void {
    if (!file) {
      return
    }
    if (!file.type.startsWith('image/')) {
      appStore.showWarning('只能选择图片文件')
      return
    }
    if (file.size > maxUploadImageBytes) {
      appStore.showWarning('图片不能超过 64MB')
      return
    }
    clearPendingEditImage()
    pendingEditImage.value = { file, src: URL.createObjectURL(file) }
  }

  /**
   * 释放浏览器对象 URL，避免用户频繁换图时泄漏内存。
   */
  function clearPendingEditImage(): void {
    revokePendingEditImage()
    pendingEditImage.value = null
  }

  /**
   * 只负责释放当前对象 URL，不改变状态引用。
   */
  function revokePendingEditImage(): void {
    if (pendingEditImage.value) {
      URL.revokeObjectURL(pendingEditImage.value.src)
    }
  }

  return {
    clearPendingEditImage,
    fileInputRef,
    handleFileInput,
    handlePaste,
    pendingEditImage,
  }
}
