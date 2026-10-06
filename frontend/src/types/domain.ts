/**
 * Wire types, mirroring domain-monitor/backend/internal/model.
 *
 * Status is classified on the agent, never here: the wall, the list view and
 * anything added later must all colour a tile from the same verdict.
 */

export type Status = 'UP' | 'DEGRADED' | 'DOWN' | 'UNKNOWN'

export type ErrorKind = '' | 'dns' | 'connect' | 'tls' | 'timeout' | 'http'

export interface HttpResult {
  checked_at: string
  up: boolean
  status_code: number
  latency_ms: number
  ttfb_ms: number
  body_bytes: number
  final_url: string
  redirects?: string[]
  error?: string
  error_kind?: ErrorKind
}

export interface CertInfo {
  checked_at: string
  not_before?: string
  not_after?: string
  days_left?: number
  issuer?: string
  subject?: string
  sans?: string[]
  tls_version?: string
  cipher?: string
  error?: string
}

export interface DnsInfo {
  checked_at: string
  addrs?: string[]
  cname?: string
  ns?: string[]
  resolve_ms: number
  error?: string
}

export interface DomainInfo {
  apex: string
  checked_at: string
  registrar?: string
  created_at?: string
  expires_at?: string
  days_left?: number
  statuses?: string[]
  source?: 'rdap' | 'whois' | string
  error?: string
}

export interface Uptime {
  day: number
  week: number
  month: number
  avg_ms: number
  p95_ms: number
  samples: number
}

export interface Incident {
  id: number
  target_id: string
  started_at: string
  ended_at?: string
  duration_sec: number
  reason: string
  last_error?: string
  active: boolean
}

export interface StatusEvent {
  at: string
  target_id: string
  from: Status
  to: Status
  detail?: string
}

export interface TargetState {
  id: string
  apex: string
  host: string
  url: string
  label: string
  is_apex: boolean
  order: number
  status: Status
  reason?: string
  http?: HttpResult
  cert?: CertInfo
  dns?: DnsInfo
  uptime: Uptime
  spark: number[]
  active_incident?: Incident
}

export interface DomainGroup {
  apex: string
  status: Status
  domain?: DomainInfo
  endpoints: TargetState[]
  up_count: number
  total: number
}

export interface ExpiryItem {
  kind: 'ssl' | 'domain'
  target_id: string
  label: string
  apex: string
  expires_at: string
  days_left: number
  issuer?: string
  registrar?: string
}

export interface Overview {
  generated_at: string
  total_domains: number
  total_endpoints: number
  up: number
  degraded: number
  down: number
  unknown: number
  uptime_day: number
  avg_latency_ms: number
  slowest_id?: string
  slowest_ms: number
  active_incidents: number
  next_ssl?: ExpiryItem
  next_domain?: ExpiryItem
  last_scan?: string
  events: StatusEvent[]
  incidents: Incident[]
  expiry: ExpiryItem[]
}

/** The payload a dashboard receives the moment it connects. */
export interface FullSnapshot {
  overview: Overview
  groups: DomainGroup[]
}

export interface HistoryPoint {
  ts: number
  avg_ms: number
  max_ms: number
  up: number
  total: number
  percent: number
}

export interface HistoryResponse {
  target_id: string
  range: string
  bucket_s: number
  points: HistoryPoint[]
}

export interface TargetDetail {
  target: TargetState
  incidents: Incident[]
}

export interface AgentConfig {
  latency_warn_ms: number
  latency_crit_ms: number
  cert_warn_days: number
  cert_crit_days: number
  domain_warn_days: number
  domain_crit_days: number
  accept_status: number[]
  http_interval_s: number
  targets_file: string
}

export type ConnectionState = 'connecting' | 'live' | 'reconnecting' | 'offline'

/** Envelope used by every WebSocket frame. */
export interface WsMessage {
  type: 'snapshot' | 'update' | 'overview' | 'event' | 'pong'
  timestamp: number
  data?: unknown
}
