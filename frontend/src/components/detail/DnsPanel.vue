<script setup lang="ts">
import { computed } from 'vue'
import { Network } from 'lucide-vue-next'
import { formatMs, formatTimeShort } from '@/utils/format'
import type { DnsInfo } from '@/types/domain'

/** Where the hostname points, and how quickly that answer comes back. */
const props = defineProps<{ dns?: DnsInfo; redirects?: string[]; finalUrl?: string }>()

const addrs = computed(() => props.dns?.addrs ?? [])
const nameservers = computed(() => props.dns?.ns ?? [])
</script>

<template>
  <section class="panel flex flex-col">
    <header class="flex items-center justify-between gap-2 border-b border-white/[0.07] px-3 py-2">
      <span class="panel-title">DNS & Rute</span>
      <Network class="h-4 w-4" :class="dns?.error ? 'text-rose-400' : 'text-slate-600'" />
    </header>

    <div v-if="!dns" class="px-3 py-6 text-center font-mono text-[11px] text-slate-600">
      menunggu resolusi DNS
    </div>

    <div v-else class="flex flex-col gap-3 p-3">
      <div
        v-if="dns.error"
        class="rounded border border-rose-500/40 bg-rose-500/10 px-2.5 py-2 font-mono text-[10px] text-rose-300"
      >
        {{ dns.error }}
      </div>

      <div class="flex flex-wrap sm:flex-nowrap items-start sm:items-center justify-between gap-2 sm:gap-3">
        <div class="min-w-0 flex-1">
          <div class="panel-title mb-1 text-[10px]">Alamat</div>
          <div class="flex flex-wrap gap-1">
            <span
              v-for="addr in addrs"
              :key="addr"
              class="num rounded border border-white/[0.07] bg-white/[0.03] px-1.5 py-0.5 text-[11px] text-slate-300 break-all"
            >{{ addr }}</span>
            <span v-if="!addrs.length" class="font-mono text-[11px] text-slate-600">tidak ada</span>
          </div>
        </div>
        <div class="shrink-0 text-left sm:text-right">
          <div class="num text-lg font-bold leading-none text-slate-200">{{ formatMs(dns.resolve_ms) }}</div>
          <div class="font-mono text-[10px] text-slate-600">waktu resolve</div>
        </div>
      </div>

      <div v-if="dns.cname">
        <div class="panel-title mb-1 text-[10px]">CNAME</div>
        <div class="font-mono text-[11px] text-slate-300 break-all">{{ dns.cname }}</div>
      </div>

      <div v-if="nameservers.length">
        <div class="panel-title mb-1 text-[10px]">Nameserver</div>
        <div class="flex flex-wrap gap-1">
          <span
            v-for="ns in nameservers"
            :key="ns"
            class="rounded border border-white/[0.07] bg-white/[0.03] px-1.5 py-0.5 font-mono text-[10px] text-slate-400 break-all"
          >{{ ns }}</span>
        </div>
      </div>

      <div v-if="redirects && redirects.length">
        <div class="panel-title mb-1 text-[10px]">Rantai Redirect</div>
        <ol class="space-y-0.5">
          <li v-for="(hop, i) in redirects" :key="hop" class="break-all font-mono text-[10px] text-slate-500">
            <span class="text-slate-700">{{ i + 1 }}.</span> {{ hop }}
          </li>
        </ol>
      </div>

      <div class="border-t border-white/[0.05] pt-2 font-mono text-[10px] text-slate-600 break-all">
        dicek {{ formatTimeShort(dns.checked_at) }}
        <span v-if="finalUrl"> · tujuan akhir {{ finalUrl }}</span>
      </div>
    </div>
  </section>
</template>
