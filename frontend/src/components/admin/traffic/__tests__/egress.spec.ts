import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { getTrafficEgressRoute } from '@/api/admin/trafficEgress'
import EgressBreakdown from '@/views/admin/traffic/components/EgressBreakdown.vue'
import EgressRouteDialog from '../EgressRouteDialog.vue'
import { routeTitle, tunnelDifference } from '../egressFormat'
import { egressErrorMessage } from '../subjectErrors'
import { egressTrendSamples } from '../trendSamples'
import { egressAccount, egressDetail, egressReport, egressRoute, egressWindow, upstreamBytes } from './egressFixtures'
import { subjectPeak } from './fixtures'

vi.mock('@/api/admin/trafficEgress', () => ({ getTrafficEgressRoute: vi.fn() }))

const t = (key: string, params?: Record<string, unknown>) => (params ? `${key}(${Object.values(params).join(',')})` : key)

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t }) }))

vi.mock('vue-chartjs', () => ({
  Bar: { name: 'Bar', props: ['data', 'options'], template: '<div data-testid="bar-chart" />' }
}))

const stubs = {
  HelpTooltip: true,
  BaseDialog: { props: ['show', 'title'], template: '<div v-if="show"><h3 data-testid="dialog-title">{{ title }}</h3><slot /></div>' }
}

beforeEach(() => {
  vi.mocked(getTrafficEgressRoute).mockReset().mockResolvedValue(egressDetail(egressRoute('direct', null), { accounts: [egressAccount(31)] }))
})

describe('egress helpers', () => {
  it('names routes, maps upstream directions and keeps the tunnel difference signed', () => {
    expect(routeTitle(egressRoute('direct', null), t)).toBe('traffic.egress.kind.direct')
    expect(routeTitle(egressRoute('proxy', 21, { route_name: null }), t)).toBe('#21')

    const samples = egressTrendSamples([
      { start: '2026-11-01T01:00:00Z', interval: 'hour', bytes: upstreamBytes(10, 30), attempts: 1, peaks: { total: subjectPeak(5) } }
    ])
    expect(samples[0].bytes).toEqual({ in: 30, out: 10, total: 40 })

    expect(tunnelDifference(egressRoute('wireguard', 11))).toBe(134_217_728n)
    expect(tunnelDifference(egressRoute('wireguard', 11, { wireguard: { tx_bytes: '0', rx_bytes: '0', total_bytes: '0', has_records: false } }))).toBeNull()
    expect(tunnelDifference(egressRoute('direct', null))).toBeNull()
  })

  it('explains egress errors in their own context', () => {
    expect(egressErrorMessage({ status: 404 }, t)).toBe('traffic.egress.errors.unsupported')
    expect(egressErrorMessage({ status: 404, code: 'OPS_DISABLED' }, t)).toBe('traffic.egress.errors.disabled')
    expect(egressErrorMessage({ status: 404, reason: 'TRAFFIC_ROUTE_NOT_FOUND' }, t)).toBe('traffic.egress.errors.notFound')
    expect(egressErrorMessage({ status: 400, reason: 'TRAFFIC_ROUTE_INVALID' }, t)).toBe('traffic.errors.invalidQuery')
    expect(egressErrorMessage({ status: 403 }, t)).toBe('traffic.egress.errors.forbidden')
  })
})

describe('EgressBreakdown', () => {
  function create(props: Record<string, unknown> = {}) {
    return mount(EgressBreakdown, { props: { data: egressReport(), error: '', ...props }, global: { stubs } })
  }

  it('splits upstream traffic by egress with shares of the upstream total', () => {
    const wrapper = create()
    expect(wrapper.get('[data-testid="egress-total"]').text()).toBe('traffic.egress.total(2 GiB)')
    const tokyo = wrapper.get('[data-testid="egress-wireguard:11"]')
    expect(tokyo.text()).toContain('traffic.egress.kind.wireguard')
    expect(tokyo.text()).toContain('Tokyo relay')
    expect(tokyo.text()).toContain('1 GiB')
    expect(tokyo.text()).toContain('50%')
    expect(tokyo.text()).toContain('traffic.egress.split(128 MiB,896 MiB)')
    expect(tokyo.text()).toContain('traffic.egress.accounts(2)')
    expect(wrapper.get('[data-testid="egress-direct:0"]').text()).toContain('traffic.egress.kind.direct')
    expect(wrapper.get('[data-testid="egress-proxy:21"]').text()).toContain('traffic.egress.deleted')
    expect(wrapper.get('[data-testid="egress-unidentified"]').text()).toContain('256 MiB')
    expect(wrapper.get('[data-testid="egress-unidentified"]').text()).toContain('13%')
  })

  it('marks attempts-only routes, late counting and overflow', () => {
    const wrapper = create({
      data: egressReport({
        window: egressWindow({ egress_started_at: '2026-11-01T01:00:00Z', overflow: true }),
        routes: [egressRoute('proxy', 22, { bytes: upstreamBytes(0, 0), total_bytes: '0', share: 0, attempts: 3, account_count: 0 })]
      })
    })
    expect(wrapper.get('[data-testid="egress-proxy:22"]').text()).toContain('traffic.egress.noBytes')
    const notices = wrapper.get('[data-testid="egress-notices"]').text()
    expect(notices).toContain('traffic.egress.coverageFrom')
    expect(notices).toContain('traffic.egress.overflow')
  })

  it('emits the selected route and shows backend errors quietly', async () => {
    const wrapper = create()
    await wrapper.get('[data-testid="egress-direct:0"]').trigger('click')
    expect(wrapper.emitted('open')?.[0]?.[0]).toMatchObject({ route_type: 'direct' })

    const failed = create({ data: null, error: 'traffic.egress.errors.unsupported' })
    expect(failed.get('[data-testid="egress-error"]').text()).toBe('traffic.egress.errors.unsupported')
  })
})

describe('EgressRouteDialog', () => {
  it('loads direct traffic with direct/0 and shows the service summary, trend and accounts', async () => {
    const wrapper = mount(EgressRouteDialog, { props: { route: egressRoute('direct', null), range: '7d' }, global: { stubs } })
    await flushPromises()

    expect(getTrafficEgressRoute).toHaveBeenCalledWith('direct', 0, { range: '7d', compare: true }, expect.anything())
    expect(wrapper.get('[data-testid="dialog-title"]').text()).toBe('traffic.egress.kind.direct')
    expect(wrapper.get('[data-testid="egress-service"]').text()).toContain('traffic.egress.section.title')
    expect(wrapper.find('[data-testid="egress-tile-tunnel"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="egress-account-31"]').text()).toContain('anthropic · oauth · ID 31')
    expect(wrapper.find('[data-testid="bar-chart"]').exists()).toBe(true)
  })

  it('shows an empty account list and a missing route', async () => {
    vi.mocked(getTrafficEgressRoute).mockResolvedValueOnce(
      egressDetail(egressRoute('proxy', 21), { accounts: [], unattributed_account: { bytes: upstreamBytes(0, 0), total_bytes: '0', attempts: 0 } })
    )
    const empty = mount(EgressRouteDialog, { props: { route: egressRoute('proxy', 21), range: '24h' }, global: { stubs } })
    await flushPromises()
    expect(empty.get('[data-testid="egress-accounts-empty"]').text()).toBe('traffic.egress.accountsTable.empty')
    expect(empty.find('[data-testid="egress-account-residual"]').exists()).toBe(false)

    vi.mocked(getTrafficEgressRoute).mockRejectedValueOnce({ status: 404, reason: 'TRAFFIC_ROUTE_NOT_FOUND' })
    const missing = mount(EgressRouteDialog, { props: { route: egressRoute('proxy', 99), range: '24h' }, global: { stubs } })
    await flushPromises()
    expect(missing.get('[data-testid="egress-service-error"]').text()).toBe('traffic.egress.errors.notFound')
  })
})
