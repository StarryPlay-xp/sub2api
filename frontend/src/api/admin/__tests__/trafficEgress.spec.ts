import { beforeEach, describe, expect, it, vi } from 'vitest'
import { apiClient } from '@/api/client'
import { getTrafficEgress, getTrafficEgressRoute } from '../trafficEgress'

vi.mock('@/api/client', () => ({ apiClient: { get: vi.fn() } }))

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(apiClient.get).mockResolvedValue({ data: {} })
})

describe('traffic egress API', () => {
  it('requests the egress summary with only the parameters in use', async () => {
    const controller = new AbortController()
    await getTrafficEgress({ range: '7d', start: '2026-10-01T00:00:00Z' }, { signal: controller.signal })
    expect(apiClient.get).toHaveBeenCalledWith('/admin/ops/traffic-egress', { params: { range: '7d' }, signal: controller.signal })
  })

  it('uses direct/0 for direct traffic and maps comparison to the previous period', async () => {
    await getTrafficEgressRoute('direct', 5, { range: '24h', compare: true })
    expect(apiClient.get).toHaveBeenLastCalledWith('/admin/ops/traffic-egress/direct/0', { params: { range: '24h', compare: 'previous' }, signal: undefined })
    await getTrafficEgressRoute('wireguard', 11, { range: 'custom', start: '2026-10-01T00:00:00Z' })
    expect(apiClient.get).toHaveBeenLastCalledWith('/admin/ops/traffic-egress/wireguard/11', {
      params: { range: 'custom', start: '2026-10-01T00:00:00Z' },
      signal: undefined
    })
  })
})
