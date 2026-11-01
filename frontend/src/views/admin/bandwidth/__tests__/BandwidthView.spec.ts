import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import BandwidthView from '../BandwidthView.vue'
import { getBandwidthLimits, saveBandwidthLimit } from '@/api/admin/bandwidth'
import { hostResponse, wgDetails, wgReport } from './fixtures'
import { liveData, rankingData } from '@/components/admin/traffic/__tests__/fixtures'

vi.mock('@/api/admin/bandwidth', () => ({ getBandwidthLimits: vi.fn(), saveBandwidthLimit: vi.fn() }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn(), showInfo: vi.fn() }) }))

const mockGetServiceTraffic = vi.fn()
const mockListProxies = vi.fn()
const mockDetails = vi.fn()
const mockTraffic = vi.fn()
const mockReplace = vi.fn()
const routeState = { query: {} as Record<string, string> }

const mockRanking = vi.fn()
const mockLive = vi.fn()
vi.mock('@/api/admin/trafficSubjects', () => ({
  getTrafficRanking: (...args: unknown[]) => mockRanking(...args),
  getTrafficLive: (...args: unknown[]) => mockLive(...args),
  getTrafficSubject: vi.fn()
}))

vi.mock('@/api/admin/ops', () => ({
  opsAPI: { getServiceTraffic: (...args: unknown[]) => mockGetServiceTraffic(...args) }
}))

vi.mock('@/api/admin/proxies', () => ({
  proxiesAPI: { list: (...args: unknown[]) => mockListProxies(...args) }
}))

vi.mock('@/api/admin/wireguard', () => ({
  details: (...args: unknown[]) => mockDetails(...args),
  traffic: (...args: unknown[]) => mockTraffic(...args)
}))

vi.mock('@/components/layout/AppLayout.vue', () => ({
  default: { template: '<div><slot /></div>' }
}))

// 详情弹窗沿用 IP 管理页的现有组件，这里只验证打开了哪个节点。
vi.mock('@/components/admin/proxy/WireGuardDialog.vue', () => ({
  default: { props: ['nodeId'], template: '<div data-testid="wg-dialog">{{ nodeId }}</div>' }
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
  Line: { props: ['data', 'options', 'plugins'], template: '<div data-testid="line-chart" />' },
  Bar: { props: ['data', 'options', 'plugins'], template: '<div data-testid="bar-chart" />' }
}))

const proxy = (id: number, name: string) => ({ id, name, protocol: 'wireguard' })

function create() {
  return mount(BandwidthView, {
    global: {
      stubs: {
        HelpTooltip: true,
        RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' },
        BaseDialog: { props: ['show', 'title'], template: '<div v-if="show"><slot /></div>' }
      }
    }
  })
}

beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
  routeState.query = {}
  vi.mocked(getBandwidthLimits).mockResolvedValue({ host: { mbps: null, version: '' }, nodes: {} })
  vi.mocked(saveBandwidthLimit).mockImplementation(async (_id, value) => ({ ...value, version: 'saved' }))
  mockGetServiceTraffic.mockResolvedValue(hostResponse())
  mockRanking.mockResolvedValue(rankingData())
  mockLive.mockResolvedValue(liveData())
  mockListProxies.mockResolvedValue({ items: [proxy(11, 'Tokyo relay'), proxy(12, 'Legacy node')], total: 2, page: 1, page_size: 100, pages: 1 })
  mockDetails.mockResolvedValue(wgDetails())
  mockTraffic.mockImplementation((id: number) =>
    Promise.resolve(id === 12 ? wgReport({ peak_history: null }) : wgReport())
  )
})

describe('BandwidthView', () => {
  it('loads nodes beyond the first page and deduplicates nodes moved between pages', async () => {
    mockListProxies
      .mockResolvedValueOnce({ items: [proxy(11, 'First node')], pages: 2 })
      .mockResolvedValueOnce({ items: [proxy(11, 'First node'), proxy(13, 'Last node')], pages: 2 })
    const wrapper = create()
    await flushPromises()
    expect(mockListProxies.mock.calls.map((call) => call[0])).toEqual([1, 2])
    expect(mockDetails.mock.calls.map((call) => call[0])).toEqual([11, 13])
    expect(wrapper.text()).toContain('Last node')
    wrapper.unmount()
  })

  it('loads host traffic and every WireGuard node for the same range', async () => {
    const wrapper = create()
    await flushPromises()

    expect(mockGetServiceTraffic.mock.calls[0][0]).toEqual({ range: '24h' })
    expect(mockListProxies).toHaveBeenCalledWith(1, 100, { protocol: 'wireguard' }, expect.anything())
    expect(mockTraffic.mock.calls.map((call) => [call[0], call[1]])).toEqual([
      [11, { range: '24h' }],
      [12, { range: '24h' }]
    ])
    expect(wrapper.get('[data-testid="host-peak"]').text()).toContain('16.78')
    expect(wrapper.get('[data-testid="wg-node-11"]').text()).toContain('33.55')
    expect(wrapper.get('[data-testid="wg-node-12"]').text()).toContain('bandwidth.nodes.legacy')
    expect(wrapper.get('[data-testid="range-text"]').text()).toContain('bandwidth.perMinute')
    wrapper.unmount()
  })

  it('switches every section to the new range and keeps it in the address', async () => {
    const wrapper = create()
    await flushPromises()
    mockGetServiceTraffic.mockResolvedValue(hostResponse({ range: '7d', interval: 'hour' }))

    await wrapper.findAll('button').find((button) => button.text() === 'bandwidth.range.7d')!.trigger('click')
    await flushPromises()

    expect(mockReplace).toHaveBeenCalledWith({ query: { range: '7d' } })
    expect(mockGetServiceTraffic.mock.calls.some((call) => call[0].range === '7d')).toBe(true)
    expect(mockTraffic.mock.calls.filter((call) => call[1].range === '7d').map((call) => call[0])).toEqual([11, 12])
    wrapper.unmount()
  })

  it('ranks users by peak bandwidth against the shared host limit and shows live usage', async () => {
    vi.mocked(getBandwidthLimits).mockResolvedValue({ host: { mbps: 20, version: '1' }, nodes: {} })
    const wrapper = create()
    await flushPromises()

    expect(wrapper.find('[data-testid="traffic-overview"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="relay-path"]').exists()).toBe(false)
    expect(mockRanking.mock.calls[0][0]).toMatchObject({ range: '24h', subject: 'user', sort: 'peak_total' })
    expect(mockLive.mock.calls[0][0]).toMatchObject({ subject: 'user' })
    // 入站峰值 16.78 Mbps 占 20 Mbps 上限 84%
    expect(wrapper.get('[data-testid="ranking-row-user:7"] [data-testid="cell-usage"]').text()).toContain('84%')
    expect(wrapper.get('[data-testid="live-row-user:7"]').text()).toContain('3.6 Mbps')

    await wrapper.findAll('button').find((button) => button.text() === 'bandwidth.range.7d')!.trigger('click')
    await flushPromises()
    expect(mockRanking.mock.calls.at(-1)?.[0]).toMatchObject({ range: '7d', page: 1 })
    wrapper.unmount()
  })

  it('opens subject details from the ranking', async () => {
    const wrapper = create()
    await flushPromises()
    await wrapper.get('[data-testid="ranking-row-user:7"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="subject-detail"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('reads the initial range from the address', async () => {
    routeState.query = { range: '30d' }
    const wrapper = create()
    await flushPromises()
    expect(mockGetServiceTraffic.mock.calls[0][0]).toEqual({ range: '30d' })
    wrapper.unmount()
  })

  it('collects data quality warnings at the top', async () => {
    mockGetServiceTraffic.mockResolvedValue(
      hostResponse(
        {
          incomplete: true,
          concurrent_runtimes: true,
          quality: { first_sample_at: '2026-10-31T20:00:00Z', last_sample_at: '2026-11-01T02:59:59Z', valid_samples: 10, invalid_samples: 2 }
        },
        {},
        { persistence_error: true }
      )
    )
    const wrapper = create()
    await flushPromises()
    const warnings = wrapper.get('[data-testid="host-warnings"]').text()
    expect(warnings).toContain('bandwidth.warnings.concurrent')
    expect(warnings).toContain('bandwidth.warnings.incomplete')
    expect(warnings).toContain('bandwidth.warnings.invalidSamples(2)')
    expect(warnings).toContain('bandwidth.warnings.partialCoverage')
    expect(warnings).toContain('bandwidth.warnings.persistenceError')
    wrapper.unmount()
  })

  it('saves a host bandwidth limit on the server and shows the headroom', async () => {
    const wrapper = create()
    await flushPromises()
    const hero = wrapper.get('[data-testid="host-hero"]')
    await hero.findAll('button').find((button) => button.text() === 'bandwidth.limit.set')!.trigger('click')
    await hero.get('input').setValue('20')
    await hero.get('form').trigger('submit')
    await flushPromises()

    expect(saveBandwidthLimit).toHaveBeenCalledWith(null, { mbps: 20, version: '' }, expect.any(AbortSignal))
    expect(localStorage.getItem('sub2api:bandwidth-limits:v1')).toBeNull()
    expect(wrapper.get('[data-testid="host-hero"]').text()).toContain('bandwidth.limit.usage(84%)')
    expect(wrapper.get('[data-testid="host-hero"]').text()).toContain('bandwidth.limit.levelWarning')
    wrapper.unmount()
  })

  it('opens the existing WireGuard dialog for node details', async () => {
    const wrapper = create()
    await flushPromises()
    await wrapper.get('[data-testid="wg-node-11"]').findAll('button').find((button) => button.text() === 'bandwidth.nodes.details')!.trigger('click')
    expect(wrapper.get('[data-testid="wg-dialog"]').text()).toBe('11')
    wrapper.unmount()
  })

  it('shows an empty state without WireGuard nodes and keeps host data when nodes are forbidden', async () => {
    mockListProxies.mockResolvedValueOnce({ items: [], total: 0, page: 1, page_size: 100, pages: 0 })
    const empty = create()
    await flushPromises()
    expect(empty.find('[data-testid="wg-empty"]').exists()).toBe(true)
    empty.unmount()

    mockListProxies.mockRejectedValueOnce({ status: 403 })
    const forbidden = create()
    await flushPromises()
    expect(forbidden.get('[data-testid="wg-nodes"]').text()).toContain('bandwidth.nodes.forbidden')
    expect(forbidden.get('[data-testid="host-peak"]').text()).toContain('16.78')
    forbidden.unmount()
  })

  it('keeps WireGuard nodes visible when host traffic is unavailable', async () => {
    mockGetServiceTraffic.mockRejectedValue({ status: 404, code: 'OPS_DISABLED' })
    const wrapper = create()
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('bandwidth.errors.disabled')
    expect(wrapper.find('[data-testid="wg-node-11"]').exists()).toBe(true)
    wrapper.unmount()
  })
})
