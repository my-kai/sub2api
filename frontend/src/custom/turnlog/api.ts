import { apiClient } from '@/api/client'
import type { TurnLog, TurnLogCaptureResult, TurnLogConfig, TurnLogOAuthAccount, TurnLogPage } from './types'

const LOGS_PATH = '/admin/custom/turn-logs'
const CONFIG_PATH = '/admin/custom/turn-log-config'
const ACCOUNTS_PATH = '/admin/custom/turn-logs/accounts'
const CAPTURE_PATH = '/admin/custom/turn-logs/capture'

/** Lists active OpenAI OAuth accounts without credentials. */
export async function listTurnLogOAuthAccounts(): Promise<TurnLogOAuthAccount[]> {
  const { data } = await apiClient.get<TurnLogOAuthAccount[]>(ACCOUNTS_PATH)
  return data
}

/** Sends one manual capture request and returns the raw upstream reply. */
export async function captureTurnState(account_id: number, model: string, proxy_id?: number | null): Promise<TurnLogCaptureResult> {
  const { data } = await apiClient.post<TurnLogCaptureResult>(CAPTURE_PATH, { account_id, model, proxy_id: proxy_id ?? null })
  return data
}

/** Lists administrator-visible matching upstream responses. */
export async function listTurnLogs(params: { page: number; page_size: number; account_id?: string; status_code?: string }): Promise<TurnLogPage> {
  const { data } = await apiClient.get<TurnLogPage>(LOGS_PATH, { params })
  return data
}

/** Loads one complete diagnostic response. */
export async function getTurnLog(id: number): Promise<TurnLog> {
  const { data } = await apiClient.get<TurnLog>(`${LOGS_PATH}/${id}`)
  return data
}

/** Reads the current retention policy. */
export async function getTurnLogConfig(): Promise<TurnLogConfig> {
  const { data } = await apiClient.get<TurnLogConfig>(CONFIG_PATH)
  return data
}

/** Updates retention days and triggers server-side cleanup. */
export async function updateTurnLogConfig(retention_days: number): Promise<TurnLogConfig> {
  const { data } = await apiClient.put<TurnLogConfig>(CONFIG_PATH, { retention_days })
  return data
}
