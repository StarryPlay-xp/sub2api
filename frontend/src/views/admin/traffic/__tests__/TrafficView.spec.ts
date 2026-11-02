import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import TrafficView from '../TrafficView.vue'
import { hostResponse, wgDetails, wgReport } from '@/views/admin/bandwidth/__tests__/fixtures'
import { detailData, rankingData } from '@/components/admin/traffic/__tests__/fixtures'
import { egressDetail, egressReport, egressRoute } from '@/components/admin/traffic/__tests__/egressFixtures'
import { hostScenario, hostStatus, hostUsage } from '@/components/admin/traffic/__tests__/hostFixtures'
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

const mockEgress = vi.fn()
const mockEgressRoute = vi.fn()

vi.mock('@/api/admin/trafficEgress', () => ({
  getTrafficEgress: (...args: unknown[]) => mockEgress(...args),
  getTrafficEgressRoute: (...args: unknown[]) => mockEgressRoute(...args)
}))

const mockHostStatus = vi.fn()
const mockHostUsage = vi.fn()

vi.mock('@/api/admin/hostTraffic', () => ({
  getHostTraffic: (...args: unknown[]) => mockHostStatus(...args),
  getHostTrafficUsage: (...args: unknown[]) => mockHostUsage(...args),
  getHostTrafficDetail: vi.fn(),
  saveHostTrafficPlan: vi.fn(),
  calibrateHostTraffic: vi.fn()
}))

const mockListProxies = vi.fn()
const mockDetails = vi.fn()
const mockTraffic = vi.fn()

vi.mock('@/api/admin/proxies', () => ({
  proxiesAPI: { list: (...args: unknown[]) => mockListProxies(...args) }
}))

vi.mock('@/api/admin/wireguard', () => ({
  details: (...args: unknown[]) => mockDetails(...args),
  traffic: (...args: unknown[]) => mockTraffic(...args)
}))

vi.mock('@/components/admin/proxy/WireGuardDialog.vue', () => ({
  default: { props: ['nodeId'], template: '<div data-testid="wg-dialog">{{ nodeId }}</div>' }
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
  mockListProxies.mockResolvedValue({ items: [{ id: 11, name: 'Tokyo relay', protocol: 'wireguard' }], total: 1, page: 1, page_size: 100, pages: 1 })
  mockDetails.mockResolvedValue(wgDetails())
  mockTraffic.mockResolvedValue(wgReport())
  mockEgress.mockResolvedValue(egressReport())
  mockHostStatus.mockResolvedValue(hostStatus())
  mockHostUsage.mockImplementation(async (range: '24h' | '7d' | '30d' | '90d') => hostUsage(range))
  mockEgressRoute.mockImplementation(async (type: 'direct' | 'proxy' | 'wireguard', id: number) =>
    egressDetail(egressRoute(type, type === 'direct' ? null : id))
  )
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

  it('shows WireGuard tunnel traffic for the same range and opens node details', async () => {
    const wrapper = create()
    await flushPromises()
    expect(mockTraffic).toHaveBeenCalledWith(11, { range: '24h' }, expect.anything())
    expect(wrapper.get('[data-testid="node-traffic-11"]').text()).toContain('1.5 GiB')

    await wrapper.get('[data-testid="node-traffic-11"]').trigger('click')
    expect(wrapper.get('[data-testid="node-detail"]').text()).toContain('Tokyo relay')
    expect(wrapper.find('[data-testid="node-detail-limit"]').exists()).toBe(false)

    await wrapper.get('[data-testid="node-detail-manage"]').trigger('click')
    expect(wrapper.get('[data-testid="wg-dialog"]').text()).toBe('11')
    wrapper.unmount()
  })

  it('splits the upstream hop by egress and opens node or egress details', async () => {
    const wrapper = create()
    await flushPromises()
    expect(mockEgress).toHaveBeenCalledWith({ range: '24h' }, expect.anything())
    expect(wrapper.get('[data-testid="relay-path"] [data-testid="egress-wireguard:11"]').text()).toContain('Tokyo relay')
    expect(wrapper.get('[data-testid="node-traffic-11"]').text()).toContain('traffic.nodes.service(1 GiB)')

    // WG 出口在节点列表中：打开节点详情，同时加载经此节点的服务流量
    await wrapper.get('[data-testid="egress-wireguard:11"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="node-detail"]').exists()).toBe(true)
    expect(mockEgressRoute).toHaveBeenLastCalledWith('wireguard', 11, { range: '24h', compare: true }, expect.anything())
    expect(wrapper.get('[data-testid="node-detail"] [data-testid="egress-tile-difference"]').text()).toContain('+128 MiB')
    wrapper.unmount()

    // 直连打开出口详情
    const direct = create()
    await flushPromises()
    await direct.get('[data-testid="egress-direct:0"]').trigger('click')
    await flushPromises()
    expect(direct.find('[data-testid="egress-detail"]').exists()).toBe(true)
    expect(mockEgressRoute).toHaveBeenLastCalledWith('direct', 0, { range: '24h', compare: true }, expect.anything())
    direct.unmount()
  })

  it('keeps the page usable when the backend has no egress statistics', async () => {
    mockEgress.mockRejectedValue({ status: 404 })
    const wrapper = create()
    await flushPromises()
    expect(wrapper.get('[data-testid="egress-error"]').text()).toBe('traffic.egress.errors.unsupported')
    expect(wrapper.get('[data-testid="traffic-total"]').text()).toContain('1.5 GiB')
    expect(wrapper.find('[data-testid="node-traffic-11"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('refreshes the report and the ranking together', async () => {
    const wrapper = create()
    await flushPromises()
    await wrapper.findAll('button').find((button) => button.text() === 'bandwidth.refresh')!.trigger('click')
    await flushPromises()
    expect(mockGetServiceTraffic).toHaveBeenCalledTimes(2)
    expect(mockRanking).toHaveBeenCalledTimes(2)
    expect(mockTraffic).toHaveBeenCalledTimes(2)
    expect(mockEgress).toHaveBeenCalledTimes(2)
    expect(mockHostStatus).toHaveBeenCalledTimes(2)
    expect(mockHostUsage).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('puts the VPS plan above the range bar and whole-server traffic in the overview', async () => {
    const wrapper = create()
    await flushPromises()
    const card = wrapper.get('[data-testid="host-quota"]')
    const rangeButton = wrapper.findAll('button').find((button) => button.text() === 'bandwidth.range.7d')!
    // 套餐卡在时间范围栏之前，不随范围变化。
    expect(card.element.compareDocumentPosition(rangeButton.element) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(card.text()).toContain('102.92 GB')
    expect(wrapper.get('[data-testid="traffic-overview"] [data-testid="host-usage-total"]').text()).toBe('10.71 GiB')
    expect(mockHostUsage).toHaveBeenLastCalledWith('24h', expect.anything())

    await rangeButton.trigger('click')
    await flushPromises()
    expect(mockHostUsage).toHaveBeenLastCalledWith('7d', expect.anything())
    expect(mockHostStatus).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('opens plan setup from the card', async () => {
    mockHostStatus.mockResolvedValue(hostScenario('noPlan'))
    const wrapper = create()
    await flushPromises()
    await wrapper.get('[data-testid="host-setup"]').trigger('click')
    expect(wrapper.find('[data-testid="host-plan-form"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('keeps the page usable when the backend has no VPS traffic statistics', async () => {
    mockHostStatus.mockRejectedValue({ status: 404 })
    mockHostUsage.mockRejectedValue({ status: 404 })
    const wrapper = create()
    await flushPromises()
    expect(wrapper.get('[data-testid="host-error"]').text()).toBe('traffic.host.errors.unsupported')
    expect(wrapper.find('[data-testid="host-usage"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="traffic-total"]').text()).toContain('1.5 GiB')
    wrapper.unmount()
  })
})
