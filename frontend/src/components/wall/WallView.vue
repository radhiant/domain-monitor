<script setup lang="ts">
import { computed, ref } from 'vue'
import { AlertTriangle, CalendarClock, Globe, LayoutGrid, Loader2, PlugZap } from 'lucide-vue-next'
import KpiBand from '@/components/wall/KpiBand.vue'
import DomainPanel from '@/components/wall/DomainPanel.vue'
import ExpiryWatchlist from '@/components/wall/ExpiryWatchlist.vue'
import IncidentFeed from '@/components/wall/IncidentFeed.vue'
import EventTicker from '@/components/wall/EventTicker.vue'
import { useDomainStore } from '@/stores/domainStore'
import { useRotation } from '@/composables/useRotation'

/**
 * The wall display.
 *
 * In TV mode nothing scrolls: the layout is a fixed column of KPI band, panel
 * area and ticker, and the panel area is the only part that flexes. If more
 * domains arrive than fit, they rotate rather than being squeezed.
 *
 * In mobile/scroll mode: all domains and panels are fully scrollable, with quick
 * section tabs to jump between domains, expiry watchlists, and active incidents.
 */
const store = useDomainStore()

const groups = computed(() => store.groups)

// Two domain panels is what reads comfortably on a 1080p panel from three
// metres. A third would halve the tile height and undo the point of the wall.
const perPage = ref(2)
const { visible, pages, page, rotating, progress } = useRotation(groups, perPage, 20000)

const isTv = computed(() => store.fitMode === 'tv' && (typeof window === 'undefined' || window.innerWidth >= 1024))
const displayGroups = computed(() => (isTv.value ? visible.value : groups.value))

// Mobile section tabs
type MobileTab = 'all' | 'domains' | 'expiry' | 'incidents'
const mobileTab = ref<MobileTab>('all')

const activeIncidentCount = computed(() => store.incidents.length)
const expiryAttentionCount = computed(() => {
  return store.expiry.filter((item) => {
    const days = item.days_left
    return days <= (item.kind === 'ssl' ? store.config.cert_warn_days : store.config.domain_warn_days)
  }).length
})
</script>

<template>
  <main
    class="bg-grid vignette relative flex min-h-0 flex-1 flex-col gap-2.5 p-2 sm:p-2.5"
    :class="isTv ? 'lg:overflow-hidden' : ''"
  >
    <!-- Waiting for the very first payload -->
    <div v-if="!store.hasData && !store.loadError" class="flex flex-1 flex-col items-center justify-center gap-3">
      <Loader2 class="h-8 w-8 animate-spin text-cyan-400" />
      <p class="font-mono text-sm text-slate-500">memuat data domain...</p>
    </div>

    <!--
      The agent is unreachable. Everything the page last knew stays on screen
      when there is data; only a cold start shows this instead, because a blank
      wall is worse than a stale one that says it is stale.
    -->
    <div
      v-else-if="!store.hasData && store.loadError"
      class="flex flex-1 flex-col items-center justify-center gap-3 text-center"
    >
      <PlugZap class="h-10 w-10 text-rose-500" />
      <p class="font-mono text-sm font-semibold text-rose-400">Agent tidak dapat dihubungi</p>
      <p class="max-w-md font-mono text-[11px] text-slate-500">{{ store.loadError }}</p>
    </div>

    <template v-else>
      <KpiBand class="shrink-0" />

      <!-- Mobile Tab Switcher (hidden on desktop xl screens) -->
      <div class="flex xl:hidden items-center gap-1 overflow-x-auto pb-0.5">
        <button
          type="button"
          class="flex items-center gap-1.5 rounded-md border px-2.5 py-1 font-mono text-[11px] font-semibold transition-colors shrink-0"
          :class="mobileTab === 'all'
            ? 'border-cyan-500/50 bg-cyan-500/10 text-cyan-300'
            : 'border-white/[0.07] bg-wall-900/80 text-slate-400 hover:text-slate-200'"
          @click="mobileTab = 'all'"
        >
          <LayoutGrid class="h-3 w-3" />
          <span>SEMUA</span>
        </button>

        <button
          type="button"
          class="flex items-center gap-1.5 rounded-md border px-2.5 py-1 font-mono text-[11px] font-semibold transition-colors shrink-0"
          :class="mobileTab === 'domains'
            ? 'border-cyan-500/50 bg-cyan-500/10 text-cyan-300'
            : 'border-white/[0.07] bg-wall-900/80 text-slate-400 hover:text-slate-200'"
          @click="mobileTab = 'domains'"
        >
          <Globe class="h-3 w-3" />
          <span>DOMAIN ({{ groups.length }})</span>
        </button>

        <button
          type="button"
          class="flex items-center gap-1.5 rounded-md border px-2.5 py-1 font-mono text-[11px] font-semibold transition-colors shrink-0"
          :class="mobileTab === 'expiry'
            ? 'border-cyan-500/50 bg-cyan-500/10 text-cyan-300'
            : 'border-white/[0.07] bg-wall-900/80 text-slate-400 hover:text-slate-200'"
          @click="mobileTab = 'expiry'"
        >
          <CalendarClock class="h-3 w-3" />
          <span>MASA BERLAKU</span>
          <span
            v-if="expiryAttentionCount > 0"
            class="ml-0.5 rounded-full bg-amber-500/20 px-1 text-[9px] font-bold text-amber-400"
          >
            {{ expiryAttentionCount }}
          </span>
        </button>

        <button
          type="button"
          class="flex items-center gap-1.5 rounded-md border px-2.5 py-1 font-mono text-[11px] font-semibold transition-colors shrink-0"
          :class="mobileTab === 'incidents'
            ? 'border-cyan-500/50 bg-cyan-500/10 text-cyan-300'
            : 'border-white/[0.07] bg-wall-900/80 text-slate-400 hover:text-slate-200'"
          @click="mobileTab = 'incidents'"
        >
          <AlertTriangle class="h-3 w-3" :class="activeIncidentCount > 0 ? 'text-rose-400' : ''" />
          <span>INSIDEN</span>
          <span
            v-if="activeIncidentCount > 0"
            class="ml-0.5 rounded-full bg-rose-500/20 px-1 text-[9px] font-bold text-rose-400 animate-pulse"
          >
            {{ activeIncidentCount }}
          </span>
        </button>
      </div>

      <!-- Main content area -->
      <div class="grid min-h-0 flex-1 grid-cols-1 gap-2.5 xl:grid-cols-[minmax(0,1fr)_340px]">
        <!-- Domain panels container: visible if tab is 'all' or 'domains', or on desktop xl -->
        <div
          v-show="mobileTab === 'all' || mobileTab === 'domains'"
          class="flex min-h-0 flex-col gap-2.5"
          :class="{ 'xl:flex': true }"
        >
          <DomainPanel
            v-for="group in displayGroups"
            :key="group.apex"
            :group="group"
            :class="isTv ? 'min-h-[150px]' : ''"
            :style="isTv ? { flex: `${group.endpoints.length + 3} 1 0%` } : undefined"
          />

          <!-- Rotation progress in TV mode when more than one page -->
          <div v-if="isTv && rotating" class="flex shrink-0 items-center gap-2 px-1">
            <span class="font-mono text-[10px] text-slate-600">
              halaman {{ page + 1 }}/{{ pages }}
            </span>
            <div class="h-[2px] flex-1 overflow-hidden rounded-full bg-white/[0.06]">
              <div
                class="h-full rounded-full bg-cyan-400/70 transition-[width] duration-200 ease-linear"
                :style="{ width: `${progress}%` }"
              ></div>
            </div>
          </div>
        </div>

        <!-- Right rail: Expiry and Incidents -->
        <!-- On desktop (xl): always visible side-by-side -->
        <!-- On mobile (< xl): shown if mobileTab is 'all', 'expiry', or 'incidents' -->
        <aside
          class="grid min-h-0 gap-2.5"
          :class="[
            'xl:grid xl:grid-rows-2',
            mobileTab === 'all' ? 'grid-cols-1' : '',
            mobileTab === 'expiry' || mobileTab === 'incidents' ? 'grid-cols-1' : 'hidden xl:grid'
          ]"
        >
          <ExpiryWatchlist
            v-show="mobileTab === 'all' || mobileTab === 'expiry'"
            :limit="mobileTab === 'expiry' ? 30 : 7"
            :class="isTv ? 'min-h-0' : 'min-h-[220px]'"
          />
          <IncidentFeed
            v-show="mobileTab === 'all' || mobileTab === 'incidents'"
            :class="isTv ? 'min-h-0' : 'min-h-[220px]'"
          />
        </aside>
      </div>

      <EventTicker />
    </template>
  </main>
</template>
