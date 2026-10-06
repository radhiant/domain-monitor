<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import {
  Globe, LayoutGrid, List, Maximize2, Minimize2, Monitor,
  RefreshCw, ScrollText, Link as LinkIcon, Check, ChevronLeft,
} from 'lucide-vue-next'
import { useDomainStore } from '@/stores/domainStore'
import { useClock } from '@/composables/useClock'
import ConnectionBadge from '@/components/common/ConnectionBadge.vue'
import { api } from '@/services/api'
import { formatTimeShort } from '@/utils/format'

const store = useDomainStore()
const { time, date } = useClock()

const isFullscreen = ref(false)
const copied = ref(false)
const rechecking = ref(false)

const syncFullscreen = () => {
  isFullscreen.value = document.fullscreenElement !== null
}

onMounted(() => document.addEventListener('fullscreenchange', syncFullscreen))
onUnmounted(() => document.removeEventListener('fullscreenchange', syncFullscreen))

const toggleFullscreen = async () => {
  try {
    if (document.fullscreenElement) {
      await document.exitFullscreen()
    } else {
      await document.documentElement.requestFullscreen()
    }
  } catch {
    // Denied or blocked by the browser; nothing useful to show.
  }
}

const copyLink = async () => {
  try {
    await navigator.clipboard.writeText(window.location.href)
    copied.value = true
    setTimeout(() => (copied.value = false), 2000)
  } catch {
    // The clipboard API is unavailable over plain HTTP in some browsers.
  }
}

const recheck = async () => {
  rechecking.value = true
  try {
    await api.recheck()
  } finally {
    setTimeout(() => (rechecking.value = false), 1500)
  }
}

const lastScan = computed(() =>
  store.overview?.last_scan ? formatTimeShort(store.overview.last_scan) : '--:--',
)

const scope = computed(() => {
  const ov = store.overview
  if (!ov) return ''
  return `${ov.total_endpoints} endpoint · ${ov.total_domains} domain`
})
</script>

<template>
  <header class="shrink-0 border-b border-white/[0.07] bg-wall-900/70 px-3 sm:px-4 py-1.5 sm:py-2 backdrop-blur-md">
    <div class="mx-auto flex max-w-[2560px] items-center justify-between gap-x-2 sm:gap-x-4">
      <!-- Identity and navigation -->
      <div class="flex items-center gap-1.5 sm:gap-3 min-w-0">
        <button
          type="button"
          class="flex items-center gap-1.5 sm:gap-2 sm:border-r border-white/[0.07] sm:pr-3 transition-opacity hover:opacity-75 shrink-0"
          title="Kembali ke tampilan dinding"
          @click="store.setView('wall')"
        >
          <Globe class="h-4 w-4 sm:h-5 sm:w-5 text-cyan-400 shrink-0" />
          <span class="font-mono text-xs sm:text-sm font-bold tracking-tight text-slate-100">
            DOMAIN<span class="text-cyan-400">MONITOR</span>
          </span>
        </button>

        <div class="flex items-center gap-1 sm:gap-1.5 shrink-0">
          <button
            type="button"
            class="flex items-center gap-1 sm:gap-1.5 rounded-md border px-2 sm:px-2.5 py-1 font-mono text-[10px] sm:text-[11px] font-semibold transition-colors"
            :class="store.view === 'wall'
              ? 'border-cyan-500/50 bg-cyan-500/10 text-cyan-300'
              : 'border-white/[0.07] bg-wall-900 text-slate-400 hover:border-cyan-500/40 hover:text-cyan-300'"
            @click="store.setView('wall')"
          >
            <LayoutGrid class="h-3 w-3 sm:h-3.5 sm:w-3.5" />
            <span>DINDING</span>
          </button>

          <button
            type="button"
            class="flex items-center gap-1 sm:gap-1.5 rounded-md border px-2 sm:px-2.5 py-1 font-mono text-[10px] sm:text-[11px] font-semibold transition-colors"
            :class="store.view === 'list'
              ? 'border-cyan-500/50 bg-cyan-500/10 text-cyan-300'
              : 'border-white/[0.07] bg-wall-900 text-slate-400 hover:border-cyan-500/40 hover:text-cyan-300'"
            @click="store.setView('list')"
          >
            <List class="h-3 w-3 sm:h-3.5 sm:w-3.5" />
            <span>TABEL</span>
          </button>

          <button
            v-if="store.view === 'detail'"
            type="button"
            class="flex items-center gap-1 sm:gap-1.5 rounded-md border border-white/[0.07] bg-wall-900 px-2 sm:px-2.5 py-1 font-mono text-[10px] sm:text-[11px] font-semibold text-slate-300 transition-colors hover:border-cyan-500/40 hover:text-cyan-300"
            @click="store.setView('wall')"
          >
            <ChevronLeft class="h-3 w-3 sm:h-3.5 sm:w-3.5" />
            <span>KEMBALI</span>
          </button>
        </div>

        <span v-if="scope" class="hidden font-mono text-[11px] text-slate-500 lg:inline truncate">
          {{ scope }}
        </span>
      </div>

      <!-- Clock, scan age and display controls -->
      <div class="flex items-center gap-1.5 sm:gap-2 font-mono text-[11px] shrink-0">
        <div class="hidden flex-col items-end leading-tight lg:flex">
          <span class="num text-base font-semibold tracking-wider text-slate-100">{{ time }}</span>
          <span class="text-[10px] text-slate-500">{{ date }}</span>
        </div>

        <div class="hidden items-center gap-1.5 rounded border border-white/[0.07] bg-wall-950/60 px-2 py-1 text-slate-400 sm:flex">
          <span class="text-slate-500">SCAN</span>
          <span class="num font-semibold text-slate-300">{{ lastScan }}</span>
        </div>

        <button
          type="button"
          class="rounded border border-white/[0.07] bg-wall-950 p-1 sm:p-1.5 text-slate-400 transition-colors hover:border-cyan-500/40 hover:text-cyan-300"
          title="Periksa ulang semua endpoint sekarang"
          aria-label="Periksa ulang semua endpoint"
          @click="recheck"
        >
          <RefreshCw class="h-3.5 w-3.5" :class="rechecking ? 'animate-spin' : ''" />
        </button>

        <button
          type="button"
          class="hidden sm:inline-flex rounded border border-white/[0.07] bg-wall-950 p-1.5 text-slate-400 transition-colors hover:border-cyan-500/40 hover:text-cyan-300"
          :title="copied ? 'Link tersalin!' : 'Salin link tampilan ini'"
          :aria-label="copied ? 'Link tersalin' : 'Salin link tampilan ini'"
          @click="copyLink"
        >
          <Check v-if="copied" class="h-3.5 w-3.5 text-emerald-400" />
          <LinkIcon v-else class="h-3.5 w-3.5" />
        </button>

        <!-- TV mode locks everything into one screen; scroll mode suits laptops. -->
        <button
          type="button"
          class="hidden md:flex items-center gap-1.5 rounded border px-2 py-1 font-semibold transition-colors"
          :class="store.fitMode === 'tv'
            ? 'border-cyan-500/40 bg-cyan-500/10 text-cyan-300'
            : 'border-white/[0.07] bg-wall-950 text-slate-400 hover:text-slate-200'"
          :title="store.fitMode === 'tv'
            ? 'Mode TV: seluruh dashboard dikunci dalam satu layar'
            : 'Mode gulir: halaman memanjang, cocok untuk layar kecil'"
          @click="store.toggleFitMode()"
        >
          <Monitor v-if="store.fitMode === 'tv'" class="h-3.5 w-3.5" />
          <ScrollText v-else class="h-3.5 w-3.5" />
          <span>{{ store.fitMode === 'tv' ? 'TV' : 'GULIR' }}</span>
        </button>

        <button
          type="button"
          class="hidden sm:inline-flex rounded border border-white/[0.07] bg-wall-950 p-1.5 text-slate-400 transition-colors hover:border-cyan-500/40 hover:text-cyan-300"
          :aria-label="isFullscreen ? 'Keluar dari layar penuh' : 'Layar penuh'"
          :title="isFullscreen ? 'Keluar dari layar penuh' : 'Layar penuh'"
          @click="toggleFullscreen"
        >
          <Minimize2 v-if="isFullscreen" class="h-3.5 w-3.5" />
          <Maximize2 v-else class="h-3.5 w-3.5" />
        </button>

        <ConnectionBadge :state="store.connection" />
      </div>
    </div>
  </header>
</template>
