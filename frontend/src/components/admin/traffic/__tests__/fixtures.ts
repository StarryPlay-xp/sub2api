// 按接口约定构造的模拟数据：单元测试和本地预览共用，后端交付前用于开发排行、详情与实时占用。
import type {
  RateTriple,
  TrafficLiveData,
  TrafficLiveItem,
  TrafficRankingData,
  TrafficRankingParams,
  TrafficRankingSort,
  TrafficSubjectBucket,
  TrafficSubjectDetailData,
  TrafficSubjectIdentity,
  TrafficSubjectStats,
  TrafficSubjectType,
  TrafficSubjectWindow
} from '@/api/admin/trafficSubjects'
import type { ServiceTrafficBytes } from '@/api/admin/ops'

export const AS_OF = '2026-11-01T03:00:00Z'
export const PEAK_AT = '2026-11-01T02:03:04Z'

export const subjectPeak = (bytesPerSecond: number, at = PEAK_AT) => ({
  bytes_per_second: bytesPerSecond,
  at,
  window_seconds: 1
})

export function subjectWindow(overrides: Partial<TrafficSubjectWindow> = {}): TrafficSubjectWindow {
  return {
    range: '24h',
    interval: 'hour',
    start: '2026-10-31T03:00:00Z',
    end: AS_OF,
    requested_start: '2026-10-31T02:59:30Z',
    requested_end: AS_OF,
    retained_from: '2026-08-03T03:00:00Z',
    as_of: AS_OF,
    end_clamped: false,
    metric_basis: 'service_payload_v1',
    attribution_started_at: '2026-10-20T00:00:00Z',
    has_records: true,
    incomplete: false,
    concurrent_runtimes: false,
    subject_overflow: false,
    ...overrides
  }
}

export function bytesOf(clientIn: number, clientOut: number, upstreamOut: number, upstreamIn: number): ServiceTrafficBytes {
  return {
    client_in: String(clientIn),
    client_out: String(clientOut),
    upstream_out: String(upstreamOut),
    upstream_in: String(upstreamIn)
  }
}

export function userIdentity(id: number, overrides: Partial<TrafficSubjectIdentity> = {}): TrafficSubjectIdentity {
  return {
    subject_type: 'user',
    user_id: id,
    user_email: `user${id}@example.com`,
    username: `user${id}`,
    user_deleted: false,
    api_key_id: null,
    api_key_name: null,
    api_key_deleted: null,
    group_id: null,
    group_name: null,
    ...overrides
  }
}

export function keyIdentity(
  id: number,
  userId: number,
  overrides: Partial<TrafficSubjectIdentity> = {}
): TrafficSubjectIdentity {
  return {
    ...userIdentity(userId),
    subject_type: 'api_key',
    api_key_id: id,
    api_key_name: `key-${id}`,
    api_key_deleted: false,
    group_id: 1,
    group_name: 'Claude',
    ...overrides
  }
}

/** 由四方向字节推出汇总字段；默认入站 1 GiB、出站 512 MiB。 */
export function subjectStats(
  identity: TrafficSubjectIdentity,
  overrides: Partial<TrafficSubjectStats> = {}
): TrafficSubjectStats {
  const bytes = overrides.bytes ?? bytesOf(1024, 536870912, 2048, 1073741824)
  const inBytes = BigInt(bytes.client_in) + BigInt(bytes.upstream_in)
  const outBytes = BigInt(bytes.client_out) + BigInt(bytes.upstream_out)
  const total = inBytes + outBytes
  const requests = overrides.requests ?? 120
  return {
    ...identity,
    bytes,
    in_bytes: String(inBytes),
    out_bytes: String(outBytes),
    total_bytes: String(total),
    share: 0.25,
    requests,
    bytes_per_request: requests > 0 ? String(total / BigInt(requests)) : null,
    active_seconds: 3600,
    active_avg_bytes_per_second: Number(total) / 3600,
    peaks: { in: subjectPeak(2097664), out: subjectPeak(1049600, '2026-11-01T01:00:00Z'), total: subjectPeak(3147264) },
    active_api_keys: identity.subject_type === 'user' ? 2 : null,
    previous: null,
    growth_bytes: null,
    ...overrides
  }
}

export function rankingData(overrides: Partial<TrafficRankingData> = {}): TrafficRankingData {
  const items = overrides.items ?? [
    subjectStats(userIdentity(7), { share: 0.6 }),
    subjectStats(userIdentity(8, { user_deleted: true }), {
      share: 0.3,
      bytes: bytesOf(100, 200, 300, 400),
      requests: 0,
      peaks: null
    })
  ]
  return {
    window: subjectWindow(),
    previous_window: null,
    subject: 'user',
    sort: 'total_bytes',
    order: 'desc',
    page: 1,
    page_size: 20,
    total: items.length,
    items,
    totals: {
      attributed: { bytes: bytesOf(1024, 536870912, 2048, 1073741824), total_bytes: '1610615808', requests: 120 },
      unattributed: { bytes: bytesOf(10, 20, 30, 40), total_bytes: '104857600' },
      all_total_bytes: '1715473408'
    },
    concentration: { subjects: items.length, top1: 0.6, top5: 0.9, top10: 0.9 },
    ...overrides
  }
}

export function detailBuckets(count = 3, start = '2026-11-01T00:00:00Z'): TrafficSubjectBucket[] {
  const base = Date.parse(start)
  return Array.from({ length: count }, (_, index) => ({
    start: new Date(base + index * 3_600_000).toISOString(),
    interval: 'hour' as const,
    bytes: bytesOf(1000 * (index + 1), 50_000 * (index + 1), 2000 * (index + 1), 60_000 * (index + 1)),
    requests: 10 * (index + 1),
    active_seconds: 300,
    peaks: index === 1 ? null : { in: subjectPeak(80_000), out: subjectPeak(60_000), total: subjectPeak(120_000) }
  }))
}

export function detailData(
  identity: TrafficSubjectIdentity = userIdentity(7),
  overrides: Partial<TrafficSubjectDetailData> = {}
): TrafficSubjectDetailData {
  return {
    window: subjectWindow(),
    previous_window: { start: '2026-10-30T03:00:00Z', end: '2026-10-31T03:00:00Z', complete: true },
    summary: subjectStats(identity, {
      previous: { total_bytes: '1073741824', peaks: null },
      growth_bytes: '536873984'
    }),
    history: detailBuckets(),
    ...overrides
  }
}

export const rate = (inbound: number, outbound: number): RateTriple => ({
  in: inbound,
  out: outbound,
  total: inbound + outbound
})

export function liveItem(identity: TrafficSubjectIdentity, overrides: Partial<TrafficLiveItem> = {}): TrafficLiveItem {
  return {
    ...identity,
    rate: rate(400_000, 200_000),
    avg_60s: rate(300_000, 150_000),
    avg_60s_seconds: 60,
    in_flight: 2,
    last_active_at: '2026-11-01T02:59:59Z',
    ...overrides
  }
}

export function liveData(overrides: Partial<TrafficLiveData> = {}): TrafficLiveData {
  return {
    subject: 'user',
    state: 'sampled',
    sampled_at: '2026-11-01T02:59:59Z',
    window_seconds: 1,
    host: { rate: rate(1_000_000, 500_000), avg_60s: rate(900_000, 450_000) },
    unattributed: { rate: rate(1_000, 500), avg_60s: rate(800, 400) },
    items: [liveItem(userIdentity(7)), liveItem(userIdentity(8), { avg_60s: rate(50_000, 25_000), in_flight: 0 })],
    ...overrides
  }
}

// ---- 本地预览用的合成数据集：按参数筛选、排序、分页，行为与接口约定一致 ----

interface MockKey {
  identity: TrafficSubjectIdentity
  weight: number
  burst: number
}

const MOCK_USERS: Array<[number, string, string, boolean]> = [
  [101, 'alice@studio.dev', 'alice', false],
  [102, 'bob@imagelab.io', 'bob', false],
  [103, 'carol@agency.cn', 'carol', false],
  [104, 'dave@example.org', 'dave', false],
  [105, 'erin@ops.team', 'erin', false],
  [106, 'frank@legacy.net', 'frank', true],
  [107, 'grace@research.ai', 'grace', false],
  [108, '', 'henry_cli', false]
]

function mockKeys(): MockKey[] {
  const groups = ['Claude', 'OpenAI', 'Gemini', 'Grok']
  const keys: MockKey[] = []
  let keyId = 500
  MOCK_USERS.forEach(([userId, email, username, deleted], userIndex) => {
    const count = 1 + ((userIndex * 7) % 4)
    for (let index = 0; index < count; index++) {
      keyId++
      keys.push({
        identity: keyIdentity(keyId, userId, {
          user_email: email,
          username,
          user_deleted: deleted,
          api_key_name: index === 0 ? `${username}-main` : `${username}-${['ci', 'batch', 'mobile'][index - 1]}`,
          api_key_deleted: deleted || (userIndex === 3 && index === 1),
          group_id: (userIndex + index) % groups.length,
          group_name: groups[(userIndex + index) % groups.length]
        }),
        weight: Math.round(4_000_000_000 / (1 + userIndex * 1.7 + index * 2.3)),
        burst: 0.6 + ((userIndex + index) % 5) * 0.35
      })
    }
  })
  return keys
}

function sumBytes(list: ServiceTrafficBytes[]): ServiceTrafficBytes {
  const total = { client_in: 0n, client_out: 0n, upstream_out: 0n, upstream_in: 0n }
  for (const item of list) {
    for (const key of Object.keys(total) as (keyof typeof total)[]) total[key] += BigInt(item[key])
  }
  return {
    client_in: String(total.client_in),
    client_out: String(total.client_out),
    upstream_out: String(total.upstream_out),
    upstream_in: String(total.upstream_in)
  }
}

function hoursOf(range: string) {
  return range === '7d' ? 168 : range === '30d' ? 720 : range === '90d' ? 2160 : 24
}

function mockWindow(range: TrafficRankingParams['range'], now: number): TrafficSubjectWindow {
  const end = new Date(now).toISOString()
  const start = new Date(Math.ceil((now - hoursOf(range) * 3_600_000) / 3_600_000) * 3_600_000).toISOString()
  return subjectWindow({
    range,
    start,
    end,
    requested_start: start,
    requested_end: end,
    as_of: end,
    // 模拟升级后第 3 天才开始有分维度数据：30/90 天范围会提示统计开始时间。
    attribution_started_at: new Date(now - 12 * 86_400_000).toISOString()
  })
}

function keyStats(key: MockKey, hours: number, factor: number, now: number): TrafficSubjectStats {
  const scale = Math.min(hours, 12 * 24) / 24
  const volume = Math.round(key.weight * scale * factor)
  const bytes = bytesOf(
    Math.round(volume * 0.06),
    Math.round(volume * 0.42),
    Math.round(volume * 0.07),
    Math.round(volume * 0.45)
  )
  const requests = Math.max(1, Math.round((volume / 380_000) * (key.burst > 1.2 ? 0.35 : 1)))
  const peakTotal = (key.weight / 12_000) * key.burst
  const at = new Date(now - ((key.identity.api_key_id ?? 0) % 17) * 3_600_000 - 1234_000).toISOString()
  return subjectStats(key.identity, {
    bytes,
    requests,
    active_seconds: Math.round(volume / (peakTotal * 0.35)),
    active_avg_bytes_per_second: peakTotal * 0.35,
    peaks: {
      in: subjectPeak(peakTotal * 0.55, at),
      out: subjectPeak(peakTotal * 0.5, at),
      total: subjectPeak(peakTotal, at)
    },
    active_api_keys: null
  })
}

function userStats(list: TrafficSubjectStats[]): TrafficSubjectStats {
  const first = list[0]
  const bytes = sumBytes(list.map((item) => item.bytes))
  const requests = list.reduce((sum, item) => sum + item.requests, 0)
  const peakOf = (key: 'in' | 'out' | 'total') => {
    const values = list.map((item) => item.peaks?.[key]).filter(Boolean) as { bytes_per_second: number; at: string }[]
    // 用户峰值按同一秒合计：模拟数据取最大 Key 峰值加其余 Key 峰值的一部分。
    const sorted = values.sort((a, b) => b.bytes_per_second - a.bytes_per_second)
    const value = sorted.reduce((sum, item, index) => sum + item.bytes_per_second * (index === 0 ? 1 : 0.3), 0)
    return subjectPeak(value, sorted[0].at)
  }
  const activeSeconds = Math.max(...list.map((item) => item.active_seconds))
  const stats = subjectStats(
    userIdentity(first.user_id, { user_email: first.user_email, username: first.username, user_deleted: first.user_deleted }),
    {
      bytes,
      requests,
      active_seconds: activeSeconds,
      peaks: { in: peakOf('in'), out: peakOf('out'), total: peakOf('total') },
      active_api_keys: list.length
    }
  )
  stats.active_avg_bytes_per_second = Number(stats.total_bytes) / activeSeconds
  return stats
}

const SORT_VALUE: Record<TrafficRankingSort, (item: TrafficSubjectStats) => number | null> = {
  total_bytes: (item) => Number(item.total_bytes),
  in_bytes: (item) => Number(item.in_bytes),
  out_bytes: (item) => Number(item.out_bytes),
  requests: (item) => item.requests,
  bytes_per_request: (item) => (item.bytes_per_request === null ? null : Number(item.bytes_per_request)),
  peak_total: (item) => item.peaks?.total?.bytes_per_second ?? null,
  peak_in: (item) => item.peaks?.in?.bytes_per_second ?? null,
  peak_out: (item) => item.peaks?.out?.bytes_per_second ?? null,
  active_avg_rate: (item) => item.active_avg_bytes_per_second,
  growth: (item) => (item.growth_bytes === null ? null : Number(item.growth_bytes))
}

function idOf(item: TrafficSubjectIdentity) {
  return item.subject_type === 'user' ? item.user_id : (item.api_key_id ?? 0)
}

function withPrevious(current: TrafficSubjectStats, previous: TrafficSubjectStats | undefined): TrafficSubjectStats {
  const prior = previous ? BigInt(previous.total_bytes) : 0n
  return {
    ...current,
    previous: { total_bytes: String(prior), peaks: previous?.peaks ?? null },
    growth_bytes: String(BigInt(current.total_bytes) - prior)
  }
}

function subjectsFor(type: TrafficSubjectType, hours: number, factor: number, now: number, userId?: number) {
  const keys = mockKeys()
  // 某个 Key 只在当前期间出现，另一个只在上一时段出现，用于展示“新增”和降幅；先剔除再按用户筛选。
  const active = keys
    .filter((key, index) => (factor < 1 ? index !== 2 : index !== keys.length - 1))
    .filter((key) => !userId || key.identity.user_id === userId)
  const keyList = active.map((key) => keyStats(key, hours, factor * (1 + (key.burst - 1) * (factor < 1 ? -0.3 : 0.2)), now))
  if (type === 'api_key') return keyList
  const byUser = new Map<number, TrafficSubjectStats[]>()
  for (const item of keyList) byUser.set(item.user_id, [...(byUser.get(item.user_id) ?? []), item])
  return [...byUser.values()].map(userStats)
}

/** 依据合成数据回答排行查询，排序规则与约定一致：null 排最后，同值按 id 升序。 */
export function mockRanking(params: TrafficRankingParams, now = Date.now()): TrafficRankingData {
  const hours = hoursOf(params.range)
  const userId = params.subject === 'api_key' ? params.user_id : undefined
  const current = subjectsFor(params.subject, hours, 1, now, userId)
  const previousList = params.compare ? subjectsFor(params.subject, hours, 0.8, now, userId) : []
  const previousById = new Map(previousList.map((item) => [idOf(item), item]))
  let items = params.compare ? current.map((item) => withPrevious(item, previousById.get(idOf(item)))) : current
  if (params.compare) {
    for (const item of previousList) {
      if (!items.some((existing) => idOf(existing) === idOf(item))) {
        const zero = subjectStats(item, { bytes: bytesOf(0, 0, 0, 0), requests: 0, active_seconds: 0, peaks: null, active_avg_bytes_per_second: null })
        items.push(withPrevious(zero, item))
      }
    }
  }
  const unattributed = String(Math.round(180_000_000 * Math.min(hours, 288) / 24))
  const allTotal = current.reduce((sum, item) => sum + BigInt(item.total_bytes), 0n) + BigInt(unattributed)
  items = items.map((item) => ({ ...item, share: Number(item.total_bytes) / Number(allTotal) }))

  const byTotal = [...current].sort((a, b) => Number(b.total_bytes) - Number(a.total_bytes))
  const scopeTotal = Number(allTotal)
  const topShare = (count: number) =>
    byTotal.length ? byTotal.slice(0, count).reduce((sum, item) => sum + Number(item.total_bytes), 0) / scopeTotal : null

  const q = params.q?.trim().toLowerCase()
  if (q) {
    items = items.filter((item) =>
      [item.user_email, item.username, params.subject === 'api_key' ? item.api_key_name : '']
        .some((value) => value?.toLowerCase().includes(q))
    )
  }
  const sort = params.sort ?? 'total_bytes'
  const direction = params.order === 'asc' ? 1 : -1
  items.sort((a, b) => {
    const left = SORT_VALUE[sort](a)
    const right = SORT_VALUE[sort](b)
    if (left === null && right === null) return idOf(a) - idOf(b)
    if (left === null) return 1
    if (right === null) return -1
    return left === right ? idOf(a) - idOf(b) : (left - right) * direction
  })
  const page = params.page ?? 1
  const size = params.page_size ?? 20
  const window = mockWindow(params.range, now)
  const previousStart = new Date(Date.parse(window.start) - (Date.parse(window.end) - Date.parse(window.start))).toISOString()
  return {
    window,
    previous_window: params.compare
      ? { start: previousStart, end: window.start, complete: Date.parse(previousStart) >= Date.parse(window.attribution_started_at ?? '') }
      : null,
    subject: params.subject,
    sort,
    order: params.order ?? 'desc',
    page,
    page_size: size,
    total: items.length,
    items: items.slice((page - 1) * size, page * size),
    totals: {
      attributed: {
        bytes: sumBytes(current.map((item) => item.bytes)),
        total_bytes: String(allTotal - BigInt(unattributed)),
        requests: current.reduce((sum, item) => sum + item.requests, 0)
      },
      unattributed: { bytes: bytesOf(0, Number(unattributed), 0, 0), total_bytes: unattributed },
      all_total_bytes: String(allTotal)
    },
    concentration: { subjects: byTotal.length, top1: topShare(1), top5: topShare(5), top10: topShare(10) }
  }
}

export function mockDetail(
  type: TrafficSubjectType,
  id: number,
  range: TrafficRankingParams['range'],
  now = Date.now()
): TrafficSubjectDetailData {
  const ranking = mockRanking({ range, subject: type, compare: true, page_size: 500 }, now)
  const summary = ranking.items.find((item) => idOf(item) === id)
  if (!summary) throw { status: 404, reason: 'TRAFFIC_SUBJECT_NOT_FOUND' }
  const window = ranking.window
  const start = Date.parse(window.start)
  const hours = Math.ceil((Date.parse(window.end) - start) / 3_600_000)
  const attributionStart = Date.parse(window.attribution_started_at ?? window.start)
  const totalPerHour = Number(summary.total_bytes) / Math.max(1, hours)
  const history: TrafficSubjectBucket[] = []
  for (let index = 0; index < hours; index++) {
    const bucketStart = start + index * 3_600_000
    if (bucketStart < attributionStart) continue
    const hour = new Date(bucketStart).getUTCHours()
    // 白天繁忙、夜间清闲，偶有空闲小时不产生记录。
    const daily = 0.25 + Math.max(0, Math.sin(((hour - 2) / 24) * Math.PI * 2)) * 1.5
    const jitter = 0.7 + ((index * 37) % 11) / 18
    if ((index * 13 + id) % 23 === 0) continue
    const volume = totalPerHour * daily * jitter
    const peakTotal = (summary.peaks?.total?.bytes_per_second ?? 0) * Math.min(1, 0.35 + daily * jitter * 0.4)
    const at = new Date(bucketStart + ((index * 997) % 3600) * 1000).toISOString()
    history.push({
      start: new Date(bucketStart).toISOString(),
      interval: 'hour',
      bytes: bytesOf(Math.round(volume * 0.06), Math.round(volume * 0.42), Math.round(volume * 0.07), Math.round(volume * 0.45)),
      requests: Math.round((summary.requests / Math.max(1, hours)) * daily * jitter),
      active_seconds: Math.min(3600, Math.round(600 * daily * jitter)),
      peaks: summary.peaks
        ? { in: subjectPeak(peakTotal * 0.55, at), out: subjectPeak(peakTotal * 0.5, at), total: subjectPeak(peakTotal, at) }
        : null
    })
  }
  return { window, previous_window: ranking.previous_window, summary, history }
}

export function mockLive(subject: TrafficSubjectType, limit = 10, now = Date.now()): TrafficLiveData {
  const ranking = mockRanking({ range: '24h', subject, sort: 'peak_total', page_size: 500 }, now)
  const tick = Math.floor(now / 3000)
  const items: TrafficLiveItem[] = ranking.items
    .map((item, index) => {
      const wave = 0.35 + Math.abs(Math.sin((tick + index * 3) / 4)) * 0.9
      const peak = item.peaks?.total?.bytes_per_second ?? 0
      const average = peak * 0.22 * wave * (index % 4 === 3 ? 0 : 1)
      const current = average * (0.6 + Math.abs(Math.cos((tick + index) / 2)) * 0.9)
      const identity = { ...item } as TrafficSubjectIdentity
      return {
        ...identity,
        rate: rate(current * 0.55, current * 0.45),
        avg_60s: rate(average * 0.55, average * 0.45),
        avg_60s_seconds: 60,
        in_flight: average > 0 ? 1 + ((tick + index) % 4) : 0,
        last_active_at: new Date(now - (average > 0 ? 1000 : 45_000)).toISOString()
      }
    })
    .filter((item) => item.avg_60s.total > 0 || item.in_flight > 0)
    .sort((a, b) => b.avg_60s.total - a.avg_60s.total)
    .slice(0, limit)
  const hostAverage = items.reduce((sum, item) => sum + item.avg_60s.total, 0) * 1.04
  const hostRate = items.reduce((sum, item) => sum + item.rate.total, 0) * 1.04
  return {
    subject,
    state: 'sampled',
    sampled_at: new Date(Math.floor(now / 1000) * 1000).toISOString(),
    window_seconds: 1,
    host: { rate: rate(hostRate * 0.55, hostRate * 0.45), avg_60s: rate(hostAverage * 0.55, hostAverage * 0.45) },
    unattributed: { rate: rate(hostRate * 0.02, hostRate * 0.02), avg_60s: rate(hostAverage * 0.02, hostAverage * 0.02) },
    items
  }
}
