<script setup lang="ts">
import { computed } from 'vue'
import { Activity, AlertTriangle, CalendarClock, Gauge, ShieldCheck, Signal } from 'lucide-vue-next'
import KpiTile from '@/components/wall/KpiTile.vue'
import { useDomainStore } from '@/stores/domainStore'
import { expiryStatus, uptimeStatus, latencyStatus } from '@/config/thresholds'
import { formatMs, formatPercent, truncate } from '@/utils/format'
import type { Status } from '@/types/domain'

const store = useDomainStore()

const ov = computed(() => store.overview)

const online = computed(() => {
  const o = ov.value
  if (!o) return { value: '--', context: '', status: 'UNKNOWN' as Status }

  // Degraded endpoints still answer, so they count as online here; the
  // separate "terganggu" figure in the context line is what flags them.
  const reachable = o.up + o.degraded
  const parts: string[] = []
  if (o.degraded) parts.push(`${o.degraded} terganggu`)
  if (o.down) parts.push(`${o.down} mati`)
  if (o.unknown) parts.push(`${o.unknown} belum dicek`)

  return {
    value: `${reachable}/${o.total_endpoints}`,
    context: parts.length ? parts.join(' · ') : 'semua endpoint normal',
    status: (o.down > 0 ? 'DOWN' : o.degraded > 0 ? 'DEGRADED' : 'UP') as Status,
  }
})

const uptime = computed(() => {
  const value = ov.value?.uptime_day ?? 0
  return {
    value: value > 0 ? formatPercent(value, 2).replace('%', '') : '--',
    status: uptimeStatus(value),
  }
})

const latency = computed(() => {
  const o = ov.value
  if (!o) return { value: '--', context: '', status: 'UNKNOWN' as Status }
  return {
    value: formatMs(o.avg_latency_ms).replace(/ (ms|s)$/, ''),
    unit: o.avg_latency_ms >= 1000 ? 's' : 'ms',
    context: o.slowest_id ? `terlambat ${truncate(o.slowest_id, 24)} ${formatMs(o.slowest_ms)}` : '',
    status: latencyStatus(o.avg_latency_ms, store.config),
  }
})

const incidents = computed(() => {
  const list = ov.value?.incidents ?? []
  return {
    value: String(list.length),
    context: list.length ? truncate(list[0].target_id, 28) : 'tidak ada gangguan aktif',
    status: (list.length ? 'DOWN' : 'UP') as Status,
  }
})

const ssl = computed(() => {
  const item = ov.value?.next_ssl
  if (!item) return { value: '--', context: 'belum ada data sertifikat', status: 'UNKNOWN' as Status }
  return {
    value: String(item.days_left),
    context: truncate(item.label, 28),
    status: expiryStatus(item.days_left, store.config.cert_warn_days, store.config.cert_crit_days),
  }
})

const domain = computed(() => {
  const item = ov.value?.next_domain
  if (!item) return { value: '--', context: 'belum ada data registrasi', status: 'UNKNOWN' as Status }
  return {
    value: String(item.days_left),
    context: `${item.label}${item.registrar ? ` · ${truncate(item.registrar, 18)}` : ''}`,
    status: expiryStatus(item.days_left, store.config.domain_warn_days, store.config.domain_crit_days),
  }
})

/**
 * Global latency trend.
 *
 * Averaging every endpoint sample by sample gives one line for the estate,
 * which is the only latency figure that means anything at this altitude.
 */
const latencyTrend = computed(() => {
  const series = store.targets.map((t) => t.spark).filter((s) => s.length > 1)
  if (!series.length) return []

  const length = Math.min(...series.map((s) => s.length))
  const out: number[] = []
  for (let i = 0; i < length; i++) {
    let sum = 0
    for (const s of series) sum += s[s.length - length + i]
    out.push(sum / series.length)
  }
  return out
})
</script>

<template>
  <section class="grid grid-cols-2 gap-2.5 md:grid-cols-3 xl:grid-cols-6">
    <KpiTile
      label="Endpoint Online"
      :value="online.value"
      :context="online.context"
      :status="online.status"
      :icon="Signal"
      emphasis
    />
    <KpiTile
      label="Uptime 24 Jam"
      :value="uptime.value"
      unit="%"
      context="rata-rata seluruh endpoint"
      :status="uptime.status"
      :icon="Activity"
      emphasis
    />
    <KpiTile
      label="Respons Rata-rata"
      :value="latency.value"
      :unit="latency.unit"
      :context="latency.context"
      :status="latency.status"
      :icon="Gauge"
      :spark="latencyTrend"
    />
    <KpiTile
      label="Insiden Aktif"
      :value="incidents.value"
      :context="incidents.context"
      :status="incidents.status"
      :icon="AlertTriangle"
      emphasis
    />
    <KpiTile
      label="SSL Terdekat"
      :value="ssl.value"
      unit="hari"
      :context="ssl.context"
      :status="ssl.status"
      :icon="ShieldCheck"
      emphasis
    />
    <KpiTile
      label="Domain Terdekat"
      :value="domain.value"
      unit="hari"
      :context="domain.context"
      :status="domain.status"
      :icon="CalendarClock"
      emphasis
    />
  </section>
</template>
