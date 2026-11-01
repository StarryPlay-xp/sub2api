import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { saveAs } from 'file-saver'
import { getTrafficLive, getTrafficRanking, getTrafficSubject } from '@/api/admin/trafficSubjects'
import LiveSubjectsCard from '../LiveSubjectsCard.vue'
import SubjectDetailDialog from '../SubjectDetailDialog.vue'
import SubjectRankingCard from '../SubjectRankingCard.vue'
import { LIVE_RANKING_POLL_MS } from '../useTrafficLive'
import {
  detailData,
  keyIdentity,
  liveData,
  liveItem,
  rankingData,
  subjectPeak,
  subjectStats,
  subjectWindow,
  userIdentity
} from './fixtures'

vi.mock('@/api/admin/trafficSubjects', () => ({
  getTrafficRanking: vi.fn(),
  getTrafficSubject: vi.fn(),
  getTrafficLive: vi.fn()
}))

const app = { showError: vi.fn(), showInfo: vi.fn(), showSuccess: vi.fn() }
vi.mock('@/stores/app', () => ({ useAppStore: () => app }))
vi.mock('file-saver', () => ({ saveAs: vi.fn() }))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}(${Object.values(params).join(',')})` : key
  })
}))

vi.mock('vue-chartjs', () => ({
  Bar: { name: 'Bar', props: ['data', 'options'], template: '<div data-testid="bar-chart" />' }
}))

const stubs = {
  HelpTooltip: true,
  BaseDialog: { props: ['show', 'title'], emits: ['close'], template: '<div v-if="show" data-testid="dialog"><h3>{{ title }}</h3><slot /></div>' },
  RouterLink: { props: ['to'], template: '<a :data-to="JSON.stringify(to)"><slot /></a>' },
  Pagination: {
    props: ['total', 'page', 'pageSize'],
    emits: ['update:page'],
    template: '<button data-testid="next-page" @click="$emit(\'update:page\', page + 1)">next</button>'
  },
  SearchInput: {
    props: ['modelValue', 'placeholder'],
    emits: ['update:modelValue', 'search'],
    template: '<input data-testid="search" :placeholder="placeholder" @change="$emit(\'search\', $event.target.value)" />'
  }
}

const mounted: VueWrapper[] = []
function track<T extends VueWrapper>(wrapper: T): T {
  mounted.push(wrapper)
  return wrapper
}

function rankingCalls() {
  return vi.mocked(getTrafficRanking).mock.calls.map((call) => call[0])
}

beforeEach(() => {
  vi.mocked(getTrafficRanking).mockReset().mockResolvedValue(rankingData())
  vi.mocked(getTrafficSubject).mockReset().mockResolvedValue(detailData())
  vi.mocked(getTrafficLive).mockReset().mockResolvedValue(liveData())
  vi.mocked(saveAs).mockReset()
  app.showError.mockReset()
  app.showInfo.mockReset()
})

afterEach(() => {
  while (mounted.length) mounted.pop()!.unmount()
  vi.useRealTimers()
})

describe('SubjectRankingCard', () => {
  function create(props: Record<string, unknown> = {}) {
    return track(mount(SubjectRankingCard, { props: { mode: 'traffic', range: '24h', ...props }, global: { stubs } }))
  }

  it('loads the user traffic ranking and shows totals, shares and the site summary', async () => {
    const wrapper = create()
    await flushPromises()

    expect(rankingCalls()[0]).toEqual({
      range: '24h',
      subject: 'user',
      sort: 'total_bytes',
      order: 'desc',
      page: 1,
      page_size: 20,
      q: '',
      user_id: undefined,
      compare: false
    })
    const first = wrapper.get('[data-testid="ranking-row-user:7"]')
    expect(first.text()).toContain('user7@example.com')
    expect(first.text()).toContain('1.5 GiB')
    expect(first.text()).toContain('60%')
    expect(first.text()).toContain('traffic.subject.activeKeys(2)')
    expect(wrapper.get('[data-testid="ranking-row-user:8"]').text()).toContain('traffic.subject.userDeleted')
    expect(wrapper.get('[data-testid="ranking-row-user:8"]').find('[data-testid="cell-perRequest"]').text()).toBe('—')
    expect(wrapper.get('[data-testid="ranking-concentration"]').text()).toContain('traffic.ranking.top(1,60%)')
    expect(wrapper.get('[data-testid="ranking-unattributed"]').text()).toContain('100 MiB')
    expect(wrapper.get('[data-testid="ranking-unattributed"]').text()).toContain('6.1%')
    expect(wrapper.get('[data-testid="ranking-coverage"]').text()).toContain('traffic.ranking.coverageFull')
  })

  it('sorts on the server, toggling order on the active column', async () => {
    const wrapper = create()
    await flushPromises()

    await wrapper.get('[data-testid="sort-requests"]').trigger('click')
    await flushPromises()
    expect(rankingCalls().at(-1)).toMatchObject({ sort: 'requests', order: 'desc', page: 1 })

    await wrapper.get('[data-testid="sort-requests"]').trigger('click')
    await flushPromises()
    expect(rankingCalls().at(-1)).toMatchObject({ sort: 'requests', order: 'asc' })
    expect(wrapper.get('[data-testid="sort-requests"]').element.closest('th')?.getAttribute('aria-sort')).toBe('ascending')
  })

  it('returns to the first page with a single request when the subject changes', async () => {
    vi.mocked(getTrafficRanking).mockResolvedValue(rankingData({ total: 45 }))
    const wrapper = create()
    await flushPromises()
    await wrapper.get('[data-testid="next-page"]').trigger('click')
    await flushPromises()
    expect(rankingCalls().at(-1)).toMatchObject({ page: 2 })

    const before = rankingCalls().length
    await wrapper.get('[data-testid="subject-api_key"]').trigger('click')
    await flushPromises()
    expect(rankingCalls().slice(before)).toEqual([expect.objectContaining({ subject: 'api_key', page: 1 })])
  })

  it('adds the change column when comparing and falls back from change sorting when comparison is off', async () => {
    vi.mocked(getTrafficRanking).mockResolvedValue(
      rankingData({
        items: [subjectStats(userIdentity(7), { previous: { total_bytes: '1073741824', peaks: null }, growth_bytes: '536873984' })]
      })
    )
    const wrapper = create()
    await flushPromises()
    expect(wrapper.find('[data-testid="sort-growth"]').exists()).toBe(false)

    await wrapper.get('[data-testid="ranking-compare"] input').setValue(true)
    await flushPromises()
    expect(rankingCalls().at(-1)).toMatchObject({ compare: true })
    expect(wrapper.get('[data-testid="cell-growth"]').text()).toContain('+512 MiB')
    expect(wrapper.get('[data-testid="ranking-notices"]').text()).toContain('traffic.ranking.notices.partialHour')

    await wrapper.get('[data-testid="sort-growth"]').trigger('click')
    await flushPromises()
    expect(rankingCalls().at(-1)).toMatchObject({ sort: 'growth', compare: true })

    await wrapper.get('[data-testid="ranking-compare"] input').setValue(false)
    await flushPromises()
    expect(rankingCalls().at(-1)).toMatchObject({ sort: 'total_bytes', compare: false })
  })

  it('searches by the debounced query', async () => {
    const wrapper = create()
    await flushPromises()
    await wrapper.get('[data-testid="search"]').setValue('alice')
    await wrapper.get('[data-testid="search"]').trigger('change')
    await flushPromises()
    expect(rankingCalls().at(-1)).toMatchObject({ q: 'alice', page: 1 })
  })

  it('ranks bandwidth by peak and compares the busier direction with the host limit', async () => {
    vi.mocked(getTrafficRanking).mockResolvedValue(
      rankingData({
        items: [
          subjectStats(userIdentity(7), {
            // 出站 1,250,000 B/s = 10 Mbps，上限 20 Mbps 时占 50%
            peaks: { in: subjectPeak(625_000), out: subjectPeak(1_250_000), total: subjectPeak(1_875_000) }
          }),
          subjectStats(userIdentity(8), { peaks: null })
        ]
      })
    )
    const wrapper = create({ mode: 'bandwidth', limitMbps: 20 })
    await flushPromises()

    expect(rankingCalls()[0]).toMatchObject({ sort: 'peak_total' })
    const row = wrapper.get('[data-testid="ranking-row-user:7"]')
    expect(row.get('[data-testid="cell-peakTotal"]').text()).toContain('15 Mbps')
    expect(row.get('[data-testid="cell-usage"]').text()).toContain('50%')
    expect(wrapper.get('[data-testid="ranking-row-user:8"]').get('[data-testid="cell-peakTotal"]').text()).toBe(
      'traffic.ranking.peakUnknown'
    )
    expect(wrapper.find('[data-testid="ranking-compare"]').exists()).toBe(false)
  })

  it('asks for a host limit before showing usage', async () => {
    const wrapper = create({ mode: 'bandwidth' })
    await flushPromises()
    expect(wrapper.get('[data-testid="ranking-notices"]').text()).toContain('traffic.ranking.notices.noLimit')
    expect(wrapper.get('[data-testid="cell-usage"]').text()).toBe('—')
  })

  it('tells an old backend apart from disabled monitoring and stops polling', async () => {
    vi.useFakeTimers()
    vi.mocked(getTrafficRanking).mockRejectedValue({ status: 404 })
    const wrapper = create()
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toBe('traffic.errors.unsupported')
    await vi.advanceTimersByTimeAsync(60_000)
    expect(getTrafficRanking).toHaveBeenCalledTimes(1)

    vi.mocked(getTrafficRanking).mockRejectedValue({ status: 404, code: 'OPS_DISABLED' })
    const disabled = create()
    await flushPromises()
    expect(disabled.get('[role="alert"]').text()).toBe('traffic.errors.disabled')
  })

  it('lists one user’s keys when embedded in user details', async () => {
    vi.mocked(getTrafficRanking).mockResolvedValue(rankingData({ subject: 'api_key', items: [subjectStats(keyIdentity(9, 7))] }))
    const wrapper = create({ userId: 7, embedded: true })
    await flushPromises()

    expect(rankingCalls()[0]).toMatchObject({ subject: 'api_key', user_id: 7 })
    expect(wrapper.find('[data-testid="ranking-summary"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="ranking-export"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="ranking-row-api_key:9"]').text()).not.toContain('user7@example.com')
  })

  it('emits the selected subject from a row', async () => {
    const wrapper = create()
    await flushPromises()
    await wrapper.get('[data-testid="ranking-row-user:7"]').trigger('click')
    expect(wrapper.emitted('select')?.[0]?.[0]).toMatchObject({ subject_type: 'user', user_id: 7 })
  })

  it('exports the current ranking as CSV with a byte order mark', async () => {
    const wrapper = create()
    await flushPromises()
    await wrapper.get('[data-testid="ranking-export"]').trigger('click')
    await flushPromises()

    expect(rankingCalls().at(-1)).toMatchObject({ page: 1, page_size: 200, sort: 'total_bytes' })
    const [blob, name] = vi.mocked(saveAs).mock.calls[0] as [Blob, string]
    expect(name).toMatch(/^traffic-ranking_user_24h_\d{8}-\d{4}\.csv$/)
    // jsdom 的 Blob 没有 text()，按字节读取以确认开头是 UTF-8 字节序标记。
    const bytes = await new Promise<Uint8Array>((resolve) => {
      const reader = new FileReader()
      reader.onload = () => resolve(new Uint8Array(reader.result as ArrayBuffer))
      reader.readAsArrayBuffer(blob)
    })
    expect([...bytes.slice(0, 3)]).toEqual([0xef, 0xbb, 0xbf])
    expect(new TextDecoder().decode(bytes)).toContain('user7@example.com')
  })
})

describe('LiveSubjectsCard', () => {
  function create() {
    return track(mount(LiveSubjectsCard, { global: { stubs } }))
  }

  it('lists who is using bandwidth now and polls while visible', async () => {
    vi.useFakeTimers()
    const wrapper = create()
    await flushPromises()

    expect(getTrafficLive).toHaveBeenCalledWith({ subject: 'user', limit: 10, sort: 'avg_60s' }, expect.anything())
    expect(wrapper.get('[data-testid="live-status"]').text()).toBe('traffic.live.status.sampled')
    const first = wrapper.get('[data-testid="live-row-user:7"]').text()
    // 近 60 秒 450,000 B/s = 3.6 Mbps，占本机 1,350,000 B/s 的 33%
    expect(first).toContain('3.6 Mbps')
    expect(first).toContain('traffic.live.now(4.8 Mbps)')
    expect(first).toContain('traffic.live.inFlight(2)')
    expect(first).toContain('33%')
    expect(wrapper.get('[data-testid="live-unattributed"]').text()).toContain('0.01 Mbps')

    await vi.advanceTimersByTimeAsync(LIVE_RANKING_POLL_MS)
    expect(getTrafficLive).toHaveBeenCalledTimes(2)
  })

  it('switches subject and shows an empty state', async () => {
    vi.mocked(getTrafficLive).mockResolvedValue(liveData({ subject: 'api_key', items: [] }))
    const wrapper = create()
    await flushPromises()
    await wrapper.get('[data-testid="subject-api_key"]').trigger('click')
    await flushPromises()
    expect(vi.mocked(getTrafficLive).mock.calls.at(-1)?.[0]).toMatchObject({ subject: 'api_key' })
    expect(wrapper.get('[data-testid="live-empty"]').text()).toBe('traffic.live.emptyKey')
  })

  it('keeps the last list on transient failures, then marks it stale', async () => {
    vi.useFakeTimers()
    const wrapper = create()
    await flushPromises()
    vi.mocked(getTrafficLive).mockRejectedValue({ status: 500 })
    await vi.advanceTimersByTimeAsync(LIVE_RANKING_POLL_MS)
    expect(wrapper.find('[data-testid="live-row-user:7"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="live-status"]').text()).toBe('traffic.live.status.sampled')

    await vi.advanceTimersByTimeAsync(LIVE_RANKING_POLL_MS * 3)
    expect(wrapper.get('[data-testid="live-status"]').text()).toBe('traffic.live.status.stale')
  })

  it('stops polling when access is forbidden', async () => {
    vi.useFakeTimers()
    vi.mocked(getTrafficLive).mockRejectedValue({ status: 403 })
    const wrapper = create()
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toBe('traffic.errors.forbidden')
    await vi.advanceTimersByTimeAsync(LIVE_RANKING_POLL_MS * 5)
    expect(getTrafficLive).toHaveBeenCalledTimes(1)
  })

  it('emits the selected live subject', async () => {
    vi.mocked(getTrafficLive).mockResolvedValue(liveData({ items: [liveItem(keyIdentity(9, 7))] }))
    const wrapper = create()
    await flushPromises()
    await wrapper.get('[data-testid="live-row-api_key:9"]').trigger('click')
    expect(wrapper.emitted('select')?.[0]?.[0]).toMatchObject({ subject_type: 'api_key', api_key_id: 9 })
  })
})

describe('SubjectDetailDialog', () => {
  function create(props: Record<string, unknown> = {}) {
    return track(
      mount(SubjectDetailDialog, {
        props: { subject: userIdentity(7), range: '24h', mode: 'bandwidth', limitMbps: 20, ...props },
        global: { stubs }
      })
    )
  }

  it('loads the period summary with the previous period and lists the user’s keys', async () => {
    vi.mocked(getTrafficRanking).mockResolvedValue(rankingData({ subject: 'api_key', items: [subjectStats(keyIdentity(9, 7))] }))
    const wrapper = create()
    await flushPromises()

    expect(getTrafficSubject).toHaveBeenCalledWith('user', 7, { range: '24h', compare: true }, expect.anything())
    expect(wrapper.get('[data-testid="detail-total"]').text()).toContain('1.5 GiB')
    expect(wrapper.get('[data-testid="detail-share"]').text()).toContain('traffic.detail.growth(+512 MiB,+50%)')
    expect(wrapper.get('[data-testid="detail-peak"]').text()).toContain('25.18 Mbps')
    // 入站峰值 2,097,664 B/s ≈ 16.78 Mbps，占 20 Mbps 上限 84%
    expect(wrapper.get('[data-testid="detail-usage"]').text()).toContain('84%')
    expect(wrapper.get('[data-testid="detail-direction-upstream_in"]').text()).toContain('1 GiB')
    expect(wrapper.find('[data-testid="bar-chart"]').exists()).toBe(true)
    expect(rankingCalls()[0]).toMatchObject({ subject: 'api_key', user_id: 7 })

    const link = JSON.parse(wrapper.get('[data-testid="detail-usage-link"]').attributes('data-to')!)
    expect(link).toMatchObject({ path: '/admin/usage', query: { user_id: '7' } })
  })

  it('shows the period average instead of the limit on the traffic page', async () => {
    const wrapper = create({ mode: 'traffic', limitMbps: null })
    await flushPromises()
    expect(wrapper.find('[data-testid="detail-usage"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="detail-average"]').text()).toContain('Mbps')
  })

  it('opens a key from the user, returns, and opens the owner from a key', async () => {
    vi.mocked(getTrafficRanking).mockResolvedValue(rankingData({ subject: 'api_key', items: [subjectStats(keyIdentity(9, 7))] }))
    vi.mocked(getTrafficSubject).mockImplementation(async (type, id) =>
      detailData(type === 'user' ? userIdentity(id) : keyIdentity(id, 7))
    )
    const wrapper = create()
    await flushPromises()

    await wrapper.get('[data-testid="ranking-row-api_key:9"]').trigger('click')
    await flushPromises()
    expect(vi.mocked(getTrafficSubject).mock.calls.at(-1)?.slice(0, 2)).toEqual(['api_key', 9])
    expect(wrapper.find('[data-testid="detail-keys"]').exists()).toBe(false)

    await wrapper.get('[data-testid="detail-back"]').trigger('click')
    await flushPromises()
    expect(vi.mocked(getTrafficSubject).mock.calls.at(-1)?.slice(0, 2)).toEqual(['user', 7])
    expect(wrapper.find('[data-testid="detail-back"]').exists()).toBe(false)

    await wrapper.setProps({ subject: keyIdentity(9, 7) })
    await flushPromises()
    await wrapper.get('[data-testid="detail-owner"]').trigger('click')
    await flushPromises()
    expect(vi.mocked(getTrafficSubject).mock.calls.at(-1)?.slice(0, 2)).toEqual(['user', 7])
  })

  it('explains missing subjects and late counting', async () => {
    vi.mocked(getTrafficSubject).mockResolvedValueOnce(
      detailData(userIdentity(7), { window: subjectWindow({ attribution_started_at: '2026-11-01T01:00:00Z' }), history: [] })
    )
    const wrapper = create()
    await flushPromises()
    expect(wrapper.get('[data-testid="detail-window"]').text()).toContain('traffic.ranking.coverageFrom')
    expect(wrapper.get('[data-testid="subject-trend-empty"]').text()).toBe('traffic.detail.noHistory')

    vi.mocked(getTrafficSubject).mockRejectedValueOnce({ status: 404, reason: 'TRAFFIC_SUBJECT_NOT_FOUND' })
    await wrapper.setProps({ subject: userIdentity(99) })
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toBe('traffic.errors.notFound')
  })
})
