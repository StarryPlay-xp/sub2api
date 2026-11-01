import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { getBandwidthLimits, saveBandwidthLimit, type BandwidthLimits } from '@/api/admin/bandwidth'
import { normalizeLimit } from '@/utils/bandwidthLimits'

export { normalizeLimit } from '@/utils/bandwidthLimits'
export const LIMITS_POLL_MS = 15_000
const emptyLimit = () => ({ mbps: null, version: '' })

// Shared server settings only. Browser values are deliberately never uploaded:
// stale local caches must not overwrite another device's configuration.
export function useBandwidthLimits() {
  const stored = ref<BandwidthLimits>({ host: emptyLimit(), nodes: {} })
  const loaded = ref(false)
  const { t } = useI18n()
  const app = useAppStore()
  const pending = new Map<string, AbortController>()
  let readController: AbortController | null = null
  let sequence = 0
  let disposed = false
  let loadErrorShown = false
  let refreshAfterWrites = false
  let timer: ReturnType<typeof setInterval> | undefined

  const hostLimit = computed(() => normalizeLimit(stored.value.host.mbps))
  function nodeLimit(id: number) {
    return normalizeLimit(stored.value.nodes[String(id)]?.mbps)
  }

  async function refresh() {
    if (disposed || pending.size) return
    const current = ++sequence
    readController?.abort()
    const controller = new AbortController()
    readController = controller
    try {
      const value = await getBandwidthLimits(controller.signal)
      if (disposed || current !== sequence) return
      stored.value = value
      loaded.value = true
      loadErrorShown = false
    } catch {
      if (disposed || controller.signal.aborted || current !== sequence) return
      if (!loadErrorShown) app.showError(t('bandwidth.limit.loadFailed'))
      loadErrorShown = true
    }
  }

  async function save(nodeId: number | null, value: number | null) {
    if (disposed) return
    if (!loaded.value) await refresh()
    if (disposed || !loaded.value) return
    const key = nodeId === null ? 'host' : String(nodeId)
    if (pending.has(key)) {
      app.showInfo(t('bandwidth.limit.saving'))
      return
    }
    const mbps = value === null ? null : normalizeLimit(value)
    if (value !== null && mbps === null) {
      app.showError(t('bandwidth.limit.invalid'))
      return
    }
    const version = (nodeId === null ? stored.value.host : stored.value.nodes[key])?.version ?? ''
    // Invalidate any older read before writing; only acknowledged values are displayed.
    sequence++
    readController?.abort()
    const controller = new AbortController()
    pending.set(key, controller)
    try {
      const saved = await saveBandwidthLimit(nodeId, { mbps, version }, controller.signal)
      if (disposed) return
      if (nodeId === null) stored.value = { ...stored.value, host: saved }
      else stored.value = { ...stored.value, nodes: { ...stored.value.nodes, [key]: saved } }
      app.showSuccess(t('bandwidth.limit.saved'))
    } catch (error) {
      if (disposed || controller.signal.aborted) return
      const status = (error as { status?: number }).status
      const conflict = status === 409
      app.showError(t(conflict ? 'bandwidth.limit.conflict' : 'bandwidth.limit.saveFailed'))
      refreshAfterWrites = true
    } finally {
      pending.delete(key)
      if (refreshAfterWrites && pending.size === 0 && !disposed) {
        refreshAfterWrites = false
        await refresh()
      }
    }
  }

  function refreshVisible() {
    if (document.visibilityState !== 'hidden') void refresh()
  }
  onMounted(() => {
    void refresh()
    timer = setInterval(refreshVisible, LIMITS_POLL_MS)
    window.addEventListener('focus', refreshVisible)
    document.addEventListener('visibilitychange', refreshVisible)
  })
  onUnmounted(() => {
    disposed = true
    sequence++
    readController?.abort()
    for (const controller of pending.values()) controller.abort()
    if (timer) clearInterval(timer)
    window.removeEventListener('focus', refreshVisible)
    document.removeEventListener('visibilitychange', refreshVisible)
  })

  return {
    hostLimit, nodeLimit, refresh,
    setHostLimit: (value: number | null) => save(null, value),
    setNodeLimit: (id: number, value: number | null) => save(id, value)
  }
}
