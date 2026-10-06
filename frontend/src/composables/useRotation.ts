import { computed, onUnmounted, ref, watch, type Ref } from 'vue'

/**
 * Paginates a list on a timer.
 *
 * Thirty endpoints fit one 1080p screen today, but this dashboard is expected
 * to grow. Rather than shrinking every tile until nothing is readable across
 * the room, extra pages rotate — and rotation stops entirely when everything
 * already fits, so the common case never moves.
 */
export function useRotation<T>(items: Ref<T[]>, perPage: Ref<number> | number, intervalMs = 20000) {
  const page = ref(0)
  const progress = ref(0)

  const size = computed(() => (typeof perPage === 'number' ? perPage : perPage.value))
  const pages = computed(() => Math.max(1, Math.ceil(items.value.length / size.value)))
  const rotating = computed(() => pages.value > 1)

  const visible = computed(() => {
    const start = page.value * size.value
    return items.value.slice(start, start + size.value)
  })

  let timer: ReturnType<typeof setInterval> | null = null
  const step = 250

  const stop = () => {
    if (timer) {
      clearInterval(timer)
      timer = null
    }
    progress.value = 0
  }

  const start = () => {
    stop()
    if (!rotating.value) return
    timer = setInterval(() => {
      progress.value += (step / intervalMs) * 100
      if (progress.value >= 100) {
        progress.value = 0
        page.value = (page.value + 1) % pages.value
      }
    }, step)
  }

  watch(rotating, start, { immediate: true })

  // A shrinking list must not strand the viewer on a page that no longer exists.
  watch(pages, () => {
    if (page.value >= pages.value) page.value = 0
  })

  onUnmounted(stop)

  return { page, pages, visible, rotating, progress }
}
