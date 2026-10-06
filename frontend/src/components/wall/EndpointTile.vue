<script setup lang="ts">
import { computed } from 'vue'
import { themeOf } from '@/config/thresholds'
import { formatDuration, formatMs, hostParts } from '@/utils/format'
import Sparkline from '@/components/common/Sparkline.vue'
import StatusDot from '@/components/common/StatusDot.vue'
import { useDomainStore } from '@/stores/domainStore'
import type { TargetState } from '@/types/domain'

/**
 * One endpoint on the wall.
 *
 * The tile answers three questions in reading order: is it alive, how fast is
 * it, and is anything about to expire. Anything else belongs on the detail
 * view — a tile that tries to show everything shows nothing at TV distance.
 */
const props = defineProps<{ target: TargetState }>()

const store = useDomainStore()

const theme = computed(() => themeOf(props.target.status))
const name = computed(() => hostParts(props.target.host, props.target.apex))

const latency = computed(() => props.target.http?.latency_ms)
const code = computed(() => props.target.http?.status_code)

const certDays = computed(() => props.target.cert?.days_left)
const certCritical = computed(
  () => certDays.value !== undefined && certDays.value <= store.config.cert_warn_days,
)

const incident = computed(() => props.target.active_incident)

const sparkStroke = computed(() => {
  switch (props.target.status) {
    case 'DOWN':
      return 'rgb(244 63 94)'
    case 'DEGRADED':
      return 'rgb(245 158 11)'
    default:
      return 'rgb(34 211 238)'
  }
})
</script>

<template>
  <button
    type="button"
    class="panel panel-flat group relative flex w-full min-w-0 flex-col justify-between gap-1 py-2 pl-3 pr-2.5 text-left transition-colors duration-300 hover:border-cyan-500/40 focus:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500/60"
    :class="[theme.border, theme.bg]"
    :aria-label="`Buka detail ${target.host}, status ${theme.label}`"
    @click="store.openTarget(target.id)"
  >
    <span class="accent-bar" :class="theme.bar"></span>

    <!-- Identity -->
    <div class="flex min-w-0 items-center gap-1.5">
      <StatusDot :status="target.status" size="sm" />
      <span class="min-w-0 flex-1 truncate font-mono text-[13px] font-semibold leading-tight text-slate-100">
        {{ name.lead }}<span class="font-normal text-slate-600">{{ name.rest }}</span>
      </span>
      <span
        v-if="code"
        class="num shrink-0 text-[10px] font-semibold"
        :class="code >= 400 ? theme.text : 'text-slate-600'"
      >{{ code }}</span>
    </div>

    <!-- Down endpoints replace the numbers with the reason and how long it has
         been going, because at that point nothing else on the tile matters. -->
    <template v-if="target.status === 'DOWN'">
      <div class="num text-[15px] font-bold leading-none text-rose-400">
        {{ target.reason || 'tidak merespons' }}
      </div>
      <div class="num text-[11px] text-slate-500">
        {{ incident ? `mati ${formatDuration(incident.duration_sec)}` : 'baru terdeteksi' }}
      </div>
    </template>

    <template v-else>
      <div class="flex items-baseline justify-between gap-2">
        <span class="num text-[17px] font-bold leading-none text-slate-100">
          {{ formatMs(latency) }}
        </span>
        <span
          v-if="certDays !== undefined"
          class="num shrink-0 text-[10px] font-semibold"
          :class="certCritical ? theme.text : 'text-slate-600'"
        >SSL {{ certDays }}h</span>
      </div>

      <Sparkline :values="target.spark" :stroke="sparkStroke" :height="18" />

      <div class="num truncate text-[10px] leading-none" :class="target.reason ? theme.text : 'text-slate-600'">
        {{ target.reason || `uptime ${target.uptime.day ? target.uptime.day.toFixed(2) : '--'}%` }}
      </div>
    </template>
  </button>
</template>
