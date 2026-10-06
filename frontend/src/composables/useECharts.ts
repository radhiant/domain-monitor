import { onMounted, onUnmounted, shallowRef, type Ref } from 'vue'
import { echarts, type EChartsOption, type ECharts } from '@/lib/echarts'

export function useECharts(chartEl: Ref<HTMLElement | null>, initialOption: EChartsOption) {
  const chartInstance = shallowRef<ECharts | null>(null)

  let resizeObserver: ResizeObserver | null = null
  let observedEl: HTMLElement | null = null
  let resizeTimer: ReturnType<typeof setTimeout> | null = null
  // Watchers with `immediate: true` run before onMounted, so their first update
  // arrives with no canvas to draw on. Hold it and apply it at init.
  let pendingOption: EChartsOption | null = null

  const initChart = () => {
    if (!chartEl.value) return

    if (chartInstance.value) {
      chartInstance.value.dispose()
    }

    // No theme name: `echarts/core` ships without the built-in 'dark' theme and
    // every series here sets its own colors against a transparent background.
    const chart = echarts.init(chartEl.value, undefined, { renderer: 'canvas' })
    chart.setOption(initialOption)
    if (pendingOption) {
      chart.setOption(pendingOption, false, true)
      pendingOption = null
    }
    chartInstance.value = chart

    // A ResizeObserver on the element already covers window resizes, so this is
    // the only listener needed. Coalescing on a short timer stops layout thrash
    // when several panels reflow together.
    //
    // Deliberately a timer rather than requestAnimationFrame: a wall display can
    // sit in a background tab, where rAF is paused and the callback would never
    // run, leaving every chart stretched at its stale canvas size.
    observedEl = chartEl.value
    resizeObserver = new ResizeObserver(() => {
      if (resizeTimer) return
      resizeTimer = setTimeout(() => {
        resizeTimer = null
        chartInstance.value?.resize()
      }, 60)
    })
    resizeObserver.observe(observedEl)
  }

  const setOption = (option: EChartsOption, notMerge = false, lazyUpdate = true) => {
    if (!chartInstance.value) {
      pendingOption = option
      return
    }
    chartInstance.value.setOption(option, notMerge, lazyUpdate)
  }

  onMounted(initChart)

  onUnmounted(() => {
    if (resizeTimer) {
      clearTimeout(resizeTimer)
      resizeTimer = null
    }
    if (resizeObserver) {
      // Disconnect unconditionally: keying this off chartEl.value leaked the
      // observer whenever the ref had already been nulled by unmount.
      if (observedEl) resizeObserver.unobserve(observedEl)
      resizeObserver.disconnect()
      resizeObserver = null
      observedEl = null
    }
    if (chartInstance.value) {
      chartInstance.value.dispose()
      chartInstance.value = null
    }
  })

  return {
    chartInstance,
    setOption
  }
}
