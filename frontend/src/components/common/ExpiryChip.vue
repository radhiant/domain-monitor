<script setup lang="ts">
import { computed } from 'vue'
import { expiryStatus, themeOf } from '@/config/thresholds'
import { formatDaysLeft } from '@/utils/format'

/** Compact "N hari" badge, coloured by how close the date is. */
const props = defineProps<{
  days?: number
  warn: number
  crit: number
  label?: string
}>()

const status = computed(() => expiryStatus(props.days, props.warn, props.crit))
const theme = computed(() => themeOf(status.value))
</script>

<template>
  <span
    v-if="days !== undefined && days !== null"
    class="num inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-semibold border"
    :class="[theme.text, theme.border, theme.bg]"
  >
    <span v-if="label" class="opacity-70">{{ label }}</span>
    <span>{{ formatDaysLeft(days) }}</span>
  </span>
</template>
