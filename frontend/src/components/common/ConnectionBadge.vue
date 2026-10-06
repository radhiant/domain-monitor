<script setup lang="ts">
import { computed } from 'vue'
import { Wifi, WifiOff, Loader2 } from 'lucide-vue-next'
import type { ConnectionState } from '@/types/domain'

const props = defineProps<{ state: ConnectionState }>()

const view = computed(() => {
  switch (props.state) {
    case 'live':
      return { label: 'LIVE', cls: 'text-emerald-400 border-emerald-500/40 bg-emerald-500/10', icon: Wifi, spin: false }
    case 'connecting':
      return { label: 'MENGHUBUNGKAN', cls: 'text-cyan-300 border-cyan-500/40 bg-cyan-500/10', icon: Loader2, spin: true }
    case 'reconnecting':
      return { label: 'MENYAMBUNG ULANG', cls: 'text-amber-400 border-amber-500/40 bg-amber-500/10', icon: Loader2, spin: true }
    default:
      return { label: 'TERPUTUS', cls: 'text-rose-400 border-rose-500/50 bg-rose-500/10', icon: WifiOff, spin: false }
  }
})
</script>

<template>
  <span
    class="inline-flex items-center gap-1.5 rounded border px-2 py-1 font-mono text-[10px] font-bold tracking-wider"
    :class="view.cls"
  >
    <component :is="view.icon" class="h-3.5 w-3.5" :class="view.spin ? 'animate-spin' : ''" />
    {{ view.label }}
  </span>
</template>
