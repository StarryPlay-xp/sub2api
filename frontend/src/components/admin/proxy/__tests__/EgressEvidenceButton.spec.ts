import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import EgressEvidenceButton from '../EgressEvidenceButton.vue'
import type { EgressSummary } from '@/api/admin/wireguard'

const api = vi.hoisted(() => ({ attempts: vi.fn() }))
vi.mock('@/api/admin/wireguard', () => api)
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
const summary: EgressSummary = { request_id: 'req', account_id: 7, ips: ['203.0.113.9'], recorded: true, unobserved: false, expired: false }
function create(value = summary) {
  return mount(EgressEvidenceButton, { props: { requestId: 'req', accountId: 7, showSummary: true, summary: value }, global: { stubs: { BaseDialog: { props: ['show', 'title'], template: '<div v-if="show"><h3>{{ title }}</h3><slot /></div>' } } } })
}
describe('exit IP display', () => {
  it('shows the historical IP directly without fetching details', () => {
    api.attempts.mockClear()
    const wrapper = create()
    expect(wrapper.get('button').text()).toBe('203.0.113.9')
    expect(api.attempts).not.toHaveBeenCalled()
    wrapper.unmount()
  })
  it('distinguishes missing, unobserved, expired and partial observations', async () => {
    const wrapper = create({ ...summary, ips: [], recorded: false })
    expect(wrapper.get('button').text()).toBe('wireguard.exitIPMissing')
    await wrapper.setProps({ summary: { ...summary, ips: [], unobserved: true } })
    expect(wrapper.get('button').text()).toBe('wireguard.exitIPUnobserved')
    await wrapper.setProps({ summary: { ...summary, ips: [], expired: true } })
    expect(wrapper.get('button').text()).toBe('wireguard.exitIPExpired')
    await wrapper.setProps({ summary: { ...summary, ips: ['203.0.113.9', '2001:db8::1'], unobserved: true } })
    expect(wrapper.get('button').text()).toContain('203.0.113.9')
    expect(wrapper.get('button').text()).toContain('2001:db8::1')
    expect(wrapper.get('button').text()).toContain('wireguard.exitIPPartial')
    wrapper.unmount()
  })
  it('opens IP details scoped to the usage account', async () => {
    const record = { attempt_id: 'one', account_id: 7, started_at: '', outcome: 'complete', snapshot: { route: 'wireguard', family: 'ipv4', evidence: { ip: '203.0.113.9', state: 'observed' } } }
    api.attempts.mockResolvedValue([record, { ...record, attempt_id: 'two', account_id: 8, snapshot: { ...record.snapshot, evidence: { ip: '203.0.113.10' } } }])
    const wrapper = create()
    await wrapper.get('button').trigger('click'); await flushPromises()
    expect(wrapper.get('h3').text()).toBe('wireguard.exitIPDetails')
    expect(wrapper.text()).not.toContain('203.0.113.10')
    wrapper.unmount()
  })
})
