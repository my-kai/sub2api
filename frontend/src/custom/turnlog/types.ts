export interface TurnLog {
  id: number
  account_id: number
  account_name: string
  status_code: number
  response_headers: Record<string, string[]>
  response_body: string
  headers_truncated: boolean
  body_truncated: boolean
  body_complete: boolean
  created_at: string
}

export interface TurnLogPage {
  items: TurnLog[]
  total: number
  page: number
  page_size: number
}

export interface TurnLogConfig {
  retention_days: number
  updated_at: string
  updated_by?: number
}

export interface TurnLogOAuthAccount {
  id: number
  name: string
  proxies: TurnLogCaptureProxy[]
}

export interface TurnLogCaptureProxy {
  id: number
  name: string
  host: string
  port: number
  protocol: string
}

export interface TurnLogCaptureResult {
  status_code: number
  response_headers: Record<string, string[]>
  response_body: string
  headers_truncated: boolean
  body_truncated: boolean
}
