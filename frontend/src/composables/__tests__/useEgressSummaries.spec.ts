import { effectScope, nextTick, ref } from 'vue'
import { flushPromises } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { egressSummaryKey, useEgressSummaries } from '../useEgressSummaries'

const api = vi.hoisted(() => ({ exitSummaries: vi.fn() }))
vi.mock('@/api/admin/wireguard', () => api)
describe('batched exit summaries', () => {
  it('deduplicates a page and cancels stale results after pagination', async () => {
    api.exitSummaries.mockReset()
    let resolveOld!: (value: unknown[]) => void
    api.exitSummaries.mockImplementationOnce(() => new Promise(resolve => { resolveOld = resolve }))
    api.exitSummaries.mockResolvedValueOnce([{ request_id: 'new', account_id: 7, ips: ['203.0.113.2'] }])
    const rows = ref([{ request_id: 'old', account_id: 7 }, { request_id: 'old', account_id: 7 }])
    const scope = effectScope()
    const state = scope.run(() => useEgressSummaries(() => rows.value, () => true))!
    expect(api.exitSummaries.mock.calls[0][0]).toEqual([{ request_id: 'old', account_id: 7 }])
    const signal = api.exitSummaries.mock.calls[0][1] as AbortSignal
    rows.value = [{ request_id: 'new', account_id: 7 }]
    await nextTick(); await flushPromises()
    expect(signal.aborted).toBe(true)
    resolveOld([{ request_id: 'old', account_id: 7, ips: ['203.0.113.1'] }]); await flushPromises()
    expect(Object.keys(state.summaries.value)).toEqual([egressSummaryKey('new', 7)])
    expect(state.loading.value).toBe(false)
    scope.stop()
  })
  it('does not fetch when the column is hidden', () => {
    api.exitSummaries.mockClear()
    const scope = effectScope()
    scope.run(() => useEgressSummaries(() => [{ request_id: 'req', account_id: 7 }], () => false))
    expect(api.exitSummaries).not.toHaveBeenCalled()
    scope.stop()
  })
})
