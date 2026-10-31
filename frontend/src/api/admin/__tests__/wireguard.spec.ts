import { beforeEach, describe, expect, it, vi } from 'vitest'
import { apiClient } from '@/api/client'
import { traffic } from '../wireguard'
import type { TrafficReport } from '../wireguard'

vi.mock('@/api/client', () => ({
  apiClient: { get: vi.fn() }
}))

const get = vi.mocked(apiClient.get)

const report: TrafficReport = {
  range: '24h',
  interval: 'minute',
  start: '2026-10-30T06:00:00Z',
  end: '2026-10-31T06:00:00Z',
  requested_start: '2026-10-30T06:00:00Z',
  requested_end: '2026-10-31T06:00:00Z',
  retained_from: '2026-10-24T06:00:00Z',
  as_of: '2026-10-31T06:00:00Z',
  end_clamped: false,
  tx_bytes: '1',
  rx_bytes: '2',
  total_bytes: '3',
  has_records: true,
  bucket_count: 1,
  incomplete: false,
  metric_basis: 'wg_udp_payload_v1',
  history: []
}

beforeEach(() => {
  vi.clearAllMocks()
  get.mockResolvedValue({ data: report })
})

describe('wireguard traffic API', () => {
  it('passes params through the API client so axios encodes offsets', async () => {
    const controller = new AbortController()
    await traffic(
      7,
      {
        range: 'custom',
        start: '2026-10-01T00:00:00+08:00',
        end: '2026-10-02T00:00:00+08:00'
      },
      controller.signal
    )
    expect(get).toHaveBeenCalledWith('/admin/proxies/7/wireguard/traffic', {
      params: {
        range: 'custom',
        start: '2026-10-01T00:00:00+08:00',
        end: '2026-10-02T00:00:00+08:00'
      },
      signal: controller.signal
    })
  })

  it('sends preset ranges without interval, start or end', async () => {
    await traffic(7, { range: '7d' })
    expect(get).toHaveBeenCalledWith('/admin/proxies/7/wireguard/traffic', {
      params: { range: '7d' },
      signal: undefined
    })
  })
})
