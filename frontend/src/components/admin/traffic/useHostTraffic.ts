import { onMounted, onUnmounted, ref, watch, type Ref } from 'vue'
import { useDocumentVisibility } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import {
  getHostTraffic,
  getHostTrafficDetail,
  getHostTrafficUsage,
  type HostTrafficDetail,
  type HostTrafficStatus,
  type HostTrafficUsage,
  type HostUsageRange
} from '@/api/admin/hostTraffic'
import { errorStatus, isBlockingError } from './subjectErrors'

type Translate = (key: string, params?: Record<string, unknown>) => string

// 后端每整分钟读一次网卡计数，套餐卡按 30 秒刷新即可；页面隐藏时不刷新。
export const HOST_POLL_MS = 30_000

interface RequestError {
  status?: number
  code?: string | number
  reason?: string
  metadata?: Record<string, unknown>
}

// 运维监控关闭：公共请求器会把 code 设为 OPS_DISABLED，后端也会给出同名 reason。
function isOpsDisabled(value: RequestError): boolean {
  return value.code === 'OPS_DISABLED' || value.reason === 'OPS_DISABLED'
}

export function hostErrorMessage(error: unknown, t: Translate): string {
  const value = (error ?? {}) as RequestError
  switch (value.reason) {
    case 'HOST_TRAFFIC_PLAN_CONFLICT':
      return t('traffic.host.errors.conflict')
    case 'HOST_TRAFFIC_NOT_READY':
      return t('traffic.host.errors.notReady')
    case 'HOST_TRAFFIC_NO_PLAN':
      return t('traffic.host.errors.noPlan')
    case 'HOST_TRAFFIC_PLAN_INVALID':
    case 'HOST_TRAFFIC_CALIBRATION_INVALID':
      return t('traffic.host.errors.invalid')
  }
  if (value.status === 403) return t('traffic.host.errors.forbidden')
  if (value.status === 404) {
    return isOpsDisabled(value) ? t('traffic.host.errors.disabled') : t('traffic.host.errors.unsupported')
  }
  if (value.status === 503) return t('traffic.host.errors.unavailable')
  return t('traffic.host.errors.load')
}

/** 后端尚未提供本机流量接口（旧版本）时为 true：页面其余部分照常使用，只隐藏相关区块。 */
export function isHostUnsupported(error: unknown): boolean {
  const value = (error ?? {}) as RequestError
  return errorStatus(error) === 404 && !isOpsDisabled(value)
}

/** 本机套餐状态：采集状态、套餐、本期用量与预警。 */
export function useHostTrafficStatus() {
  const { t } = useI18n()
  const status = ref<HostTrafficStatus | null>(null)
  const loading = ref(false)
  const loaded = ref(false)
  const error = ref('')
  const unsupported = ref(false)
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
      else void load()
    }, HOST_POLL_MS)
  }

  async function load() {
    if (disposed) return
    const current = ++seq
    controller?.abort()
    const request = new AbortController()
    controller = request
    loading.value = true
    try {
      const result = await getHostTraffic({ signal: request.signal })
      if (current !== seq || disposed) return
      status.value = result
      error.value = ''
      unsupported.value = false
      blocked.value = false
    } catch (e) {
      if (current !== seq || disposed || request.signal.aborted) return
      error.value = hostErrorMessage(e, t)
      unsupported.value = isHostUnsupported(e)
      blocked.value = isBlockingError(e)
    } finally {
      if (current === seq && !disposed) {
        loading.value = false
        loaded.value = true
        schedule()
      }
    }
  }

  function refresh() {
    blocked.value = false
    void load()
  }

  /** 保存套餐或校准后直接采用接口返回的最新状态，并重新计时。 */
  function apply(next: HostTrafficStatus) {
    seq++
    controller?.abort()
    status.value = next
    error.value = ''
    loading.value = false
    loaded.value = true
    schedule()
  }

  watch(visibility, (value) => {
    if (value === 'visible' && !blocked.value && !loading.value && loaded.value) void load()
  })

  onMounted(() => void load())
  onUnmounted(() => {
    disposed = true
    if (timer) clearTimeout(timer)
    controller?.abort()
  })

  return { status, loading, loaded, error, unsupported, refresh, apply }
}

/** 所选范围内的本机网卡流量，用于流量概览；随范围切换重新加载。 */
export function useHostTrafficUsage(range: Ref<HostUsageRange>) {
  const { t } = useI18n()
  const usage = ref<HostTrafficUsage | null>(null)
  const loading = ref(false)
  const error = ref('')
  const unsupported = ref(false)

  let seq = 0
  let controller: AbortController | null = null
  let disposed = false

  async function load(reset: boolean) {
    if (disposed) return
    const current = ++seq
    controller?.abort()
    const request = new AbortController()
    controller = request
    if (reset) usage.value = null
    loading.value = true
    try {
      const result = await getHostTrafficUsage(range.value, { signal: request.signal })
      if (current !== seq || disposed) return
      usage.value = result
      error.value = ''
      unsupported.value = false
    } catch (e) {
      if (current !== seq || disposed || request.signal.aborted) return
      usage.value = null
      error.value = hostErrorMessage(e, t)
      unsupported.value = isHostUnsupported(e)
    } finally {
      if (current === seq && !disposed) loading.value = false
    }
  }

  watch(range, () => void load(true))
  onMounted(() => void load(true))
  onUnmounted(() => {
    disposed = true
    controller?.abort()
  })

  return { usage, loading, error, unsupported, refresh: () => void load(false) }
}

/** 套餐详情：当前账期每日用量、历史账期与校准记录。 */
export function useHostTrafficDetail() {
  const { t } = useI18n()
  const detail = ref<HostTrafficDetail | null>(null)
  const loading = ref(false)
  const error = ref('')
  let controller: AbortController | null = null
  let disposed = false

  async function load() {
    controller?.abort()
    const request = new AbortController()
    controller = request
    loading.value = true
    error.value = ''
    try {
      const result = await getHostTrafficDetail({ signal: request.signal })
      if (disposed || request.signal.aborted) return
      detail.value = result
    } catch (e) {
      if (disposed || request.signal.aborted) return
      error.value = hostErrorMessage(e, t)
    } finally {
      if (!disposed && controller === request) loading.value = false
    }
  }

  onMounted(() => void load())
  onUnmounted(() => {
    disposed = true
    controller?.abort()
  })

  return { detail, loading, error, reload: load }
}
