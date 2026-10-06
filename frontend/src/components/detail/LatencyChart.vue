<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useECharts } from '@/composables/useECharts'
import { useDomainStore } from '@/stores/domainStore'
import { formatMs, formatTimeShort } from '@/utils/format'
import type { EChartsOption } from '@/lib/echarts'
import type { HistoryPoint } from '@/types/domain'

/**
 * Response time over the selected window.
 *
 * The threshold lines matter as much as the series: without them a reader has
 * to remember what "slow" means for this estate, and the chart becomes a shape
 * rather than a judgement.
 */
const props = defineProps<{ points: HistoryPoint[]; loading?: boolean }>()

const store = useDomainStore()
const chartEl = ref<HTMLElement | null>(null)

const baseOption: EChartsOption = {
  animation: false,
  grid: { top: 14, right: 14, bottom: 22, left: 52 },
  tooltip: {
    trigger: 'axis',
    backgroundColor: 'rgba(10, 15, 26, 0.95)',
    borderColor: 'rgba(255,255,255,0.1)',
    textStyle: { color: '#e2e8f0', fontFamily: 'JetBrains Mono, monospace', fontSize: 11 },
  },
  xAxis: {
    type: 'category',
    boundaryGap: false,
    axisLine: { lineStyle: { color: 'rgba(255,255,255,0.08)' } },
    axisTick: { show: false },
    axisLabel: { color: '#64748b', fontFamily: 'JetBrains Mono, monospace', fontSize: 10 },
  },
  yAxis: {
    type: 'value',
    splitLine: { lineStyle: { color: 'rgba(255,255,255,0.05)' } },
    axisLabel: {
      color: '#64748b',
      fontFamily: 'JetBrains Mono, monospace',
      fontSize: 10,
      formatter: (value: number) => `${value} ms`,
    },
  },
}

const { setOption } = useECharts(chartEl, baseOption)

const isMobile = computed(() => typeof window !== 'undefined' && window.innerWidth < 640)

const option = computed<EChartsOption>(() => ({
  grid: {
    top: 14,
    right: isMobile.value ? 8 : 14,
    bottom: 22,
    left: isMobile.value ? 40 : 52,
  },
  xAxis: {
    data: props.points.map((p) => formatTimeShort(p.ts * 1000)),
  },
  series: [
    {
      type: 'line',
      name: 'Respons',
      smooth: 0.25,
      symbol: 'none',
      lineStyle: { width: 2, color: '#22d3ee' },
      areaStyle: {
        color: {
          type: 'linear',
          x: 0, y: 0, x2: 0, y2: 1,
          colorStops: [
            { offset: 0, color: 'rgba(34, 211, 238, 0.28)' },
            { offset: 1, color: 'rgba(34, 211, 238, 0)' },
          ],
        },
      },
      data: props.points.map((p) => Math.round(p.avg_ms)),
      tooltip: { valueFormatter: (value: unknown) => formatMs(Number(value)) },
      markLine: {
        silent: true,
        symbol: 'none',
        label: {
          color: '#94a3b8',
          fontFamily: 'JetBrains Mono, monospace',
          fontSize: 9,
          position: 'insideEndTop',
        },
        data: [
          {
            yAxis: store.config.latency_warn_ms,
            lineStyle: { color: 'rgba(245, 158, 11, 0.5)', type: 'dashed' },
            label: { formatter: 'lambat' },
          },
          {
            yAxis: store.config.latency_crit_ms,
            lineStyle: { color: 'rgba(244, 63, 94, 0.5)', type: 'dashed' },
            label: { formatter: 'sangat lambat' },
          },
        ],
      },
    },
  ],
}))

watch(option, (next) => setOption(next), { immediate: true })
</script>

<template>
  <div class="relative h-full w-full">
    <div ref="chartEl" class="h-full w-full"></div>
    <div
      v-if="loading"
      class="absolute inset-0 flex items-center justify-center bg-wall-950/60 font-mono text-[11px] text-slate-500"
    >
      memuat riwayat...
    </div>
    <div
      v-else-if="!points.length"
      class="absolute inset-0 flex items-center justify-center font-mono text-[11px] text-slate-600"
    >
      belum ada riwayat pada rentang ini
    </div>
  </div>
</template>
