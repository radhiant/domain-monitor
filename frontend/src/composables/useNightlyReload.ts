import { onMounted, onUnmounted } from 'vue'

/**
 * Reloads the page once a night.
 *
 * A browser left on a TV for weeks accumulates memory no amount of careful
 * component code prevents. A reload at 04:00, when nobody is looking, is the
 * cheapest insurance against a display that has quietly become unresponsive by
 * the time someone needs it.
 */
export function useNightlyReload(hour = 4) {
  let timer: ReturnType<typeof setInterval> | null = null

  onMounted(() => {
    timer = setInterval(() => {
      const now = new Date()
      if (now.getHours() === hour && now.getMinutes() === 0 && now.getSeconds() < 30) {
        window.location.reload()
      }
    }, 20000)
  })

  onUnmounted(() => {
    if (timer) clearInterval(timer)
  })
}
