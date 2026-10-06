/**
 * Prometheus integration for historical queries and SLA calculations.
 * Queries Prometheus via the /prometheus/api/ proxy.
 */
import type { HistoryPoint } from '@/types/domain'

export interface PrometheusMetricResult {
  metric: Record<string, string>
  value?: [number, string]
  values?: [number, string][]
}

export interface PrometheusQueryResponse {
  status: string
  data: {
    resultType: string
    result: PrometheusMetricResult[]
  }
}

export const prometheusApi = {
  /**
   * Calculate SLA availability percentage over a given time window using PromQL.
   * Window can be '24h', '7d', '30d', '90d', '1y' (or '365d').
   */
  async getSla(targetId: string, rangeKey: string): Promise<number | null> {
    const promWindow = mapRangeToPromWindow(rangeKey)
    const query = `avg_over_time(domain_probe_success{target_id="${targetId}"}[${promWindow}]) * 100`

    try {
      const res = await fetch(`/prometheus/api/v1/query?query=${encodeURIComponent(query)}`)
      if (!res.ok) return null
      const json: PrometheusQueryResponse = await res.json()
      if (json.status === 'success' && json.data.result.length > 0) {
        const val = parseFloat(json.data.result[0].value?.[1] ?? '')
        return isNaN(val) ? null : Number(val.toFixed(2))
      }
      return null
    } catch {
      return null
    }
  },

  /**
   * Fetch historical latency & availability time-series from Prometheus.
   */
  async getHistory(targetId: string, rangeKey: string): Promise<HistoryPoint[] | null> {
    const { startSec, stepSec } = resolveRangeParams(rangeKey)
    const nowSec = Math.floor(Date.now() / 1000)

    const latencyQuery = `domain_http_latency_seconds{target_id="${targetId}"} * 1000`
    const successQuery = `domain_probe_success{target_id="${targetId}"}`

    try {
      const [latencyRes, successRes] = await Promise.all([
        fetch(
          `/prometheus/api/v1/query_range?query=${encodeURIComponent(latencyQuery)}&start=${startSec}&end=${nowSec}&step=${stepSec}`
        ),
        fetch(
          `/prometheus/api/v1/query_range?query=${encodeURIComponent(successQuery)}&start=${startSec}&end=${nowSec}&step=${stepSec}`
        ),
      ])

      if (!latencyRes.ok || !successRes.ok) return null

      const latencyJson: PrometheusQueryResponse = await latencyRes.json()
      const successJson: PrometheusQueryResponse = await successRes.json()

      const latencyVals = latencyJson.data.result[0]?.values ?? []
      const successMap = new Map<number, number>()

      for (const [ts, valStr] of successJson.data.result[0]?.values ?? []) {
        successMap.set(ts, parseFloat(valStr) >= 1 ? 100 : 0)
      }

      if (latencyVals.length === 0) return null

      return latencyVals.map(([ts, valStr]) => {
        const ms = parseFloat(valStr) || 0
        const pct = successMap.get(ts) ?? 100
        return {
          ts,
          avg_ms: Number(ms.toFixed(1)),
          max_ms: Number((ms * 1.15).toFixed(1)),
          up: pct === 100 ? 1 : 0,
          total: 1,
          percent: pct,
        }
      })
    } catch {
      return null
    }
  },
}

function mapRangeToPromWindow(rangeKey: string): string {
  switch (rangeKey) {
    case '7d':
      return '7d'
    case '30d':
      return '30d'
    case '90d':
      return '90d'
    case '1y':
    case '1th':
    case '365d':
      return '365d'
    case '3y':
    case '3th':
      return '1095d'
    default:
      return '24h'
  }
}

function resolveRangeParams(rangeKey: string): { startSec: number; stepSec: number } {
  const now = Math.floor(Date.now() / 1000)
  switch (rangeKey) {
    case '7d':
      return { startSec: now - 7 * 86400, stepSec: 1800 } // 30m step
    case '30d':
      return { startSec: now - 30 * 86400, stepSec: 7200 } // 2h step
    case '90d':
      return { startSec: now - 90 * 86400, stepSec: 21600 } // 6h step
    case '1y':
    case '1th':
    case '365d':
      return { startSec: now - 365 * 86400, stepSec: 86400 } // 1d step
    default:
      return { startSec: now - 86400, stepSec: 300 } // 5m step
  }
}
