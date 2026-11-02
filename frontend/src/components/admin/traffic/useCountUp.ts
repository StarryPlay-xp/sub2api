import { onUnmounted, ref, watch, type Ref } from 'vue'

function prefersReducedMotion(): boolean {
  // 没有 matchMedia（如测试环境）时直接显示终值。
  return (
    typeof window === 'undefined' ||
    typeof window.matchMedia !== 'function' ||
    typeof window.requestAnimationFrame !== 'function' ||
    window.matchMedia('(prefers-reduced-motion: reduce)').matches
  )
}

/** 数字滚动：值变化时从旧值缓动到新值；系统要求减少动画时直接显示终值。 */
export function useCountUp(source: Ref<number | null>, duration = 900) {
  const value = ref<number | null>(null)
  let frame = 0

  function stop() {
    if (frame) cancelAnimationFrame(frame)
    frame = 0
  }

  watch(
    source,
    (target) => {
      stop()
      if (target === null || prefersReducedMotion()) {
        value.value = target
        return
      }
      const from = value.value ?? 0
      if (from === target) {
        value.value = target
        return
      }
      const start = performance.now()
      const step = (now: number) => {
        const progress = Math.min(1, (now - start) / duration)
        const eased = 1 - Math.pow(1 - progress, 3)
        value.value = from + (target - from) * eased
        frame = progress < 1 ? requestAnimationFrame(step) : 0
      }
      frame = requestAnimationFrame(step)
    },
    { immediate: true }
  )

  onUnmounted(stop)
  return value
}
