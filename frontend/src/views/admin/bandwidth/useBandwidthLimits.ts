import { computed } from 'vue'
import { useLocalStorage } from '@vueuse/core'

// 带宽上限只用于本页对比展示，保存在当前浏览器，不写入后端，也不影响任何限速。
export const BANDWIDTH_LIMITS_KEY = 'sub2api:bandwidth-limits:v1'
const MAX_MBPS = 1_000_000

interface StoredLimits {
  host: number | null
  nodes: Record<string, number>
}

export function normalizeLimit(value: unknown): number | null {
  const number = typeof value === 'number' ? value : Number(value)
  return Number.isFinite(number) && number > 0 && number <= MAX_MBPS ? number : null
}

export function useBandwidthLimits() {
  const stored = useLocalStorage<StoredLimits>(
    BANDWIDTH_LIMITS_KEY,
    { host: null, nodes: {} },
    { mergeDefaults: true }
  )

  const hostLimit = computed(() => normalizeLimit(stored.value.host))

  function setHostLimit(value: number | null) {
    stored.value = { ...stored.value, host: normalizeLimit(value) }
  }

  function nodeLimit(id: number) {
    return normalizeLimit(stored.value.nodes?.[String(id)])
  }

  function setNodeLimit(id: number, value: number | null) {
    const nodes = { ...(stored.value.nodes ?? {}) }
    const limit = normalizeLimit(value)
    if (limit === null) delete nodes[String(id)]
    else nodes[String(id)] = limit
    stored.value = { ...stored.value, nodes }
  }

  return { hostLimit, setHostLimit, nodeLimit, setNodeLimit }
}
