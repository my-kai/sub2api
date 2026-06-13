import { readonly, ref } from 'vue'
import { fetchImageGenerationStatus } from '../api'

const enabled = ref(true)
const loading = ref(false)
const loaded = ref(false)
const error = ref('')
let inFlight: Promise<boolean> | null = null
let requestVersion = 0

/**
 * useImageGenerationStatus 共享用户侧生图开关状态。
 *
 * 菜单和生图页都读取同一个状态；默认显示，等后端明确返回 disabled 后再隐藏入口，
 * 避免页面初次渲染时因为状态未加载导致菜单闪烁消失。
 */
export function useImageGenerationStatus() {
  return {
    enabled: readonly(enabled),
    error: readonly(error),
    loaded: readonly(loaded),
    loading: readonly(loading),
    load: loadImageGenerationStatus,
  }
}

/**
 * 从后端读取公开 enabled 状态；并发调用复用同一个请求。
 */
async function loadImageGenerationStatus(options: { force?: boolean } = {}): Promise<boolean> {
  if (inFlight && !options.force) return inFlight
  if (loaded.value && !options.force) {
    return enabled.value
  }

  const version = ++requestVersion
  loading.value = true
  error.value = ''
  const request = fetchImageGenerationStatus()
    .then((status) => {
      if (version === requestVersion) {
        enabled.value = status.enabled !== false
        loaded.value = true
      }
      return enabled.value
    })
    .catch((err: unknown) => {
      if (version === requestVersion) {
        error.value = err instanceof Error ? err.message : '生图状态读取失败'
        // 状态接口失败不能让用户侧菜单永久消失；后端创建任务仍会做最终兜底。
        enabled.value = true
        loaded.value = true
      }
      return enabled.value
    })
    .finally(() => {
      if (version === requestVersion) {
        loading.value = false
      }
      if (inFlight === request) {
        inFlight = null
      }
    })
  inFlight = request
  return request
}
