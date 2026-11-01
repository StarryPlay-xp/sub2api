import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { Bar, Line } from 'vue-chartjs'
import BandwidthMeter from '../components/BandwidthMeter.vue'
import HostBandwidthHero from '../components/HostBandwidthHero.vue'
import PeakTrendChart from '../components/PeakTrendChart.vue'
import { formatSampleTime } from '@/utils/trafficFormat'
import { PEAK_AT, hostCurrent, hostReport } from './fixtures'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}(${Object.values(params).join(',')})` : key
  })
}))

vi.mock('vue-chartjs', () => ({
  Line: { name: 'Line', props: ['data', 'options', 'plugins'], template: '<div data-testid="line-chart" />' },
  Bar: { name: 'Bar', props: ['data', 'options', 'plugins'], template: '<div data-testid="bar-chart" />' }
}))

const stubs = { HelpTooltip: true }

describe('BandwidthMeter', () => {
  it('invites setting a limit when none is saved', async () => {
    const wrapper = mount(BandwidthMeter, { props: { peakMbps: 40, limitMbps: null, emptyText: 'empty' } })
    expect(wrapper.text()).toContain('empty')
    await wrapper.get('button').trigger('click')
    const input = wrapper.get('input')
    await input.setValue('0')
    await wrapper.get('form').trigger('submit')
    expect(wrapper.text()).toContain('bandwidth.limit.invalid')
    expect(wrapper.emitted('update:limit')).toBeUndefined()
    await input.setValue('100')
    await wrapper.get('form').trigger('submit')
    expect(wrapper.emitted('update:limit')).toEqual([[100]])
  })

  it('shows usage with a status label and a live marker', () => {
    const warning = mount(BandwidthMeter, { props: { peakMbps: 74, currentMbps: 20, limitMbps: 100, emptyText: '' } })
    expect(warning.text()).toContain('bandwidth.limit.usage(74%)')
    expect(warning.get('[data-testid="meter-level"]').text()).toContain('bandwidth.limit.levelWarning')
    expect(warning.get('[role="meter"]').attributes('aria-valuenow')).toBe('74')
    expect(warning.get('[data-testid="meter-current"]').attributes('style')).toContain('left: 20%')

    const over = mount(BandwidthMeter, { props: { peakMbps: 130, limitMbps: 100, emptyText: '' } })
    expect(over.get('[data-testid="meter-level"]').text()).toContain('bandwidth.limit.levelOver')
    expect(over.get('[role="meter"]').attributes('aria-valuenow')).toBe('100')

    const normal = mount(BandwidthMeter, { props: { peakMbps: 12, limitMbps: 100, emptyText: '' } })
    expect(normal.get('[data-testid="meter-level"]').text()).toContain('bandwidth.limit.levelNormal')
  })
})

describe('HostBandwidthHero', () => {
  function create(props: Record<string, unknown> = {}) {
    return mount(HostBandwidthHero, {
      props: {
        report: hostReport(),
        current: hostCurrent(),
        currentStatus: 'sampled',
        liveSamples: [],
        limit: null,
        loading: false,
        ...props
      },
      global: { stubs }
    })
  }

  it('leads with the busier direction peak in Mbps', () => {
    const wrapper = create()
    expect(wrapper.get('[data-testid="host-peak"]').text()).toContain('16.78')
    expect(wrapper.get('[data-testid="host-peak"]').text()).toContain('Mbps')
    expect(wrapper.text()).toContain('bandwidth.direction.in')
    const detail = wrapper.get('[data-testid="host-peak-detail"]').text()
    expect(detail).toContain(formatSampleTime(PEAK_AT))
    expect(detail).toContain('bandwidth.host.otherLabel(bandwidth.direction.out)')
    expect(detail).toContain('8.4 Mbps')
    expect(detail).toContain(formatSampleTime('2026-11-01T01:00:00Z'))
  })

  it('keeps the peak unknown when merged peaks are unavailable', () => {
    const wrapper = create({ report: hostReport({ peaks: null, concurrent_runtimes: true }) })
    expect(wrapper.get('[data-testid="host-peak"]').text()).toContain('bandwidth.host.unknown')
    expect(wrapper.text()).toContain('bandwidth.host.concurrent')

    const empty = create({ report: hostReport({ has_records: false, peaks: null }) })
    expect(empty.get('[data-testid="host-peak"]').text()).toBe('—')
  })

  it('shows live rates only for normal one-second samples', () => {
    const sampled = create()
    expect(sampled.get('[data-testid="live-in"]').text()).toContain('0.034 Mbps')
    expect(sampled.get('[data-testid="live-status"]').text()).toContain('bandwidth.host.liveSampled(1)')

    for (const [status, label] of [
      ['warming_up', 'bandwidth.host.liveWarmingUp'],
      ['invalid_interval', 'bandwidth.host.liveDelayed'],
      ['stale', 'bandwidth.host.liveStale']
    ]) {
      const wrapper = create({ currentStatus: status })
      expect(wrapper.get('[data-testid="live-status"]').text()).toContain(label)
      expect(wrapper.get('[data-testid="live-in"]').text()).toContain('—')
      expect(wrapper.find('[data-testid="meter-current"]').exists()).toBe(false)
    }
  })
})

describe('PeakTrendChart', () => {
  function create(props: Record<string, unknown> = {}) {
    return mount(PeakTrendChart, {
      props: { report: hostReport(), limit: null, loading: false, ...props },
      global: { stubs }
    })
  }

  it('plots per-bucket peaks in Mbps and leaves gaps as null', () => {
    const wrapper = create()
    const data = wrapper.findComponent(Line).props('data') as {
      datasets: { data: { y: number | null }[] }[]
    }
    const inbound = data.datasets[0].data.map((point) => point.y)
    expect(inbound.filter((value) => value !== null)).toHaveLength(1)
    expect(inbound.find((value) => value !== null)).toBeCloseTo(16.781, 2)
    expect(inbound.some((value) => value === null)).toBe(true)
    expect(wrapper.findComponent(Bar).exists()).toBe(true)
  })

  it('keeps the limit inside the y axis and lists the busiest windows in the table view', async () => {
    const wrapper = create({ limit: 100 })
    const options = wrapper.findComponent(Line).props('options') as { scales: { y: { suggestedMax: number } } }
    expect(options.scales.y.suggestedMax).toBeGreaterThanOrEqual(100)
    expect(wrapper.text()).toContain('bandwidth.trend.limit(100 Mbps)')

    await wrapper.findAll('button').find((button) => button.text() === 'bandwidth.trend.viewTable')!.trigger('click')
    const rows = wrapper.findAll('[data-testid="peak-table"] tbody tr')
    expect(rows).toHaveLength(1)
    expect(rows[0].text()).toContain('16.78 Mbps')
    expect(rows[0].text()).toContain('bandwidth.direction.in')
    expect(rows[0].text()).toContain('17%')
  })

  it('explains empty periods and hidden concurrent peaks', () => {
    const empty = create({ report: hostReport({ has_records: false, history: [], peaks: null }) })
    expect(empty.get('[data-testid="trend-overlay"]').text()).toContain('bandwidth.trend.noRecords')
    expect(empty.findComponent(Line).exists()).toBe(false)

    const concurrent = create({
      report: hostReport({
        concurrent_runtimes: true,
        peaks: null,
        history: hostReport().history.map((bucket) => ({ ...bucket, peaks: null }))
      })
    })
    expect(concurrent.get('[data-testid="trend-overlay"]').text()).toContain('bandwidth.trend.concurrent')
    expect(concurrent.findComponent(Bar).exists()).toBe(true)
  })
})
