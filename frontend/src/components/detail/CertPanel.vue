<script setup lang="ts">
import { computed } from 'vue'
import { ShieldAlert, ShieldCheck } from 'lucide-vue-next'
import { expiryStatus, themeOf } from '@/config/thresholds'
import { formatDateShort, formatDaysLeft } from '@/utils/format'
import { useDomainStore } from '@/stores/domainStore'
import type { CertInfo } from '@/types/domain'

/** Certificate facts, plus a bar showing how far through its life it is. */
const props = defineProps<{ cert?: CertInfo }>()

const store = useDomainStore()

const status = computed(() =>
  expiryStatus(props.cert?.days_left, store.config.cert_warn_days, store.config.cert_crit_days),
)
const theme = computed(() => themeOf(status.value))

/** Elapsed share of the validity window, for the progress bar. */
const elapsed = computed(() => {
  const c = props.cert
  if (!c?.not_before || !c?.not_after) return 0
  const start = new Date(c.not_before).getTime()
  const end = new Date(c.not_after).getTime()
  if (end <= start) return 100
  return Math.min(100, Math.max(0, ((Date.now() - start) / (end - start)) * 100))
})

const sans = computed(() => props.cert?.sans ?? [])
</script>

<template>
  <section class="panel flex flex-col">
    <header class="flex items-center justify-between gap-2 border-b border-white/[0.07] px-3 py-2">
      <span class="panel-title">Sertifikat SSL</span>
      <component
        :is="cert?.error ? ShieldAlert : ShieldCheck"
        class="h-4 w-4"
        :class="cert?.error ? 'text-rose-400' : theme.text"
      />
    </header>

    <div v-if="!cert" class="px-3 py-6 text-center font-mono text-[11px] text-slate-600">
      endpoint ini tidak menggunakan HTTPS
    </div>

    <div v-else class="flex flex-col gap-3 p-3">
      <div v-if="cert.error" class="rounded border border-rose-500/40 bg-rose-500/10 px-2.5 py-2">
        <div class="font-mono text-[11px] font-semibold text-rose-400">Verifikasi gagal</div>
        <div class="font-mono text-[10px] text-rose-300/80">{{ cert.error }}</div>
      </div>

      <div class="flex items-end justify-between gap-3">
        <div>
          <div class="num text-3xl font-bold leading-none" :class="theme.text">
            {{ cert.days_left ?? '--' }}
          </div>
          <div class="mt-1 font-mono text-[10px] uppercase tracking-wider text-slate-500">
            hari tersisa
          </div>
        </div>
        <div class="text-right max-w-[60%]">
          <div class="font-mono text-[11px] text-slate-300 break-words">{{ cert.issuer || 'issuer tidak diketahui' }}</div>
          <div class="num text-[10px] text-slate-600 break-words">{{ cert.tls_version }} · {{ cert.cipher }}</div>
        </div>
      </div>

      <div>
        <div class="h-1.5 overflow-hidden rounded-full bg-white/[0.06]">
          <div
            class="h-full rounded-full transition-all duration-700"
            :class="theme.bar"
            :style="{ width: `${elapsed}%` }"
          ></div>
        </div>
        <div class="mt-1 flex justify-between font-mono text-[10px] text-slate-600">
          <span>{{ cert.not_before ? formatDateShort(cert.not_before) : '--' }}</span>
          <span>{{ formatDaysLeft(cert.days_left) }}</span>
          <span>{{ cert.not_after ? formatDateShort(cert.not_after) : '--' }}</span>
        </div>
      </div>

      <div v-if="sans.length">
        <div class="panel-title mb-1.5 text-[10px]">Nama Tercakup · {{ sans.length }}</div>
        <div class="flex max-h-24 flex-wrap gap-1 overflow-y-auto">
          <span
            v-for="san in sans"
            :key="san"
            class="rounded border border-white/[0.07] bg-white/[0.03] px-1.5 py-0.5 font-mono text-[10px] text-slate-400 break-all"
          >{{ san }}</span>
        </div>
      </div>
    </div>
  </section>
</template>
