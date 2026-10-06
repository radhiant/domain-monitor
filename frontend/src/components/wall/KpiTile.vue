<script setup lang="ts">
import { computed, type Component } from 'vue'
import { themeOf } from '@/config/thresholds'
import Sparkline from '@/components/common/Sparkline.vue'
import type { Status } from '@/types/domain'

/**
 * One figure in the KPI band.
 *
 * The whole tile is built around a single large number, because that is the
 * only thing readable from across the room. Everything else — label, unit,
 * context line — is deliberately small: it is there for someone standing at
 * the screen, not for someone glancing at it.
 */
const props = withDefaults(
  defineProps<{
    label: string
    value: string
    unit?: string
    context?: string
    status?: Status
    icon?: Component
    spark?: number[]
    emphasis?: boolean
  }>(),
  { status: 'UNKNOWN', emphasis: false },
)

const theme = computed(() => themeOf(props.status))

// Neutral figures (a count, an average) stay white; only figures that carry a
// verdict take a colour, so colour keeps meaning something.
const valueClass = computed(() => (props.emphasis ? theme.value.text : 'text-slate-100'))

const sparkStroke = computed(() => {
  switch (props.status) {
    case 'DOWN':
      return 'rgb(244 63 94)'
    case 'DEGRADED':
      return 'rgb(245 158 11)'
    case 'UP':
      return 'rgb(16 185 129)'
    default:
      return 'rgb(34 211 238)'
  }
})
</script>

<template>
  <div
    class="panel flex min-w-0 flex-col justify-between gap-1 px-2.5 sm:px-4 py-2 sm:py-3"
    :class="emphasis && status !== 'UP' ? theme.border : ''"
  >
    <div class="flex items-center justify-between gap-1.5 sm:gap-2">
      <span class="panel-title truncate text-[10px] sm:text-[11px]">{{ label }}</span>
      <component :is="icon" v-if="icon" class="h-3.5 w-3.5 sm:h-4 sm:w-4 shrink-0 text-slate-600" />
    </div>

    <div class="flex items-baseline gap-1 sm:gap-1.5">
      <span
        class="num text-2xl sm:text-kpi-sm font-bold leading-none transition-colors duration-500 2xl:text-kpi"
        :class="valueClass"
      >{{ value }}</span>
      <span v-if="unit" class="num text-xs sm:text-sm font-medium text-slate-500">{{ unit }}</span>
    </div>

    <div class="min-h-[20px] sm:min-h-[22px]">
      <Sparkline v-if="spark && spark.length > 1" :values="spark" :stroke="sparkStroke" :height="18" />
      <p v-else-if="context" class="truncate font-mono text-[10px] sm:text-[11px] text-slate-500" :title="context">
        {{ context }}
      </p>
    </div>

    <p
      v-if="spark && spark.length > 1 && context"
      class="truncate font-mono text-[10px] sm:text-[11px] text-slate-500"
      :title="context"
    >
      {{ context }}
    </p>
  </div>
</template>
