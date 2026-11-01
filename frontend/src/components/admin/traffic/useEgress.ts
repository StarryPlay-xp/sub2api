import { onMounted, onUnmounted, ref, watch, type Ref } from 'vue'
import { useDocumentVisibility } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import {
  getTrafficEgress,
  getTrafficEgressRoute,
  type EgressRouteType,
  type TrafficEgressData,
  type TrafficEgressDetailData
} from '@/api/admin/trafficEgress'
import { egressErrorMessage, isBlockingError } from './subjectErrors'
import type { PresetRange } from './usePresetRange'

// 小时汇总约 5 秒保存一次，出口构成随排行一起按 30 秒刷新；页面隐藏时不刷新。
export const EGRESS_POLL_MS = 30_000

/** 所选期间各出口的上游流量，用于流量路径与节点列表。 */
export function useEgressReport(range: Ref<PresetRange>) {
  const { t } = useI18n()
  const data = ref<TrafficEgressData | null>(null)
  const loading = ref(false)
  const error = ref('')
  const blocked = ref(false)
  const visibility = useDocumentVisibility()

  let seq = 0
  let controller: AbortController | null = null
  let timer: ReturnType<typeof setTimeout> | undefined
  let disposed = false

  function schedule() {
    if (timer) clearTimeout(timer)
    timer = undefined
    if (disposed || blocked.value) return
    timer = setTimeout(() => {
      if (visibility.value === 'hidden') schedule()
      else void load(false)
    }, EGRESS_POLL_MS)
  }

  async function load(reset: boolean) {
    if (disposed) return
    const current = ++seq
    controller?.abort()
    const request = new AbortController()
    controller = request
    if (reset) {
      data.value = null
      error.value = ''
    }
    loading.value = true
    try {
      const result = await getTrafficEgress({ range: range.value }, { signal: request.signal })
      if (current !== seq || disposed) return
      data.value = result
      error.value = ''
      blocked.value = false
    } catch (e) {
      if (current !== seq || disposed || request.signal.aborted) return
      data.value = null
      error.value = egressErrorMessage(e, t)
      blocked.value = isBlockingError(e)
    } finally {
      if (current === seq && !disposed) {
        loading.value = false
        schedule()
      }
    }
  }

  function refresh() {
    blocked.value = false
    void load(false)
  }

  watch(range, () => {
    blocked.value = false
    void load(true)
  })

  watch(visibility, (value) => {
    if (value === 'visible' && !blocked.value && !loading.value && data.value) void load(false)
  })

  onMounted(() => void load(true))
  onUnmounted(() => {
    disposed = true
    if (timer) clearTimeout(timer)
    controller?.abort()
  })

  return { data, loading, error, blocked, refresh }
}

export interface EgressRouteRef {
  type: EgressRouteType
  id: number
}

/** 单个出口的期间汇总、小时趋势与上游账号排行，带上一时段用于环比。 */
export function useEgressRouteDetail(route: Ref<EgressRouteRef | null>, range: Ref<PresetRange>) {
  const { t } = useI18n()
  const data = ref<TrafficEgressDetailData | null>(null)
  const loading = ref(false)
  const error = ref('')

  let seq = 0
  let controller: AbortController | null = null
  let disposed = false

  async function load() {
    const target = route.value
    const current = ++seq
    controller?.abort()
    data.value = null
    error.value = ''
    if (!target || disposed) {
      loading.value = false
      return
    }
    const request = new AbortController()
    controller = request
    loading.value = true
    try {
      const result = await getTrafficEgressRoute(target.type, target.id, { range: range.value, compare: true }, { signal: request.signal })
      if (current !== seq || disposed) return
      data.value = result
    } catch (e) {
      if (current !== seq || disposed || request.signal.aborted) return
      error.value = egressErrorMessage(e, t)
    } finally {
      if (current === seq && !disposed) loading.value = false
    }
  }

  watch(
    () => [route.value?.type, route.value?.id, range.value],
    () => void load(),
    { immediate: true }
  )

  onUnmounted(() => {
    disposed = true
    controller?.abort()
  })

  return { data, loading, error, reload: load }
}
