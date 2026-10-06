import type { AgentConfig, Status } from '@/types/domain'

/**
 * Fallback thresholds.
 *
 * The agent is the authority — it classifies every status and publishes its
 * own numbers on /api/v1/config. These values only cover the moment before
 * that request lands, and keep the expiry chips rendering if it fails.
 */
export const DEFAULTS: AgentConfig = {
  latency_warn_ms: 800,
  latency_crit_ms: 2000,
  cert_warn_days: 30,
  cert_crit_days: 14,
  domain_warn_days: 60,
  domain_crit_days: 30,
  accept_status: [401, 403],
  http_interval_s: 60,
  targets_file: '',
}

const RANK: Record<Status, number> = {
  UNKNOWN: 0,
  UP: 1,
  DEGRADED: 2,
  DOWN: 3,
}

/** The more alarming of two statuses. */
export function worseOf(a: Status, b: Status): Status {
  return RANK[b] > RANK[a] ? b : a
}

/**
 * One palette per status, so every surface — tile, dot, chip, ring, chart —
 * says the same thing with the same colour.
 */
export interface StatusTheme {
  label: string
  text: string
  dot: string
  bar: string
  border: string
  bg: string
  ring: string
  glow: string
}

const THEMES: Record<Status, StatusTheme> = {
  UP: {
    label: 'NORMAL',
    text: 'text-emerald-400',
    dot: 'bg-emerald-400',
    bar: 'bg-emerald-400',
    border: 'border-emerald-500/25',
    bg: 'bg-emerald-500/[0.04]',
    ring: 'stroke-emerald-400',
    glow: '',
  },
  DEGRADED: {
    label: 'TERGANGGU',
    text: 'text-amber-400',
    dot: 'bg-amber-400',
    bar: 'bg-amber-400',
    border: 'border-amber-500/40',
    bg: 'bg-amber-500/[0.06]',
    ring: 'stroke-amber-400',
    glow: 'glow-warn',
  },
  DOWN: {
    label: 'MATI',
    text: 'text-rose-400',
    dot: 'bg-rose-500',
    bar: 'bg-rose-500',
    border: 'border-rose-500/60',
    bg: 'bg-rose-500/[0.08]',
    ring: 'stroke-rose-500',
    glow: 'glow-down',
  },
  UNKNOWN: {
    label: 'MEMERIKSA',
    text: 'text-slate-500',
    dot: 'bg-slate-600',
    bar: 'bg-slate-600',
    border: 'border-white/[0.07]',
    bg: '',
    ring: 'stroke-slate-600',
    glow: '',
  },
}

export function themeOf(status: Status): StatusTheme {
  return THEMES[status] ?? THEMES.UNKNOWN
}

/**
 * Grades a remaining-days figure for the expiry chips and watchlist.
 * Anything already past its date is critical, not merely a warning.
 */
export function expiryStatus(days: number | undefined, warn: number, crit: number): Status {
  if (days === undefined || days === null) return 'UNKNOWN'
  if (days <= crit) return 'DOWN'
  if (days <= warn) return 'DEGRADED'
  return 'UP'
}

/** Grades a latency against the configured bands. */
export function latencyStatus(ms: number, cfg: AgentConfig): Status {
  if (ms >= cfg.latency_crit_ms) return 'DOWN'
  if (ms >= cfg.latency_warn_ms) return 'DEGRADED'
  return 'UP'
}

/**
 * Uptime bands for the KPI tile. Three nines over a day is roughly a minute
 * and a half of downtime, which is the point at which someone should look.
 */
export function uptimeStatus(percent: number): Status {
  if (percent <= 0) return 'UNKNOWN'
  if (percent < 99) return 'DOWN'
  if (percent < 99.5) return 'DEGRADED'
  return 'UP'
}
