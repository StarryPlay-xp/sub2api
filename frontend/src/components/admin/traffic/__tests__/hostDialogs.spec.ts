import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import HostCalibrationDialog from '../HostCalibrationDialog.vue'
import HostDetailDialog from '../HostDetailDialog.vue'
import HostPlanDialog from '../HostPlanDialog.vue'
import { hostErrorMessage, isHostUnsupported } from '../useHostTraffic'
import { hostDetail, hostScenario, hostStatus } from './hostFixtures'

const mockSave = vi.fn()
const mockCalibrate = vi.fn()
const mockDetail = vi.fn()

vi.mock('@/api/admin/hostTraffic', () => ({
  saveHostTrafficPlan: (...args: unknown[]) => mockSave(...args),
  calibrateHostTraffic: (...args: unknown[]) => mockCalibrate(...args),
  getHostTrafficDetail: (...args: unknown[]) => mockDetail(...args)
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}(${Object.values(params).join(',')})` : key
  })
}))

vi.mock('vue-chartjs', () => ({
  Bar: { props: ['data', 'options'], template: '<div data-testid="bar-chart" />' }
}))

const stubs = {
  BaseDialog: { props: ['show', 'title'], template: '<div v-if="show"><h3>{{ title }}</h3><slot /><slot name="footer" /></div>' },
  HelpTooltip: true
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('HostPlanDialog', () => {
  it('derives the monthly reset day from the next reset date and saves with an optional fill-in', async () => {
    const saved = hostStatus()
    mockSave.mockResolvedValue(saved)
    const wrapper = mount(HostPlanDialog, { props: { status: hostScenario('noPlan') }, global: { stubs } })
    expect(wrapper.text()).toContain('traffic.host.form.createTitle')

    await wrapper.get('[data-testid="host-plan-quota"]').setValue('1.5')
    await wrapper.get('[data-testid="host-plan-next-reset"]').setValue('2026-11-24')
    expect(wrapper.get('[data-testid="host-plan-rule"]').text()).toContain('traffic.host.resetRule(24,00:00,UTC)')
    await wrapper.get('[data-testid="host-plan-current-used"]').setValue('102.92')
    await wrapper.get('[data-testid="host-plan-form"]').trigger('submit')
    await flushPromises()

    expect(mockSave).toHaveBeenCalledWith({
      version: null,
      quota_value: '1.5',
      quota_unit: 'TB',
      billing_mode: 'both',
      reset_day: 24,
      reset_time: '00:00',
      timezone: 'UTC',
      unit_base: 1000,
      interfaces: [],
      alerts: { enabled: true, warning_percent: 80, critical_percent: 95, projection: true },
      current_used: { value: '102.92', unit: 'GB' }
    })
    expect(wrapper.emitted('saved')?.[0]).toEqual([saved, true])
  })

  it('validates required fields before saving', async () => {
    const wrapper = mount(HostPlanDialog, { props: { status: hostScenario('noPlan') }, global: { stubs } })
    await wrapper.get('[data-testid="host-plan-form"]').trigger('submit')
    expect(mockSave).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('traffic.host.form.errors.quota')
    expect(wrapper.text()).toContain('traffic.host.form.errors.date')
  })

  it('warns that a month-end date may come from a later reset day', async () => {
    const wrapper = mount(HostPlanDialog, { props: { status: hostScenario('noPlan') }, global: { stubs } })
    await wrapper.get('[data-testid="host-plan-next-reset"]').setValue('2027-02-28')
    expect(wrapper.find('[data-testid="host-plan-clamped"]').exists()).toBe(true)
  })

  it('edits an existing plan with its version and applies a suggested unit base', async () => {
    mockSave.mockResolvedValue(hostStatus())
    const wrapper = mount(HostPlanDialog, {
      props: { status: hostScenario('unitHint'), patch: { unit_base: 1024 } },
      global: { stubs }
    })
    expect(wrapper.text()).toContain('traffic.host.form.editTitle')
    expect((wrapper.get('[data-testid="host-plan-next-reset"]').element as HTMLInputElement).value).toBe('2026-11-24')
    expect(wrapper.find('[data-testid="host-plan-current-used"]').exists()).toBe(false)
    expect((wrapper.get('[data-testid="host-plan-base-1024"]').element as HTMLInputElement).checked).toBe(true)
    await wrapper.get('[data-testid="host-plan-billing-outbound"] input').setValue(true)
    await wrapper.get('[data-testid="host-plan-form"]').trigger('submit')
    await flushPromises()
    expect(mockSave.mock.calls[0][0]).toMatchObject({ version: 'v1', unit_base: 1024, billing_mode: 'outbound', reset_day: 24, current_used: null })
    expect(wrapper.emitted('saved')?.[0][1]).toBe(false)
  })

  it('shows conflicts without closing', async () => {
    mockSave.mockRejectedValue({ status: 409, reason: 'HOST_TRAFFIC_PLAN_CONFLICT' })
    const wrapper = mount(HostPlanDialog, { props: { status: hostStatus() }, global: { stubs } })
    await wrapper.get('[data-testid="host-plan-form"]').trigger('submit')
    await flushPromises()
    expect(wrapper.get('[data-testid="host-plan-error"]').text()).toBe('traffic.host.errors.conflict')
    expect(wrapper.emitted('saved')).toBeUndefined()
  })
})

describe('HostCalibrationDialog', () => {
  it('fills in earlier usage when the period started mid-way', async () => {
    const status = hostScenario('partial')
    mockCalibrate.mockResolvedValue({ kind: 'fill', status: hostScenario('filled') })
    const wrapper = mount(HostCalibrationDialog, { props: { status }, global: { stubs } })
    expect(wrapper.text()).toContain('traffic.host.calibrate.fillTitle')
    expect(wrapper.find('[data-testid="host-calibration-override"]').exists()).toBe(false)
    await wrapper.get('[data-testid="host-calibration-value"]').setValue('102.92')
    await wrapper.get('[data-testid="host-calibration-form"]').trigger('submit')
    await flushPromises()
    expect(mockCalibrate).toHaveBeenCalledWith({ value: '102.92', unit: 'GB', mode: 'auto', plan_version: 'v1' })
    expect(wrapper.emitted('saved')?.[0][1]).toBe('fill')
  })

  it('only reconciles a complete period unless the provider value is chosen explicitly', async () => {
    mockCalibrate.mockResolvedValue({ kind: 'override', status: hostStatus() })
    const wrapper = mount(HostCalibrationDialog, { props: { status: hostStatus() }, global: { stubs } })
    expect(wrapper.get('[data-testid="host-calibration-explain"]').text()).toBe('traffic.host.calibrate.reconcileExplain')
    await wrapper.get('[data-testid="host-calibration-value"]').setValue('101.5')
    await wrapper.get('[data-testid="host-calibration-override"]').setValue(true)
    await wrapper.get('[data-testid="host-calibration-form"]').trigger('submit')
    await flushPromises()
    expect(mockCalibrate.mock.calls[0][0]).toMatchObject({ mode: 'override' })
  })

  it('reports missing fresh readings', async () => {
    mockCalibrate.mockRejectedValue({ status: 409, reason: 'HOST_TRAFFIC_NOT_READY' })
    const wrapper = mount(HostCalibrationDialog, { props: { status: hostStatus() }, global: { stubs } })
    await wrapper.get('[data-testid="host-calibration-value"]').setValue('1')
    await wrapper.get('[data-testid="host-calibration-form"]').trigger('submit')
    await flushPromises()
    expect(wrapper.get('[data-testid="host-calibration-error"]').text()).toBe('traffic.host.errors.notReady')
  })
})

describe('HostDetailDialog', () => {
  it('lists daily usage, past periods, reconciliation records and interfaces', async () => {
    mockDetail.mockResolvedValue(hostDetail())
    const wrapper = mount(HostDetailDialog, { props: { status: hostStatus() }, global: { stubs } })
    await flushPromises()
    expect(wrapper.find('[data-testid="bar-chart"]').exists()).toBe(true)
    const periods = wrapper.get('[data-testid="host-detail-periods"]').text()
    expect(periods).toContain('09-24 → 10-24')
    expect(periods).toContain('1.01 TB')
    expect(periods).toContain('traffic.host.detail.bound.estimate')
    expect(wrapper.get('[data-testid="host-detail-calibrations"]').text()).toContain('+1.1%')
    expect(wrapper.get('[data-testid="host-detail-interfaces"]').text()).toContain('IPv4 / IPv6')
    expect(wrapper.get('[data-testid="host-detail-collection"]').text()).toContain('traffic.host.detail.sourceHostMount')
  })
})

describe('host traffic errors', () => {
  const t = (key: string) => key

  it('tells disabled ops monitoring apart from an old backend', () => {
    expect(hostErrorMessage({ status: 404, code: 'OPS_DISABLED' }, t)).toBe('traffic.host.errors.disabled')
    expect(hostErrorMessage({ status: 404, reason: 'OPS_DISABLED' }, t)).toBe('traffic.host.errors.disabled')
    expect(isHostUnsupported({ status: 404, reason: 'OPS_DISABLED' })).toBe(false)
    expect(hostErrorMessage({ status: 404 }, t)).toBe('traffic.host.errors.unsupported')
    expect(isHostUnsupported({ status: 404 })).toBe(true)
  })
})
