<script setup lang="ts">
import { computed } from 'vue'
import { CalendarClock, ShieldCheck } from 'lucide-vue-next'
import { expiryStatus, themeOf } from '@/config/thresholds'
import { formatDateShort } from '@/utils/format'
import { useDomainStore } from '@/stores/domainStore'
import type { ExpiryItem, Status } from '@/types/domain'

/**
 * Certificates and domain registrations in one list, soonest first.
 *
 * Keeping them separate would be tidier but wrong: the operator has exactly
 * one question here — what expires next — and the answer should not depend on
 * which of two lists they happened to look at.
 */
const props = withDefaults(defineProps<{ limit?: number }>(), { limit: 7 })

const store = useDomainStore()

function statusOf(item: ExpiryItem): Status {
  return item.kind === 'ssl'
    ? expiryStatus(item.days_left, store.config.cert_warn_days, store.config.cert_crit_days)
    : expiryStatus(item.days_left, store.config.domain_warn_days, store.config.domain_crit_days)
}

const rows = computed(() =>
  store.expiry.slice(0, props.limit).map((item) => ({
    item,
    status: statusOf(item),
    theme: themeOf(statusOf(item)),
  })),
)

const urgent = computed(() => rows.value.filter((r) => r.status !== 'UP').length)
</script>

<template>
  <section class="panel flex min-h-0 flex-col">
    <header class="flex shrink-0 items-center justify-between gap-2 border-b border-white/[0.07] px-3 py-2">
      <span class="panel-title">Masa Berlaku</span>
      <span
        class="num text-[10px] font-semibold"
        :class="urgent ? 'text-amber-400' : 'text-slate-600'"
      >{{ urgent ? `${urgent} perlu perhatian` : 'aman' }}</span>
    </header>

    <ul class="min-h-0 flex-1 divide-y divide-white/[0.05] overflow-y-auto">
      <li
        v-for="row in rows"
        :key="`${row.item.kind}-${row.item.target_id}`"
        class="flex items-center gap-2.5 px-3 py-[7px]"
      >
        <component
          :is="row.item.kind === 'ssl' ? ShieldCheck : CalendarClock"
          class="h-3.5 w-3.5 shrink-0"
          :class="row.status === 'UP' ? 'text-slate-600' : row.theme.text"
        />

        <div class="min-w-0 flex-1">
          <div class="truncate font-mono text-[12px] leading-tight text-slate-200">
            {{ row.item.label }}
          </div>
          <div class="num truncate text-[10px] leading-tight text-slate-600">
            {{ row.item.kind === 'ssl' ? row.item.issuer || 'sertifikat' : row.item.registrar || 'registrasi domain' }}
            · {{ formatDateShort(row.item.expires_at) }}
          </div>
        </div>

        <div class="shrink-0 text-right">
          <div class="num text-[15px] font-bold leading-none" :class="row.theme.text">
            {{ row.item.days_left }}
          </div>
          <div class="text-[9px] uppercase tracking-wider text-slate-600">hari</div>
        </div>
      </li>

      <li v-if="!rows.length" class="px-3 py-6 text-center font-mono text-[11px] text-slate-600">
        menunggu data sertifikat dan registrasi
      </li>
    </ul>
  </section>
</template>
