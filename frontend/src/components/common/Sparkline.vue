<script setup lang="ts">
import { computed } from 'vue'

/**
 * Latency sparkline, drawn as a plain SVG path.
 *
 * Deliberately not ECharts: the wall shows one of these per endpoint, and
 * thirty canvas instances redrawing on every probe would cost far more than
 * the entire rest of the page. An SVG path costs a string.
 */
const props = withDefaults(
  defineProps<{
    values: number[]
    stroke?: string
    height?: number
    width?: number
    filled?: boolean
  }>(),
  {
    stroke: 'rgb(34 211 238)',
    height: 22,
    width: 100,
    filled: true,
  },
)

const geometry = computed(() => {
  const values = props.values ?? []
  if (values.length < 2) return null

  const max = Math.max(...values)
  const min = Math.min(...values)
  // A flat series would divide by zero; draw it down the middle instead.
  const span = max - min || 1
  const stepX = props.width / (values.length - 1)
  // Inset by a pixel top and bottom so the stroke is never clipped.
  const usable = props.height - 2

  const points = values.map((value, i) => {
    const x = i * stepX
    const y = 1 + usable - ((value - min) / span) * usable
    return `${x.toFixed(1)},${y.toFixed(1)}`
  })

  return {
    line: `M${points.join(' L')}`,
    area: `M0,${props.height} L${points.join(' L')} L${props.width},${props.height} Z`,
  }
})

const gradientId = `spark-${Math.random().toString(36).slice(2, 9)}`
</script>

<template>
  <svg
    v-if="geometry"
    :viewBox="`0 0 ${width} ${height}`"
    :height="height"
    class="w-full"
    preserveAspectRatio="none"
    aria-hidden="true"
  >
    <defs v-if="filled">
      <linearGradient :id="gradientId" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0%" :stop-color="stroke" stop-opacity="0.28" />
        <stop offset="100%" :stop-color="stroke" stop-opacity="0" />
      </linearGradient>
    </defs>

    <path v-if="filled" :d="geometry.area" :fill="`url(#${gradientId})`" />
    <path
      :d="geometry.line"
      fill="none"
      :stroke="stroke"
      stroke-width="1.5"
      stroke-linejoin="round"
      stroke-linecap="round"
      vector-effect="non-scaling-stroke"
    />
  </svg>

  <!-- Keeps the tile height stable before enough samples have arrived. -->
  <div v-else :style="{ height: `${height}px` }" aria-hidden="true"></div>
</template>
