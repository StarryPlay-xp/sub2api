import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h, ref } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import {
  LIVE_POLL_MS,
  REPORT_POLL_MS,
  useServiceTrafficFeed,
  type PresetRange
} from '../useServiceTrafficFeed'
import { deferred, hostResponse } from './fixtures'

const mockGetServiceTraffic = vi.fn()

vi.mock('@/api/admin/ops', () => ({
  opsAPI: { getServiceTraffic: (...args: unknown[]) => mockGetServiceTraffic(...args) }
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

function setup(initial: PresetRange = '24h') {
  const range = ref<PresetRange>(initial)
  let feed!: ReturnType<typeof useServiceTrafficFeed>
  const wrapper = mount(
    defineComponent({
      setup() {
        feed = useServiceTrafficFeed(range)
        return () => h('div')
      }
    })
  )
  return { wrapper, range, feed: () => feed }
}

beforeEach(() => {
  mockGetServiceTraffic.mockReset()
})

afterEach(() => {
  vi.useRealTimers()
})

describe('useServiceTrafficFeed', () => {
  it('loads the selected range with only the range parameter', async () => {
    mockGetServiceTraffic.mockResolvedValue(hostResponse())
    const { wrapper, feed } = setup()
    await flushPromises()

    expect(mockGetServiceTraffic.mock.calls[0][0]).toEqual({ range: '24h' })
    expect(feed().report.value?.total_bytes).toBe('1610615808')
    expect(feed().currentStatus.value).toBe('sampled')
    wrapper.unmount()
  })

  it('polls live rates with a minute-aligned recent window every 2 seconds', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    mockGetServiceTraffic.mockResolvedValueOnce(hostResponse()).mockResolvedValue(
      hostResponse({ as_of: '2026-11-01T03:00:02Z' }, { sampled_at: '2026-11-01T03:00:01Z', bytes_per_second: { in: 8192, out: 1024 } })
    )
    const { wrapper, feed } = setup()
    await flushPromises()

    await vi.advanceTimersByTimeAsync(LIVE_POLL_MS)
    await flushPromises()
    const [params] = mockGetServiceTraffic.mock.calls[1]
    expect(params.range).toBe('custom')
    expect(params.interval).toBe('minute')
    expect(Date.parse(params.start) % 60_000).toBe(0)
    expect(params.end).toBeUndefined()
    expect(feed().current.value?.bytes_per_second.in).toBe(8192)
    expect(feed().liveSamples.value.map((sample) => sample.in)).toEqual([4196, 8192])
    wrapper.unmount()

    const calls = mockGetServiceTraffic.mock.calls.length
    await vi.advanceTimersByTimeAsync(REPORT_POLL_MS * 2)
    expect(mockGetServiceTraffic.mock.calls.length).toBe(calls)
  })

  it('does not let an older report response overwrite a newer live sample', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    const slowReport = deferred<ReturnType<typeof hostResponse>>()
    mockGetServiceTraffic
      .mockResolvedValueOnce(hostResponse())
      .mockResolvedValueOnce(hostResponse({ as_of: '2026-11-01T03:00:10Z' }, { sampled_at: '2026-11-01T03:00:09Z', bytes_per_second: { in: 999 } }))
      .mockReturnValueOnce(slowReport.promise)
      .mockResolvedValue(hostResponse({ as_of: '2026-11-01T03:00:12Z' }))
    const { wrapper, feed, range } = setup()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(LIVE_POLL_MS)
    await flushPromises()
    expect(feed().current.value?.bytes_per_second.in).toBe(999)

    range.value = '7d'
    await flushPromises()
    slowReport.resolve(hostResponse({ range: '7d', as_of: '2026-11-01T03:00:05Z' }, { bytes_per_second: { in: 1 } }))
    await flushPromises()
    expect(feed().report.value?.range).toBe('7d')
    expect(feed().current.value?.bytes_per_second.in).toBe(999)
    wrapper.unmount()
  })

  it('clears the previous range immediately and ignores a stale response', async () => {
    const first = deferred<ReturnType<typeof hostResponse>>()
    const second = deferred<ReturnType<typeof hostResponse>>()
    mockGetServiceTraffic.mockResolvedValueOnce(hostResponse())
    const { wrapper, feed, range } = setup()
    await flushPromises()

    mockGetServiceTraffic.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise)
    range.value = '7d'
    await flushPromises()
    expect(feed().report.value).toBeNull()
    range.value = '30d'
    await flushPromises()
    expect(mockGetServiceTraffic.mock.calls.slice(1, 3).map((call) => call[0])).toEqual([{ range: '7d' }, { range: '30d' }])
    expect(mockGetServiceTraffic.mock.calls[1][1].signal.aborted).toBe(true)

    second.resolve(hostResponse({ range: '30d', total_bytes: '3' }))
    await flushPromises()
    first.resolve(hostResponse({ range: '7d', total_bytes: '2' }))
    await flushPromises()
    expect(feed().report.value?.total_bytes).toBe('3')
    wrapper.unmount()
  })

  it('reports warming up, delayed and stale samples instead of trusted rates', async () => {
    mockGetServiceTraffic.mockResolvedValueOnce(
      hostResponse({}, { state: 'warming_up', sampled_at: '0001-01-01T00:00:00Z', window_seconds: 0, bytes_per_second: {} })
    )
    const warming = setup()
    await flushPromises()
    expect(warming.feed().currentStatus.value).toBe('warming_up')
    expect(warming.feed().liveSamples.value).toHaveLength(0)
    warming.wrapper.unmount()

    mockGetServiceTraffic.mockResolvedValueOnce(hostResponse({}, { state: 'invalid_interval', window_seconds: 3.2 }))
    const delayed = setup()
    await flushPromises()
    expect(delayed.feed().currentStatus.value).toBe('invalid_interval')
    delayed.wrapper.unmount()

    mockGetServiceTraffic.mockResolvedValueOnce(hostResponse({ as_of: '2026-11-01T03:00:30Z' }, { sampled_at: '2026-11-01T03:00:00Z' }))
    const stale = setup()
    await flushPromises()
    expect(stale.feed().currentStatus.value).toBe('stale')
    stale.wrapper.unmount()
  })

  it('stops polling for restricted administrators and maps the error', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    mockGetServiceTraffic.mockRejectedValue({ status: 403 })
    const { wrapper, feed } = setup()
    await flushPromises()

    expect(feed().error.value).toBe('bandwidth.errors.forbidden')
    expect(feed().blocked.value).toBe(true)
    await vi.advanceTimersByTimeAsync(REPORT_POLL_MS * 2)
    expect(mockGetServiceTraffic).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('maps disabled monitoring and retention errors', async () => {
    mockGetServiceTraffic.mockRejectedValueOnce({ status: 404, code: 'OPS_DISABLED' })
    const disabled = setup()
    await flushPromises()
    expect(disabled.feed().error.value).toBe('bandwidth.errors.disabled')
    disabled.wrapper.unmount()

    mockGetServiceTraffic.mockRejectedValueOnce({ status: 400, reason: 'TRAFFIC_RETENTION_EXCEEDED' })
    const retention = setup()
    await flushPromises()
    expect(retention.feed().error.value).toBe('bandwidth.errors.retentionExceeded')
    retention.wrapper.unmount()
  })
})
