import type {
  PromptAuditV2Config,
  PromptAuditV2Endpoint,
  PromptAuditV2EventFilters,
  PromptAuditV2UpdateRequest,
} from './types'

/** Produces an editable deep copy without retaining reactive references. */
export function clonePromptAuditV2Config(config: PromptAuditV2Config): PromptAuditV2Config {
  return structuredClone(config)
}

/** Creates a complete endpoint draft whose stable ID is generated once. */
export function createPromptAuditV2Endpoint(order: number): PromptAuditV2Endpoint {
  return {
    id: crypto.randomUUID(),
    name: '',
    base_url: '',
    api_key: '',
    has_api_key: false,
    model: '',
    priority: 0,
    timeout_ms: 10000,
    enabled: true,
    order,
  }
}

/** Converts the page draft to the full replacement contract. */
export function buildPromptAuditV2Update(config: PromptAuditV2Config): PromptAuditV2UpdateRequest {
  return {
    expected_config_version: config.config_version,
    enabled: config.enabled,
    prompt_template: config.prompt_template,
    worker_count: Number(config.worker_count),
    queue_capacity: Number(config.queue_capacity),
    enabled_protocols: [...config.enabled_protocols],
    log_retention_days: Number(config.log_retention_days),
    endpoints: config.endpoints.map((endpoint, order) => ({ ...endpoint, order })),
    rule: {
      ...config.rule,
      confidence_threshold: Number(config.rule.confidence_threshold),
      window_minutes: Number(config.rule.window_minutes),
      trigger_count: Number(config.rule.trigger_count),
      restriction_minutes: config.rule.action === 'warning' ? Number(config.rule.restriction_minutes) : null,
    },
  }
}

/** Returns a new empty filter set for reset operations. */
export function emptyPromptAuditV2Filters(): PromptAuditV2EventFilters {
  return {
    start_at: '',
    end_at: '',
    user_id: '',
    action: '',
    protocol: '',
    min_confidence: '',
    max_confidence: '',
  }
}
