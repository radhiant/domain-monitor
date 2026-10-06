<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { ExternalLink, Loader2 } from 'lucide-vue-next'
import LatencyChart from '@/components/detail/LatencyChart.vue'
import UptimeBars from '@/components/detail/UptimeBars.vue'
import CertPanel from '@/components/detail/CertPanel.vue'
import DnsPanel from '@/components/detail/DnsPanel.vue'
import IncidentTable from '@/components/detail/IncidentTable.vue'
import StatusDot from '@/components/common/StatusDot.vue'
import { themeOf } from '@/config/thresholds'
import { api } from '@/services/api'
import { useDomainStore } from '@/stores/domainStore'
import { formatMs, formatPercent } from '@/utils/format'
import { prometheusApi } from '@/services/prometheus'
import type { HistoryPoint, Incident } from '@/types/domain'

/**
 * Everything known about one endpoint.
 *
 * The live figures come from the store, which the WebSocket already keeps
 * current; historical series and SLA metrics are queried dynamically
 * using Prometheus and SQLite fallback.
 */
const store = useDomainStore()

const RANGES = [
  { key: '24h', label: '24 JAM' },
  { key: '7d', label: '7 HARI' },
  { key: '30d', label: '30 HARI' },
  { key: '90d', label: '90 HARI' },
  { key: '1y', label: '1 TAHUN' },
]

const range = ref('24h')
const points = ref<HistoryPoint[]>([])
const incidents = ref<Incident[]>([])
const slaPercent = ref<number | null>(null)
const loading = ref(false)

const target = computed(() => store.selected)
const theme = computed(() => themeOf(target.value?.status ?? 'UNKNOWN'))

const displaySla = computed(() => {
  if (slaPercent.value !== null) return slaPercent.value
  if (!points.value.length) return null
  let up = 0, total = 0
  for (const p of points.value) {
    up += p.up
    total += p.total
  }
  return total > 0 ? (100 * up) / total : null
})

async function load() {
  const id = store.selectedId
  if (!id) return

  loading.value = true
  try {
    const fetchHistory = async () => {
      if (range.value === '90d' || range.value === '1y') {
        const promPts = await prometheusApi.getHistory(id, range.value)
        if (promPts && promPts.length > 0) {
          return { points: promPts, target_id: id, range: range.value, bucket_s: 0 }
        }
      }
      return api.history(id, range.value)
    }

    const [history, detail, promSla] = await Promise.all([
      fetchHistory(),
      api.target(id),
      prometheusApi.getSla(id, range.value),
    ])
    points.value = history.points
    incidents.value = detail.incidents
    slaPercent.value = promSla
  } catch {
    points.value = []
    incidents.value = []
    slaPercent.value = null
  } finally {
    loading.value = false
  }
}

// The history table only gains a row per probe interval, so a slow refresh is
// plenty; the live numbers above it update over the WebSocket regardless.
let timer: ReturnType<typeof setInterval> | null = null

onMounted(() => {
  load()
  timer = setInterval(load, 60000)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
})

watch(() => [store.selectedId, range.value], load)
</script>

<template>
  <main class="bg-grid min-h-0 flex-1 overflow-auto p-2 sm:p-2.5">
    <div v-if="!target" class="flex h-full items-center justify-center">
      <Loader2 class="h-8 w-8 animate-spin text-cyan-400" />
    </div>

    <div v-else class="flex flex-col gap-2.5">
      <!-- Identity and the summary figures for this endpoint -->
      <section class="panel flex flex-col md:flex-row md:items-center justify-between gap-4 px-3 sm:px-4 py-3">
        <div class="flex min-w-0 items-center gap-3">
          <StatusDot :status="target.status" size="lg" />
          <div class="min-w-0 flex-1">
            <h1 class="truncate font-mono text-lg sm:text-xl font-bold tracking-tight text-slate-100">
              {{ target.host }}
            </h1>
            <div class="flex flex-wrap items-center gap-2 font-mono text-[11px]">
              <span class="font-bold shrink-0" :class="theme.text">{{ theme.label }}</span>
              <span v-if="target.reason" class="text-slate-500 shrink-0">{{ target.reason }}</span>
              <a
                :href="target.url"
                target="_blank"
                rel="noopener noreferrer"
                class="inline-flex items-center gap-1 text-slate-500 transition-colors hover:text-cyan-400 truncate max-w-[200px] sm:max-w-none"
              >
                <span class="truncate">{{ target.url }}</span>
                <ExternalLink class="h-3 w-3 shrink-0" />
              </a>
            </div>
          </div>
        </div>

        <div class="grid grid-cols-2 sm:grid-cols-3 md:flex md:flex-wrap items-center gap-3 sm:gap-4 md:gap-x-6 md:gap-y-2 border-t md:border-t-0 border-white/[0.06] pt-3 md:pt-0">
          <div class="text-left md:text-right">
            <div class="num text-xl sm:text-2xl font-bold leading-none text-slate-100">
              {{ formatMs(target.http?.latency_ms) }}
            </div>
            <div class="panel-title text-[10px]">respons</div>
          </div>
          <div class="text-left md:text-right">
            <div
              class="num text-xl sm:text-2xl font-bold leading-none"
              :class="displaySla !== null && displaySla >= 99.5 ? 'text-emerald-400' : displaySla !== null && displaySla >= 99.0 ? 'text-cyan-400' : 'text-amber-400'"
            >
              {{ displaySla !== null ? formatPercent(displaySla) : '--' }}
            </div>
            <div class="panel-title text-[10px]">SLA ({{ RANGES.find(r => r.key === range)?.label }})</div>
          </div>
          <div class="text-left md:text-right">
            <div class="num text-xl sm:text-2xl font-bold leading-none text-slate-100">
              {{ target.uptime.samples ? formatPercent(target.uptime.day) : '--' }}
            </div>
            <div class="panel-title text-[10px]">uptime 24j</div>
          </div>
          <div class="text-left md:text-right">
            <div class="num text-xl sm:text-2xl font-bold leading-none text-slate-100">
              {{ target.uptime.samples ? formatPercent(target.uptime.week) : '--' }}
            </div>
            <div class="panel-title text-[10px]">uptime 7h</div>
          </div>
          <div class="text-left md:text-right col-span-2 sm:col-span-1">
            <div class="num text-xl sm:text-2xl font-bold leading-none text-slate-100">
              {{ formatMs(target.uptime.p95_ms) }}
            </div>
            <div class="panel-title text-[10px]">p95</div>
          </div>
        </div>
      </section>

      <!-- Response time and availability over the selected window -->
      <section class="panel flex flex-col">
        <header class="flex flex-col sm:flex-row sm:items-center justify-between gap-2.5 sm:gap-3 border-b border-white/[0.07] px-3 py-2">
          <div class="flex flex-wrap items-center gap-2">
            <span class="panel-title">Waktu Respons</span>
            <span
              v-if="displaySla !== null"
              class="inline-flex items-center gap-1 rounded bg-emerald-500/10 border border-emerald-500/30 px-2 py-0.5 text-[10px] font-mono font-bold text-emerald-400"
            >
              SLA {{ RANGES.find(r => r.key === range)?.label }}: {{ formatPercent(displaySla) }}
            </span>
          </div>
          <div class="flex items-center gap-1 overflow-x-auto pb-0.5 sm:pb-0">
            <button
              v-for="r in RANGES"
              :key="r.key"
              type="button"
              class="shrink-0 rounded border px-2 py-0.5 font-mono text-[10px] font-semibold transition-colors"
              :class="range === r.key
                ? 'border-cyan-500/50 bg-cyan-500/10 text-cyan-300'
                : 'border-white/[0.07] text-slate-500 hover:text-slate-300'"
              @click="range = r.key"
            >{{ r.label }}</button>
          </div>
        </header>

        <div class="h-48 sm:h-56 p-2">
          <LatencyChart :points="points" :loading="loading" />
        </div>

        <div class="border-t border-white/[0.07] px-3 py-2">
          <div class="panel-title mb-1.5 text-[10px]">Ketersediaan</div>
          <div class="h-10">
            <UptimeBars :points="points" />
          </div>
        </div>
      </section>

      <!-- Certificate, DNS and outage history -->
      <div class="grid gap-2.5 grid-cols-1 lg:grid-cols-3">
        <CertPanel :cert="target.cert" />
        <DnsPanel
          :dns="target.dns"
          :redirects="target.http?.redirects"
          :final-url="target.http?.final_url"
        />
        <IncidentTable :incidents="incidents" class="max-h-80" />
      </div>
    </div>
  </main>
</template>
