<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import AppHeader from '@/components/common/AppHeader.vue'
import WallView from '@/components/wall/WallView.vue'
import ListView from '@/components/list/ListView.vue'
import DetailView from '@/components/detail/DetailView.vue'
import { useDomainStore } from '@/stores/domainStore'
import { useNightlyReload } from '@/composables/useNightlyReload'

/**
 * Three views, no router.
 *
 * The view lives in the query string, so each display can be pinned to its own
 * URL: the TV opens ?view=wall and never leaves it, while a desk browser opens
 * ?view=list or a specific ?view=detail&target=host.
 */
const store = useDomainStore()

const isTv = computed(() => store.fitMode === 'tv')

useNightlyReload(4)

onMounted(() => store.init())
onUnmounted(() => store.stopStream())
</script>

<template>
  <div
    class="flex min-h-screen flex-col bg-wall-950 text-slate-200 selection:bg-cyan-500/25 selection:text-cyan-100"
    :class="isTv ? 'lg:h-full lg:overflow-hidden' : 'min-h-full'"
  >
    <AppHeader />

    <WallView v-if="store.view === 'wall'" />
    <ListView v-else-if="store.view === 'list'" />
    <DetailView v-else />

    <footer
      class="flex shrink-0 flex-wrap items-center justify-between gap-1 border-t border-white/[0.06] px-3 sm:px-4 py-1.5 sm:py-1 font-mono text-[9px] sm:text-[10px] text-slate-600"
    >
      <span>Domain Monitor · Uptime, SSL, DNS &amp; Masa Berlaku</span>
      <span v-if="store.overview">
        {{ store.overview.total_endpoints }} endpoint · {{ store.overview.total_domains }} domain
      </span>
    </footer>
  </div>
</template>
