<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { AlertTriangle, CheckCircle2, ArrowDownRight, ArrowUpRight } from 'lucide-vue-next'
import { formatDuration, formatTimeShort, truncate } from '@/utils/format'
import { useDomainStore } from '@/stores/domainStore'

/**
 * Open outages first, then what recently changed.
 *
 * Durations are recomputed on a local timer rather than only when the agent
 * pushes: an outage that has lasted twelve minutes should read twelve minutes,
 * not whatever it was at the last probe.
 */
const store = useDomainStore()

const now = ref(Date.now())
let timer: ReturnType<typeof setInterval> | null = null

onMounted(() => {
  timer = setInterval(() => (now.value = Date.now()), 1000)
})
onUnmounted(() => {
  if (timer) clearInterval(timer)
})

const active = computed(() =>
  store.incidents.map((inc) => ({
    ...inc,
    elapsed: Math.max(0, Math.floor((now.value - new Date(inc.started_at).getTime()) / 1000)),
  })),
)

// Recoveries only. A DOWN transition is already represented by the open
// incident above it, so listing it again would just be the same news twice.
const recovered = computed(() =>
  store.events.filter((e) => e.from === 'DOWN' && e.to !== 'DOWN').slice(0, 4),
)
</script>

<template>
  <section class="panel flex min-h-0 flex-col">
    <header class="flex shrink-0 items-center justify-between gap-2 border-b border-white/[0.07] px-3 py-2">
      <span class="panel-title">Insiden</span>
      <span
        class="num text-[10px] font-semibold"
        :class="active.length ? 'text-rose-400' : 'text-emerald-400'"
      >{{ active.length ? `${active.length} aktif` : 'nihil' }}</span>
    </header>

    <div class="min-h-0 flex-1 overflow-y-auto">
      <ul v-if="active.length" class="divide-y divide-white/[0.05]">
        <li
          v-for="inc in active"
          :key="inc.id"
          class="flex items-start gap-2.5 border-l-2 border-rose-500 bg-rose-500/[0.06] px-3 py-2"
        >
          <AlertTriangle class="mt-0.5 h-4 w-4 shrink-0 text-rose-400" />
          <div class="min-w-0 flex-1">
            <div class="truncate font-mono text-[12px] font-semibold leading-tight text-slate-100">
              {{ inc.target_id }}
            </div>
            <div class="num truncate text-[10px] leading-tight text-rose-300/80">
              {{ inc.reason || 'tidak merespons' }}
              <span v-if="inc.last_error"> · {{ truncate(inc.last_error, 30) }}</span>
            </div>
          </div>
          <div class="num shrink-0 text-[13px] font-bold text-rose-400">
            {{ formatDuration(inc.elapsed) }}
          </div>
        </li>
      </ul>

      <div v-else class="flex flex-col items-center justify-center gap-1.5 px-3 py-5">
        <CheckCircle2 class="h-6 w-6 text-emerald-500/70" />
        <span class="font-mono text-[11px] text-slate-500">semua endpoint merespons</span>
      </div>

      <div v-if="recovered.length" class="border-t border-white/[0.05]">
        <div class="px-3 pb-1 pt-2">
          <span class="panel-title text-[10px]">Baru Pulih</span>
        </div>
        <ul class="pb-1">
          <li
            v-for="event in recovered"
            :key="`${event.target_id}-${event.at}`"
            class="flex items-center gap-2 px-3 py-1"
          >
            <component
              :is="event.to === 'UP' ? ArrowUpRight : ArrowDownRight"
              class="h-3 w-3 shrink-0"
              :class="event.to === 'UP' ? 'text-emerald-400' : 'text-amber-400'"
            />
            <span class="min-w-0 flex-1 truncate font-mono text-[11px] text-slate-400">
              {{ event.target_id }}
            </span>
            <span class="num shrink-0 text-[10px] text-slate-600">{{ formatTimeShort(event.at) }}</span>
          </li>
        </ul>
      </div>
    </div>
  </section>
</template>
