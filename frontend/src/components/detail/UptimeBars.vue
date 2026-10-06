<script setup lang="ts">
import { computed } from 'vue'
import { formatPercent, formatTimeShort } from '@/utils/format'
import type { HistoryPoint } from '@/types/domain'

/**
 * Availability as one bar per bucket, in the shape of a public status page.
 *
 * The latency chart above answers "how fast"; this answers "was it up", which
 * a line chart of a percentage does badly — a single failed check inside a
 * bucket should be visible as a mark, not smoothed into a curve.
 */
const props = defineProps<{ points: HistoryPoint[] }>()

const bars = computed(() =>
  props.points.map((p) => {
    let color = 'bg-emerald-500'
    if (p.percent < 100) color = 'bg-amber-400'
    if (p.percent < 50) color = 'bg-rose-500'
    if (p.total === 0) color = 'bg-white/[0.06]'

    return {
      key: p.ts,
      color,
      title: `${formatTimeShort(p.ts * 1000)} · ${formatPercent(p.percent, 1)} (${p.up}/${p.total})`,
      // Even a fully failed bucket keeps some height, so gaps in the strip mean
      // "no data" and never get confused with "down".
      height: p.total === 0 ? 35 : 45 + (p.percent / 100) * 55,
    }
  }),
)
</script>

<template>
  <div class="flex h-full items-end gap-[2px]">
    <div
      v-for="bar in bars"
      :key="bar.key"
      class="min-w-0 flex-1 rounded-sm transition-all duration-500"
      :class="bar.color"
      :style="{ height: `${bar.height}%` }"
      :title="bar.title"
    ></div>

    <div v-if="!bars.length" class="w-full text-center font-mono text-[11px] text-slate-600">
      belum ada data ketersediaan
    </div>
  </div>
</template>
