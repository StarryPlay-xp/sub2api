import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import WireGuardDialog from '../WireGuardDialog.vue'

const api = vi.hoisted(() => ({ capabilities: vi.fn(), details: vi.fn(), save: vi.fn(), setPaused: vi.fn(), probe: vi.fn() }))
vi.mock('@/api/admin/wireguard', () => api)
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('vue-chartjs', () => ({ Line: { template: '<div />' } }))
const data = (paused = false) => ({ node: { id: 7, name: 'Node', config_version: 2, paused, active: true, tx_bytes: 32, rx_bytes: 64 }, rate: { state: 'running', tx_bytes_per_second: 1, rx_bytes_per_second: 2 }, history: [], observations: [] })
function create() { return mount(WireGuardDialog, { props: { nodeId: 7 }, global: { stubs: { BaseDialog: { template: '<div><slot /></div>' } } } }) }
beforeEach(() => { vi.useFakeTimers(); vi.clearAllMocks(); api.capabilities.mockResolvedValue({ key_configured: true }); api.details.mockResolvedValue(data()); api.save.mockResolvedValue(data().node) })
afterEach(() => { vi.useRealTimers() })
describe('WireGuardDialog', () => {
  it('never prefills a configuration and clears it after a successful replacement', async () => {
    const wrapper = create(); await flushPromises()
    const config = wrapper.get<HTMLTextAreaElement>('#wg-config')
    expect(config.element.value).toBe('')
    await config.setValue('synthetic private configuration')
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(api.save).toHaveBeenCalledWith(7, expect.objectContaining({ config_version: 2, config: 'synthetic private configuration' }))
    expect(config.element.value).toBe('')
    wrapper.unmount()
  })
  it('disables probes on paused nodes and stops polling when closed', async () => {
    api.details.mockResolvedValue(data(true))
    const wrapper = create(); await flushPromises()
    const probes = wrapper.findAll('button').filter(b => /wireguard.probe[46]/.test(b.text()))
    expect(probes).toHaveLength(2)
    expect(probes.every(b => b.attributes('disabled') !== undefined)).toBe(true)
    const signal = api.details.mock.calls[0][2] as AbortSignal
    await vi.advanceTimersByTimeAsync(2000); await flushPromises()
    expect(api.details).toHaveBeenCalledTimes(2)
    wrapper.unmount()
    expect(signal.aborted).toBe(true)
    await vi.advanceTimersByTimeAsync(6000)
    expect(api.details).toHaveBeenCalledTimes(2)
  })
  it('requires the deployment encryption key before saving', async () => {
    api.capabilities.mockResolvedValue({ key_configured: false })
    const wrapper = create(); await flushPromises()
    expect(wrapper.text()).toContain('wireguard.keyMissing')
    expect(wrapper.get('form button').attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })
})
