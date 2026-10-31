import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ImageUpdatePanel from '../ImageUpdatePanel.vue'
import { getImageUpdateStatus, startImageUpdate } from '@/api/admin/system'

vi.mock('@/api/admin/system', () => ({ getImageUpdateStatus: vi.fn(), startImageUpdate: vi.fn() }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key, te: () => true }) }))

describe('ImageUpdatePanel', () => {
  beforeEach(() => { vi.useFakeTimers(); vi.resetAllMocks() })
  afterEach(() => { vi.useRealTimers() })
  const props = { hasUpdate: true, latestVersion: '1.2.4', currentVersion: '1.2.3' }

  it('does not offer binary updates when the host updater is missing', async () => {
    vi.mocked(getImageUpdateStatus).mockResolvedValue({ enabled: false, phase: 'unconfigured' })
    const wrapper = mount(ImageUpdatePanel, { props })
    await flushPromises()
    expect(wrapper.find('button').exists()).toBe(false)
    expect(wrapper.text()).toContain('version.imagePhases.unconfigured')
    wrapper.unmount()
  })

  it('waits for durable verification, including across a disconnect', async () => {
    vi.mocked(getImageUpdateStatus).mockResolvedValue({ enabled: true, phase: 'idle' })
    vi.mocked(startImageUpdate).mockResolvedValue({ enabled: true, id: 'job', phase: 'queued' })
    const wrapper = mount(ImageUpdatePanel, { props })
    await flushPromises()
    vi.mocked(getImageUpdateStatus).mockRejectedValueOnce(new Error('offline'))
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.emitted('updated')).toBeUndefined()
    expect(wrapper.text()).toContain('version.imageReconnecting')
    vi.mocked(getImageUpdateStatus).mockResolvedValue({ enabled: true, id: 'job', phase: 'verifying', version: '1.2.4' })
    await vi.advanceTimersByTimeAsync(3000)
    expect(wrapper.emitted('updated')).toBeUndefined()
    vi.mocked(getImageUpdateStatus).mockResolvedValue({ enabled: true, id: 'job', phase: 'succeeded', version: '1.2.4' })
    await vi.advanceTimersByTimeAsync(3000)
    expect(wrapper.emitted('updated')).toHaveLength(1)
    wrapper.unmount()
  })

  it('does not refresh forever when reopening a completed update', async () => {
    vi.mocked(getImageUpdateStatus).mockResolvedValue({ enabled: true, id: 'job', phase: 'succeeded', version: '1.2.4' })
    const wrapper = mount(ImageUpdatePanel, { props: { ...props, currentVersion: '1.2.4', hasUpdate: false } })
    await flushPromises()
    expect(wrapper.emitted('updated')).toBeUndefined()
    wrapper.unmount()
  })

  it('blocks another update after a failure requiring inspection', async () => {
    vi.mocked(getImageUpdateStatus).mockResolvedValue({ enabled: true, phase: 'needs_attention', error: 'WG verification failed' })
    const wrapper = mount(ImageUpdatePanel, { props })
    await flushPromises()
    expect(wrapper.find('button').exists()).toBe(false)
    expect(wrapper.text()).toContain('WG verification failed')
    expect(wrapper.text()).toContain('version.imageRecovery')
    wrapper.unmount()
  })
})
