import { onMounted, onUnmounted, ref } from 'vue'
import { formatDateLong, nowTime24 } from '@/utils/format'

/**
 * A shared wall clock.
 *
 * The date is only recomputed when the day actually changes, so the header
 * does not re-run locale formatting once a second for eight hours.
 */
export function useClock() {
  const time = ref(nowTime24())
  const date = ref(formatDateLong())

  let timer: ReturnType<typeof setInterval> | null = null
  let day = new Date().getDate()

  onMounted(() => {
    timer = setInterval(() => {
      time.value = nowTime24()
      const today = new Date().getDate()
      if (today !== day) {
        day = today
        date.value = formatDateLong()
      }
    }, 1000)
  })

  onUnmounted(() => {
    if (timer) clearInterval(timer)
  })

  return { time, date }
}
