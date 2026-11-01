import { beforeEach, describe, expect, it, vi } from 'vitest'
import { apiClient } from '@/api/client'
import { getBandwidthLimits, saveBandwidthLimit } from '../bandwidth'

vi.mock('@/api/client', () => ({ apiClient: { get: vi.fn(), put: vi.fn() } }))
beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(apiClient.get).mockResolvedValue({ data: { host: { mbps: null, version: '' }, nodes: {} } })
  vi.mocked(apiClient.put).mockResolvedValue({ data: { mbps: null, version: 'new' } })
})

describe('bandwidth limit API', () => {
  it('loads shared settings with cancellation', async () => {
    const controller = new AbortController()
    const result = await getBandwidthLimits(controller.signal)
    expect(apiClient.get).toHaveBeenCalledWith('/admin/ops/bandwidth-limits', { signal: controller.signal })
    expect(result.host.mbps).toBeNull()
  })

  it('updates only the chosen target with its version, including explicit clears', async () => {
    await saveBandwidthLimit(null, { mbps: 100, version: 'old' })
    expect(apiClient.put).toHaveBeenCalledWith('/admin/ops/bandwidth-limits/host', { mbps: 100, version: 'old' }, { signal: undefined })
    await saveBandwidthLimit(11, { mbps: null, version: 'node-old' })
    expect(apiClient.put).toHaveBeenCalledWith('/admin/ops/bandwidth-limits/nodes/11', { mbps: null, version: 'node-old' }, { signal: undefined })
  })
})
