export type PromptAuditV2Action = 'ban' | 'warning'

export interface PromptAuditV2Protocol {
  value: string
  label: string
}

export interface PromptAuditV2Endpoint {
  id: string
  name: string
  base_url: string
  api_key?: string
  has_api_key: boolean
  model: string
  priority: number
  timeout_ms: number
  enabled: boolean
  order: number
}

export interface PromptAuditV2Rule {
	confidence_threshold: number
	window_minutes: number
	trigger_count: number
	action: PromptAuditV2Action
	restriction_minutes?: number | null
}

export interface PromptAuditV2Config {
  enabled: boolean
  prompt_template: string
  worker_count: number
  queue_capacity: number
  enabled_protocols: string[]
  log_retention_days: number
  config_version: number
	updated_at: string
	updated_by?: number
	endpoints: PromptAuditV2Endpoint[]
	rule: PromptAuditV2Rule
	protocols: PromptAuditV2Protocol[]
}

export interface PromptAuditV2UpdateRequest {
  expected_config_version: number
  enabled: boolean
  prompt_template: string
  worker_count: number
  queue_capacity: number
  enabled_protocols: string[]
	log_retention_days: number
	endpoints: PromptAuditV2Endpoint[]
	rule: PromptAuditV2Rule
}

export interface PromptAuditV2ProbeResult {
  ok: boolean
  status: string
  http_status: number
  latency_ms: number
  error_code?: string
  checked_at: string
}

/** Carries the current unsaved audit draft for one side-effect-free test. */
export interface PromptAuditV2PromptTestRequest {
  prompt_template: string
  user_input: string
  endpoints: PromptAuditV2Endpoint[]
}

/** Describes the first valid audit model result returned by draft priority. */
export interface PromptAuditV2PromptTestResult {
  confidence: number
  reason: string
  endpoint_name: string
  audit_model: string
  latency_ms: number
}

export interface PromptAuditV2EndpointRuntime {
  ok: boolean
  status: string
  latency_ms: number
  checked_at: string
}

export interface PromptAuditV2Runtime {
  status: string
  config_version: number
  health_config_version: number
  health_checked_at?: string
  health_error_code?: string
  worker_count: number
  queue_capacity: number
  queued: number
  processing: number
  failovers: number
  hits: number
  rejected: number
  unavailable: number
  email_pending: number
  email_failed: number
  endpoints: Record<string, PromptAuditV2EndpointRuntime>
  last_error_code?: string
  last_error_at?: string
}

export interface PromptAuditV2Event {
  id: number
  request_id: string
  user_id: number
  username: string
  user_email: string
  api_key_id: number
  api_key_name: string
  protocol: string
  request_model: string
  endpoint_id: string
  endpoint_name: string
  audit_model: string
  confidence: number
  reason: string
  latency_ms: number
	rule_threshold: number
  rule_window_minutes: number
  rule_trigger_count: number
  rule_action: PromptAuditV2Action
  rule_restriction_minutes?: number
  window_hit_count: number
  threshold_reached: boolean
  final_action: string
  action_result: string
  message_sha256: string
  message_chars: number
  message?: string
  created_at: string
}

export interface PromptAuditV2EventPage {
  items: PromptAuditV2Event[]
  total: number
  page: number
  page_size: number
}

export interface PromptAuditV2EventFilters {
  start_at: string
  end_at: string
  user_id: string
	action: string
  protocol: string
  min_confidence: string
  max_confidence: string
}
