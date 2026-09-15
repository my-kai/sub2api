import { apiClient } from '@/api/client'
import type {
  PromptAuditV2Config,
  PromptAuditV2Endpoint,
  PromptAuditV2Event,
  PromptAuditV2EventFilters,
  PromptAuditV2EventPage,
  PromptAuditV2PromptTestRequest,
  PromptAuditV2PromptTestResult,
  PromptAuditV2ProbeResult,
  PromptAuditV2Runtime,
  PromptAuditV2UpdateRequest,
} from './types'

const BASE_PATH = '/admin/custom/prompt-audit-v2'

/** Reads the secret-redacted complete configuration. */
export async function getPromptAuditV2Config(): Promise<PromptAuditV2Config> {
  const { data } = await apiClient.get<PromptAuditV2Config>(`${BASE_PATH}/config`)
  return data
}

/** Replaces the complete configuration using the current version. */
export async function updatePromptAuditV2Config(payload: PromptAuditV2UpdateRequest): Promise<PromptAuditV2Config> {
  const { data } = await apiClient.put<PromptAuditV2Config>(`${BASE_PATH}/config`, payload)
  return data
}

/** Tests one draft endpoint without saving its API key. */
export async function probePromptAuditV2Endpoint(endpoint: PromptAuditV2Endpoint): Promise<PromptAuditV2ProbeResult> {
  const { data } = await apiClient.post<PromptAuditV2ProbeResult>(`${BASE_PATH}/endpoints/probe`, { endpoint })
  return data
}

/**
 * Runs the current unsaved prompt and endpoint draft without applying risk actions.
 * @param payload Prompt, user message, and model services currently shown on the page.
 * @returns The first strict model result in configured priority order.
 * @throws The API error when the draft is invalid or every enabled service fails.
 */
export async function testPromptAuditV2Prompt(
  payload: PromptAuditV2PromptTestRequest,
): Promise<PromptAuditV2PromptTestResult> {
  const { data } = await apiClient.post<PromptAuditV2PromptTestResult>(`${BASE_PATH}/prompt/test`, payload)
  return data
}

/** Reads the active worker and persistent mail queue state. */
export async function getPromptAuditV2Runtime(): Promise<PromptAuditV2Runtime> {
  const { data } = await apiClient.get<PromptAuditV2Runtime>(`${BASE_PATH}/runtime`)
  return data
}

/** Lists rule-hit events with the current filters. */
export async function listPromptAuditV2Events(
  filters: PromptAuditV2EventFilters,
  page: number,
  pageSize: number,
): Promise<PromptAuditV2EventPage> {
  const params = Object.fromEntries(
    Object.entries({ ...filters, page, page_size: pageSize }).filter(([, value]) => value !== ''),
  )
  const { data } = await apiClient.get<PromptAuditV2EventPage>(`${BASE_PATH}/events`, { params })
  return data
}

/** Loads and decrypts one event's complete current user message. */
export async function getPromptAuditV2Event(id: number): Promise<PromptAuditV2Event> {
  const { data } = await apiClient.get<PromptAuditV2Event>(`${BASE_PATH}/events/${id}`)
  return data
}
