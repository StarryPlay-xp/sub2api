import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import TrafficView from '../TrafficView.vue'
import { hostResponse } from '@/views/admin/bandwidth/__tests__/fixtures'
import { detailData, rankingData } from '@/components/admin/traffic/__tests__/fixtures'
import { REPORT_POLL_MS } from '@/views/admin/bandwidth/useServiceTrafficFeed'

const mockGetServiceTraffic = vi.fn()
const mockRanking = vi.fn()
const mockSubject = vi.fn()
const mockReplace = vi.fn()
const routeState = { query: {} as Record<string, string> }

vi.mock('@/api/admin/ops', () => ({
  opsAPI: { getServiceTraffic: (...args: unknown[]) => mockGetServiceTraffic(...args) }
}))

vi.mock('@/api/admin/trafficSubjects', () => ({
  getTrafficRanking: (...args: unknown[]) => mockRanking(...args),
  getTrafficSubject: (...args: unknown[]) => mockSubject(...args),
  getTrafficLive: vi.fn()
}))

vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn(), showInfo: vi.fn() }) }))

vi.mock('@/components/layout/AppLayout.vue', () => ({
  default: { template: '<div><slot /></div>' }
}))

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
  useRouter: () => ({ replace: mockReplace })
}))

vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}(${Object.values(params).join(',')})` : key
  })
}))

vi.mock('vue-chartjs', () => ({
  Bar: { props: ['data', 'options', 'plugins'], template: '<div data-testid="bar-chart" />' }
}))

function create() {
  return mount(TrafficView, {
    global: {
      stubs: {
        HelpTooltip: true,
        RouterLink: { props: ['to'], template: '<a><slot /></a>' },
        BaseDialog: { props: ['show', 'title'], template: '<div v-if="show"><slot /></div>' }
      }
    }
  })
}

beforeEach(() => {
  vi.clearAllMocks()
  routeState.query = {}
  mockGetServiceTraffic.mockResolvedValue(hostResponse())
  mockRanking.mockResolvedValue(rankingData())
  mockSubject.mockResolvedValue(detailData())
})

afterEach(() => {
  vi.useRealTimers()
})

describe('TrafficView', () => {
  it('shows the period totals, composition and the traffic ranking without live polling', async () => {
    vi.useFakeTimers()
    const wrapper = create()
    await flushPromises()

    expect(wrapper.get('[data-testid="traffic-total"]').text()).toContain('1.5 GiB')
    expect(wrapper.find('[data-testid="relay-path"]').exists()).toBe(true)
    expect(mockRanking.mock.calls[0][0]).toMatchObject({ range: '24h', subject: 'user', sort: 'total_bytes' })
    expect(wrapper.get('[data-testid="ranking-row-user:7"]').text()).toContain('1.5 GiB')

    await vi.advanceTimersByTimeAsync(REPORT_POLL_MS - 1000)
    // 流量页不查询每 2 秒的实时速率，只有首次报告请求。
    expect(mockGetServiceTraffic.mock.calls).toEqual([[{ range: '24h' }, expect.anything()]])
    wrapper.unmount()
  })

  it('keeps the range in the address and reloads both sections', async () => {
    routeState.query = { range: '30d' }
    const wrapper = create()
    await flushPromises()
    expect(mockGetServiceTraffic.mock.calls[0][0]).toEqual({ range: '30d' })
    expect(mockRanking.mock.calls[0][0]).toMatchObject({ range: '30d' })

    await wrapper.findAll('button').find((button) => button.text() === 'bandwidth.range.7d')!.trigger('click')
    await flushPromises()
    expect(mockReplace).toHaveBeenCalledWith({ query: { range: '7d' } })
    expect(mockRanking.mock.calls.at(-1)?.[0]).toMatchObject({ range: '7d', page: 1 })
    wrapper.unmount()
  })

  it('opens details for a ranked user with the selected range', async () => {
    const wrapper = create()
    await flushPromises()
    await wrapper.get('[data-testid="ranking-row-user:7"]').trigger('click')
    await flushPromises()
    expect(mockSubject).toHaveBeenCalledWith('user', 7, { range: '24h', compare: true }, expect.anything())
    expect(wrapper.find('[data-testid="detail-average"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('refreshes the report and the ranking together', async () => {
    const wrapper = create()
    await flushPromises()
    await wrapper.findAll('button').find((button) => button.text() === 'bandwidth.refresh')!.trigger('click')
    await flushPromises()
    expect(mockGetServiceTraffic).toHaveBeenCalledTimes(2)
    expect(mockRanking).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })
})
