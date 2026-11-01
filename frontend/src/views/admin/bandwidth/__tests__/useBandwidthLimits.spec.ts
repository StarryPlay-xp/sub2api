import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { getBandwidthLimits, saveBandwidthLimit, type BandwidthLimits } from '@/api/admin/bandwidth'
import { LIMITS_POLL_MS, normalizeLimit, useBandwidthLimits } from '../useBandwidthLimits'

const notices = vi.hoisted(() => ({ showError: vi.fn(), showSuccess: vi.fn(), showInfo: vi.fn() }))
vi.mock('@/stores/app', () => ({ useAppStore: () => notices }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/api/admin/bandwidth', () => ({ getBandwidthLimits: vi.fn(), saveBandwidthLimit: vi.fn() }))
const get = vi.mocked(getBandwidthLimits)
const save = vi.mocked(saveBandwidthLimit)
const wrappers: VueWrapper[] = []
let server: BandwidthLimits
let revision: number

function device() {
  let limits!: ReturnType<typeof useBandwidthLimits>
  const wrapper = mount(defineComponent({ setup() { limits = useBandwidthLimits(); return () => null } }))
  wrappers.push(wrapper)
  return { limits, wrapper }
}
function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((r) => { resolve = r })
  return { promise, resolve }
}

beforeEach(() => {
  vi.useFakeTimers()
  vi.resetAllMocks()
  localStorage.clear()
  revision = 1
  server = { host: { mbps: 100, version: 'r1' }, nodes: { '11': { mbps: 50, version: 'r1' } } }
  get.mockImplementation(async () => JSON.parse(JSON.stringify(server)))
  save.mockImplementation(async (id, input) => {
    const current = id === null ? server.host : server.nodes[String(id)]
    if (input.version !== (current?.version ?? '')) throw { status: 409 }
    const next = { mbps: input.mbps, version: `r${++revision}` }
    if (id === null) server.host = next
    else server.nodes[String(id)] = next
    return next
  })
})
afterEach(() => {
  for (const wrapper of wrappers.splice(0)) wrapper.unmount()
  vi.useRealTimers()
})

describe('shared bandwidth limits', () => {
  it('accepts only positive finite limits', () => {
    expect(normalizeLimit('100')).toBe(100)
    expect(normalizeLimit(0.5)).toBe(0.5)
    for (const value of [0, -1, 'abc', Infinity, 1_000_001]) expect(normalizeLimit(value)).toBeNull()
  })

  it('reads the server without importing stale browser values', async () => {
    localStorage.setItem('sub2api:bandwidth-limits:v1', JSON.stringify({ host: 999, nodes: { '11': 999 } }))
    const { limits } = device()
    await flushPromises()
    expect(limits.hostLimit.value).toBe(100)
    expect(limits.nodeLimit(11)).toBe(50)
    expect(limits.nodeLimit(12)).toBeNull()
    expect(save).not.toHaveBeenCalled()
  })

  it('shares saves and clears across devices through refresh and polling', async () => {
    const a = device().limits
    const b = device().limits
    await flushPromises()
    await a.setHostLimit(200)
    expect(save).toHaveBeenCalledWith(null, { mbps: 200, version: 'r1' }, expect.any(AbortSignal))
    expect(a.hostLimit.value).toBe(200)
    expect(b.hostLimit.value).toBe(100)
    await vi.advanceTimersByTimeAsync(LIMITS_POLL_MS)
    expect(b.hostLimit.value).toBe(200)
    await b.setNodeLimit(11, null)
    await a.refresh()
    expect(a.nodeLimit(11)).toBeNull()
    expect(a.hostLimit.value).toBe(200)
    await a.setHostLimit(null)
    await b.refresh()
    expect(b.hostLimit.value).toBeNull()
    expect(server.host.version).not.toBe('')
    expect(localStorage.getItem('sub2api:bandwidth-limits:v1')).toBeNull()
  })

  it('rejects stale device saves and reloads the winning value', async () => {
    const a = device().limits
    const b = device().limits
    await flushPromises()
    await a.setHostLimit(200)
    await b.setHostLimit(300)
    expect(server.host.mbps).toBe(200)
    expect(b.hostLimit.value).toBe(200)
    expect(notices.showError).toHaveBeenCalledWith('bandwidth.limit.conflict')
  })

  it('does not display an unconfirmed save or allow a duplicate pending save', async () => {
    const { limits } = device()
    await flushPromises()
    const waiting = deferred<{ mbps: number; version: string }>()
    save.mockReturnValueOnce(waiting.promise)
    const saving = limits.setHostLimit(200)
    expect(limits.hostLimit.value).toBe(100)
    await limits.setHostLimit(300)
    expect(save).toHaveBeenCalledTimes(1)
    expect(notices.showInfo).toHaveBeenCalledWith('bandwidth.limit.saving')
    waiting.resolve({ mbps: 200, version: 'r2' })
    await saving
    expect(limits.hostLimit.value).toBe(200)
  })

  it('keeps confirmed values on failure and never reports success', async () => {
    const { limits } = device()
    await flushPromises()
    save.mockRejectedValueOnce({ status: 500 })
    await limits.setHostLimit(200)
    expect(limits.hostLimit.value).toBe(100)
    expect(notices.showSuccess).not.toHaveBeenCalled()
    expect(notices.showError).toHaveBeenCalledWith('bandwidth.limit.saveFailed')
  })

  it('does not save without a successful initial read', async () => {
    get.mockRejectedValue({ status: 500 })
    const { limits } = device()
    await flushPromises()
    await limits.setHostLimit(200)
    expect(save).not.toHaveBeenCalled()
    expect(notices.showError).toHaveBeenCalledTimes(1)
  })

  it('ignores an older in-flight read after a save', async () => {
    const { limits } = device()
    await flushPromises()
    const read = deferred<BandwidthLimits>()
    get.mockReturnValueOnce(read.promise)
    const refreshing = limits.refresh()
    await limits.setHostLimit(200)
    read.resolve({ host: { mbps: 1, version: 'old' }, nodes: {} })
    await refreshing
    expect(limits.hostLimit.value).toBe(200)
  })

  it('refreshes on focus and stops work after leaving the page', async () => {
    const { limits, wrapper } = device()
    await flushPromises()
    server.host = { mbps: 300, version: 'external' }
    window.dispatchEvent(new Event('focus'))
    await flushPromises()
    expect(limits.hostLimit.value).toBe(300)
    const callCount = get.mock.calls.length
    wrapper.unmount()
    window.dispatchEvent(new Event('focus'))
    await vi.advanceTimersByTimeAsync(LIMITS_POLL_MS * 2)
    expect(get).toHaveBeenCalledTimes(callCount)
  })
})
