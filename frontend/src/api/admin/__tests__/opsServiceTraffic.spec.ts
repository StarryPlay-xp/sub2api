import { beforeEach, describe, expect, it, vi } from 'vitest'
import { apiClient } from '@/api/client'
import { getServiceTraffic } from '../ops'

vi.mock('@/api/client', () => ({
  apiClient: { get: vi.fn() },
  buildGatewayUrl: vi.fn()
}))

const get = vi.mocked(apiClient.get)

beforeEach(() => {
  vi.clearAllMocks()
  get.mockResolvedValue({ data: {} })
})

describe('service traffic API', () => {
  it('sends preset ranges without start, end or page filters', async () => {
    const controller = new AbortController()
    await getServiceTraffic(
      { range: '7d', start: '2026-10-01T00:00:00Z', end: '2026-10-02T00:00:00Z' },
      { signal: controller.signal }
    )
    expect(get).toHaveBeenCalledWith('/admin/ops/service-traffic', {
      params: { range: '7d' },
      signal: controller.signal
    })
  })

  it('passes interval and custom bounds through for custom ranges', async () => {
    await getServiceTraffic({
      range: 'custom',
      interval: 'hour',
      start: '2026-10-01T00:00:00+08:00'
    })
    expect(get).toHaveBeenCalledWith('/admin/ops/service-traffic', {
      params: { range: 'custom', interval: 'hour', start: '2026-10-01T00:00:00+08:00' },
      signal: undefined
    })
  })
})
