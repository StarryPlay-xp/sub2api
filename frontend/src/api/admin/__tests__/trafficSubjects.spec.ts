import { beforeEach, describe, expect, it, vi } from 'vitest'
import { apiClient } from '@/api/client'
import { getTrafficLive, getTrafficRanking, getTrafficSubject } from '../trafficSubjects'

vi.mock('@/api/client', () => ({ apiClient: { get: vi.fn() } }))

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(apiClient.get).mockResolvedValue({ data: {} })
})

describe('traffic subject API', () => {
  it('sends only the ranking parameters that have values', async () => {
    const controller = new AbortController()
    await getTrafficRanking(
      { range: '24h', subject: 'user', sort: 'total_bytes', order: 'desc', page: 2, page_size: 20, q: '  ', compare: false },
      { signal: controller.signal }
    )
    expect(apiClient.get).toHaveBeenCalledWith('/admin/ops/traffic-ranking', {
      params: { range: '24h', subject: 'user', sort: 'total_bytes', order: 'desc', page: 2, page_size: 20 },
      signal: controller.signal
    })
  })

  it('adds the user filter only for API keys and maps comparison to the previous period', async () => {
    await getTrafficRanking({ range: '7d', subject: 'user', user_id: 7, q: ' alice ', compare: true })
    expect(vi.mocked(apiClient.get).mock.calls[0][1]?.params).toEqual({ range: '7d', subject: 'user', q: 'alice', compare: 'previous' })

    await getTrafficRanking({ range: '7d', subject: 'api_key', user_id: 7 })
    expect(vi.mocked(apiClient.get).mock.calls[1][1]?.params).toEqual({ range: '7d', subject: 'api_key', user_id: 7 })
  })

  it('keeps custom boundaries only for custom ranges', async () => {
    await getTrafficRanking({ range: '24h', subject: 'user', start: '2026-10-01T00:00:00Z' })
    expect(vi.mocked(apiClient.get).mock.calls[0][1]?.params).toEqual({ range: '24h', subject: 'user' })

    await getTrafficSubject('api_key', 9, { range: 'custom', start: '2026-10-01T00:00:00Z', end: '2026-10-02T00:00:00Z', compare: true })
    expect(apiClient.get).toHaveBeenLastCalledWith('/admin/ops/traffic-subjects/api_key/9', {
      params: { range: 'custom', start: '2026-10-01T00:00:00Z', end: '2026-10-02T00:00:00Z', compare: 'previous' },
      signal: undefined
    })
  })

  it('requests live usage for the chosen subject', async () => {
    await getTrafficLive({ subject: 'api_key', limit: 10, sort: 'avg_60s' })
    expect(apiClient.get).toHaveBeenCalledWith('/admin/ops/traffic-live', {
      params: { subject: 'api_key', limit: 10, sort: 'avg_60s' },
      signal: undefined
    })
  })
})
