import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import HostQuotaCard from '../components/HostQuotaCard.vue'
import HostUsageStrip from '../components/HostUsageStrip.vue'
import { HOST_AS_OF, hostScenario, hostUsage, type HostScenario } from '@/components/admin/traffic/__tests__/hostFixtures'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}(${Object.values(params).join(',')})` : key
  })
}))

vi.mock('@vueuse/core', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@vueuse/core')>()),
  useNow: () => ({ value: new Date(HOST_AS_OF) })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copied: { value: false }, copyToClipboard: vi.fn() })
}))

function create(name: HostScenario | null, extra: Record<string, unknown> = {}) {
  return mount(HostQuotaCard, {
    props: { status: name ? hostScenario(name) : null, loaded: true, loading: false, error: '', unsupported: false, ...extra },
    global: { stubs: { HelpTooltip: true } }
  })
}

describe('HostQuotaCard', () => {
  it('shows exact usage, remaining quota, projection and the next reset', () => {
    const wrapper = create('complete')
    expect(wrapper.get('[data-testid="host-used"]').text()).toContain('102.92 GB')
    expect(wrapper.get('[data-testid="host-used"]').text()).toContain('traffic.host.ofQuota(1.5 TB)')
    expect(wrapper.get('[data-testid="host-percent"]').text()).toBe('6.86%')
    expect(wrapper.get('[data-testid="host-remaining"]').text()).toContain('1.4 TB')
    expect(wrapper.get('[data-testid="host-projection"]').text()).toContain('342.6 GB')
    expect(wrapper.get('[data-testid="host-reset"]').text()).toContain('traffic.host.countdown.days(22)')
    expect(wrapper.get('[data-testid="host-reset"]').text()).toContain('11-24 · traffic.host.resetRule(24,00:00,UTC)')
    expect(wrapper.get('[data-testid="host-tone"]').text()).toBe('traffic.host.tone.normal')
    expect(wrapper.get('[data-testid="host-chips"]').text()).toContain('10-24 → 11-24 · UTC')
    expect(wrapper.get('[data-testid="host-chips"]').text()).toContain('eth0')
    expect(wrapper.get('[data-testid="host-coverage"]').text()).toContain('traffic.host.coverage.exact')
    expect(wrapper.get('[data-testid="host-coverage"]').text()).toContain('traffic.host.coverage.minorRestarts(1)')
    expect(wrapper.find('[data-testid="host-fill"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="host-last-sample"]').text()).toContain('traffic.host.age.seconds(30)')
  })

  it('splits the usage bar by direction and marks thresholds and elapsed time', () => {
    const wrapper = create('complete')
    expect(wrapper.find('[data-testid="host-bar-rx"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="host-bar-tx"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="host-pace"]').exists()).toBe(true)
    const legend = wrapper.get('[data-testid="host-legend"]').text()
    expect(legend).toContain('traffic.host.direction.inbound')
    expect(legend).toContain('traffic.host.legend.thresholds(80% / 95%)')
    expect(legend).toContain('traffic.host.pace.label(30.4%)')
  })

  it('shows both directions with share and live rate', () => {
    const wrapper = create('complete')
    const rx = wrapper.get('[data-testid="host-direction-rx"]').text()
    const tx = wrapper.get('[data-testid="host-direction-tx"]').text()
    expect(rx).toContain('41.2 GB')
    expect(rx).toContain('traffic.host.direction.share(40%)')
    expect(rx).toContain('3.3 Mbps')
    expect(tx).toContain('61.72 GB')
    expect(tx).toContain('11.04 Mbps')
    expect(wrapper.get('[data-testid="host-direction"]').text()).toContain('traffic.host.direction.rule.both')
  })

  it('marks the billed direction for single-direction plans', () => {
    const status = hostScenario('complete')
    status.plan = { ...status.plan!, billing_mode: 'outbound' }
    const wrapper = mount(HostQuotaCard, {
      props: { status, loaded: true, loading: false, error: '', unsupported: false },
      global: { stubs: { HelpTooltip: true } }
    })
    expect(wrapper.get('[data-testid="host-direction-tx"]').text()).toContain('traffic.host.direction.billed')
    expect(wrapper.get('[data-testid="host-direction-rx"]').text()).toContain('traffic.host.direction.notBilled')
    expect(wrapper.find('[data-testid="host-bar-rx"]').exists()).toBe(false)
  })

  it('marks usage as a lower bound and offers to fill in earlier usage when counting started mid-period', async () => {
    const wrapper = create('partial')
    expect(wrapper.get('[data-testid="host-used"]').text()).toContain('traffic.host.bound.atLeast')
    expect(wrapper.get('[data-testid="host-remaining"]').text()).toContain('traffic.host.bound.atMost')
    expect(wrapper.get('[data-testid="host-coverage"]').text()).toContain('traffic.host.coverage.lower')
    expect(wrapper.get('[data-testid="host-projection"]').text()).toContain('traffic.host.projectionPending')
    await wrapper.get('[data-testid="host-fill"]').trigger('click')
    expect(wrapper.emitted('calibrate')).toHaveLength(1)
  })

  it('shows provider fill-in as an estimate with its own bar segment', () => {
    const wrapper = create('filled')
    expect(wrapper.get('[data-testid="host-used"]').text()).toContain('traffic.host.bound.about')
    expect(wrapper.get('[data-testid="host-used"]').text()).toContain('102.92 GB')
    expect(wrapper.find('[data-testid="host-bar-adjustment"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="host-coverage"]').text()).toContain('traffic.host.coverage.fillLine(+95.1 GB)')
    expect(wrapper.get('[data-testid="host-direction-note"]').text()).toContain('traffic.host.direction.measuredOnly(+95.1 GB)')
  })

  it('shows overage, firing alerts and the missing alert email', () => {
    const wrapper = create('over')
    expect(wrapper.get('[data-testid="host-remaining"]').text()).toContain('traffic.host.over')
    expect(wrapper.get('[data-testid="host-remaining"]').text()).toContain('80 GB')
    expect(wrapper.get('[data-testid="host-tone"]').text()).toBe('traffic.host.tone.over')
    expect(wrapper.get('[data-testid="host-notice-alert-usage-critical"]').text()).toContain('traffic.host.alerts.usage_critical')
    expect(wrapper.get('[data-testid="host-alerts"]').text()).toContain('traffic.host.alerts.onWithProjection(80,95)')
    expect(wrapper.text()).toContain('traffic.host.alerts.emailOff')
  })

  it('suggests switching units after steady reconciliation deviation', async () => {
    const wrapper = create('unitHint')
    expect(wrapper.get('[data-testid="host-notice-reconcile"]').text()).toContain('−6.9%')
    await wrapper.get('[data-testid="host-notice-action-unit"]').trigger('click')
    expect(wrapper.emitted('edit')?.[0]).toEqual([{ unit_base: 1024 }])
  })

  it('warns when readings stopped instead of showing live rates', () => {
    const wrapper = create('stale')
    expect(wrapper.get('[data-testid="host-notice-stale"]').text()).toContain('traffic.host.stale(traffic.host.duration.minutes(25)')
    expect(wrapper.get('[data-testid="host-quota"]').text()).not.toContain('Mbps')
  })

  it('shows usage since counting started and asks to set up the plan', async () => {
    const wrapper = create('noPlan')
    expect(wrapper.get('[data-testid="host-since-total"]').text()).toBe('7.28 GiB')
    await wrapper.get('[data-testid="host-setup"]').trigger('click')
    expect(wrapper.emitted('setup')).toHaveLength(1)
    expect(wrapper.find('[data-testid="host-edit"]').exists()).toBe(false)
  })

  it('asks to confirm the interface when several are found', async () => {
    const wrapper = create('ambiguous')
    await wrapper.get('[data-testid="host-notice-action-ambiguous"]').trigger('click')
    expect(wrapper.emitted('setup')).toHaveLength(1)
  })

  it('guides enabling the host mount and never shows container numbers', () => {
    const wrapper = create('needsMount')
    expect(wrapper.find('[data-testid="host-mount-guide"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="host-mount-snippet"]').text()).toContain('target: /host/sys')
    expect(wrapper.get('[data-testid="host-mount-snippet"]').text()).toContain('read_only: true')
    expect(wrapper.find('[data-testid="host-used"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="host-since-total"]').exists()).toBe(false)
  })

  it('explains unsupported environments, first readings and old backends', () => {
    expect(create('unsupported').find('[data-testid="host-unsupported"]').exists()).toBe(true)
    expect(create('starting').find('[data-testid="host-starting"]').exists()).toBe(true)
    const old = create(null, { error: 'traffic.host.errors.unsupported', unsupported: true })
    expect(old.get('[data-testid="host-error"]').text()).toBe('traffic.host.errors.unsupported')
    expect(old.get('[data-testid="host-error"]').attributes('role')).toBeUndefined()
  })
})

describe('HostUsageStrip', () => {
  it('shows whole-server traffic for the range with coverage', () => {
    const wrapper = mount(HostUsageStrip, {
      props: { usage: hostUsage('24h', { observed_seconds: 43_200 }), loading: false, error: '' },
      global: { stubs: { HelpTooltip: true } }
    })
    expect(wrapper.get('[data-testid="host-usage-total"]').text()).toBe('10.71 GiB')
    expect(wrapper.text()).toContain('traffic.host.usage.coverage(50%)')
  })

  it('does not show numbers when the host view is unavailable', () => {
    const wrapper = mount(HostUsageStrip, {
      props: { usage: hostUsage('24h', { state: 'needs_host_mount', has_records: false }), loading: false, error: '' },
      global: { stubs: { HelpTooltip: true } }
    })
    expect(wrapper.get('[data-testid="host-usage-message"]').text()).toBe('traffic.host.usage.notEnabled')
    expect(wrapper.find('[data-testid="host-usage-total"]').exists()).toBe(false)
  })
})
