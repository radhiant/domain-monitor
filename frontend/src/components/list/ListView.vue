<script setup lang="ts">
import { computed, ref } from 'vue'
import { ArrowDown, ArrowUp, Search } from 'lucide-vue-next'
import StatusDot from '@/components/common/StatusDot.vue'
import Sparkline from '@/components/common/Sparkline.vue'
import { expiryStatus, themeOf } from '@/config/thresholds'
import { formatMs, formatPercent, formatTimeShort } from '@/utils/format'
import { useDomainStore } from '@/stores/domainStore'
import type { TargetState } from '@/types/domain'

/**
 * The desk view.
 *
 * The wall is built for glancing; this is built for looking things up. Same
 * data, sortable and searchable, dense enough to see every endpoint at once
 * on a normal monitor, with mobile cards on phones.
 */
const store = useDomainStore()

type SortKey = 'host' | 'status' | 'latency' | 'uptime' | 'ssl' | 'code'

const sortKey = ref<SortKey>('status')
const sortAsc = ref(false)
const query = ref('')
const mobileViewMode = ref<'cards' | 'table'>('cards')

const STATUS_ORDER: Record<string, number> = { DOWN: 3, DEGRADED: 2, UNKNOWN: 1, UP: 0 }

function valueOf(target: TargetState, key: SortKey): number | string {
  switch (key) {
    case 'host':
      return target.host
    case 'status':
      return STATUS_ORDER[target.status] ?? 0
    case 'latency':
      return target.http?.latency_ms ?? -1
    case 'uptime':
      return target.uptime.day ?? -1
    case 'ssl':
      return target.cert?.days_left ?? Number.MAX_SAFE_INTEGER
    case 'code':
      return target.http?.status_code ?? 0
  }
}

const rows = computed(() => {
  const needle = query.value.trim().toLowerCase()
  const filtered = needle
    ? store.targets.filter((t) => t.host.includes(needle) || t.apex.includes(needle))
    : [...store.targets]

  return filtered.sort((a, b) => {
    const av = valueOf(a, sortKey.value)
    const bv = valueOf(b, sortKey.value)
    const cmp = typeof av === 'string' && typeof bv === 'string' ? av.localeCompare(bv) : Number(av) - Number(bv)
    return sortAsc.value ? cmp : -cmp
  })
})

function sortBy(key: SortKey) {
  if (sortKey.value === key) {
    sortAsc.value = !sortAsc.value
    return
  }
  sortKey.value = key
  // Text sorts read naturally ascending; every metric here is more useful
  // worst-first, which is what an operator opens this view to find.
  sortAsc.value = key === 'host'
}

const columns: { key: SortKey; label: string; align: string }[] = [
  { key: 'host', label: 'Endpoint', align: 'text-left' },
  { key: 'status', label: 'Status', align: 'text-left' },
  { key: 'code', label: 'Kode', align: 'text-right' },
  { key: 'latency', label: 'Respons', align: 'text-right' },
  { key: 'uptime', label: 'Uptime 24j', align: 'text-right' },
  { key: 'ssl', label: 'SSL', align: 'text-right' },
]
</script>

<template>
  <main class="bg-grid min-h-0 flex-1 overflow-auto p-2 sm:p-2.5">
    <div class="panel flex min-h-full flex-col">
      <header class="flex shrink-0 flex-col sm:flex-row sm:items-center justify-between gap-2.5 sm:gap-3 border-b border-white/[0.07] px-3 py-2">
        <div class="flex items-center justify-between gap-2">
          <span class="panel-title">Semua Endpoint · {{ rows.length }}</span>

          <!-- Mobile view toggle: cards vs table -->
          <div class="flex sm:hidden items-center gap-1">
            <button
              type="button"
              class="rounded px-2 py-0.5 font-mono text-[10px] font-semibold border transition-colors"
              :class="mobileViewMode === 'cards'
                ? 'border-cyan-500/50 bg-cyan-500/10 text-cyan-300'
                : 'border-white/[0.07] text-slate-500'"
              @click="mobileViewMode = 'cards'"
            >
              KARTU
            </button>
            <button
              type="button"
              class="rounded px-2 py-0.5 font-mono text-[10px] font-semibold border transition-colors"
              :class="mobileViewMode === 'table'
                ? 'border-cyan-500/50 bg-cyan-500/10 text-cyan-300'
                : 'border-white/[0.07] text-slate-500'"
              @click="mobileViewMode = 'table'"
            >
              TABEL
            </button>
          </div>
        </div>

        <label class="flex items-center gap-2 rounded border border-white/[0.07] bg-wall-950 px-2.5 py-1.5 sm:py-1 w-full sm:w-auto">
          <Search class="h-3.5 w-3.5 text-slate-500 shrink-0" />
          <input
            v-model="query"
            type="search"
            placeholder="cari host atau domain..."
            class="w-full sm:w-48 bg-transparent font-mono text-xs sm:text-[11px] text-slate-200 placeholder:text-slate-600 focus:outline-none"
          />
        </label>
      </header>

      <!-- Mobile Card List (shown on mobile when cards mode is active) -->
      <div v-if="mobileViewMode === 'cards'" class="sm:hidden divide-y divide-white/[0.05]">
        <div
          v-for="target in rows"
          :key="target.id"
          class="p-3 transition-colors active:bg-white/[0.04] cursor-pointer flex flex-col gap-1.5"
          @click="store.openTarget(target.id)"
        >
          <!-- Row 1: status & host -->
          <div class="flex items-center justify-between gap-2">
            <div class="flex items-center gap-2 min-w-0">
              <StatusDot :status="target.status" size="sm" />
              <span class="font-mono text-xs font-semibold text-slate-200 truncate">{{ target.host }}</span>
            </div>
            <span class="num text-[11px] font-bold shrink-0" :class="themeOf(target.status).text">
              {{ themeOf(target.status).label }}
            </span>
          </div>

          <!-- Reason if any -->
          <div v-if="target.reason" class="font-mono text-[10px] text-rose-400 pl-4 truncate">
            {{ target.reason }}
          </div>

          <!-- Row 2: Metrics grid -->
          <div class="flex items-center justify-between gap-2 font-mono text-[11px] pl-4 text-slate-400">
            <div class="flex items-center gap-2">
              <span :class="(target.http?.status_code ?? 0) >= 400 ? themeOf(target.status).text : 'text-slate-400'">
                {{ target.http?.status_code ? `HTTP ${target.http.status_code}` : '--' }}
              </span>
              <span>·</span>
              <span class="text-slate-200 font-semibold">{{ formatMs(target.http?.latency_ms) }}</span>
            </div>
            <div class="flex items-center gap-2">
              <span>SSL <span :class="themeOf(expiryStatus(target.cert?.days_left, store.config.cert_warn_days, store.config.cert_crit_days)).text">{{ target.cert?.days_left ? `${target.cert.days_left}h` : '--' }}</span></span>
              <span>·</span>
              <span class="text-slate-300">{{ target.uptime.samples ? formatPercent(target.uptime.day) : '--' }}</span>
            </div>
          </div>
        </div>

        <div v-if="!rows.length" class="px-3 py-8 text-center font-mono text-[11px] text-slate-600">
          tidak ada endpoint yang cocok
        </div>
      </div>

      <!-- Desktop & Mobile Table View -->
      <div
        class="min-w-0 flex-1 overflow-x-auto"
        :class="mobileViewMode === 'cards' ? 'hidden sm:block' : 'block'"
      >
        <table class="w-full min-w-[820px] border-collapse">
          <thead>
            <tr class="border-b border-white/[0.07]">
              <th
                v-for="col in columns"
                :key="col.key"
                class="cursor-pointer select-none px-3 py-2 font-mono text-[10px] font-semibold uppercase tracking-[0.18em] text-slate-500 transition-colors hover:text-cyan-300"
                :class="col.align"
                @click="sortBy(col.key)"
              >
                <span class="inline-flex items-center gap-1">
                  {{ col.label }}
                  <component
                    :is="sortAsc ? ArrowUp : ArrowDown"
                    v-if="sortKey === col.key"
                    class="h-3 w-3 text-cyan-400"
                  />
                </span>
              </th>
              <th class="px-3 py-2 text-left font-mono text-[10px] font-semibold uppercase tracking-[0.18em] text-slate-500">
                Tren
              </th>
              <th class="px-3 py-2 text-right font-mono text-[10px] font-semibold uppercase tracking-[0.18em] text-slate-500">
                Cek
              </th>
            </tr>
          </thead>

          <tbody>
            <tr
              v-for="target in rows"
              :key="target.id"
              class="cursor-pointer border-b border-white/[0.04] transition-colors hover:bg-white/[0.03]"
              @click="store.openTarget(target.id)"
            >
              <td class="px-3 py-1.5">
                <div class="flex items-center gap-2">
                  <StatusDot :status="target.status" size="sm" />
                  <span class="font-mono text-[12px] text-slate-200">{{ target.host }}</span>
                </div>
              </td>

              <td class="px-3 py-1.5">
                <span class="num text-[11px] font-bold" :class="themeOf(target.status).text">
                  {{ themeOf(target.status).label }}
                </span>
                <span v-if="target.reason" class="ml-2 font-mono text-[10px] text-slate-600">
                  {{ target.reason }}
                </span>
              </td>

              <td class="num px-3 py-1.5 text-right text-[11px]"
                  :class="(target.http?.status_code ?? 0) >= 400 ? themeOf(target.status).text : 'text-slate-400'">
                {{ target.http?.status_code || '--' }}
              </td>

              <td class="num px-3 py-1.5 text-right text-[12px] text-slate-200">
                {{ formatMs(target.http?.latency_ms) }}
              </td>

              <td class="num px-3 py-1.5 text-right text-[12px] text-slate-300">
                {{ target.uptime.samples ? formatPercent(target.uptime.day) : '--' }}
              </td>

              <td class="num px-3 py-1.5 text-right text-[12px]"
                  :class="themeOf(expiryStatus(target.cert?.days_left, store.config.cert_warn_days, store.config.cert_crit_days)).text">
                {{ target.cert?.days_left ?? '--' }}
              </td>

              <td class="w-28 px-3 py-1.5">
                <Sparkline :values="target.spark" :height="16" :filled="false" />
              </td>

              <td class="num px-3 py-1.5 text-right text-[10px] text-slate-600">
                {{ target.http ? formatTimeShort(target.http.checked_at) : '--' }}
              </td>
            </tr>

            <tr v-if="!rows.length">
              <td colspan="8" class="px-3 py-8 text-center font-mono text-[11px] text-slate-600">
                tidak ada endpoint yang cocok
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </main>
</template>
