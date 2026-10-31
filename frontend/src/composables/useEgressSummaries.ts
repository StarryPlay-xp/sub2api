import { ref, watch } from 'vue'
import { exitSummaries, type EgressQuery, type EgressSummary } from '@/api/admin/wireguard'

export function egressSummaryKey(requestID: string, accountID: number) {
  return JSON.stringify([requestID, accountID])
}

export function useEgressSummaries(
  rows: () => { request_id?: string | null; account_id?: number | null }[],
  enabled: () => boolean,
) {
  const summaries = ref<Record<string, EgressSummary>>({})
  const loading = ref(false)
  const error = ref(false)
  watch([rows, enabled], async ([items, active], _, onCleanup) => {
    const controller = new AbortController()
    onCleanup(() => controller.abort())
    summaries.value = {}
    error.value = false
    loading.value = false
    if (!active) return
    const requests = new Map<string, EgressQuery>()
    for (const row of items) {
      if (row.request_id && row.account_id && row.account_id > 0) {
        requests.set(egressSummaryKey(row.request_id, row.account_id), { request_id: row.request_id, account_id: row.account_id })
      }
    }
    const queries = [...requests.values()]
    if (!queries.length) return
    loading.value = true
    try {
      const result: Record<string, EgressSummary> = {}
      for (let start = 0; start < queries.length; start += 200) {
        const batch = await exitSummaries(queries.slice(start, start + 200), controller.signal)
        if (controller.signal.aborted) return
        for (const item of batch) result[egressSummaryKey(item.request_id, item.account_id)] = item
      }
      summaries.value = result
    } catch {
      if (!controller.signal.aborted) error.value = true
    } finally {
      if (!controller.signal.aborted) loading.value = false
    }
  }, { immediate: true })
  return { summaries, loading, error }
}
