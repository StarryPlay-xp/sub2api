import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useDocumentVisibility } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import { getTrafficLive, type TrafficLiveData, type TrafficSubjectType } from '@/api/admin/trafficSubjects'
import { isMissingTime } from '@/utils/trafficFormat'
import { isBlockingError, subjectErrorMessage } from './subjectErrors'

// 实时占用只读服务端内存，页面可见时约 3 秒刷新一次。
export const LIVE_RANKING_POLL_MS = 3_000
export const LIVE_RANKING_LIMIT = 10
// 连续失败超过此时长，列表按“已停止更新”提示，不再当作当前数据。
const LIVE_RANKING_STALE_MS = 10_000

export type LiveRankingStatus = 'sampled' | 'warming_up' | 'invalid_interval' | 'stale' | 'unavailable'

export function useTrafficLive(initialSubject: TrafficSubjectType = 'user') {
  const { t } = useI18n()
  const subject = ref<TrafficSubjectType>(initialSubject)
  const data = ref<TrafficLiveData | null>(null)
  const loading = ref(false)
  const error = ref('')
  const blocked = ref(false)
  const receivedAt = ref(0)
  const nowTick = ref(Date.now())
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
      else void poll(false)
    }, LIVE_RANKING_POLL_MS)
  }

  async function poll(reset: boolean) {
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
    nowTick.value = Date.now()
    try {
      const result = await getTrafficLive(
        { subject: subject.value, limit: LIVE_RANKING_LIMIT, sort: 'avg_60s' },
        { signal: request.signal }
      )
      if (current !== seq || disposed) return
      data.value = result
      error.value = ''
      blocked.value = false
      receivedAt.value = Date.now()
      nowTick.value = receivedAt.value
    } catch (e) {
      if (current !== seq || disposed || request.signal.aborted) return
      // 偶发失败保留上次数据，超过陈旧时长后再提示；不可恢复的错误立即显示。
      blocked.value = isBlockingError(e)
      if (blocked.value || !data.value) {
        data.value = null
        error.value = subjectErrorMessage(e, t)
      }
    } finally {
      if (current === seq && !disposed) {
        loading.value = false
        schedule()
      }
    }
  }

  const status = computed<LiveRankingStatus>(() => {
    const value = data.value
    if (blocked.value || !value) return 'unavailable'
    if (nowTick.value - receivedAt.value > LIVE_RANKING_STALE_MS) return 'stale'
    if (value.state === 'warming_up' || isMissingTime(value.sampled_at)) return 'warming_up'
    if (value.state === 'invalid_interval') return 'invalid_interval'
    return 'sampled'
  })

  function refresh() {
    blocked.value = false
    void poll(false)
  }

  watch(subject, () => {
    blocked.value = false
    void poll(true)
  })

  watch(visibility, (value) => {
    if (value === 'visible' && !blocked.value) void poll(false)
  })

  onMounted(() => {
    void poll(true)
  })

  onUnmounted(() => {
    disposed = true
    if (timer) clearTimeout(timer)
    controller?.abort()
  })

  return { subject, data, loading, error, blocked, status, refresh }
}
