import { onMounted, onUnmounted, ref, watch, type Ref } from 'vue'
import { useDocumentVisibility } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import { proxiesAPI } from '@/api/admin/proxies'
import * as wireguardAPI from '@/api/admin/wireguard'
import type { Proxy } from '@/types'
import type { PresetRange } from './useServiceTrafficFeed'

export interface WireGuardNodeState {
  proxy: Proxy
  details: wireguardAPI.WGDetails | null
  report: wireguardAPI.TrafficReport | null
  error: string
}

// 节点详情（含当前速率）和所选期间报告每 15 秒刷新；节点数量通常很少，逐个并行请求。
export const NODES_POLL_MS = 15_000
const NODE_PAGE_SIZE = 100

export function useWireGuardNodes(range: Ref<PresetRange>) {
  const { t } = useI18n()
  const nodes = ref<WireGuardNodeState[]>([])
  const loading = ref(false)
  const loaded = ref(false)
  const error = ref('')
  const visibility = useDocumentVisibility()

  let seq = 0
  let controller: AbortController | null = null
  let timer: ReturnType<typeof setTimeout> | undefined
  let disposed = false
  let blocked = false

  function trafficMessage(e: unknown) {
    const status = (e as { status?: number }).status
    if (status === 403) return t('bandwidth.nodes.forbidden')
    if (status === 503) return t('bandwidth.nodes.unavailable')
    return t('bandwidth.nodes.trafficError')
  }

  function schedule() {
    if (timer) clearTimeout(timer)
    timer = undefined
    if (disposed || blocked) return
    timer = setTimeout(() => {
      if (visibility.value === 'hidden') schedule()
      else void load(false)
    }, NODES_POLL_MS)
  }

  async function load(reset: boolean) {
    if (disposed) return
    const current = ++seq
    controller?.abort()
    const request = new AbortController()
    controller = request
    // 切换范围时清掉旧报告，节点列表保留以免版面跳动。
    if (reset) nodes.value = nodes.value.map((node) => ({ ...node, report: null, error: '' }))
    loading.value = true
    try {
      const page = await proxiesAPI.list(1, NODE_PAGE_SIZE, { protocol: 'wireguard' }, { signal: request.signal })
      if (current !== seq || disposed) return
      const results = await Promise.all(
        page.items.map(async (proxy): Promise<WireGuardNodeState> => {
          const [details, report] = await Promise.allSettled([
            wireguardAPI.details(proxy.id, 'minute', request.signal),
            wireguardAPI.traffic(proxy.id, { range: range.value }, request.signal)
          ])
          return {
            proxy,
            details: details.status === 'fulfilled' ? details.value : null,
            report: report.status === 'fulfilled' ? report.value : null,
            error: report.status === 'rejected' ? trafficMessage(report.reason) : ''
          }
        })
      )
      if (current !== seq || disposed) return
      nodes.value = results
      error.value = ''
      blocked = false
    } catch (e) {
      if (current !== seq || disposed) return
      nodes.value = []
      const status = (e as { status?: number }).status
      error.value = status === 403 ? t('bandwidth.nodes.forbidden') : t('bandwidth.nodes.loadError')
      blocked = status === 403
    } finally {
      if (current === seq && !disposed) {
        loading.value = false
        loaded.value = true
        schedule()
      }
    }
  }

  function refresh() {
    blocked = false
    void load(false)
  }

  watch(range, () => {
    blocked = false
    void load(true)
  })

  watch(visibility, (value) => {
    if (value === 'visible' && !loading.value && !blocked) void load(false)
  })

  onMounted(() => {
    void load(true)
  })

  onUnmounted(() => {
    disposed = true
    if (timer) clearTimeout(timer)
    controller?.abort()
  })

  return { nodes, loading, loaded, error, refresh }
}
