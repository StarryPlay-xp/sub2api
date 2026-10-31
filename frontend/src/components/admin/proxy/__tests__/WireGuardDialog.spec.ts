import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import WireGuardDialog from '../WireGuardDialog.vue'
import type { TrafficReport } from '@/api/admin/wireguard'

const api = vi.hoisted(() => ({
  capabilities: vi.fn(),
  details: vi.fn(),
  traffic: vi.fn(),
  save: vi.fn(),
  setPaused: vi.fn(),
  probe: vi.fn()
}))
vi.mock('@/api/admin/wireguard', () => api)
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('vue-chartjs', () => ({ Bar: { template: '<div data-testid="chart" />' } }))

const data = (paused = false) => ({
  node: { id: 7, name: 'Node', config_version: 2, paused, active: true, tx_bytes: 32, rx_bytes: 64 },
  rate: { state: 'running', tx_bytes_per_second: 1, rx_bytes_per_second: 2 },
  history: [],
  observations: []
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
  bucket_count: 1,
  incomplete: false,
  metric_basis: 'wg_udp_payload_v1',
  history: [{ start: '2026-10-31T05:00:00Z', tx_bytes: '1024', rx_bytes: '2048' }],
  ...overrides
})

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

function create() {
  return mount(WireGuardDialog, {
    props: { nodeId: 7 },
    global: { stubs: { BaseDialog: { template: '<div><slot /></div>' } } }
  })
}

function buttonByText(wrapper: ReturnType<typeof create>, text: string) {
  return wrapper.findAll('button').find((button) => button.text() === text)
}

beforeEach(() => {
  vi.useFakeTimers()
  vi.clearAllMocks()
  api.capabilities.mockResolvedValue({ key_configured: true })
  api.details.mockResolvedValue(data())
  api.traffic.mockResolvedValue(report())
  api.save.mockResolvedValue(data().node)
})
afterEach(() => {
  vi.useRealTimers()
})

describe('WireGuardDialog', () => {
  it('never prefills a configuration and clears it after a successful replacement', async () => {
    const wrapper = create()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()
    expect(wrapper.find('[data-testid="chart"]').exists()).toBe(true)
    const config = wrapper.get<HTMLTextAreaElement>('#wg-config')
    expect(config.element.value).toBe('')
    await config.setValue('synthetic private configuration')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(api.save).toHaveBeenCalledWith(
      7,
      expect.objectContaining({ config_version: 2, config: 'synthetic private configuration' })
    )
    expect(config.element.value).toBe('')
    wrapper.unmount()
  })

  it('disables probes on paused nodes and stops polling when closed', async () => {
    api.details.mockResolvedValue(data(true))
    const wrapper = create()
    await flushPromises()
    const probes = wrapper.findAll('button').filter((b) => /wireguard.probe[46]/.test(b.text()))
    expect(probes).toHaveLength(2)
    expect(probes.every((b) => b.attributes('disabled') !== undefined)).toBe(true)
    const signal = api.details.mock.calls[0][2] as AbortSignal
    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()
    expect(api.details).toHaveBeenCalledTimes(2)
    wrapper.unmount()
    expect(signal.aborted).toBe(true)
    await vi.advanceTimersByTimeAsync(6000)
    expect(api.details).toHaveBeenCalledTimes(2)
  })

  it('requires the deployment encryption key before saving', async () => {
    api.capabilities.mockResolvedValue({ key_configured: false })
    const wrapper = create()
    await flushPromises()
    expect(wrapper.text()).toContain('wireguard.keyMissing')
    expect(wrapper.get('form button').attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })

  it('loads 24h by default and sends 7d/30d preset params', async () => {
    const wrapper = create()
    await flushPromises()
    expect(api.traffic).toHaveBeenNthCalledWith(1, 7, { range: '24h' }, expect.any(AbortSignal))

    await buttonByText(wrapper, 'wireguard.range7d')!.trigger('click')
    await flushPromises()
    expect(api.traffic).toHaveBeenLastCalledWith(7, { range: '7d' }, expect.any(AbortSignal))

    await buttonByText(wrapper, 'wireguard.range30d')!.trigger('click')
    await flushPromises()
    expect(api.traffic).toHaveBeenLastCalledWith(7, { range: '30d' }, expect.any(AbortSignal))
    wrapper.unmount()
  })

  it('does not let a slower old range response overwrite the newest one', async () => {
    const first = deferred<TrafficReport>()
    const second = deferred<TrafficReport>()
    const third = deferred<TrafficReport>()
    api.traffic
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise)
      .mockReturnValueOnce(third.promise)

    const wrapper = create()
    await flushPromises()
    first.resolve(report())
    await flushPromises()

    await buttonByText(wrapper, 'wireguard.range7d')!.trigger('click')
    await flushPromises()
    await buttonByText(wrapper, 'wireguard.range30d')!.trigger('click')
    await flushPromises()

    third.resolve(
      report({ range: '30d', tx_bytes: '3221225472', rx_bytes: '0', total_bytes: '3221225472' })
    )
    await flushPromises()
    second.resolve(
      report({ range: '7d', tx_bytes: '1073741824', rx_bytes: '0', total_bytes: '1073741824' })
    )
    await flushPromises()

    expect(wrapper.text()).toContain('3 GiB')
    expect(wrapper.text()).not.toContain('1 GiB')
    wrapper.unmount()
  })

  it('maps retention errors and clears the previous period totals', async () => {
    const wrapper = create()
    await flushPromises()
    expect(wrapper.text()).toContain('512 MiB')

    api.traffic.mockRejectedValueOnce({
      status: 400,
      reason: 'TRAFFIC_RETENTION_EXCEEDED',
      metadata: { retained_from: '2026-08-02T06:00:00Z' }
    })
    await buttonByText(wrapper, 'wireguard.range90d')!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('wireguard.trafficRetentionExceeded')
    expect(wrapper.text()).not.toContain('512 MiB')
    wrapper.unmount()
  })

  it('sends inclusive custom dates from the panel', async () => {
    const wrapper = create()
    await flushPromises()

    await buttonByText(wrapper, 'wireguard.customRange')!.trigger('click')
    await flushPromises()
    const checkboxes = wrapper.findAll('input[type="checkbox"]')
    await checkboxes[0].setValue(false)
    const dates = wrapper.findAll('input[type="date"]')
    await dates[0].setValue('2026-09-30')
    await dates[1].setValue('2026-09-30')
    await buttonByText(wrapper, 'wireguard.customApply')!.trigger('click')
    await flushPromises()

    const call = api.traffic.mock.calls[api.traffic.mock.calls.length - 1]
    expect(call[1]).toMatchObject({ range: 'custom' })
    const params = call[1] as { start: string; end: string }
    expect(new Date(params.start).getTime()).toBe(new Date(2026, 8, 30).getTime())
    expect(new Date(params.end).getTime()).toBe(new Date(2026, 9, 1).getTime())
    wrapper.unmount()
  })
})
