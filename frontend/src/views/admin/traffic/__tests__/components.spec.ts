import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { Bar } from 'vue-chartjs'
import RelayPathCard from '../components/RelayPathCard.vue'
import TrafficOverview from '../components/TrafficOverview.vue'
import { hostReport, peak } from '@/views/admin/bandwidth/__tests__/fixtures'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}(${Object.values(params).join(',')})` : key
  })
}))

vi.mock('vue-chartjs', () => ({
  Bar: { name: 'Bar', props: ['data', 'options', 'plugins'], template: '<div data-testid="bar-chart" />' }
}))

const stubs = { HelpTooltip: true }

describe('RelayPathCard', () => {
  it('places each direction on its hop with traffic and peak', () => {
    const wrapper = mount(RelayPathCard, { props: { report: hostReport() } })
    expect(wrapper.get('[data-testid="relay-upstream_in"]').text()).toContain('1 GiB')
    expect(wrapper.get('[data-testid="relay-upstream_in"]').text()).toContain('traffic.relay.peak(16.78 Mbps)')
    expect(wrapper.get('[data-testid="relay-client_out"]').text()).toContain('512 MiB')
  })

  it('shows unknown peaks and dashes for empty periods', () => {
    const wrapper = mount(RelayPathCard, {
      props: { report: hostReport({ has_records: false, peaks: { client_in: peak(0) } }) }
    })
    expect(wrapper.get('[data-testid="relay-client_in"]').text()).toContain('—')
    expect(wrapper.get('[data-testid="relay-client_in"]').text()).toContain('traffic.relay.peak(0 Mbps)')
    expect(wrapper.get('[data-testid="relay-upstream_in"]').text()).toContain('traffic.relay.peakUnknown')
  })
})

describe('TrafficOverview', () => {
  function create(props: Record<string, unknown> = {}) {
    return mount(TrafficOverview, {
      props: { report: hostReport(), loading: false, ...props },
      global: { stubs }
    })
  }

  it('summarizes total, daily average and average bandwidth from the observed window', () => {
    const wrapper = create()
    expect(wrapper.get('[data-testid="traffic-total"]').text()).toContain('1.5 GiB')
    expect(wrapper.get('[data-testid="traffic-total"]').text()).toContain(
      'traffic.overview.inOut(1 GiB,512 MiB)'
    )
    expect(wrapper.get('[data-testid="traffic-daily"]').text()).toContain('1.5 GiB')
    expect(wrapper.get('[data-testid="traffic-daily"]').text()).toContain(
      'traffic.overview.observed(traffic.overview.observedHours(24))'
    )
    expect(wrapper.get('[data-testid="traffic-average"]').text()).toContain('0.149 Mbps')
  })

  it('shows the four directions with their share of total traffic', () => {
    const wrapper = create()
    expect(wrapper.get('[data-testid="traffic-client_out"]').text()).toContain('512 MiB')
    expect(wrapper.get('[data-testid="traffic-client_out"]').text()).toContain('33.3%')
    expect(wrapper.get('[data-testid="traffic-upstream_in"]').text()).toContain('1 GiB')
    expect(wrapper.get('[data-testid="traffic-upstream_in"]').text()).toContain('66.6%')
  })

  it('stacks inbound and outbound traffic on a shared time axis', () => {
    const wrapper = create()
    const data = wrapper.findComponent(Bar).props('data') as {
      datasets: { data: { y: number | null }[] }[]
    }
    expect(data.datasets).toHaveLength(2)
    expect(data.datasets[0].data.some((point) => point.y !== null)).toBe(true)
    expect(data.datasets[0].data.some((point) => point.y === null)).toBe(true)
  })

  it('keeps values unknown and explains empty periods', () => {
    const wrapper = create({ report: hostReport({ has_records: false, history: [] }) })
    expect(wrapper.get('[data-testid="traffic-total"]').text()).toContain('—')
    expect(wrapper.get('[data-testid="traffic-daily"]').text()).toContain('—')
    expect(wrapper.get('[data-testid="traffic-average"]').text()).toContain('—')
    expect(wrapper.get('[data-testid="traffic-chart-empty"]').text()).toContain(
      'traffic.overview.noRecords'
    )
    expect(wrapper.findComponent(Bar).exists()).toBe(false)
  })

  it('shows a skeleton until the first report arrives', () => {
    const wrapper = create({ report: null, loading: true })
    expect(wrapper.find('[data-testid="traffic-skeleton"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="traffic-total"]').exists()).toBe(false)
  })
})
