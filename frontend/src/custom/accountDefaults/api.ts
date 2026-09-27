// 账号添加默认配置（二开 custom 模块前端 API）
import { apiClient } from '@/api/client'

/** 代理带入模式：fixed=固定 N 个；random=随机 1 个。 */
export type ProxyMode = 'fixed' | 'random'

/** 管理员配置的默认值结构（与后端 accountdefaults.Config 对应）。 */
export interface AccountDefaultsConfig {
  models_by_platform: Record<string, string[]>
  proxy_mode: ProxyMode | ''
  proxy_fixed_ids: number[]
  allow_local_egress: boolean
}

/** 建账号表单拿到的解析结果（随机模式已抽好）。 */
export interface ResolvedAccountDefaults {
  models_by_platform: Record<string, string[]>
  proxy_mode: ProxyMode | ''
  egress_proxy_ids: number[]
  egress_include_local: boolean
}

export async function getAccountDefaults(): Promise<AccountDefaultsConfig> {
  const { data } = await apiClient.get<AccountDefaultsConfig>('/admin/custom/account-defaults')
  return data
}

export async function updateAccountDefaults(
  payload: AccountDefaultsConfig
): Promise<AccountDefaultsConfig> {
  const { data } = await apiClient.put<AccountDefaultsConfig>('/admin/custom/account-defaults', payload)
  return data
}

/** 拉取解析后的默认值（建账号表单打开时调用，random 模式服务端抽一次）。 */
export async function resolveAccountDefaults(): Promise<ResolvedAccountDefaults> {
  const { data } = await apiClient.get<ResolvedAccountDefaults>('/admin/custom/account-defaults/resolve')
  return data
}
