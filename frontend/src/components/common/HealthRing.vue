<script setup lang="ts">
import { computed } from 'vue'
import { themeOf } from '@/config/thresholds'
import type { Status } from '@/types/domain'

/** Proportion of a group that is healthy, as a ring around the count. */
const props = withDefaults(
  defineProps<{ value: number; total: number; status: Status; size?: number }>(),
  { size: 44 },
)

const theme = computed(() => themeOf(props.status))

const radius = computed(() => props.size / 2 - 3)
const circumference = computed(() => 2 * Math.PI * radius.value)
const ratio = computed(() => (props.total > 0 ? props.value / props.total : 0))
const dash = computed(() => `${circumference.value * ratio.value} ${circumference.value}`)
</script>

<template>
  <div class="relative shrink-0" :style="{ width: `${size}px`, height: `${size}px` }">
    <svg :width="size" :height="size" class="-rotate-90">
      <circle
        :cx="size / 2" :cy="size / 2" :r="radius"
        fill="none" stroke-width="3" class="stroke-white/[0.08]"
      />
      <circle
        :cx="size / 2" :cy="size / 2" :r="radius"
        fill="none" stroke-width="3" stroke-linecap="round"
        :stroke-dasharray="dash"
        :class="theme.ring"
        class="transition-all duration-700"
      />
    </svg>
    <div class="absolute inset-0 flex items-center justify-center">
      <span class="num text-[11px] font-bold text-slate-200">{{ value }}/{{ total }}</span>
    </div>
  </div>
</template>
