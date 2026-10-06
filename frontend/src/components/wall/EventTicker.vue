<script setup lang="ts">
import { computed } from 'vue'
import { themeOf } from '@/config/thresholds'
import { formatTimeShort } from '@/utils/format'
import { useDomainStore } from '@/stores/domainStore'

/**
 * Scrolling strip of the most recent status transitions.
 *
 * The content is duplicated once and the track is translated by exactly -50%,
 * which is what makes the loop seamless: the second copy is under the cursor
 * at the moment the first one runs out.
 */
const store = useDomainStore()

const events = computed(() => store.events.slice(0, 18))

// A single item would slide across and leave a gap, so the marquee only runs
// once there is enough content to fill the strip twice.
const scrolling = computed(() => events.value.length > 3)
</script>

<template>
  <div class="panel panel-flat flex shrink-0 items-center gap-3 overflow-hidden px-3 py-1.5">
    <span class="panel-title shrink-0 border-r border-white/[0.07] pr-3">Aktivitas</span>

    <div v-if="!events.length" class="font-mono text-[11px] text-slate-600">
      belum ada perubahan status sejak agent berjalan
    </div>

    <div v-else class="min-w-0 flex-1 overflow-hidden">
      <div class="ticker-track" :class="scrolling ? 'animate-ticker' : ''">
        <template v-for="copy in scrolling ? 2 : 1" :key="copy">
          <span
            v-for="event in events"
            :key="`${copy}-${event.target_id}-${event.at}`"
            class="mr-8 inline-flex items-center gap-2 font-mono text-[11px]"
          >
            <span class="num text-slate-600">{{ formatTimeShort(event.at) }}</span>
            <span class="text-slate-300">{{ event.target_id }}</span>
            <span class="font-bold" :class="themeOf(event.to).text">{{ themeOf(event.to).label }}</span>
            <span v-if="event.detail" class="text-slate-600">{{ event.detail }}</span>
          </span>
        </template>
      </div>
    </div>
  </div>
</template>
