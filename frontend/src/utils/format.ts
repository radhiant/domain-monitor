/**
 * Display formatters.
 *
 * Everything on a wall display is read at a glance from several metres away,
 * so the rules here favour short, unambiguous strings over precision: "1.2 s"
 * beats "1243 ms", and "3 hari" beats a timestamp nobody will do arithmetic on.
 */

const TIMEZONE = 'Asia/Jakarta'

/** "HH:MM:SS" dalam zona waktu Asia/Jakarta (WIB). */
export function formatTime24(input: Date | string | number): string {
  const d = toDate(input)
  return d.toLocaleTimeString('id-ID', {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
    timeZone: TIMEZONE,
  }).replace(/\./g, ':')
}

/** "HH:MM" dalam zona waktu Asia/Jakarta (WIB). */
export function formatTimeShort(input: Date | string | number): string {
  const d = toDate(input)
  return d.toLocaleTimeString('id-ID', {
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
    timeZone: TIMEZONE,
  }).replace(/\./g, ':')
}

/** Current wall clock as "HH:MM:SS" dalam zona waktu Asia/Jakarta (WIB). */
export function nowTime24(): string {
  return formatTime24(new Date())
}

/** "Kamis, 28 Agustus 2026" dalam zona waktu Asia/Jakarta (WIB). */
export function formatDateLong(input: Date | string | number = new Date()): string {
  return toDate(input).toLocaleDateString('id-ID', {
    weekday: 'long',
    day: 'numeric',
    month: 'long',
    year: 'numeric',
    timeZone: TIMEZONE,
  })
}

/** "28 Agu 2026" dalam zona waktu Asia/Jakarta (WIB). */
export function formatDateShort(input: Date | string | number): string {
  return toDate(input).toLocaleDateString('id-ID', {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
    timeZone: TIMEZONE,
  })
}

/**
 * Response time. Sub-second values stay in milliseconds because that is the
 * unit operators compare; anything slower switches to seconds so the number
 * never grows past four digits on screen.
 */
export function formatMs(ms: number | undefined | null): string {
  if (ms === undefined || ms === null || Number.isNaN(ms)) return '--'
  if (ms >= 1000) return `${(ms / 1000).toFixed(2)} s`
  return `${Math.round(ms)} ms`
}

/** Uptime percentage with the precision the figure deserves. */
export function formatPercent(value: number | undefined | null, digits = 2): string {
  if (value === undefined || value === null || Number.isNaN(value)) return '--'
  return `${value.toFixed(digits)}%`
}

/** Remaining days, phrased for the expiry watchlist. */
export function formatDaysLeft(days: number | undefined | null): string {
  if (days === undefined || days === null) return '--'
  if (days < 0) return `lewat ${Math.abs(days)} hari`
  if (days === 0) return 'hari ini'
  return `${days} hari`
}

/** Elapsed seconds as "3h 12m" / "12m 30s" / "30s", for incident duration. */
export function formatDuration(seconds: number | undefined | null): string {
  if (!seconds || seconds < 0) return '0s'
  const total = Math.floor(seconds)
  const days = Math.floor(total / 86400)
  const hours = Math.floor((total % 86400) / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  const secs = total % 60

  if (days > 0) return `${days}h ${hours}j`
  if (hours > 0) return `${hours}j ${minutes}m`
  if (minutes > 0) return `${minutes}m ${secs}s`
  return `${secs}s`
}

/** "baru saja" / "5 menit lalu" / "2 jam lalu", for the ticker and feeds. */
export function relativeTime(input: Date | string | number): string {
  const seconds = Math.floor((Date.now() - toDate(input).getTime()) / 1000)
  if (seconds < 45) return 'baru saja'
  if (seconds < 3600) return `${Math.floor(seconds / 60)} menit lalu`
  if (seconds < 86400) return `${Math.floor(seconds / 3600)} jam lalu`
  return `${Math.floor(seconds / 86400)} hari lalu`
}

/**
 * Splits a hostname into the part that identifies it and the part that repeats
 * across the whole panel. The tile renders the first in white and the second
 * in grey, so a wall of subdomains scans as a list of names rather than a wall
 * of the same domain written 20 times.
 */
export function hostParts(host: string, apex: string): { lead: string; rest: string } {
  if (host === apex) return { lead: apex, rest: '' }
  const suffix = `.${apex}`
  if (host.endsWith(suffix)) {
    return { lead: host.slice(0, -suffix.length), rest: suffix }
  }
  return { lead: host, rest: '' }
}

/** Trims a long value to fit a fixed-width cell. */
export function truncate(value: string | undefined, max: number): string {
  if (!value) return ''
  return value.length > max ? `${value.slice(0, max - 1)}…` : value
}

function toDate(input: Date | string | number): Date {
  if (input instanceof Date) return input
  if (typeof input === 'number') return new Date(input)
  return new Date(input)
}
