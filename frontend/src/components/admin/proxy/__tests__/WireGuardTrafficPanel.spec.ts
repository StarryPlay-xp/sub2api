import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { Bar } from 'vue-chartjs'
import WireGuardTrafficPanel from '../WireGuardTrafficPanel.vue'
import type { TrafficReport, WGDetails, WGNode } from '@/api/admin/wireguard'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('vue-chartjs', () => ({
  Bar: {
    props: ['data', 'options', 'plugins'],
    template: '<div data-testid="chart" />'
  }
}))

const node = (overrides: Partial<WGNode> = {}): WGNode => ({
  id: 7,
  name: 'Node',
  config_version: 2,
  active: true,
  paused: false,
  tx_bytes: '999',
  rx_bytes: '888',
  incomplete: false,
  metric_basis: '',
  ...overrides
})

const rate = (overrides: Partial<WGDetails['rate']> = {}): WGDetails['rate'] => ({
  state: 'sampled',
  tx_bytes_per_second: 2048,
  rx_bytes_per_second: 0,
  sampled_at: '2026-10-31T05:00:00Z',
  ...overrides
})

const report = (overrides: Partial<TrafficReport> = {}): TrafficReport => ({
  range: '24h',
  interval: 'minute',
  start: '2026-10-30T06:00:00Z',
  end: '2026-10-31T06:00:00Z',
  requested_start: '2026-10-30T06:00:00Z',
  requested_end: '2026-10-31T06:00:00Z',
  retained_from: '2026-10-24T06:00:00Z',
  as_of: '2026-10-31T06:00:00Z',
  end_clamped: false,
  tx_bytes: '536870912',
  rx_bytes: '1073741824',
  total_bytes: '1610612736',
  has_records: true,
  bucket_count: 2,
  incomplete: false,
  metric_basis: 'wg_udp_payload_v1',
  history: [
    { start: '2026-10-31T05:00:00Z', tx_bytes: '1024', rx_bytes: '2048' },
    { start: '2026-10-31T05:01:00Z', tx_bytes: '512', rx_bytes: '1046528' }
  ],
  ...overrides
})

function create(props: Record<string, unknown> = {}) {
  return mount(WireGuardTrafficPanel, {
    props: {
      node: node(),
      rate: rate(),
      report: report(),
      selectedRange: '24h',
      loading: false,
      error: '',
      telemetryGap: false,
      ...props
    },
    global: { stubs: { HelpTooltip: true } }
  })
}

function buttonByText(wrapper: ReturnType<typeof create>, text: string) {
  return wrapper.findAll('button').find((button) => button.text() === text)
}

describe('WireGuardTrafficPanel', () => {
  it('uses server period totals and live rates instead of lifetime counters', () => {
    const wrapper = create()
    expect(wrapper.text()).toContain('512 MiB')
    expect(wrapper.text()).toContain('1 GiB')
    expect(wrapper.text()).toContain('1.5 GiB')
    expect(wrapper.text()).not.toContain('999 B')
    expect(wrapper.text()).toContain('2 KiB/s')
    expect(wrapper.text()).toContain('0 B/s')
    wrapper.unmount()
  })

  it('keeps live rates in the card header so long rates never wrap under the total', () => {
    const wrapper = create({ rate: rate({ tx_bytes_per_second: 987_654_321 }) })
    const values = wrapper.findAll('.text-2xl')
    expect(values.map((value) => value.text())).toEqual(['512 MiB', '1 GiB', '1.5 GiB'])
    expect(values.every((value) => !value.text().includes('/s'))).toBe(true)
    const rates = wrapper.findAll('[title="wireguard.rate"]')
    expect(rates).toHaveLength(2)
    expect(rates[0].text()).toBe('941.9 MiB/s')
    expect(rates.every((item) => item.element.parentElement?.textContent?.includes('wireguard.'))).toBe(true)
    wrapper.unmount()
  })

  it('focuses the chart on the recorded span and can switch to the full period', async () => {
    const wrapper = create()
    const chart = () => wrapper.findComponent(Bar)
    const xScale = () =>
      (chart().props('options') as { scales: { x: { min: number; max: number } } }).scales.x
    const sent = () =>
      (chart().props('data') as { datasets: { data: { x: number; y: number | null }[] }[] })
        .datasets[0].data

    expect(xScale().min).toBe(Date.parse('2026-10-31T05:00:00Z'))
    expect(xScale().max).toBe(Date.parse('2026-10-31T05:02:00Z'))
    expect(sent().map((point) => point.y)).toEqual([1, 0.5])

    await buttonByText(wrapper, 'wireguard.chartFull')!.trigger('click')
    expect(xScale().min).toBeLessThanOrEqual(Date.parse('2026-10-30T06:00:00Z'))
    expect(xScale().max).toBeGreaterThanOrEqual(Date.parse('2026-10-31T06:00:00Z'))
    const recorded = sent().filter((point) => point.y !== null)
    expect(recorded).toHaveLength(1)
    // 两个桶合并成一柱后合计超过 1 MiB，纵轴单位随之切换为 MiB
    expect(recorded[0].y).toBeCloseTo(1536 / 1024 ** 2)
    expect(wrapper.text()).toContain('wireguard.noRecordLegend')
    wrapper.unmount()
  })

  it('marks gaps without records and hides the focus switch when records cover the period', () => {
    const gapped = create({
      report: report({
        history: [
          { start: '2026-10-31T05:00:00Z', tx_bytes: '1024', rx_bytes: '0' },
          { start: '2026-10-31T05:05:00Z', tx_bytes: '1024', rx_bytes: '0' }
        ]
      })
    })
    expect(gapped.text()).toContain('wireguard.noRecordLegend')
    expect(gapped.text()).toContain('wireguard.gapsHint')
    gapped.unmount()

    const covered = create({
      report: report({ start: '2026-10-31T05:00:00Z', end: '2026-10-31T05:02:00Z' })
    })
    expect(buttonByText(covered, 'wireguard.chartFocus')).toBeUndefined()
    expect(covered.text()).not.toContain('wireguard.noRecordLegend')
    covered.unmount()
  })

  it('renders a chart only when records exist and distinguishes an empty period', () => {
    const withRecords = create()
    expect(withRecords.find('[data-testid="chart"]').exists()).toBe(true)
    withRecords.unmount()

    const empty = create({
      report: report({ has_records: false, bucket_count: 0, history: [] })
    })
    expect(empty.find('[data-testid="chart"]').exists()).toBe(false)
    expect(empty.text()).toContain('wireguard.noRecords')
    expect(empty.text()).toContain('wireguard.noRecordsHint')
    const metrics = empty.findAll('.text-2xl')
    expect(metrics.every((metric) => metric.text() === '—')).toBe(true)
    empty.unmount()
  })

  it('emits preset ranges', async () => {
    const wrapper = create()
    await buttonByText(wrapper, 'wireguard.range7d')!.trigger('click')
    await buttonByText(wrapper, 'wireguard.range30d')!.trigger('click')
    await buttonByText(wrapper, 'wireguard.range24h')!.trigger('click')
    await buttonByText(wrapper, 'wireguard.range90d')!.trigger('click')
    expect(wrapper.emitted('select')).toEqual([
      [{ range: '7d' }],
      [{ range: '30d' }],
      [{ range: '24h' }],
      [{ range: '90d' }]
    ])
    wrapper.unmount()
  })

  it('includes the whole end day and crosses month boundaries', async () => {
    const wrapper = create()
    await buttonByText(wrapper, 'wireguard.customRange')!.trigger('click')
    const checkboxes = wrapper.findAll('input[type="checkbox"]')
    await checkboxes[0].setValue(false)
    const dates = wrapper.findAll('input[type="date"]')
    await dates[0].setValue('2026-09-30')
    await dates[1].setValue('2026-09-30')
    await buttonByText(wrapper, 'wireguard.customApply')!.trigger('click')

    const emitted = wrapper.emitted('select')!
    const selection = emitted[emitted.length - 1][0] as { range: string; start: string; end?: string }
    expect(selection.range).toBe('custom')
    expect(new Date(selection.start).getTime()).toBe(new Date(2026, 8, 30).getTime())
    expect(new Date(selection.end!).getTime()).toBe(new Date(2026, 8, 30 + 1).getTime())
    wrapper.unmount()
  })

  it('omits the end when counting from a chosen day through now', async () => {
    const wrapper = create()
    await buttonByText(wrapper, 'wireguard.customRange')!.trigger('click')
    const dates = wrapper.findAll('input[type="date"]')
    await dates[0].setValue('2026-10-01')
    await buttonByText(wrapper, 'wireguard.customApply')!.trigger('click')

    const emitted = wrapper.emitted('select')!
    const selection = emitted[emitted.length - 1][0] as { range: string; start: string; end?: string }
    expect(selection.range).toBe('custom')
    expect(new Date(selection.start).getTime()).toBe(new Date(2026, 9, 1).getTime())
    expect(selection.end).toBeUndefined()
    wrapper.unmount()
  })

  it('interprets custom dates as UTC when requested', async () => {
    const wrapper = create()
    await buttonByText(wrapper, 'wireguard.customRange')!.trigger('click')
    const dates = wrapper.findAll('input[type="date"]')
    await dates[0].setValue('2026-10-01')
    const checkboxes = wrapper.findAll('input[type="checkbox"]')
    await checkboxes[1].setValue(true)
    await buttonByText(wrapper, 'wireguard.customApply')!.trigger('click')

    const emitted = wrapper.emitted('select')!
    const selection = emitted[emitted.length - 1][0] as { start: string; end?: string }
    expect(selection.start).toBe('2026-10-01T00:00:00.000Z')
    expect(selection.end).toBeUndefined()
    wrapper.unmount()
  })

  it('blocks a custom start earlier than the retained range', async () => {
    const wrapper = create({ retainedFrom: new Date(Date.now() - 90 * 86_400_000).toISOString() })
    await buttonByText(wrapper, 'wireguard.customRange')!.trigger('click')
    const checkboxes = wrapper.findAll('input[type="checkbox"]')
    await checkboxes[1].setValue(true)
    const dates = wrapper.findAll('input[type="date"]')
    await dates[0].setValue('2020-01-01')
    expect(buttonByText(wrapper, 'wireguard.customApply')!.attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('wireguard.customRetentionExceeded')
    wrapper.unmount()
  })

  it('hides rates and shows the stopped state when the runtime is not running', () => {
    const wrapper = create({ rate: rate({ state: 'not_running' }) })
    expect(wrapper.text()).toContain('wireguard.notRunning')
    const rates = wrapper.findAll('[title="wireguard.rate"]')
    expect(rates).toHaveLength(2)
    expect(rates.every((item) => item.text() === '—')).toBe(true)
    wrapper.unmount()
  })

  it('surfaces incomplete records and telemetry gaps', () => {
    const wrapper = create({ report: report({ incomplete: true }), telemetryGap: true })
    expect(wrapper.text()).toContain('wireguard.incomplete')
    expect(wrapper.text()).toContain('wireguard.telemetryGap')
    wrapper.unmount()
  })

  it('clears totals and shows an error when the request fails', () => {
    const wrapper = create({ report: null, error: 'request failed' })
    expect(wrapper.get('[role="alert"]').text()).toContain('request failed')
    expect(wrapper.text()).not.toContain('512 MiB')
    expect(wrapper.text()).toContain('wireguard.trafficLoadError')
    expect(wrapper.find('[data-testid="chart"]').exists()).toBe(false)
    wrapper.unmount()
  })
})
