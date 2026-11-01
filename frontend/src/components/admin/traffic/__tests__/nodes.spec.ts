import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { getTrafficEgressRoute } from '@/api/admin/trafficEgress'
import type { Proxy } from '@/types'
import NodeBandwidthTable from '@/views/admin/bandwidth/components/NodeBandwidthTable.vue'
import type { WireGuardNodeState } from '@/views/admin/bandwidth/useWireGuardNodes'
import { peak, wgDetails, wgReport } from '@/views/admin/bandwidth/__tests__/fixtures'
import NodeTrafficCard from '@/views/admin/traffic/components/NodeTrafficCard.vue'
import NodeDetailDialog from '../NodeDetailDialog.vue'
import { nodeObservedSeconds, nodeUsage } from '../nodeFormat'
import { nodeTrendSamples } from '../trendSamples'
import { egressDetail, egressRoute } from './egressFixtures'

vi.mock('@/api/admin/trafficEgress', () => ({ getTrafficEgressRoute: vi.fn() }))

beforeEach(() => {
  vi.mocked(getTrafficEgressRoute).mockReset().mockResolvedValue(egressDetail())
})

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}(${Object.values(params).join(',')})` : key
  })
}))

vi.mock('vue-chartjs', () => ({
  Bar: { name: 'Bar', props: ['data', 'options'], template: '<div data-testid="bar-chart" />' }
}))

const stubs = {
  HelpTooltip: true,
  BaseDialog: { props: ['show', 'title'], emits: ['close'], template: '<div v-if="show"><h3>{{ title }}</h3><slot /></div>' }
}

function node(id: number, name: string, overrides: Partial<WireGuardNodeState> = {}): WireGuardNodeState {
  return {
    proxy: { id, name, protocol: 'wireguard', account_count: 3 } as unknown as Proxy,
    details: wgDetails(),
    report: wgReport(),
    error: '',
    ...overrides
  }
}

describe('node helpers', () => {
  it('maps received bytes to inbound and sent bytes to outbound, keeping legacy peaks unknown', () => {
    const samples = nodeTrendSamples([
      { start: '2026-11-01T02:00:00Z', tx_bytes: '10', rx_bytes: '30', peak_history: { peaks: { tx: peak(1), rx: peak(2), total: peak(3) }, quality: { first_sample_at: null, last_sample_at: null, valid_samples: 1, invalid_samples: 0 } } },
      { start: '2026-11-01T03:00:00Z', tx_bytes: '5', rx_bytes: '5', peak_history: null }
    ])
    expect(samples[0].bytes).toEqual({ in: 30, out: 10, total: 40 })
    expect(samples[0].peaks).toEqual({ in: peak(2), out: peak(1), total: peak(3) })
    expect(samples[1].peaks).toBeNull()
  })

  it('compares the same-second node peak with the node limit and averages only observed time', () => {
    // 4,194,304 B/s ≈ 33.55 Mbps
    expect(nodeUsage(wgReport(), 50)).toBeCloseTo(0.671, 3)
    expect(nodeUsage(wgReport({ peak_history: null }), 50)).toBeNull()
    expect(nodeUsage(wgReport(), null)).toBeNull()
    expect(nodeObservedSeconds(wgReport())).toBe(86_398)
    expect(nodeObservedSeconds(wgReport({ peak_history: null }))).toBeNull()
  })
})

describe('NodeBandwidthTable', () => {
  const nodes = () => [
    node(11, 'Tokyo relay'),
    node(12, 'Legacy node', { report: wgReport({ peak_history: null }), details: wgDetails('not_running', { paused: true }) }),
    node(13, 'Busy node', {
      report: wgReport({
        peak_history: {
          peaks: { tx: peak(5_000_000), rx: peak(1_000_000), total: peak(5_500_000) },
          quality: { first_sample_at: null, last_sample_at: null, valid_samples: 1, invalid_samples: 3 }
        }
      })
    })
  ]

  function create(limits: Record<number, number> = {}) {
    return mount(NodeBandwidthTable, {
      props: { nodes: nodes(), limitOf: (id: number) => limits[id] ?? null },
      global: { stubs }
    })
  }

  it('ranks nodes by the same-second peak and keeps unknown peaks last', () => {
    const wrapper = create({ 11: 50 })
    const ids = wrapper.findAll('tbody tr').map((row) => row.attributes('data-testid'))
    expect(ids).toEqual(['wg-node-13', 'wg-node-11', 'wg-node-12'])

    const tokyo = wrapper.get('[data-testid="wg-node-11"]')
    expect(tokyo.get('[data-testid="node-peak"]').text()).toContain('33.55 Mbps')
    expect(tokyo.get('[data-testid="node-peak-tx"]').text()).toBe('21.27 Mbps')
    expect(tokyo.get('[data-testid="node-peak-rx"]').text()).toBe('18.47 Mbps')
    expect(tokyo.get('[data-testid="node-current"]').text()).toBe('3 Mbps')
    expect(tokyo.get('[data-testid="node-usage"]').text()).toContain('67%')
    expect(tokyo.get('[data-testid="node-state"]').text()).toBe('bandwidth.nodes.running')

    const legacy = wrapper.get('[data-testid="wg-node-12"]')
    expect(legacy.get('[data-testid="node-peak"]').text()).toBe('bandwidth.nodes.legacy')
    expect(legacy.get('[data-testid="node-state"]').text()).toBe('bandwidth.nodes.paused')
    expect(legacy.get('[data-testid="node-current"]').text()).toBe('bandwidth.nodes.notRunning')
    expect(legacy.get('[data-testid="node-usage"]').text()).toBe('bandwidth.limit.set')

    expect(wrapper.get('[data-testid="wg-node-13"]').text()).toContain('bandwidth.nodes.invalidSamples(3)')
  })

  it('sorts by another column and toggles the order', async () => {
    const wrapper = create()
    await wrapper.get('[data-testid="node-sort-peakRx"]').trigger('click')
    expect(wrapper.findAll('tbody tr').map((row) => row.attributes('data-testid'))).toEqual(['wg-node-11', 'wg-node-13', 'wg-node-12'])
    await wrapper.get('[data-testid="node-sort-peakRx"]').trigger('click')
    expect(wrapper.findAll('tbody tr').map((row) => row.attributes('data-testid'))).toEqual(['wg-node-13', 'wg-node-11', 'wg-node-12'])
  })

  it('opens a node from its row', async () => {
    const wrapper = create()
    await wrapper.get('[data-testid="wg-node-11"]').trigger('click')
    expect(wrapper.emitted('open')?.[0]).toEqual([11])
  })
})

describe('NodeDetailDialog', () => {
  function create(props: Record<string, unknown> = {}) {
    return mount(NodeDetailDialog, {
      props: { node: node(11, 'Tokyo relay'), range: '24h', mode: 'bandwidth', limitMbps: 50, ...props },
      global: { stubs }
    })
  }

  it('summarizes tunnel traffic, peaks, current rate and the limit', () => {
    const wrapper = create()
    expect(wrapper.get('[data-testid="node-detail-status"]').text()).toContain('bandwidth.nodeDetail.accounts(3)')
    expect(wrapper.get('[data-testid="node-detail-total"]').text()).toContain('1.5 GiB')
    expect(wrapper.get('[data-testid="node-detail-total"]').text()).toContain('bandwidth.nodeDetail.split(512 MiB,1 GiB)')
    expect(wrapper.get('[data-testid="node-detail-peak"]').text()).toContain('33.55 Mbps')
    expect(wrapper.get('[data-testid="node-detail-directions"]').text()).toContain('21.27 Mbps / 18.47 Mbps')
    expect(wrapper.get('[data-testid="node-detail-current"]').text()).toContain('3 Mbps')
    // 1.5 GiB 按观察 86,398 秒折算到一天
    expect(wrapper.get('[data-testid="node-detail-daily"]').text()).toContain('1.5 GiB')
    expect(wrapper.get('[data-testid="node-detail-limit"]').text()).toContain('bandwidth.limit.usage(67%)')
    expect(wrapper.find('[data-testid="node-detail-usage"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="bar-chart"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="node-detail-basis"]').text()).toBe('bandwidth.nodeDetail.basis')
  })

  it('compares service traffic through the node with the tunnel and ranks upstream accounts', async () => {
    const wrapper = create()
    await flushPromises()

    expect(getTrafficEgressRoute).toHaveBeenCalledWith('wireguard', 11, { range: '24h', compare: true }, expect.anything())
    const section = wrapper.get('[data-testid="egress-service"]')
    expect(section.text()).toContain('traffic.egress.section.nodeTitle')
    expect(section.get('[data-testid="egress-tile-service"]').text()).toContain('1 GiB')
    expect(section.get('[data-testid="egress-tile-tunnel"]').text()).toContain('1.13 GiB')
    // 隧道 1.125 GiB − 服务 1 GiB = +128 MiB，保留正负号
    expect(section.get('[data-testid="egress-tile-difference"]').text()).toContain('+128 MiB')
    expect(section.get('[data-testid="egress-tile-growth"]').text()).toContain('+256 MiB')
    expect(section.get('[data-testid="egress-account-31"]').text()).toContain('account-31')
    expect(section.get('[data-testid="egress-account-31"]').text()).toContain('75%')
    expect(section.get('[data-testid="egress-account-32"]').text()).toContain('traffic.egress.accountsTable.deleted')
    expect(section.get('[data-testid="egress-account-residual"]').text()).toContain('128 MiB')
  })

  it('keeps a negative tunnel difference and explains a backend without egress data', async () => {
    vi.mocked(getTrafficEgressRoute).mockResolvedValueOnce(
      egressDetail(
        egressRoute('wireguard', 11, { wireguard: { tx_bytes: '1', rx_bytes: '1', total_bytes: '536870912', has_records: true } })
      )
    )
    const negative = create()
    await flushPromises()
    expect(negative.get('[data-testid="egress-tile-difference"]').text()).toContain('−512 MiB')
    expect(negative.get('[data-testid="egress-tile-difference"]').text()).toContain('traffic.egress.section.differenceNegative')

    vi.mocked(getTrafficEgressRoute).mockRejectedValueOnce({ status: 404 })
    const old = create()
    await flushPromises()
    expect(old.get('[data-testid="egress-service-error"]').text()).toBe('traffic.egress.errors.unsupported')
    expect(old.get('[data-testid="node-detail-total"]').text()).toContain('1.5 GiB')
  })

  it('hands configuration to the manage dialog and passes limit changes up', async () => {
    const wrapper = create()
    await wrapper.get('[data-testid="node-detail-manage"]').trigger('click')
    expect(wrapper.emitted('manage')).toEqual([[11]])
  })

  it('explains legacy history and hides the limit editor on the traffic page', () => {
    const wrapper = create({
      node: node(11, 'Tokyo relay', { report: wgReport({ peak_history: null }), details: wgDetails('warming_up') }),
      mode: 'traffic',
      limitMbps: null
    })
    expect(wrapper.find('[data-testid="node-detail-limit"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="node-detail-peak"]').text()).toContain('bandwidth.host.unknown')
    expect(wrapper.get('[data-testid="node-detail-current"]').text()).toContain('bandwidth.nodes.rateWarmingUp')
    expect(wrapper.get('[data-testid="node-detail-notices"]').text()).toContain('bandwidth.nodes.legacy')
    expect(wrapper.get('[data-testid="node-detail-daily"]').text()).toContain('—')
  })

  it('surfaces per-node load errors', () => {
    const wrapper = create({ node: node(11, 'Tokyo relay', { report: null, error: 'node failed' }) })
    expect(wrapper.get('[role="alert"]').text()).toBe('node failed')
    expect(wrapper.get('[data-testid="node-detail-total"]').text()).toContain('—')
  })
})

describe('NodeTrafficCard', () => {
  it('lists nodes by tunnel traffic with their share and a non-additive note', async () => {
    const wrapper = mount(NodeTrafficCard, {
      props: {
        nodes: [
          node(11, 'Tokyo relay'),
          node(12, 'Small node', { report: wgReport({ tx_bytes: '1073741824', rx_bytes: '0', total_bytes: '1073741824' }) }),
          node(13, 'Empty node', { report: wgReport({ has_records: false }) })
        ],
        loaded: true,
        loading: false,
        error: ''
      },
      global: { stubs }
    })

    const rows = wrapper.findAll('[data-testid^="node-traffic-1"]').map((row) => row.attributes('data-testid'))
    expect(rows).toEqual(['node-traffic-11', 'node-traffic-12', 'node-traffic-13'])
    // 1.5 GiB / (1.5 + 1) GiB = 60%
    expect(wrapper.get('[data-testid="node-traffic-11"]').text()).toContain('1.5 GiB')
    expect(wrapper.get('[data-testid="node-traffic-11"]').text()).toContain('60%')
    expect(wrapper.get('[data-testid="node-traffic-13"]').text()).toContain('bandwidth.nodes.noRecords')
    expect(wrapper.get('[data-testid="node-traffic-summary"]').text()).toContain('2.5 GiB')
    expect(wrapper.text()).toContain('traffic.nodes.note')

    await wrapper.setProps({ egressRoutes: [egressRoute('wireguard', 11), egressRoute('direct', null)] })
    expect(wrapper.get('[data-testid="node-traffic-11"]').text()).toContain('traffic.nodes.service(1 GiB)')
    expect(wrapper.get('[data-testid="node-traffic-12"]').text()).not.toContain('traffic.nodes.service')

    await wrapper.get('[data-testid="node-traffic-12"]').trigger('click')
    expect(wrapper.emitted('open')?.[0]).toEqual([12])
  })

  it('shows an empty state without nodes', () => {
    const wrapper = mount(NodeTrafficCard, { props: { nodes: [], loaded: true, loading: false, error: '' }, global: { stubs } })
    expect(wrapper.get('[data-testid="node-traffic-empty"]').text()).toBe('bandwidth.nodes.empty')
    expect(wrapper.find('[data-testid="node-traffic-summary"]').exists()).toBe(false)
  })
})
