<script setup lang="ts">
import { computed } from 'vue'
import { themeOf } from '@/config/thresholds'
import type { Status } from '@/types/domain'

const props = withDefaults(
  defineProps<{ status: Status; size?: 'sm' | 'md' | 'lg' }>(),
  { size: 'md' },
)

const theme = computed(() => themeOf(props.status))

const box = computed(() => ({ sm: 'h-2 w-2', md: 'h-2.5 w-2.5', lg: 'h-3.5 w-3.5' }[props.size]))

// Only failure pulses. If everything animated, the animation would stop
// meaning anything and just make a wall display tiring to sit near.
const pulses = computed(() => props.status === 'DOWN')
</script>

<template>
  <span class="relative flex shrink-0" :class="box">
    <span
      v-if="pulses"
      class="absolute inline-flex h-full w-full rounded-full opacity-70 animate-ping"
      :class="theme.dot"
    ></span>
    <span class="relative inline-flex rounded-full h-full w-full" :class="theme.dot"></span>
  </span>
</template>
