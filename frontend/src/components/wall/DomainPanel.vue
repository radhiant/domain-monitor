<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { Server } from 'lucide-vue-next'
import EndpointTile from '@/components/wall/EndpointTile.vue'
import HealthRing from '@/components/common/HealthRing.vue'
import ExpiryChip from '@/components/common/ExpiryChip.vue'
import StatusDot from '@/components/common/StatusDot.vue'
import { themeOf } from '@/config/thresholds'
import { useRotation } from '@/composables/useRotation'
import { useDomainStore } from '@/stores/domainStore'
import { truncate } from '@/utils/format'
import type { DomainGroup } from '@/types/domain'

/** One apex domain and every endpoint underneath it. */
const props = defineProps<{ group: DomainGroup }>()

const store = useDomainStore()

const theme = computed(() => themeOf(props.group.status))
const registrar = computed(() => props.group.domain?.registrar)

/**
 * Which provider runs the zone, not the individual nameserver hostnames.
 *
 * "frank.ns.cloudflare.com, grace.ns.cloudflare.com" tells an operator nothing
 * they cannot infer; "cloudflare.com" is the fact worth a line on the wall,
 * and a second entry appearing here means the zone is split.
 */
const nameservers = computed(() => {
  const ns = props.group.endpoints.find((t) => t.is_apex)?.dns?.ns
  if (!ns || !ns.length) return ''

  const providers = new Set(
    ns.map((host) => {
      const parts = host.split('.')
      return parts.length > 2 ? parts.slice(-2).join('.') : host
    }),
  )
  return [...providers].join(' · ')
})

/*
 * Tile geometry.
 *
 * TILE_MIN_W and TILE_MIN_H are the smallest a tile may get and still be read
 * from across a room. TILE_ASPECT is the shape it should aim for: wide enough
 * for a hostname, short enough that a domain fills its panel in rows rather
 * than one enormous stripe.
 */
const TILE_MIN_W = 180
const TILE_MIN_H = 84
const TILE_ASPECT = 2.3
const GAP = 8

const bodyEl = ref<HTMLElement | null>(null)
const bodyW = ref(0)
const bodyH = ref(0)

let observer: ResizeObserver | null = null

onMounted(() => {
  if (!bodyEl.value) return
  observer = new ResizeObserver(([entry]) => {
    bodyW.value = entry.contentRect.width
    bodyH.value = entry.contentRect.height
  })
  observer.observe(bodyEl.value)
})

onUnmounted(() => observer?.disconnect())

const endpoints = computed(() => props.group.endpoints)

/**
 * Chooses the column count from the space actually available.
 *
 * Breakpoints cannot do this job: the same "5 columns at 1536px and up" that
 * looks right on a 1080p panel puts seventeen hair-thin columns on a 4K one,
 * and a single row then stretches to the full panel height. So every column
 * count that fits is measured, and the one whose tiles land closest to the
 * intended shape wins.
 *
 * If nothing fits, the panel packs as many legible tiles as it can and the
 * remainder rotates — clipping them would hide endpoints behind the panel
 * edge, and an endpoint nobody can see is worse than one that takes fifteen
 * seconds to come back around.
 */
const layout = computed(() => {
  const width = bodyW.value
  const height = bodyH.value
  const count = endpoints.value.length

  if (!width || !height || !count) {
    return { columns: 1, capacity: Math.max(1, count) }
  }

  let best: { columns: number; score: number } | null = null

  for (let columns = 1; columns <= count; columns++) {
    const tileW = (width - (columns - 1) * GAP) / columns
    // Tiles only get narrower from here, so nothing wider is worth testing.
    if (tileW < TILE_MIN_W) break

    const rows = Math.ceil(count / columns)
    const tileH = (height - (rows - 1) * GAP) / rows
    if (tileH < TILE_MIN_H) continue

    // Log-ratio so being half as wide as intended scores the same as being
    // twice as wide, rather than favouring one direction.
    const score = Math.abs(Math.log(tileW / tileH / TILE_ASPECT))
    if (!best || score < best.score) best = { columns, score }
  }

  if (best) {
    return { columns: best.columns, capacity: count }
  }

  const columns = Math.max(1, Math.floor((width + GAP) / (TILE_MIN_W + GAP)))
  const rows = Math.max(1, Math.floor((height + GAP) / (TILE_MIN_H + GAP)))
  return { columns, capacity: Math.max(1, columns * rows) }
})

const capacity = computed(() => layout.value.capacity)
const { visible, pages, page, rotating } = useRotation(endpoints, capacity, 15000)

const gridStyle = computed(() => ({
  gridTemplateColumns: `repeat(${layout.value.columns}, minmax(0, 1fr))`,
  gridAutoRows: 'minmax(0, 1fr)',
  gap: `${GAP}px`,
}))

const isTv = computed(() => store.fitMode === 'tv' && (typeof window === 'undefined' || window.innerWidth >= 1024))
</script>

<template>
  <section class="panel flex flex-col" :class="isTv ? 'min-h-0' : ''">
    <!-- Domain header: identity on the left, registration facts on the right -->
    <header class="flex shrink-0 flex-wrap items-center justify-between gap-x-3 gap-y-1.5 border-b border-white/[0.07] px-3 py-2">
      <div class="flex items-center gap-2.5 min-w-0 flex-1">
        <StatusDot :status="group.status" size="lg" />

        <div class="min-w-0 flex-1">
          <div class="flex items-baseline gap-2">
            <h2 class="truncate font-mono text-sm sm:text-[15px] font-bold tracking-tight text-slate-100">
              {{ group.apex }}
            </h2>
            <span class="num text-[10px] font-bold tracking-wider" :class="theme.text">
              {{ theme.label }}
            </span>
          </div>
          <p class="truncate font-mono text-[10px] text-slate-500">
            <span v-if="registrar">{{ truncate(registrar, 30) }}</span>
            <span v-if="registrar && nameservers"> · </span>
            <span v-if="nameservers">ns {{ nameservers }}</span>
            <span v-if="!registrar && !nameservers">menunggu data registrasi</span>
          </p>
        </div>
      </div>

      <div class="flex items-center gap-2 shrink-0">
        <!-- Page dots only appear when the domain outgrew its panel in TV mode -->
        <div v-if="isTv && rotating" class="flex items-center gap-1">
          <span
            v-for="p in pages"
            :key="p"
            class="h-1.5 w-1.5 rounded-full transition-colors duration-300"
            :class="p - 1 === page ? 'bg-cyan-400' : 'bg-white/20'"
          ></span>
        </div>

        <ExpiryChip
          :days="group.domain?.days_left"
          :warn="store.config.domain_warn_days"
          :crit="store.config.domain_crit_days"
          label="domain"
        />

        <HealthRing :value="group.up_count" :total="group.total" :status="group.status" :size="36" />
      </div>
    </header>

    <!-- TV mode: dynamic tile layout with rotation -->
    <div v-if="isTv" ref="bodyEl" class="min-h-0 flex-1 overflow-hidden p-2">
      <div class="grid h-full" :style="gridStyle">
        <EndpointTile
          v-for="endpoint in visible"
          :key="endpoint.id"
          v-memo="[endpoint.status, endpoint.http?.latency_ms, endpoint.cert?.days_left, endpoint.reason]"
          :target="endpoint"
        />
      </div>

      <div
        v-if="!group.endpoints.length"
        class="flex h-full flex-col items-center justify-center gap-2 text-slate-700"
      >
        <Server class="h-8 w-8" />
        <span class="font-mono text-[11px]">belum ada endpoint pada domain ini</span>
      </div>
    </div>

    <!-- Mobile or scroll mode: all endpoints in responsive grid with natural height -->
    <div v-else class="p-2 sm:p-2.5">
      <div class="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-2">
        <EndpointTile
          v-for="endpoint in endpoints"
          :key="endpoint.id"
          v-memo="[endpoint.status, endpoint.http?.latency_ms, endpoint.cert?.days_left, endpoint.reason]"
          :target="endpoint"
        />
      </div>

      <div
        v-if="!group.endpoints.length"
        class="flex py-8 flex-col items-center justify-center gap-2 text-slate-700"
      >
        <Server class="h-8 w-8" />
        <span class="font-mono text-[11px]">belum ada endpoint pada domain ini</span>
      </div>
    </div>
  </section>
</template>
