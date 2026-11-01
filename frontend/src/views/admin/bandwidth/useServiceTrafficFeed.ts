import { computed, onMounted, onUnmounted, ref, watch, type Ref } from 'vue'
import { useDocumentVisibility } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import {
  opsAPI,
  type ServiceTrafficCurrent,
  type ServiceTrafficReport,
  type ServiceTrafficResponse
} from '@/api/admin/ops'
import type { TrafficRange } from '@/api/admin/wireguard'
import { isMissingTime } from '@/utils/trafficFormat'

export type PresetRange = Exclude<TrafficRange, 'custom'>
export type CurrentStatus = 'sampled' | 'warming_up' | 'invalid_interval' | 'stale' | 'unavailable'

export interface LiveSample {
  at: number
  in: number
  out: number
}

// 历史约 5 秒保存一次，所选期间的报告每 15 秒刷新即可；
// 实时速率另用只覆盖最近一分钟的查询每 2 秒刷新，避免反复读取整段历史。
export const REPORT_POLL_MS = 15_000
export const LIVE_POLL_MS = 2_000
export const LIVE_HISTORY = 90
// 以服务端 as_of 对比 sampled_at 判断陈旧，避免依赖浏览器时钟。
const STALE_MS = 5_000
// 实时查询连续失败超过此时长，当前值视为陈旧。
const LIVE_TIMEOUT_MS = 10_000

function statusOf(error: unknown) {
  return (error as { status?: number }).status
}

export function useServiceTrafficFeed(range: Ref<PresetRange>) {
  const { t } = useI18n()

  const report = ref<ServiceTrafficReport | null>(null)
  const current = ref<ServiceTrafficCurrent | null>(null)
  const currentAsOf = ref<string | null>(null)
  const lastPersistedAt = ref<string | null>(null)
  const persistenceError = ref(false)
  const loading = ref(false)
  const error = ref('')
  // 权限不足或监控关闭不会自行恢复，停止轮询，直到手动刷新或切换范围。
  const blocked = ref(false)
  const liveSamples = ref<LiveSample[]>([])
  const liveReceivedAt = ref(0)
  const nowTick = ref(Date.now())
  const visibility = useDocumentVisibility()

  let serverOffset = 0
  let reportSeq = 0
  let liveSeq = 0
  let reportController: AbortController | null = null
  let liveController: AbortController | null = null
  let reportTimer: ReturnType<typeof setTimeout> | undefined
  let liveTimer: ReturnType<typeof setTimeout> | undefined
  let disposed = false

  function errorMessage(e: unknown) {
    const value = e as { status?: number; reason?: string; code?: string }
    switch (value.reason) {
      case 'TRAFFIC_RANGE_INVALID':
        return t('bandwidth.errors.rangeInvalid')
      case 'TRAFFIC_INTERVAL_INVALID':
        return t('bandwidth.errors.intervalInvalid')
      case 'TRAFFIC_DATE_INVALID':
        return t('bandwidth.errors.dateInvalid')
      case 'TRAFFIC_BOUNDARY_UNALIGNED':
        return t('bandwidth.errors.boundaryUnaligned')
      case 'TRAFFIC_RETENTION_EXCEEDED':
        return t('bandwidth.errors.retentionExceeded')
    }
    if (value.status === 403) return t('bandwidth.errors.forbidden')
    if (value.status === 404 || value.code === 'OPS_DISABLED') return t('bandwidth.errors.disabled')
    if (value.status === 503) return t('bandwidth.errors.unavailable')
    return t('bandwidth.errors.load')
  }

  // 报告和实时查询都带 current；整段报告可能比实时查询返回得晚，只接受更新的那份。
  function applyLive(result: ServiceTrafficResponse) {
    const asOf = Date.parse(result.report?.as_of ?? '')
    const previous = Date.parse(currentAsOf.value ?? '')
    if (Number.isFinite(previous) && Number.isFinite(asOf) && asOf < previous) return
    if (Number.isFinite(asOf)) serverOffset = asOf - Date.now()
    current.value = result.current
    currentAsOf.value = result.report?.as_of ?? null
    lastPersistedAt.value = result.last_persisted_at
    persistenceError.value = result.persistence_error
    liveReceivedAt.value = Date.now()
    nowTick.value = liveReceivedAt.value
    const sample = result.current
    if (sample?.state === 'sampled' && !isMissingTime(sample.sampled_at)) {
      const at = Date.parse(sample.sampled_at)
      const last = liveSamples.value[liveSamples.value.length - 1]
      if (!last || at > last.at) {
        liveSamples.value = [
          ...liveSamples.value,
          { at, in: sample.bytes_per_second.in ?? 0, out: sample.bytes_per_second.out ?? 0 }
        ].slice(-LIVE_HISTORY)
      }
    }
  }

  function clearTimers() {
    if (reportTimer) clearTimeout(reportTimer)
    if (liveTimer) clearTimeout(liveTimer)
    reportTimer = undefined
    liveTimer = undefined
  }

  function scheduleReport() {
    if (reportTimer) clearTimeout(reportTimer)
    reportTimer = undefined
    if (disposed || blocked.value) return
    reportTimer = setTimeout(() => {
      if (visibility.value === 'hidden') scheduleReport()
      else void loadReport(false)
    }, REPORT_POLL_MS)
  }

  function scheduleLive() {
    if (liveTimer) clearTimeout(liveTimer)
    liveTimer = undefined
    if (disposed || blocked.value) return
    liveTimer = setTimeout(() => {
      if (visibility.value === 'hidden') scheduleLive()
      else void pollLive()
    }, LIVE_POLL_MS)
  }

  async function loadReport(reset: boolean) {
    if (disposed) return
    const seq = ++reportSeq
    reportController?.abort()
    const request = new AbortController()
    reportController = request
    // 切换范围时先清空，避免沿用上一个范围的总量和峰值。
    if (reset) {
      report.value = null
      error.value = ''
    }
    loading.value = true
    try {
      const result = await opsAPI.getServiceTraffic({ range: range.value }, { signal: request.signal })
      if (seq !== reportSeq || disposed) return
      report.value = result.report
      error.value = ''
      blocked.value = false
      applyLive(result)
    } catch (e) {
      if (seq !== reportSeq || disposed) return
      report.value = null
      error.value = errorMessage(e)
      const status = statusOf(e)
      blocked.value = status === 403 || status === 404
    } finally {
      if (seq === reportSeq && !disposed) {
        loading.value = false
        scheduleReport()
        if (!liveTimer) scheduleLive()
      }
    }
  }

  async function pollLive() {
    if (disposed || blocked.value) return
    const seq = ++liveSeq
    liveController?.abort()
    const request = new AbortController()
    liveController = request
    nowTick.value = Date.now()
    try {
      // 起点按服务端时间对齐 UTC 整分钟并回退一分钟，浏览器时钟有偏差时也不会落在未来。
      const serverNow = Date.now() + serverOffset
      const start = new Date(Math.floor(serverNow / 60_000) * 60_000 - 60_000).toISOString()
      const result = await opsAPI.getServiceTraffic(
        { range: 'custom', interval: 'minute', start },
        { signal: request.signal }
      )
      if (seq !== liveSeq || disposed) return
      applyLive(result)
    } catch (e) {
      if (seq !== liveSeq || disposed) return
      const status = statusOf(e)
      if (status === 403 || status === 404) blocked.value = true
      // 其他失败保留上次数据，超过 LIVE_TIMEOUT_MS 后按陈旧提示。
    } finally {
      if (seq === liveSeq && !disposed) scheduleLive()
    }
  }

  const currentStatus = computed<CurrentStatus>(() => {
    if (blocked.value) return 'unavailable'
    const value = current.value
    if (!value) return 'unavailable'
    if (value.state === 'warming_up' || isMissingTime(value.sampled_at)) return 'warming_up'
    if (value.state === 'invalid_interval') return 'invalid_interval'
    if (value.state !== 'sampled') return 'unavailable'
    if (nowTick.value - liveReceivedAt.value > LIVE_TIMEOUT_MS) return 'stale'
    const asOf = Date.parse(currentAsOf.value ?? '')
    if (Number.isFinite(asOf) && asOf - Date.parse(value.sampled_at) > STALE_MS) return 'stale'
    return 'sampled'
  })

  function refresh() {
    blocked.value = false
    void loadReport(false)
    void pollLive()
  }

  watch(range, () => {
    blocked.value = false
    void loadReport(true)
  })

  watch(visibility, (value) => {
    if (value === 'visible' && !blocked.value) {
      nowTick.value = Date.now()
      void pollLive()
      if (!loading.value) void loadReport(false)
    }
  })

  onMounted(() => {
    void loadReport(true)
  })

  onUnmounted(() => {
    disposed = true
    clearTimers()
    reportController?.abort()
    liveController?.abort()
  })

  return {
    report,
    current,
    currentStatus,
    lastPersistedAt,
    persistenceError,
    loading,
    error,
    blocked,
    liveSamples,
    refresh
  }
}
