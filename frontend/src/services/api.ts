import type {
  AgentConfig,
  DomainGroup,
  ExpiryItem,
  HistoryResponse,
  Incident,
  Overview,
  StatusEvent,
  TargetDetail,
  TargetState,
} from '@/types/domain'

/**
 * Base path for the agent.
 *
 * Empty by default: in development Vite proxies /api to the agent, and in
 * production Nginx serves the dashboard and proxies /api from the same origin.
 * Set VITE_API_BASE only when the dashboard is hosted apart from the agent.
 */
const BASE = import.meta.env.VITE_API_BASE || ''

/** Optional Bearer token, matching API_TOKEN on the agent. */
const TOKEN = import.meta.env.VITE_API_TOKEN || ''

function headers(): HeadersInit {
  return TOKEN ? { Authorization: `Bearer ${TOKEN}` } : {}
}

async function get<T>(path: string): Promise<T> {
  const res = await fetch(`${BASE}/api/v1${path}`, {
    headers: headers(),
    cache: 'no-store',
  })
  if (!res.ok) {
    throw new Error(`${path} responded ${res.status}`)
  }
  return (await res.json()) as T
}

export const api = {
  config: () => get<AgentConfig>('/config'),
  overview: () => get<Overview>('/overview'),
  domains: () => get<DomainGroup[]>('/domains'),
  targets: () => get<TargetState[]>('/targets'),
  target: (id: string) => get<TargetDetail>(`/targets/${encodeURIComponent(id)}`),
  history: (id: string, range: string) =>
    get<HistoryResponse>(`/targets/${encodeURIComponent(id)}/history?range=${range}`),
  incidents: (limit = 50) => get<Incident[]>(`/incidents?limit=${limit}`),
  expiry: () => get<ExpiryItem[]>('/expiry'),
  events: () => get<StatusEvent[]>('/events'),

  async recheck(): Promise<void> {
    await fetch(`${BASE}/api/v1/recheck`, { method: 'POST', headers: headers() })
  },
}

/**
 * WebSocket URL for the live stream.
 *
 * The token travels as a query parameter because a browser cannot set headers
 * on a WebSocket handshake.
 */
export function streamUrl(): string {
  const base = BASE || window.location.origin
  const url = new URL(`${base}/ws/v1`, window.location.href)
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  if (TOKEN) url.searchParams.set('token', TOKEN)
  return url.toString()
}
