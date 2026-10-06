<script setup lang="ts">
import { History } from 'lucide-vue-next'
import { formatDateShort, formatDuration, formatTimeShort, truncate } from '@/utils/format'
import type { Incident } from '@/types/domain'

/** Outage history for one endpoint. */
defineProps<{ incidents: Incident[] }>()
</script>

<template>
  <section class="panel flex min-h-0 flex-col">
    <header class="flex items-center justify-between gap-2 border-b border-white/[0.07] px-3 py-2">
      <span class="panel-title">Riwayat Gangguan</span>
      <History class="h-4 w-4 text-slate-600" />
    </header>

    <div class="min-h-0 flex-1 overflow-y-auto">
      <table v-if="incidents.length" class="w-full border-collapse">
        <tbody>
          <tr
            v-for="inc in incidents"
            :key="inc.id"
            class="border-b border-white/[0.04]"
            :class="inc.active ? 'bg-rose-500/[0.06]' : ''"
          >
            <td class="px-3 py-1.5 min-w-0">
              <div class="num text-[11px] text-slate-300">
                {{ formatDateShort(inc.started_at) }} {{ formatTimeShort(inc.started_at) }}
              </div>
              <div class="font-mono text-[10px] text-slate-600 truncate">
                {{ inc.reason || 'tidak merespons' }}
                <span v-if="inc.last_error"> · {{ truncate(inc.last_error, 34) }}</span>
              </div>
            </td>
            <td class="px-3 py-1.5 text-right shrink-0">
              <div class="num text-[12px] font-bold" :class="inc.active ? 'text-rose-400' : 'text-slate-300'">
                {{ formatDuration(inc.duration_sec) }}
              </div>
              <div class="font-mono text-[10px]" :class="inc.active ? 'text-rose-400' : 'text-slate-600'">
                {{ inc.active ? 'berlangsung' : 'pulih' }}
              </div>
            </td>
          </tr>
        </tbody>
      </table>

      <div v-else class="px-3 py-8 text-center font-mono text-[11px] text-slate-600">
        belum pernah tercatat gangguan
      </div>
    </div>
  </section>
</template>
